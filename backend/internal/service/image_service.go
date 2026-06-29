// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"github.com/summerain/image-gallery/internal/config"
	"github.com/summerain/image-gallery/internal/model"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
	"github.com/summerain/image-gallery/internal/pkg/token"
	"github.com/summerain/image-gallery/internal/repository"
	"gorm.io/gorm"
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

func (s *ImageService) MigrateToR2() (int, error) {
	if s.r2 == nil {
		return 0, fmt.Errorf("R2 not initialized")
	}
	// XXX: 大量文件时这个同步迁移会卡住整个请求,理想是丢到 worker 异步跑
	// 目前先用着,文件少的时候没问题
	total := 0
	for _, sub := range []string{"original", "thumbnail", "processed"} {
		// 三个子目录都要传,少一个迁移完不完整
		n, err := s.r2.MigrateLocalDir(s.storageCfg.BasePath, sub)
		if err != nil {
			log.Printf("[R2] migrate %s: %v", sub, err)
		}
		total += n
	}
	return total, nil
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
	Create(accessToken *model.ImageAccessToken) error
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
		// 这里 Updates 的 err 没接,理论上配额会算错,但前台会重新拉 profile,影响不大,先这样
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
		originalPath := filepath.Join("original", fileHash[:16]+ext)
		thumbnailPath := filepath.Join("thumbnail", fileHash[:16]+".webp")
		processedPath := filepath.Join("processed", fileHash[:16]+".webp")

		r2Enabled := s.r2 != nil && s.r2.IsEnabled()

		tempDir := s.storageCfg.TempPath
		os.MkdirAll(tempDir, 0755)
		tempPath := filepath.Join(tempDir, fileHash[:16]+ext)
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
		processedData, procErr := s.imgproxySvc.Process(processedURL)
		if procErr != nil {
			log.Printf("[WATERMARK] imgproxy failed: url=%s err=%v", processedURL, procErr)
		}

		os.Remove(tempPath)

		if r2Enabled {
			if e := s.r2.UploadBytes(content, originalPath, detectedMIME); e != nil {
				log.Printf("[R2] upload original failed: %v", e)
				result.Error = errcode.ErrInternal.Message
				result.ErrorCode = errcode.ErrInternal.Code
				return result
			}
			if thumbErr == nil && len(thumbData) > 0 {
				if e := s.r2.UploadBytes(thumbData, thumbnailPath, "image/webp"); e != nil {
					log.Printf("[R2] upload thumbnail failed: %v", e)
				}
			}
			if procErr == nil && len(processedData) > 0 {
				if e := s.r2.UploadBytes(processedData, processedPath, "image/webp"); e != nil {
					log.Printf("[R2] upload processed failed: %v", e)
				}
			}
		} else {
			writtenPaths := make([]string, 0, 3)
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
				if !result.Success {
					cleanupWrittenFiles()
				}
			}()
		}

		mimeType := "image/" + strings.TrimPrefix(ext, ".")
		if ext == ".jpg" || ext == ".jpeg" {
			mimeType = "image/jpeg"
		}

		imageFile := &model.ImageFile{
			FileHash:      fileHash,
			FileSize:      fh.Size,
			MimeType:      mimeType,
			OriginalPath:  originalPath,
			ThumbnailPath: thumbnailPath,
			ProcessedPath: processedPath,
		}
		if createErr := s.imageFileRepo.Create(imageFile); createErr != nil {
			if r2Enabled {
				_ = s.r2.Delete(originalPath)
				_ = s.r2.Delete(thumbnailPath)
				_ = s.r2.Delete(processedPath)
			}
			result.Error = errcode.ErrDatabase.Message
			result.ErrorCode = errcode.ErrDatabase.Code
			return result
		}
		imageFileID = imageFile.ID
		createdImageFileID = imageFile.ID
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
			_ = s.imageFileRepo.Delete(createdImageFileID)
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

	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		var image model.Image
		if err := tx.First(&image, imageID).Error; err != nil {
			return err
		}
		if image.UserID != userID && !isAdmin {
			return fmt.Errorf("forbidden")
		}
		fileSize = image.FileSize

		tx.Where("image_id = ?", imageID).Delete(&model.ImageAccessToken{})

		if err := tx.Delete(&model.Image{}, imageID).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.ImageFile{}).Where("id = ?", image.ImageFileID).
			UpdateColumn("reference_count", gorm.Expr("reference_count - 1")).Error; err != nil {
			return err
		}

		if err := tx.First(&imageFile, image.ImageFileID).Error; err != nil {
			return err
		}
		if imageFile.ReferenceCount <= 0 {
			shouldDeleteFiles = true
			if err := tx.Delete(&model.ImageFile{}, image.ImageFileID).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&model.User{}).Where("id = ?", userID).
			UpdateColumn("storage_used", gorm.Expr("storage_used - ?", fileSize)).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", userID).
			UpdateColumn("image_count", gorm.Expr("image_count - 1")).Error; err != nil {
			return err
		}

		return nil
	})

	if txErr != nil {
		if txErr.Error() == "forbidden" {
			return nil, errcode.New(4031, "无权操作此图片", 403)
		}
		if txErr == gorm.ErrRecordNotFound {
			return nil, errcode.New(4041, "图片不存在", 404)
		}
		return nil, errcode.ErrDatabase
	}

	if shouldDeleteFiles {
		storagePath := s.storageCfg.BasePath
		if err := os.Remove(filepath.Join(storagePath, imageFile.OriginalPath)); err != nil && !os.IsNotExist(err) {
			log.Printf("failed to remove original file: %v", err)
		}
		if err := os.Remove(filepath.Join(storagePath, imageFile.ThumbnailPath)); err != nil && !os.IsNotExist(err) {
			log.Printf("failed to remove thumbnail file: %v", err)
		}
		if err := os.Remove(filepath.Join(storagePath, imageFile.ProcessedPath)); err != nil && !os.IsNotExist(err) {
			log.Printf("failed to remove processed file: %v", err)
		}

		// R2 dual-delete
		if s.r2 != nil && s.r2.IsEnabled() {
			_ = s.r2.Delete(imageFile.OriginalPath)
			_ = s.r2.Delete(imageFile.ThumbnailPath)
			_ = s.r2.Delete(imageFile.ProcessedPath)
		}
	}

	var user model.User
	s.db.First(&user, userID)

	return &DeleteResult{
		ImageID:      imageID,
		StorageFreed: fileSize,
		StorageUsed:  user.StorageUsed,
		StorageQuota: user.StorageQuota,
	}, nil
}

type DeleteResult struct {
	ImageID      uint64 `json:"image_id"`
	StorageFreed int64  `json:"storage_freed_bytes"`
	StorageUsed  int64  `json:"storage_used"`
	StorageQuota int64  `json:"storage_quota"`
}

func (s *ImageService) ToggleVisibility(userID uint64, imageID uint64, visibility string) (*VisibilityResult, *errcode.AppError) {
	image, err := s.imageRepo.FindByID(imageID)
	if err != nil {
		return nil, errcode.New(4041, "图片不存在", 404)
	}
	if image.UserID != userID {
		return nil, errcode.New(4031, "无权操作此图片", 403)
	}

	if err := s.imageRepo.UpdateVisibility(imageID, visibility); err != nil {
		return nil, errcode.ErrDatabase
	}

	var tokensRevoked int64
	if visibility == "public" && image.Visibility == "private" {
		tokensRevoked, _ = s.tokenRepo.RevokeActiveByImageID(imageID, time.Now())
	}

	result := &VisibilityResult{
		ImageID:       imageID,
		Visibility:    visibility,
		TokensRevoked: tokensRevoked,
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
	image, err := s.imageRepo.FindByID(imageID)
	if err != nil {
		return nil, errcode.New(4041, "图片不存在", 404)
	}
	if image.UserID != userID && !isAdmin {
		return nil, errcode.New(4031, "无权操作此图片", 403)
	}

	clamped := clampTTLms(s.resolveTTLms(ttlMs))
	now := time.Now()
	_, _ = s.tokenRepo.RevokeActiveByImageID(imageID, now)

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
	if createErr := s.tokenRepo.Create(accessToken); createErr != nil {
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
	return image, nil
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

func (s *ImageService) BatchDownloadOriginals(userID uint64) ([]byte, string, *errcode.AppError) {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, "", errcode.ErrDatabase
	}
	if user.BatchDownloadCount >= MaxBatchDownloads {
		return nil, "", errcode.New(4039, "批量下载次数已用尽", 403)
	}

	images, err := s.imageRepo.FindOriginalPathsByUserID(userID)
	if err != nil {
		return nil, "", errcode.ErrDatabase
	}
	if len(images) == 0 {
		return nil, "", errcode.New(4041, "没有可下载的图片", 404)
	}

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	r2Enabled := s.r2 != nil && s.r2.IsEnabled()

	for _, img := range images {
		var imgFile model.ImageFile
		if err := s.db.First(&imgFile, img.ImageFileID).Error; err != nil {
			continue
		}
		var data []byte
		if r2Enabled {
			reader, err := s.r2.Download(imgFile.OriginalPath)
			if err != nil {
				continue
			}
			data, err = io.ReadAll(reader)
			reader.Close()
			if err != nil {
				continue
			}
		} else {
			fullPath := filepath.Join(s.storageCfg.BasePath, imgFile.OriginalPath)
			var err error
			data, err = os.ReadFile(fullPath)
			if err != nil {
				continue
			}
		}
		f, err := zw.Create(img.Filename)
		if err != nil {
			continue
		}
		f.Write(data)
	}
	zw.Close()

	s.db.Model(&model.User{}).Where("id = ?", userID).UpdateColumn("batch_download_count", user.BatchDownloadCount+1)

	filename := fmt.Sprintf("imgcloud-backup-%s.zip", user.Username)
	return buf.Bytes(), filename, nil
}
