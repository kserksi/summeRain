// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/pkg/token"
	"github.com/kserksi/summerain/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errImageStillProcessing  = errors.New("image is still processing")
	errImageForbidden        = errors.New("image operation forbidden")
	errR2UploadTargetChanged = errors.New("R2 upload target changed while upload was in flight")
	errR2UploadFailed        = errors.New("R2 object upload failed")
	errR2UploadIntentLost    = errors.New("R2 upload cleanup intent is no longer pending")
)

const (
	v1R2UploadTimeout         = 2 * time.Minute
	v1R2UploadIntentLease     = 5 * time.Minute
	v1R2UploadLeaseSafety     = 10 * time.Second
	v1R2UploadIntentAggregate = "v1_r2_upload"
)

func (s *ImageService) IsR2Enabled() bool {
	return s.r2 != nil && s.r2.IsEnabled()
}

func (s *ImageService) R2PublicURL(key string) string {
	if s.r2 == nil {
		return ""
	}
	return s.r2.PublicURL(key)
}

func (s *ImageService) R2PublicURLForTarget(key, endpoint, bucket string) (string, error) {
	if s.r2 == nil {
		return "", fmt.Errorf("R2 not configured")
	}
	return s.r2.PublicURLForTarget(endpoint, bucket, key)
}

func (s *ImageService) ResolveV1StorageTarget(pipelineVersion uint16, imageFile *model.ImageFile) (V1StorageTarget, error) {
	storageRoot := ""
	if s.storageCfg != nil {
		storageRoot = s.storageCfg.BasePath
	}
	endpoint, bucket, configured := "", "", false
	if s.r2 != nil {
		endpoint, bucket, configured = s.r2.CurrentTarget()
	}
	return ResolveV1StorageTarget(storageRoot, pipelineVersion, imageFile, endpoint, bucket, configured)
}

func (s *ImageService) ReloadR2() {
	if s.r2 != nil {
		s.r2.reload()
	}
}

func (s *ImageService) R2Download(key string) (io.ReadCloser, error) {
	if s.r2 == nil || !s.r2.IsEnabled() {
		return nil, fmt.Errorf("R2 not enabled")
	}
	return s.r2.Download(key)
}

func (s *ImageService) R2DownloadForTarget(ctx context.Context, key, endpoint, bucket string) (io.ReadCloser, error) {
	if s.r2 == nil {
		return nil, fmt.Errorf("R2 not configured")
	}
	return s.r2.DownloadForTarget(ctx, endpoint, bucket, key)
}

const MaxBatchDownloads = 10

type ImageService struct {
	db              *gorm.DB
	rdb             *redis.Client
	imageRepo       imageRepository
	imageFileRepo   *repository.ImageFileRepo
	tokenRepo       imageAccessTokenRepository
	uploadQueueRepo *repository.UploadQueueRepo
	configRepo      *repository.SystemConfigRepo
	imgproxySvc     *ImgproxyService
	notificationSvc *NotificationService
	storageCfg      *config.StorageConfig
	r2              *R2Service
}

type imageRepository interface {
	Create(image *model.Image) error
	FindByID(id uint64) (*model.Image, error)
	FindByUniqueLink(link string) (*model.Image, error)
	FindByUserID(userID uint64, cursor string, limit int, sort string, visibility string, search string) ([]*model.Image, string, error)
	FindOriginalPathsByUserID(userID uint64) ([]*model.Image, error)
	Delete(id uint64) error
	UpdateVisibility(id uint64, visibility string) error
	IncrementViewCount(id uint64) error
}

type imageAccessTokenRepository interface {
	ReplaceActiveForImage(actorID, imageID uint64, isAdmin bool, accessToken *model.ImageAccessToken, revokedAt time.Time) error
	FindActiveByImageID(imageID uint64) (*model.ImageAccessToken, error)
	FindByToken(token string) (*model.ImageAccessToken, error)
	RevokeActiveByImageID(imageID uint64, revokedAt time.Time) (int64, error)
	IncrementUsage(id uint64) error
}

func NewImageService(
	db *gorm.DB,
	rdb *redis.Client,
	imageRepo imageRepository,
	imageFileRepo *repository.ImageFileRepo,
	tokenRepo imageAccessTokenRepository,
	uploadQueueRepo *repository.UploadQueueRepo,
	configRepo *repository.SystemConfigRepo,
	imgproxySvc *ImgproxyService,
	notificationSvc *NotificationService,
	storageCfg *config.StorageConfig,
	r2 *R2Service,
) *ImageService {
	return &ImageService{
		db:              db,
		rdb:             rdb,
		imageRepo:       imageRepo,
		imageFileRepo:   imageFileRepo,
		tokenRepo:       tokenRepo,
		uploadQueueRepo: uploadQueueRepo,
		configRepo:      configRepo,
		imgproxySvc:     imgproxySvc,
		notificationSvc: notificationSvc,
		storageCfg:      storageCfg,
		r2:              r2,
	}
}

type UploadResult struct {
	Filename     string `json:"filename"`
	Success      bool   `json:"success"`
	ImageID      uint64 `json:"image_id,omitempty"`
	UniqueLink   string `json:"unique_link,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	ProcessedURL string `json:"processed_url,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorCode    int    `json:"error_code,omitempty"`
}

type UploadResponse struct {
	UploadID       uint64         `json:"upload_id"`
	Total          int            `json:"total"`
	Results        []UploadResult `json:"results"`
	StorageUsed    int64          `json:"storage_used"`
	StorageQuota   int64          `json:"storage_quota"`
	StoragePercent float64        `json:"storage_percent"`
}

func (s *ImageService) Upload(userID uint64, files []*multipart.FileHeader, visibility string) (*UploadResponse, *errcode.AppError) {
	// 20 个上限是拍脑袋定的,够用了,真要改扔到 config 里
	if len(files) > 20 {
		return nil, errcode.ErrFileCountExceeded
	}

	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, errcode.ErrDatabase
	}

	var totalSize int64
	for _, fh := range files {
		totalSize += fh.Size
	}

	if user.StorageUsed+totalSize > user.StorageQuota {
		return nil, errcode.ErrQuotaFull
	}

	queue := &model.UploadQueue{
		UserID: userID,
		Status: "processing",
	}
	if err := s.uploadQueueRepo.Create(queue); err != nil {
		return nil, errcode.ErrDatabase
	}

	if visibility == "" {
		visibility = "public"
	}

	results := make([]UploadResult, 0, len(files))
	var successSize int64
	var successCount int

	for _, fh := range files {
		result := s.processFile(userID, fh, visibility)
		if result.Success {
			successSize += fh.Size
			successCount++
		}
		results = append(results, result)
	}

	if successSize > 0 {
		// TODO: 此处 Updates 的 err 未处理,配额可能短暂不准;前端会重新拉 profile 自愈.
		s.db.Model(&model.User{}).Where("id = ?", userID).
			Updates(map[string]interface{}{
				"storage_used": gorm.Expr("storage_used + ?", successSize),
				"image_count":  gorm.Expr("image_count + ?", successCount),
			})
	}

	fileInfoJSON, _ := json.Marshal(results)
	if err := s.uploadQueueRepo.UpdateStatusAndFileInfo(queue.ID, "completed", "", string(fileInfoJSON)); err != nil {
		return nil, errcode.ErrDatabase
	}

	s.db.First(&user, userID)

	usagePercent := float64(user.StorageUsed) / float64(user.StorageQuota) * 100
	if usagePercent >= 90 && usagePercent < 100 {
		s.notificationSvc.Create(userID, "image.quota_warning", "存储空间即将用完", fmt.Sprintf("当前使用率 %.0f%%", usagePercent))
	}

	resp := &UploadResponse{
		UploadID:       queue.ID,
		Total:          len(files),
		Results:        results,
		StorageUsed:    user.StorageUsed,
		StorageQuota:   user.StorageQuota,
		StoragePercent: float64(user.StorageUsed) / float64(user.StorageQuota) * 100,
	}

	return resp, nil
}

func (s *ImageService) processFile(userID uint64, fh *multipart.FileHeader, visibility string) UploadResult {
	result := UploadResult{Filename: fh.Filename}

	if fh.Size > 10*1024*1024 {
		result.Error = errcode.ErrFileTooLarge.Message
		result.ErrorCode = errcode.ErrFileTooLarge.Code
		return result
	}

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	allowed := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true}
	if !allowed[ext] {
		result.Error = errcode.ErrUnsupportedType.Message
		result.ErrorCode = errcode.ErrUnsupportedType.Code
		return result
	}

	file, err := fh.Open()
	if err != nil {
		result.Error = errcode.ErrInternal.Message
		result.ErrorCode = errcode.ErrInternal.Code
		return result
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		result.Error = errcode.ErrInternal.Message
		result.ErrorCode = errcode.ErrInternal.Code
		return result
	}

	detectedMIME := http.DetectContentType(content)
	if !allowedImageMIME(detectedMIME) {
		result.Error = "文件内容与扩展名不匹配"
		result.ErrorCode = errcode.ErrUnsupportedType.Code
		return result
	}

	hash := sha256.Sum256(content)
	fileHash := hex.EncodeToString(hash[:])

	existing, err := s.imageFileRepo.FindByHash(fileHash)
	var imageFileID uint64
	var createdImageFileID uint64

	if err == nil && existing != nil {
		imageFileID = existing.ID
		// Dedup: another image now references this file — keep reference_count
		// in sync so later deletion only removes the file when truly unreferenced.
		s.db.Model(&model.ImageFile{}).Where("id = ?", existing.ID).
			UpdateColumn("reference_count", gorm.Expr("reference_count + 1"))
	} else {
		objectSuffix, suffixErr := randomHex(8)
		if suffixErr != nil {
			result.Error = errcode.ErrInternal.Message
			result.ErrorCode = errcode.ErrInternal.Code
			return result
		}
		objectName := fileHash[:16] + "-" + objectSuffix
		originalPath := filepath.Join("original", objectName+ext)
		thumbnailPath := filepath.Join("thumbnail", objectName+".webp")
		processedPath := filepath.Join("processed", objectName+".webp")

		var r2Target *r2UploadSnapshot
		if s.r2 != nil {
			r2Target, _ = s.r2.uploadSnapshot()
		}
		r2Enabled := r2Target != nil
		remoteEndpoint := ""
		remoteBucket := ""
		if r2Enabled {
			remoteEndpoint = r2Target.endpoint
			remoteBucket = r2Target.bucket
		}
		cleanupLocalOnFailure := false

		tempDir := s.storageCfg.TempPath
		os.MkdirAll(tempDir, 0755)
		tempPath := filepath.Join(tempDir, objectName+ext)
		os.WriteFile(tempPath, content, 0644)

		thumbURL := s.imgproxySvc.ThumbnailURL(tempPath)
		thumbData, thumbErr := s.imgproxySvc.Process(thumbURL)

		wmEnabled := false
		wmText := ""
		wmPosition := ""
		wmOpacity := ""
		wmSize := ""
		wmColor := ""
		if cfg, err := s.configRepo.FindByKey("watermark_enabled"); err == nil && cfg.ConfigValue == "true" {
			wmEnabled = true
			if t, err := s.configRepo.FindByKey("watermark_text"); err == nil {
				wmText = t.ConfigValue
			}
			if p, err := s.configRepo.FindByKey("watermark_position"); err == nil {
				wmPosition = p.ConfigValue
			}
			if o, err := s.configRepo.FindByKey("watermark_opacity"); err == nil {
				wmOpacity = o.ConfigValue
			}
			if sz, err := s.configRepo.FindByKey("watermark_size"); err == nil {
				wmSize = sz.ConfigValue
			}
			if cl, err := s.configRepo.FindByKey("watermark_color"); err == nil {
				wmColor = cl.ConfigValue
			}
		}
		processedURL := s.imgproxySvc.ProcessedURL(tempPath, wmEnabled, wmText, wmOpacity, wmPosition, wmSize, wmColor)
		var processedData []byte
		var procErr error
		processWatermarked := func() error {
			processedData, procErr = s.imgproxySvc.Process(processedURL)
			return procErr
		}
		if wmEnabled {
			procErr = WithCurrentWatermark(context.Background(), s.storageCfg.BasePath, processWatermarked)
		} else {
			procErr = processWatermarked()
		}
		if procErr != nil {
			log.Printf("[WATERMARK] imgproxy failed: url=%s err=%v", processedURL, procErr)
		}

		os.Remove(tempPath)

		if !r2Enabled {
			writtenPaths := make([]string, 0, 3)
			cleanupLocalOnFailure = true
			cleanupWrittenFiles := func() {
				for _, p := range writtenPaths {
					if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
						log.Printf("failed to cleanup upload artifact %s: %v", p, err)
					}
				}
			}

			originalDir := filepath.Join(s.storageCfg.BasePath, "original")
			os.MkdirAll(originalDir, 0755)
			fullOriginalPath := filepath.Join(s.storageCfg.BasePath, originalPath)
			if writeErr := os.WriteFile(fullOriginalPath, content, 0644); writeErr != nil {
				result.Error = errcode.ErrInternal.Message
				result.ErrorCode = errcode.ErrInternal.Code
				return result
			}
			writtenPaths = append(writtenPaths, fullOriginalPath)

			if thumbErr == nil && len(thumbData) > 0 {
				thumbnailDir := filepath.Join(s.storageCfg.BasePath, "thumbnail")
				os.MkdirAll(thumbnailDir, 0755)
				thumbFullPath := filepath.Join(s.storageCfg.BasePath, thumbnailPath)
				if err := os.WriteFile(thumbFullPath, thumbData, 0644); err == nil {
					writtenPaths = append(writtenPaths, thumbFullPath)
				}
			}

			if procErr == nil && len(processedData) > 0 {
				processedDir := filepath.Join(s.storageCfg.BasePath, "processed")
				os.MkdirAll(processedDir, 0755)
				processedFullPath := filepath.Join(s.storageCfg.BasePath, processedPath)
				if err := os.WriteFile(processedFullPath, processedData, 0644); err == nil {
					writtenPaths = append(writtenPaths, processedFullPath)
				}
			}

			defer func() {
				if !result.Success && cleanupLocalOnFailure {
					cleanupWrittenFiles()
				}
			}()
		}

		mimeType := "image/" + strings.TrimPrefix(ext, ".")
		if ext == ".jpg" || ext == ".jpeg" {
			mimeType = "image/jpeg"
		}

		imageFile := &model.ImageFile{
			FileHash:       fileHash,
			FileSize:       fh.Size,
			MimeType:       mimeType,
			OriginalPath:   originalPath,
			ThumbnailPath:  thumbnailPath,
			ProcessedPath:  processedPath,
			RemoteEndpoint: remoteEndpoint,
			RemoteBucket:   remoteBucket,
		}
		if r2Enabled {
			imageFile.RemoteBackend = V1StorageBackendR2
		} else {
			imageFile.RemoteBackend = V1StorageBackendLocal
		}
		var createErr error
		if r2Enabled {
			createErr = s.persistR2UploadedImageFile(
				imageFile,
				r2Target,
				content,
				detectedMIME,
				thumbData,
				thumbErr == nil && len(thumbData) > 0,
				processedData,
				procErr == nil && len(processedData) > 0,
			)
		} else {
			createErr = s.imageFileRepo.Create(imageFile)
		}
		if createErr != nil {
			if errors.Is(createErr, errR2UploadTargetChanged) ||
				errors.Is(createErr, errR2UploadFailed) ||
				errors.Is(createErr, errR2UploadIntentLost) ||
				errors.Is(createErr, context.DeadlineExceeded) {
				result.Error = errcode.ErrInternal.Message
				result.ErrorCode = errcode.ErrInternal.Code
				return result
			}
			result.Error = errcode.ErrDatabase.Message
			result.ErrorCode = errcode.ErrDatabase.Code
			return result
		}
		imageFileID = imageFile.ID
		createdImageFileID = imageFile.ID
		// From this point the ImageFile row owns the artifacts. If Image creation
		// fails, compensation must preserve concurrent references and enqueue a
		// durable deletion instead of unlinking files directly in this request.
		cleanupLocalOnFailure = false
	}

	uniqueLink := s.generateUniqueLink()

	image := &model.Image{
		UserID:      userID,
		ImageFileID: imageFileID,
		UniqueLink:  uniqueLink,
		Title:       fh.Filename,
		Visibility:  visibility,
		FileSize:    fh.Size,
	}
	if createErr := s.imageRepo.Create(image); createErr != nil {
		if createdImageFileID != 0 {
			if cleanupErr := s.compensateFailedImageCreate(createdImageFileID, time.Now()); cleanupErr != nil {
				log.Printf("failed to compensate image file %d after image creation error: %v", createdImageFileID, cleanupErr)
			}
		}
		result.Error = errcode.ErrDatabase.Message
		result.ErrorCode = errcode.ErrDatabase.Code
		return result
	}

	result.Success = true
	result.ImageID = image.ID
	result.UniqueLink = uniqueLink
	result.ThumbnailURL = fmt.Sprintf("/i/%s?type=thumbnail", uniqueLink)
	result.ProcessedURL = fmt.Sprintf("/i/%s", uniqueLink)
	return result
}

type v1R2UploadArtifact struct {
	path        string
	data        []byte
	contentType string
	required    bool
}

func (s *ImageService) persistR2UploadedImageFile(
	imageFile *model.ImageFile,
	r2Target *r2UploadSnapshot,
	originalData []byte,
	originalContentType string,
	thumbnailData []byte,
	uploadThumbnail bool,
	processedData []byte,
	uploadProcessed bool,
) error {
	intent, err := s.stageR2UploadCleanupIntent(imageFile, r2Target, time.Now())
	if err != nil {
		return err
	}

	artifacts := []v1R2UploadArtifact{{
		path: imageFile.OriginalPath, data: originalData, contentType: originalContentType, required: true,
	}}
	if uploadThumbnail {
		artifacts = append(artifacts, v1R2UploadArtifact{
			path: imageFile.ThumbnailPath, data: thumbnailData, contentType: "image/webp",
		})
	}
	if uploadProcessed {
		artifacts = append(artifacts, v1R2UploadArtifact{
			path: imageFile.ProcessedPath, data: processedData, contentType: "image/webp",
		})
	}

	uploadDeadline := time.Now().Add(v1R2UploadTimeout)
	if intent.LeaseExpiresAt == nil {
		activationErr := s.activateR2UploadCleanupIntent(intent, errors.New("R2 upload intent has no lease expiry"))
		return errors.Join(errR2UploadIntentLost, activationErr)
	}
	leaseDeadline := intent.LeaseExpiresAt.Add(-v1R2UploadLeaseSafety)
	if leaseDeadline.Before(uploadDeadline) {
		uploadDeadline = leaseDeadline
	}
	if !uploadDeadline.After(time.Now()) {
		activationErr := s.activateR2UploadCleanupIntent(intent, context.DeadlineExceeded)
		return errors.Join(context.DeadlineExceeded, activationErr)
	}
	ctx, cancel := context.WithDeadline(context.Background(), uploadDeadline)
	defer cancel()
	for _, artifact := range artifacts {
		if err := r2Target.uploadBytes(ctx, artifact.data, artifact.path, artifact.contentType); err != nil {
			if !artifact.required {
				log.Printf("[R2] upload optional artifact %s failed: %v", artifact.path, err)
				continue
			}
			uploadErr := fmt.Errorf("%w: %v", errR2UploadFailed, err)
			activationErr := s.activateR2UploadCleanupIntent(intent, uploadErr)
			return errors.Join(uploadErr, activationErr)
		}
	}

	if err := s.finalizeR2UploadedImageFile(imageFile, r2Target, intent, time.Now()); err != nil {
		activationErr := s.activateR2UploadCleanupIntent(intent, err)
		return errors.Join(err, activationErr)
	}
	return nil
}

func (s *ImageService) stageR2UploadCleanupIntent(imageFile *model.ImageFile, r2Target *r2UploadSnapshot, now time.Time) (*model.OutboxEvent, error) {
	if s.db == nil || imageFile == nil || r2Target == nil {
		return nil, errors.New("stage R2 upload cleanup intent: invalid input")
	}
	paths := []string{imageFile.OriginalPath, imageFile.ThumbnailPath, imageFile.ProcessedPath}
	remoteObjects := make([]model.StorageDeleteRemoteObject, 0, len(paths))
	for _, path := range paths {
		remoteObjects = append(remoteObjects, model.StorageDeleteRemoteObject{
			Path: path, Backend: V1StorageBackendR2, Endpoint: r2Target.endpoint, Bucket: r2Target.bucket,
		})
	}
	payload, err := json.Marshal(model.StorageDeletePayload{Paths: paths, RemoteObjects: remoteObjects})
	if err != nil {
		return nil, err
	}
	intentID, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	leaseToken, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	leaseOwner := "v1-r2-upload:" + intentID
	leaseExpiresAt := now.Add(v1R2UploadIntentLease)
	intent := &model.OutboxEvent{
		AggregateType:  v1R2UploadIntentAggregate,
		AggregateID:    intentID,
		EventType:      model.OutboxEventTypeStorageDelete,
		DedupeKey:      "v1-r2-upload:" + intentID,
		Status:         model.OutboxEventStatusPublishing,
		Payload:        string(payload),
		MaxAttempts:    storageDeleteMaxAttempts,
		AvailableAt:    leaseExpiresAt,
		LeaseOwner:     &leaseOwner,
		LeaseToken:     &leaseToken,
		LeaseExpiresAt: &leaseExpiresAt,
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockV2Storage(tx); err != nil {
			return err
		}
		matches, err := r2UploadTargetMatchesTx(tx, r2Target)
		if err != nil {
			return err
		}
		if !matches {
			return errR2UploadTargetChanged
		}
		return tx.Create(intent).Error
	})
	if err != nil {
		return nil, err
	}
	return intent, nil
}

func r2UploadTargetMatchesTx(tx *gorm.DB, r2Target *r2UploadSnapshot) (bool, error) {
	var configs []model.SystemConfig
	if err := tx.Where("config_key IN ?", []string{"r2_enabled", "r2_endpoint", "r2_bucket"}).Find(&configs).Error; err != nil {
		return false, err
	}
	values := make(map[string]string, len(configs))
	for _, item := range configs {
		values[item.ConfigKey] = item.ConfigValue
	}
	matches := len(values) == 3 && values["r2_enabled"] == "true" &&
		normalizeR2Endpoint(values["r2_endpoint"]) == r2Target.endpoint &&
		strings.TrimSpace(values["r2_bucket"]) == r2Target.bucket
	return matches, nil
}

func (s *ImageService) finalizeR2UploadedImageFile(imageFile *model.ImageFile, r2Target *r2UploadSnapshot, intent *model.OutboxEvent, now time.Time) error {
	if intent == nil || intent.LeaseToken == nil {
		return errR2UploadIntentLost
	}
	var operationErr error
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockV2Storage(tx); err != nil {
			return err
		}
		var storedIntent model.OutboxEvent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ? AND lease_token = ?", intent.ID, model.OutboxEventStatusPublishing, *intent.LeaseToken).
			First(&storedIntent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errR2UploadIntentLost
			}
			return err
		}
		matches, err := r2UploadTargetMatchesTx(tx, r2Target)
		if err != nil {
			return err
		}
		if !matches {
			operationErr = errR2UploadTargetChanged
			return activateR2UploadCleanupIntentTx(tx, storedIntent.ID, *intent.LeaseToken, now, operationErr)
		}
		if storedIntent.LeaseExpiresAt == nil || !storedIntent.LeaseExpiresAt.After(now) {
			operationErr = errR2UploadIntentLost
			return activateR2UploadCleanupIntentTx(tx, storedIntent.ID, *intent.LeaseToken, now, operationErr)
		}
		if err := tx.Create(imageFile).Error; err != nil {
			operationErr = err
			return activateR2UploadCleanupIntentTx(tx, storedIntent.ID, *intent.LeaseToken, now, err)
		}
		return publishR2UploadCleanupIntentTx(tx, storedIntent.ID, *intent.LeaseToken, now)
	})
	if err != nil {
		return err
	}
	return operationErr
}

func (s *ImageService) activateR2UploadCleanupIntent(intent *model.OutboxEvent, cause error) error {
	if s.db == nil || intent == nil || intent.LeaseToken == nil {
		return errR2UploadIntentLost
	}
	now := time.Now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockV2Storage(tx); err != nil {
			return err
		}
		result := activateR2UploadCleanupIntentTx(tx, intent.ID, *intent.LeaseToken, now, cause)
		if result == nil {
			return nil
		}
		if !errors.Is(result, errR2UploadIntentLost) {
			return result
		}
		var existing model.OutboxEvent
		if err := tx.Select("id", "status").First(&existing, intent.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errR2UploadIntentLost
			}
			return err
		}
		// Published means finalize committed; every other live status remains a
		// durable cleanup responsibility owned by the worker or another retry.
		return nil
	})
}

func activateR2UploadCleanupIntentTx(tx *gorm.DB, intentID uint64, leaseToken string, now time.Time, cause error) error {
	lastError := "R2 upload failed"
	if cause != nil {
		lastError = cause.Error()
		if len(lastError) > 4096 {
			lastError = lastError[:4096]
		}
	}
	result := tx.Model(&model.OutboxEvent{}).
		Where("id = ? AND status = ? AND lease_token = ?", intentID, model.OutboxEventStatusPublishing, leaseToken).
		Updates(map[string]interface{}{
			"status":           model.OutboxEventStatusPending,
			"available_at":     now,
			"lease_owner":      nil,
			"lease_token":      nil,
			"lease_expires_at": nil,
			"last_error":       lastError,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errR2UploadIntentLost
	}
	return nil
}

func publishR2UploadCleanupIntentTx(tx *gorm.DB, intentID uint64, leaseToken string, now time.Time) error {
	result := tx.Model(&model.OutboxEvent{}).
		Where("id = ? AND status = ? AND lease_token = ?", intentID, model.OutboxEventStatusPublishing, leaseToken).
		Updates(map[string]interface{}{
			"status":           model.OutboxEventStatusPublished,
			"published_at":     now,
			"lease_owner":      nil,
			"lease_token":      nil,
			"lease_expires_at": nil,
			"last_error":       "",
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errR2UploadIntentLost
	}
	return nil
}

// compensateFailedImageCreate transfers artifacts from an orphaned ImageFile
// to the durable storage-deletion outbox. The authoritative images table is
// rechecked under a row lock because a concurrent deduplicated upload may have
// started referencing the file after it was created.
func (s *ImageService) compensateFailedImageCreate(imageFileID uint64, now time.Time) error {
	if s.db == nil || imageFileID == 0 {
		return errors.New("compensate failed image create: invalid database or image file")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockV2Storage(tx); err != nil {
			return err
		}
		var imageFile model.ImageFile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&imageFile, imageFileID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		var references int64
		if err := tx.Model(&model.Image{}).Where("image_file_id = ?", imageFile.ID).Count(&references).Error; err != nil {
			return err
		}
		if references > 0 {
			return tx.Model(&imageFile).UpdateColumn("reference_count", references).Error
		}

		if err := tx.Delete(&imageFile).Error; err != nil {
			return err
		}
		paths := []string{imageFile.OriginalPath, imageFile.ThumbnailPath, imageFile.ProcessedPath}
		if avifPath, ok := V1PersistentVariantPath(imageFile.ProcessedPath, "avif"); ok {
			paths = append(paths, avifPath)
		}
		var remoteObjects []model.StorageDeleteRemoteObject
		if imageFile.RemoteBackend == "r2" {
			for _, path := range []string{imageFile.OriginalPath, imageFile.ThumbnailPath, imageFile.ProcessedPath} {
				if path != "" {
					remoteObjects = append(remoteObjects, model.StorageDeleteRemoteObject{
						Path: path, Backend: "r2", Endpoint: imageFile.RemoteEndpoint, Bucket: imageFile.RemoteBucket,
					})
				}
			}
		}
		return EnqueueStorageDeleteWithRemote(
			tx,
			"image_file",
			fmt.Sprint(imageFile.ID),
			fmt.Sprintf("image-file:%d:create-compensation", imageFile.ID),
			paths,
			remoteObjects,
			now,
		)
	})
}

func allowedImageMIME(mimeType string) bool {
	allowedMIME := map[string]bool{
		"image/png":  true,
		"image/jpeg": true,
		"image/webp": true,
		"image/gif":  true,
	}
	return allowedMIME[mimeType]
}

func (s *ImageService) GetByID(id uint64) (*model.Image, *errcode.AppError) {
	image, err := s.imageRepo.FindByID(id)
	if err != nil {
		return nil, errcode.New(4041, "图片不存在", 404)
	}
	return image, nil
}

func (s *ImageService) GetImageFile(imageFileID uint64) (*model.ImageFile, *errcode.AppError) {
	file, err := s.imageFileRepo.FindByID(imageFileID)
	if err != nil {
		return nil, errcode.New(4041, "图片文件不存在", 404)
	}
	return file, nil
}

func (s *ImageService) ListByUser(userID uint64, cursor string, limit int, sort string, visibility string, search string) ([]*model.Image, string, *errcode.AppError) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	images, nextCursor, err := s.imageRepo.FindByUserID(userID, cursor, limit, sort, visibility, search)
	if err != nil {
		return nil, "", errcode.ErrDatabase
	}
	return images, nextCursor, nil
}

func (s *ImageService) Delete(userID uint64, imageID uint64) (*DeleteResult, *errcode.AppError) {
	return s.deleteImage(userID, imageID, false)
}

func (s *ImageService) AdminDelete(imageID uint64) (*DeleteResult, *errcode.AppError) {
	return s.deleteImage(0, imageID, true)
}

func (s *ImageService) deleteImage(userID uint64, imageID uint64, isAdmin bool) (*DeleteResult, *errcode.AppError) {
	var imageFile model.ImageFile
	var fileSize int64
	var shouldDeleteFiles bool
	var ownerID uint64
	var variantPaths []string
	var storageTarget V1StorageTarget
	now := time.Now()

	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockV2Storage(tx); err != nil {
			return err
		}
		var image model.Image
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&image, imageID).Error; err != nil {
			return err
		}
		if image.UserID != userID && !isAdmin {
			return errImageForbidden
		}
		if image.PipelineVersion >= model.ImagePipelineVersionV2 && image.ProcessingStatus != model.ImageProcessingStatusCompleted {
			return errImageStillProcessing
		}
		ownerID = image.UserID
		fileSize = image.FileSize
		if image.PipelineVersion >= model.ImagePipelineVersionV2 {
			var variants []model.ImageVariant
			if err := tx.Where("image_id = ?", imageID).Find(&variants).Error; err != nil {
				return err
			}
			fileSize = 0
			for _, variant := range variants {
				if variant.IsActive {
					fileSize += variant.FileSize
				}
				if variant.StoragePath != "" {
					variantPaths = append(variantPaths, variant.StoragePath)
				}
			}
			if err := tx.Where("image_id = ?", imageID).Delete(&model.ImageVariant{}).Error; err != nil {
				return err
			}
			assetLink := image.UniqueLink
			if image.OriginAlias != nil {
				assetLink = *image.OriginAlias
			}
			payload, _ := json.Marshal(map[string]interface{}{
				"image_id": imageID, "old_asset_link": image.UniqueLink, "asset_link": assetLink,
			})
			if err := tx.Create(&model.OutboxEvent{
				AggregateType: "image", AggregateID: fmt.Sprint(imageID), EventType: "image.cdn.purge",
				DedupeKey: fmt.Sprintf("image:%d:delete:%d", imageID, time.Now().UnixNano()),
				Status:    model.OutboxEventStatusPending, Payload: string(payload), AvailableAt: time.Now(),
			}).Error; err != nil {
				return err
			}
		} else {
			payload, _ := json.Marshal(map[string]interface{}{"image_id": imageID, "asset_link": image.UniqueLink})
			if err := tx.Create(&model.OutboxEvent{
				AggregateType: "image", AggregateID: fmt.Sprint(imageID), EventType: "image.cdn.purge",
				DedupeKey: fmt.Sprintf("image:%d:delete:%d", imageID, time.Now().UnixNano()),
				Status:    model.OutboxEventStatusPending, Payload: string(payload), AvailableAt: time.Now(),
			}).Error; err != nil {
				return err
			}
		}

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&imageFile, image.ImageFileID).Error; err != nil {
			return err
		}

		if err := tx.Where("image_id = ?", imageID).Delete(&model.ImageAccessToken{}).Error; err != nil {
			return err
		}

		if err := tx.Delete(&model.Image{}, imageID).Error; err != nil {
			return err
		}

		var remainingReferences int64
		if err := tx.Model(&model.Image{}).Where("image_file_id = ?", image.ImageFileID).Count(&remainingReferences).Error; err != nil {
			return err
		}
		if remainingReferences == 0 {
			resolvedTarget, resolveErr := s.ResolveV1StorageTarget(image.PipelineVersion, &imageFile)
			if resolveErr != nil {
				return fmt.Errorf("resolve image storage target: %w", resolveErr)
			}
			storageTarget = resolvedTarget
			shouldDeleteFiles = true
			if err := tx.Delete(&imageFile).Error; err != nil {
				return err
			}
		} else if err := tx.Model(&imageFile).UpdateColumn("reference_count", remainingReferences).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.User{}).Where("id = ?", ownerID).
			UpdateColumn("storage_used", gorm.Expr("GREATEST(storage_used - ?, 0)", fileSize)).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", ownerID).
			UpdateColumn("image_count", gorm.Expr("GREATEST(image_count - 1, 0)")).Error; err != nil {
			return err
		}

		deletePaths := append([]string(nil), variantPaths...)
		if shouldDeleteFiles {
			deletePaths = append(deletePaths, imageFile.OriginalPath, imageFile.ThumbnailPath, imageFile.ProcessedPath)
			if avifPath, ok := V1PersistentVariantPath(imageFile.ProcessedPath, "avif"); ok {
				deletePaths = append(deletePaths, avifPath)
			}
		}
		var remoteObjects []model.StorageDeleteRemoteObject
		if shouldDeleteFiles && storageTarget.Backend == V1StorageBackendR2 {
			for _, path := range []string{imageFile.OriginalPath, imageFile.ThumbnailPath, imageFile.ProcessedPath} {
				if path != "" {
					remoteObjects = append(remoteObjects, model.StorageDeleteRemoteObject{
						Path: path, Backend: V1StorageBackendR2, Endpoint: storageTarget.Endpoint, Bucket: storageTarget.Bucket,
					})
				}
			}
		}
		if err := EnqueueStorageDeleteWithRemote(
			tx,
			"image",
			fmt.Sprint(imageID),
			fmt.Sprintf("image:%d:storage-delete:%d", imageID, now.UnixNano()),
			deletePaths,
			remoteObjects,
			now,
		); err != nil {
			return err
		}

		return nil
	})

	if txErr != nil {
		if errors.Is(txErr, errImageStillProcessing) {
			return nil, errcode.New(4093, "图片仍在处理或清理中，请稍后重试", http.StatusConflict)
		}
		if errors.Is(txErr, errImageForbidden) {
			return nil, errcode.New(4031, "无权操作此图片", 403)
		}
		if txErr == gorm.ErrRecordNotFound {
			return nil, errcode.New(4041, "图片不存在", 404)
		}
		return nil, errcode.ErrDatabase
	}

	var user model.User
	s.db.First(&user, ownerID)

	return &DeleteResult{
		ImageID:      imageID,
		StorageFreed: fileSize,
		StorageUsed:  user.StorageUsed,
		StorageQuota: user.StorageQuota,
	}, nil
}

func cleanupUnreferencedV2Files(db *gorm.DB, storageRoot string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := lockV2Storage(tx); err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(paths))
		for _, relativePath := range paths {
			if relativePath == "" {
				continue
			}
			if _, exists := seen[relativePath]; exists {
				continue
			}
			seen[relativePath] = struct{}{}
			var variantReferences int64
			if err := tx.Model(&model.ImageVariant{}).Where("storage_path = ?", relativePath).Count(&variantReferences).Error; err != nil {
				return err
			}
			var fileReferences int64
			if err := tx.Model(&model.ImageFile{}).
				Where("original_path = ? OR thumbnail_path = ? OR processed_path = ?", relativePath, relativePath, relativePath).
				Count(&fileReferences).Error; err != nil {
				return err
			}
			if variantReferences > 0 || fileReferences > 0 {
				continue
			}
			candidate := relativePath
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(storageRoot, candidate)
			}
			fullPath, pathErr := pathInside(storageRoot, candidate)
			if pathErr != nil {
				log.Printf("refusing to remove V2 file outside storage root: %s", relativePath)
				continue
			}
			if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
				log.Printf("failed to remove unreferenced V2 file %s: %v", relativePath, err)
			}
		}
		return nil
	})
}

type DeleteResult struct {
	ImageID      uint64 `json:"image_id"`
	StorageFreed int64  `json:"storage_freed_bytes"`
	StorageUsed  int64  `json:"storage_used"`
	StorageQuota int64  `json:"storage_quota"`
}

func (s *ImageService) ToggleVisibility(userID uint64, imageID uint64, visibility string) (*VisibilityResult, *errcode.AppError) {
	var tokensRevoked int64
	var assetLink string
	now := time.Now()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var locked model.Image
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, imageID).Error; err != nil {
			return err
		}
		if locked.UserID != userID {
			return errImageForbidden
		}
		assetLink = locked.UniqueLink
		if locked.PipelineVersion >= model.ImagePipelineVersionV2 && visibility == "private" {
			assetLink += "S"
		}
		if locked.Visibility == visibility {
			return nil
		}

		updates := map[string]interface{}{"visibility": visibility}
		oldAssetLink := locked.UniqueLink
		if locked.OriginAlias != nil {
			oldAssetLink = *locked.OriginAlias
		}
		if locked.PipelineVersion >= model.ImagePipelineVersionV2 {
			updates["origin_alias"] = assetLink
		}
		if err := tx.Model(&locked).Updates(updates).Error; err != nil {
			return err
		}
		if visibility == "public" && locked.Visibility == "private" {
			result := tx.Model(&model.ImageAccessToken{}).
				Where("image_id = ? AND revoked_at IS NULL", imageID).Update("revoked_at", now)
			if result.Error != nil {
				return result.Error
			}
			tokensRevoked = result.RowsAffected
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"image_id": imageID, "old_asset_link": oldAssetLink,
			"new_asset_link": assetLink, "asset_link": assetLink,
		})
		return tx.Create(&model.OutboxEvent{
			AggregateType: "image",
			AggregateID:   fmt.Sprint(imageID),
			EventType:     "image.cdn.purge",
			DedupeKey:     fmt.Sprintf("image:%d:visibility:%d", imageID, now.UnixNano()),
			Status:        model.OutboxEventStatusPending,
			Payload:       string(payload),
			AvailableAt:   now,
		}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(4041, "图片不存在", 404)
	}
	if errors.Is(err, errImageForbidden) {
		return nil, errcode.New(4031, "无权操作此图片", 403)
	}
	if err != nil {
		return nil, errcode.ErrDatabase
	}

	result := &VisibilityResult{
		ImageID:       imageID,
		Visibility:    visibility,
		TokensRevoked: tokensRevoked,
		AssetLink:     assetLink,
	}
	if tokensRevoked > 0 {
		result.Warning = "private → public 切换已撤销此图片的全部访问令牌"
	}
	return result, nil
}

type VisibilityResult struct {
	ImageID       uint64 `json:"image_id"`
	Visibility    string `json:"visibility"`
	TokensRevoked int64  `json:"tokens_revoked"`
	Warning       string `json:"warning,omitempty"`
	AssetLink     string `json:"asset_link,omitempty"`
}

const (
	PrivateTokenMinTTLms     int64 = 600000
	PrivateTokenMaxTTLms     int64 = 259200000
	PrivateTokenDefaultTTLms int64 = 3600000
)

type AccessTokenValidation int

const (
	TokenValid AccessTokenValidation = iota
	TokenExpired
	TokenRevoked
	TokenNotFound
)

func clampTTLms(ttlMs int64) int64 {
	if ttlMs <= 0 {
		return PrivateTokenDefaultTTLms
	}
	if ttlMs < PrivateTokenMinTTLms {
		return PrivateTokenMinTTLms
	}
	if ttlMs > PrivateTokenMaxTTLms {
		return PrivateTokenMaxTTLms
	}
	return ttlMs
}

// resolveTTLms returns the caller-provided TTL, or the admin-configured default
// (private_token_ttl_default_ms) when not provided (>0). Falls back to the
// built-in default if the config is missing/invalid.
func (s *ImageService) resolveTTLms(ttlMs int64) int64 {
	if ttlMs > 0 {
		return ttlMs
	}
	if s.configRepo != nil {
		if cfg, err := s.configRepo.FindByKey("private_token_ttl_default_ms"); err == nil {
			if v, err := strconv.ParseInt(strings.TrimSpace(cfg.ConfigValue), 10, 64); err == nil && v > 0 {
				return v
			}
		}
	}
	return PrivateTokenDefaultTTLms
}

type AccessTokenResult struct {
	TokenID   uint64    `json:"token_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Warning   string    `json:"warning"`
}

// IssueAccessToken creates the single active token for an image. Any existing
// active token is revoked first (one-active-per-image invariant). The token
// string is immutable once issued; re-issue produces a brand new string.
func (s *ImageService) IssueAccessToken(userID, imageID uint64, isAdmin bool, ttlMs int64) (*AccessTokenResult, *errcode.AppError) {
	clamped := clampTTLms(s.resolveTTLms(ttlMs))
	now := time.Now()

	plaintext, _, tokenErr := token.Generate(32)
	if tokenErr != nil {
		return nil, errcode.ErrInternal
	}
	expiresAt := now.Add(time.Duration(clamped) * time.Millisecond)

	accessToken := &model.ImageAccessToken{
		ImageID:   imageID,
		UserID:    userID,
		Token:     plaintext,
		ExpiresAt: expiresAt,
	}
	if replaceErr := s.tokenRepo.ReplaceActiveForImage(userID, imageID, isAdmin, accessToken, now); replaceErr != nil {
		if errors.Is(replaceErr, repository.ErrAccessTokenImageNotFound) {
			return nil, errcode.New(4041, "图片不存在", 404)
		}
		if errors.Is(replaceErr, repository.ErrAccessTokenForbidden) {
			return nil, errcode.New(4031, "无权操作此图片", 403)
		}
		return nil, errcode.ErrDatabase
	}

	return &AccessTokenResult{
		TokenID:   accessToken.ID,
		Token:     plaintext,
		ExpiresAt: expiresAt,
		Warning:   "请立即保存此令牌。令牌字符不可变，吊销后需重新申请。",
	}, nil
}

type RevokeAccessTokenResult struct {
	ImageID uint64 `json:"image_id"`
	Revoked bool   `json:"revoked"`
}

// RevokeAccessToken revokes the image's active token (owner/admin only). After
// revocation the image is permanently unshareable until owner/admin re-issues.
func (s *ImageService) RevokeAccessToken(userID, imageID uint64, isAdmin bool) (*RevokeAccessTokenResult, *errcode.AppError) {
	image, err := s.imageRepo.FindByID(imageID)
	if err != nil {
		return nil, errcode.New(4041, "图片不存在", 404)
	}
	if image.UserID != userID && !isAdmin {
		return nil, errcode.New(4031, "无权操作此图片", 403)
	}

	rows, err := s.tokenRepo.RevokeActiveByImageID(imageID, time.Now())
	if err != nil {
		return nil, errcode.ErrDatabase
	}
	return &RevokeAccessTokenResult{ImageID: imageID, Revoked: rows > 0}, nil
}

// ActiveAccessToken returns the image's current active token for owner/admin
// display (plaintext). Returns (nil, nil) when no active token exists.
func (s *ImageService) ActiveAccessToken(userID, imageID uint64, isAdmin bool) (*model.ImageAccessToken, *errcode.AppError) {
	image, err := s.imageRepo.FindByID(imageID)
	if err != nil {
		return nil, errcode.New(4041, "图片不存在", 404)
	}
	if image.UserID != userID && !isAdmin {
		return nil, errcode.New(4031, "无权操作此图片", 403)
	}
	t, err := s.tokenRepo.FindActiveByImageID(imageID)
	if err != nil {
		return nil, nil
	}
	return t, nil
}

// ValidateAccessToken classifies a presented token for /i/ private access.
func (s *ImageService) ValidateAccessToken(imageID uint64, presentedToken string) AccessTokenValidation {
	t, err := s.tokenRepo.FindByToken(presentedToken)
	if err != nil || t.ImageID != imageID {
		return TokenNotFound
	}
	if t.RevokedAt != nil {
		return TokenRevoked
	}
	if t.ExpiresAt.Before(time.Now()) {
		return TokenExpired
	}
	_ = s.tokenRepo.IncrementUsage(t.ID)
	return TokenValid
}

func (s *ImageService) GetByUniqueLink(link string) (*model.Image, *errcode.AppError) {
	image, err := s.imageRepo.FindByUniqueLink(link)
	if err != nil {
		return nil, errcode.New(4041, "图片不存在", 404)
	}
	if image.PipelineVersion >= model.ImagePipelineVersionV2 && (image.OriginAlias == nil || *image.OriginAlias != link) {
		return nil, errcode.New(4041, "图片不存在", 404)
	}
	return image, nil
}

func (s *ImageService) GetActiveVariant(imageID uint64, kind string) (*model.ImageVariant, *errcode.AppError) {
	var variant model.ImageVariant
	if err := s.db.Where("image_id = ? AND kind = ? AND status = ? AND is_active = ?", imageID, kind, model.ImageVariantStatusReady, true).
		Order("revision DESC").First(&variant).Error; err != nil {
		return nil, errcode.New(4041, "图片变体尚未就绪", 404)
	}
	return &variant, nil
}

func (s *ImageService) GetUploadQueue(id uint64) (*model.UploadQueue, *errcode.AppError) {
	queue, err := s.uploadQueueRepo.FindByID(id)
	if err != nil {
		return nil, errcode.New(4041, "上传记录不存在", 404)
	}
	return queue, nil
}

func (s *ImageService) IncrementView(imageID uint64) {
	key := fmt.Sprintf("views:%d", imageID)
	s.rdb.Incr(context.Background(), key)
}

func (s *ImageService) GetImageFileByHash(hashPrefix string) (*model.ImageFile, *errcode.AppError) {
	var imageFile model.ImageFile
	if err := s.db.Where("file_hash LIKE ?", hashPrefix+"%").First(&imageFile).Error; err != nil {
		return nil, errcode.New(4041, "图片不存在", 404)
	}
	return &imageFile, nil
}

func (s *ImageService) GetByImageFileID(fileID uint64) (*model.Image, *errcode.AppError) {
	var image model.Image
	if err := s.db.Where("image_file_id = ?", fileID).First(&image).Error; err != nil {
		return nil, errcode.New(4041, "图片不存在", 404)
	}
	return &image, nil
}

func (s *ImageService) generateUniqueLink() string {
	for i := 0; i < 5; i++ {
		b := make([]byte, 6)
		if _, err := rand.Read(b); err != nil {
			log.Printf("failed to read random bytes for link: %v", err)
			continue
		}
		link := hex.EncodeToString(b)[:12]
		var count int64
		s.db.Model(&model.Image{}).Where("unique_link = ?", link).Count(&count)
		if count == 0 {
			return link
		}
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("crypto/rand unavailable: %v", err)
	}
	return hex.EncodeToString(b)
}

// BatchDownloadOriginals streams a ZIP archive directly to dst. onReady is
// called after all preflight checks and before the first archive byte is
// written, allowing an HTTP caller to set headers without corrupting preflight
// error responses.
func (s *ImageService) BatchDownloadOriginals(ctx context.Context, userID uint64, dst io.Writer, onReady func(filename string)) *errcode.AppError {
	if dst == nil {
		return errcode.ErrInternal
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var user model.User
	if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return errcode.ErrDatabase
	}
	if user.BatchDownloadCount >= MaxBatchDownloads {
		return errcode.New(4039, "批量下载次数已用尽", 403)
	}

	images, err := s.imageRepo.FindOriginalPathsByUserID(userID)
	if err != nil {
		return errcode.ErrDatabase
	}
	if len(images) == 0 {
		return errcode.New(4041, "没有可下载的图片", 404)
	}

	// Reserve one of the limited downloads atomically. This prevents concurrent
	// requests from all observing the same stale counter value.
	reserved := s.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ? AND batch_download_count < ?", userID, MaxBatchDownloads).
		UpdateColumn("batch_download_count", gorm.Expr("batch_download_count + 1"))
	if reserved.Error != nil {
		return errcode.ErrDatabase
	}
	if reserved.RowsAffected == 0 {
		return errcode.New(4039, "批量下载次数已用尽", 403)
	}
	committed := false
	defer func() {
		if !committed {
			s.db.Model(&model.User{}).
				Where("id = ? AND batch_download_count > 0", userID).
				UpdateColumn("batch_download_count", gorm.Expr("batch_download_count - 1"))
		}
	}()

	if onReady != nil {
		onReady(fmt.Sprintf("imgcloud-backup-%s.zip", user.Username))
	}
	zw := zip.NewWriter(dst)
	for _, image := range images {
		if ctx.Err() != nil {
			_ = zw.Close()
			return errcode.ErrInternal
		}
		var imageFile model.ImageFile
		if err := s.db.WithContext(ctx).First(&imageFile, image.ImageFileID).Error; err != nil {
			continue
		}

		reader, openErr := s.openOriginalForArchive(ctx, image.PipelineVersion, imageFile)
		if openErr != nil {
			continue
		}
		if err := writeOriginalZipEntry(zw, image.Filename, reader); err != nil {
			_ = zw.Close()
			return errcode.ErrInternal
		}
	}
	if err := zw.Close(); err != nil {
		return errcode.ErrInternal
	}

	committed = true
	return nil
}

func (s *ImageService) openOriginalForArchive(ctx context.Context, pipelineVersion uint16, imageFile model.ImageFile) (io.ReadCloser, error) {
	target, err := s.ResolveV1StorageTarget(pipelineVersion, &imageFile)
	if err != nil {
		return nil, err
	}
	if target.Backend == V1StorageBackendR2 {
		return s.R2DownloadForTarget(ctx, imageFile.OriginalPath, target.Endpoint, target.Bucket)
	}
	fullPath, err := pathInside(s.storageCfg.BasePath, filepath.Join(s.storageCfg.BasePath, imageFile.OriginalPath))
	if err != nil {
		return nil, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(s.storageCfg.BasePath)
	if err != nil {
		return nil, err
	}
	resolvedPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return nil, err
	}
	resolvedRelative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || resolvedRelative == "." || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) {
		return nil, errors.New("archive source escapes storage root")
	}
	file, err := os.Open(resolvedPath)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("archive source is not a regular file")
	}
	return file, nil
}

func writeOriginalZipEntry(zw *zip.Writer, filename string, reader io.ReadCloser) error {
	// Images are already compressed. Store them verbatim to avoid spending CPU
	// on deflate while a user is draining the archive.
	filename = filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	if filename == "." || filename == string(filepath.Separator) || filename == "" {
		filename = "image"
	}
	entry, err := zw.CreateHeader(&zip.FileHeader{
		Name:   filename,
		Method: zip.Store,
	})
	if err != nil {
		_ = reader.Close()
		return err
	}
	_, copyErr := io.Copy(entry, reader)
	closeErr := reader.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
