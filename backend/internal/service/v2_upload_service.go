// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/repository"
	"golang.org/x/sys/unix"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errV2QuotaExceeded    = errors.New("v2 upload quota exceeded")
	errV2StateConflict    = errors.New("v2 upload state conflict")
	errV2SessionExpired   = errors.New("v2 upload session expired")
	errV2UploadIncomplete = errors.New("v2 upload incomplete")
	errV2PromotionFailed  = errors.New("v2 immutable promotion failed")
	errV2StoragePressure  = errors.New("v2 storage pressure")
	errV2SessionLimit     = errors.New("v2 active session limit reached")
	errV2PartStream       = errors.New("v2 upload part stream failed")
	errV2PartSize         = errors.New("v2 upload part size mismatch")
)

const v2CapacityAdmissionTimeout = 2 * time.Second

type v2CapacityLock struct {
	ID uint8
}

type V2UploadService struct {
	db               *gorm.DB
	cfg              *config.Config
	configRepo       *repository.SystemConfigRepo
	imgproxy         *ImgproxyService
	limiter          *v2UploadLimiter
	capacityGateOnce sync.Once
	capacityGate     chan struct{}
}

type v2PromotedPart struct {
	SourcePath   string
	RelativePath string
	Size         int64
	Hash         string
}

type immutableTargetPrevalidation struct {
	path string
	file *os.File
	info os.FileInfo
}

func NewV2UploadService(db *gorm.DB, cfg *config.Config, configRepo *repository.SystemConfigRepo, imgproxy *ImgproxyService) (*V2UploadService, error) {
	if err := os.MkdirAll(cfg.Storage.StagingPath, 0750); err != nil {
		return nil, fmt.Errorf("create V2 staging directory: %w", err)
	}
	if err := os.Chmod(cfg.Storage.StagingPath, 0750); err != nil {
		return nil, fmt.Errorf("set V2 staging directory permissions: %w", err)
	}
	if err := syncV2DirectoryAncestors(cfg.Storage.StagingPath, 3); err != nil {
		return nil, fmt.Errorf("sync V2 staging directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.Storage.BasePath, "v2"), 0750); err != nil {
		return nil, fmt.Errorf("create V2 storage directory: %w", err)
	}
	if err := syncV2DirectoryAncestors(filepath.Join(cfg.Storage.BasePath, "v2"), 2); err != nil {
		return nil, fmt.Errorf("sync V2 storage directory: %w", err)
	}
	return &V2UploadService{
		db:         db,
		cfg:        cfg,
		configRepo: configRepo,
		imgproxy:   imgproxy,
		limiter:    newV2UploadLimiter(cfg.ImageV2.GlobalUploadConcurrency, cfg.ImageV2.PerUserConcurrency),
	}, nil
}

func (s *V2UploadService) Recipe() V2RecipeResponse {
	response := V2RecipeResponse{
		V2Enabled:       s.cfg.ImageV2.Enabled,
		PipelineVersion: model.ImagePipelineVersionV2,
		RecipeVersion:   s.cfg.ImageV2.RecipeVersion,
		MaxPartBytes:    s.cfg.ImageV2.MaxPartBytes,
		MaxPixels:       s.cfg.ImageV2.MaxPixels,
		SessionTTLMs:    s.cfg.ImageV2.SessionTTL.Milliseconds(),
	}
	response.Variants = append(response.Variants,
		struct {
			Kind     string `json:"kind"`
			Width    int    `json:"width,omitempty"`
			Height   int    `json:"height,omitempty"`
			LongEdge int    `json:"long_edge,omitempty"`
			Quality  uint8  `json:"quality"`
			Fit      string `json:"fit"`
		}{Kind: model.ImageVariantKindMaster, Quality: 80, Fit: "original"},
		struct {
			Kind     string `json:"kind"`
			Width    int    `json:"width,omitempty"`
			Height   int    `json:"height,omitempty"`
			LongEdge int    `json:"long_edge,omitempty"`
			Quality  uint8  `json:"quality"`
			Fit      string `json:"fit"`
		}{Kind: model.ImageVariantKindGallery, Width: v2GalleryWidth, Height: v2GalleryHeight, Quality: 60, Fit: "cover"},
		struct {
			Kind     string `json:"kind"`
			Width    int    `json:"width,omitempty"`
			Height   int    `json:"height,omitempty"`
			LongEdge int    `json:"long_edge,omitempty"`
			Quality  uint8  `json:"quality"`
			Fit      string `json:"fit"`
		}{Kind: model.ImageVariantKindAdmin, Width: v2AdminWidth, Height: v2AdminHeight, Quality: 60, Fit: "cover"},
		struct {
			Kind     string `json:"kind"`
			Width    int    `json:"width,omitempty"`
			Height   int    `json:"height,omitempty"`
			LongEdge int    `json:"long_edge,omitempty"`
			Quality  uint8  `json:"quality"`
			Fit      string `json:"fit"`
		}{Kind: model.ImageVariantKindPublishSource, LongEdge: v2PublishEdge, Quality: 80, Fit: "contain"},
	)
	return response
}

func (s *V2UploadService) Init(ctx context.Context, userID uint64, idempotencyKey string, req *V2InitUploadRequest) (*V2UploadResponse, *errcode.AppError) {
	if !s.cfg.ImageV2.Enabled {
		return nil, errcode.New(5031, "V2 上传暂未启用", 503)
	}
	if appErr := validateV2Manifest(req, s.cfg.ImageV2); appErr != nil {
		return nil, appErr
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 64 {
		return nil, errcode.New(3005, "Idempotency-Key 必须包含 1 到 64 个字符", 400)
	}
	manifestJSON, err := json.Marshal(req)
	if err != nil {
		return nil, errcode.New(3005, "无效的上传清单", 400)
	}
	manifestSum := sha256.Sum256(manifestJSON)
	manifestHash := hex.EncodeToString(manifestSum[:])
	var existing model.UploadSession
	err = s.db.WithContext(ctx).Preload("Parts").Preload("Image").
		Where("user_id = ? AND idempotency_key = ?", userID, idempotencyKey).First(&existing).Error
	if err == nil {
		if !uploadManifestMatches(&existing, manifestHash) {
			return nil, errcode.ErrUploadConflict
		}
		return buildV2UploadResponse(&existing), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrDatabase
	}

	// The server-side watermark pass can temporarily coexist with every client
	// part, so reserve a bounded publish output in addition to uploaded bytes.
	reservedBytes := int64(v2PublishOutputReserveBytes)
	for _, part := range req.Parts {
		reservedBytes += part.Size
	}
	if appErr := s.checkStoragePressure(reservedBytes); appErr != nil {
		return nil, appErr
	}
	releaseCapacity, admitted := s.acquireCapacityGate(ctx)
	if !admitted {
		return nil, errcode.ErrUploadBusy
	}
	defer releaseCapacity()

	uploadKey, err := randomURLToken(24)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	stagingPath := filepath.Join(s.cfg.Storage.StagingPath, uploadKey)
	if err := os.Mkdir(stagingPath, 0750); err != nil {
		return nil, errcode.ErrInternal
	}
	if err := os.Chmod(stagingPath, 0750); err != nil {
		_ = os.Remove(stagingPath)
		return nil, errcode.ErrInternal
	}
	if err := syncV2Directory(filepath.Dir(stagingPath)); err != nil {
		_ = os.Remove(stagingPath)
		return nil, errcode.ErrInternal
	}
	cleanupEmptyStaging := true
	defer func() {
		if cleanupEmptyStaging {
			_ = os.Remove(stagingPath)
		}
	}()

	now := time.Now()
	manifestText := string(manifestJSON)
	session := model.UploadSession{
		UploadKey:         uploadKey,
		UserID:            userID,
		Status:            model.UploadSessionStatusInitiated,
		PipelineVersion:   model.ImagePipelineVersionV2,
		Visibility:        req.Visibility,
		Filename:          req.Filename,
		SourceMimeType:    strings.ToLower(req.Source.MimeType),
		SourceWidth:       req.Source.Width,
		SourceHeight:      req.Source.Height,
		ProcessorVersion:  req.ProcessorVersion,
		RecipeVersion:     req.RecipeVersion,
		ExpectedPartCount: uint8(len(req.Parts)),
		ReservedBytes:     reservedBytes,
		StagingPath:       stagingPath,
		ManifestHash:      &manifestHash,
		ClientManifest:    &manifestText,
		FocalX:            req.FocalX,
		FocalY:            req.FocalY,
		ExpiresAt:         now.Add(s.cfg.ImageV2.SessionTTL),
	}
	session.IdempotencyKey = &idempotencyKey
	parts := make([]model.UploadPart, 0, len(req.Parts))
	for _, part := range req.Parts {
		parts = append(parts, model.UploadPart{
			Kind:             part.Kind,
			Status:           model.UploadPartStatusPending,
			ExpectedSize:     part.Size,
			ExpectedHash:     part.SHA256,
			ExpectedWidth:    part.Width,
			ExpectedHeight:   part.Height,
			ExpectedMimeType: part.MimeType,
			StagingPath:      filepath.Join(stagingPath, part.Kind+".ready"),
		})
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockV2Storage(tx); err != nil {
			return err
		}
		var globalReserved int64
		if err := tx.Model(&model.UploadSession{}).
			Where(v2ActiveReservationCondition, []string{
				model.UploadSessionStatusInitiated,
				model.UploadSessionStatusUploading,
			}, now, model.UploadSessionStatusProcessing).
			Select("COALESCE(SUM(reserved_bytes), 0)").Scan(&globalReserved).Error; err != nil {
			return err
		}
		if appErr := s.checkStoragePressure(globalReserved + reservedBytes); appErr != nil {
			return errV2StoragePressure
		}
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			return err
		}
		var activeSessionCount int64
		if err := tx.Model(&model.UploadSession{}).
			Where("user_id = ? AND "+v2ActiveReservationCondition, userID, []string{
				model.UploadSessionStatusInitiated,
				model.UploadSessionStatusUploading,
			}, now, model.UploadSessionStatusProcessing).
			Count(&activeSessionCount).Error; err != nil {
			return err
		}
		if activeSessionCount >= v2MaximumActiveSessionsPerUser {
			return errV2SessionLimit
		}
		var activeReserved int64
		if err := tx.Model(&model.UploadSession{}).
			Where("user_id = ? AND "+v2ActiveReservationCondition, userID, []string{
				model.UploadSessionStatusInitiated,
				model.UploadSessionStatusUploading,
			}, now, model.UploadSessionStatusProcessing).
			Select("COALESCE(SUM(reserved_bytes), 0)").Scan(&activeReserved).Error; err != nil {
			return err
		}
		if user.StorageUsed+activeReserved+reservedBytes > user.StorageQuota {
			return errV2QuotaExceeded
		}
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		for i := range parts {
			parts[i].UploadSessionID = session.ID
		}
		return tx.Create(&parts).Error
	})
	if err != nil {
		var existing model.UploadSession
		if findErr := s.db.WithContext(ctx).Preload("Parts").Preload("Image").
			Where("user_id = ? AND idempotency_key = ?", userID, idempotencyKey).First(&existing).Error; findErr == nil {
			if !uploadManifestMatches(&existing, manifestHash) {
				return nil, errcode.ErrUploadConflict
			}
			return buildV2UploadResponse(&existing), nil
		}
	}
	if errors.Is(err, errV2QuotaExceeded) {
		return nil, errcode.ErrQuotaFull
	}
	if errors.Is(err, errV2StoragePressure) {
		return nil, errcode.ErrStoragePressure
	}
	if errors.Is(err, errV2SessionLimit) {
		return nil, errcode.ErrUploadBusy
	}
	if err != nil {
		return nil, errcode.ErrDatabase
	}
	cleanupEmptyStaging = false
	session.Parts = parts
	return buildV2UploadResponse(&session), nil
}

func (s *V2UploadService) Status(ctx context.Context, userID uint64, uploadKey string) (*V2UploadResponse, *errcode.AppError) {
	session, appErr := s.findSession(ctx, userID, uploadKey)
	if appErr != nil {
		return nil, appErr
	}
	return buildV2UploadResponse(session), nil
}

func (s *V2UploadService) BatchStatus(ctx context.Context, userID uint64, req *V2BatchStatusRequest) (*V2BatchStatusResponse, *errcode.AppError) {
	uploadIDs, appErr := normalizeV2UploadIDs(req)
	if appErr != nil {
		return nil, appErr
	}

	var sessions []model.UploadSession
	err := s.db.WithContext(ctx).Preload("Parts").Preload("Image").
		Where("user_id = ? AND upload_key IN ?", userID, uploadIDs).Find(&sessions).Error
	if err != nil {
		return nil, errcode.ErrDatabase
	}

	result, complete := buildV2BatchStatusResponse(uploadIDs, sessions)
	if !complete {
		// Missing and unauthorized IDs deliberately share one all-or-nothing result.
		return nil, errcode.ErrUploadSessionMissing
	}
	return result, nil
}

func (s *V2UploadService) PutPart(ctx context.Context, userID uint64, uploadKey, kind, contentType string, contentLength int64, body io.Reader) (*V2UploadPartResponse, *errcode.AppError) {
	session, appErr := s.findSession(ctx, userID, uploadKey)
	if appErr != nil {
		return nil, appErr
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, errcode.ErrUploadSessionMissing
	}
	if session.Status != model.UploadSessionStatusInitiated && session.Status != model.UploadSessionStatusUploading {
		return nil, errcode.ErrUploadConflict
	}
	part := findV2Part(session.Parts, kind)
	if part == nil {
		return nil, errcode.New(3005, "未知的上传部件", 404)
	}
	if part.Status == model.UploadPartStatusReceived || part.Status == model.UploadPartStatusFinalized {
		resp := v2PartResponse(uploadKey, *part)
		return &resp, nil
	}
	if contentLength >= 0 && contentLength != part.ExpectedSize {
		return nil, errcode.New(3002, "上传部件大小与清单不一致", 413)
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "image/webp" {
		return nil, errcode.ErrUnsupportedType
	}
	release, ok := s.limiter.acquire(userID)
	if !ok {
		return nil, errcode.ErrUploadBusy
	}
	defer release()
	if appErr := s.checkStorageHardPressure(part.ExpectedSize); appErr != nil {
		return nil, appErr
	}

	if err := os.MkdirAll(session.StagingPath, 0750); err != nil {
		return nil, errcode.ErrInternal
	}
	if err := os.Chmod(session.StagingPath, 0750); err != nil {
		return nil, errcode.ErrInternal
	}
	tempToken, err := randomHex(8)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	tempPath := filepath.Join(session.StagingPath, kind+"."+tempToken+".part")
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0640)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	if err := file.Chmod(0640); err != nil {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return nil, errcode.ErrInternal
	}
	removeTemp := true
	cleanupEmptyStaging := true
	defer func() {
		_ = file.Close()
		if removeTemp {
			_ = os.Remove(tempPath)
		}
		if cleanupEmptyStaging {
			// Never remove another request's files; os.Remove only succeeds when
			// this stale request recreated an otherwise empty session directory.
			_ = os.Remove(session.StagingPath)
		}
	}()

	written, actualHash, info, streamErr := streamV2WebPPart(file, body, part.ExpectedSize)
	if errors.Is(streamErr, errV2PartStream) {
		return nil, errcode.New(3006, "上传流读取失败", 400)
	}
	if errors.Is(streamErr, errV2PartSize) {
		return nil, errcode.New(3002, "上传部件大小与清单不一致", 413)
	}
	if streamErr != nil || info.Animated {
		return nil, errcode.ErrUnsupportedType
	}
	if actualHash != part.ExpectedHash {
		return nil, errcode.New(3007, "上传部件哈希校验失败", 422)
	}
	if info.Width != part.ExpectedWidth || info.Height != part.ExpectedHeight {
		return nil, errcode.New(3008, "上传部件尺寸与清单不一致", 422)
	}
	if err := file.Sync(); err != nil {
		return nil, errcode.ErrInternal
	}
	if err := file.Close(); err != nil {
		return nil, errcode.ErrInternal
	}
	var receivedAt time.Time
	renamed := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedSession model.UploadSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND upload_key = ?", session.ID, userID, uploadKey).
			First(&lockedSession).Error; err != nil {
			return err
		}
		receivedAt = time.Now()
		if !lockedSession.ExpiresAt.After(receivedAt) {
			return errV2SessionExpired
		}
		if lockedSession.Status != model.UploadSessionStatusInitiated && lockedSession.Status != model.UploadSessionStatusUploading {
			return errV2StateConflict
		}

		var locked model.UploadPart
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND upload_session_id = ?", part.ID, lockedSession.ID).
			First(&locked).Error; err != nil {
			return err
		}
		if locked.Status == model.UploadPartStatusReceived || locked.Status == model.UploadPartStatusFinalized {
			return nil
		}
		if locked.Status != model.UploadPartStatusPending {
			return errV2StateConflict
		}
		// Cleanup and cancellation update the same locked session row. Keeping
		// the canonical rename inside this short transaction prevents an old
		// request from recreating a file after cleanup has finalized.
		if err := os.Rename(tempPath, locked.StagingPath); err != nil {
			return err
		}
		renamed = true
		if err := syncV2Directory(lockedSession.StagingPath); err != nil {
			return err
		}
		if err := tx.Model(&locked).Updates(map[string]interface{}{
			"status":           model.UploadPartStatusReceived,
			"actual_size":      written,
			"actual_hash":      actualHash,
			"actual_width":     info.Width,
			"actual_height":    info.Height,
			"actual_mime_type": "image/webp",
			"received_at":      receivedAt,
		}).Error; err != nil {
			return err
		}
		result := tx.Model(&model.UploadSession{}).
			Where("id = ? AND status IN ? AND expires_at > ?", lockedSession.ID, []string{
				model.UploadSessionStatusInitiated,
				model.UploadSessionStatusUploading,
			}, receivedAt).
			Updates(map[string]interface{}{
				"status":              model.UploadSessionStatusUploading,
				"received_part_count": gorm.Expr("received_part_count + 1"),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errV2StateConflict
		}
		return nil
	})
	if err == nil {
		cleanupEmptyStaging = false
		if renamed {
			removeTemp = false
		}
	}
	if errors.Is(err, errV2SessionExpired) {
		return nil, errcode.ErrUploadSessionMissing
	}
	if errors.Is(err, errV2StateConflict) {
		return nil, errcode.ErrUploadConflict
	}
	if err != nil {
		return nil, errcode.ErrDatabase
	}
	part.Status = model.UploadPartStatusReceived
	part.ActualSize = written
	part.ActualHash = actualHash
	part.ActualWidth = info.Width
	part.ActualHeight = info.Height
	part.ActualMimeType = "image/webp"
	part.ReceivedAt = &receivedAt
	resp := v2PartResponse(uploadKey, *part)
	return &resp, nil
}

type v2PartStreamReader struct {
	reader io.Reader
	err    error
}

func (r *v2PartStreamReader) Read(buffer []byte) (int, error) {
	read, err := r.reader.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) && r.err == nil {
		r.err = err
	}
	return read, err
}

type v2PartStreamWriter struct {
	writer  io.Writer
	written int64
	err     error
}

func (w *v2PartStreamWriter) Write(buffer []byte) (int, error) {
	written, err := w.writer.Write(buffer)
	w.written += int64(written)
	if err == nil && written != len(buffer) {
		err = io.ErrShortWrite
	}
	if err != nil && w.err == nil {
		w.err = err
	}
	return written, err
}

// streamV2WebPPart validates the complete container while the same bytes are
// written and hashed. The one-byte look-ahead rejects a body longer than its
// manifest without rereading the staged file from disk.
func streamV2WebPPart(destination io.Writer, body io.Reader, expectedSize int64) (int64, string, webPInfo, error) {
	if destination == nil || body == nil || expectedSize < 1 {
		return 0, "", webPInfo{}, errV2PartSize
	}
	source := &v2PartStreamReader{reader: body}
	hasher := sha256.New()
	sink := &v2PartStreamWriter{writer: io.MultiWriter(destination, hasher)}
	stream := io.TeeReader(io.LimitReader(source, expectedSize+1), sink)
	info, inspectErr := inspectCompleteWebP(stream, expectedSize)
	_, drainErr := io.Copy(io.Discard, stream)

	if source.err != nil {
		return sink.written, "", webPInfo{}, fmt.Errorf("%w: %v", errV2PartStream, source.err)
	}
	if sink.err != nil {
		return sink.written, "", webPInfo{}, fmt.Errorf("%w: %v", errV2PartStream, sink.err)
	}
	if drainErr != nil {
		return sink.written, "", webPInfo{}, fmt.Errorf("%w: %v", errV2PartStream, drainErr)
	}
	if sink.written != expectedSize {
		return sink.written, "", webPInfo{}, errV2PartSize
	}
	if inspectErr != nil {
		return sink.written, "", webPInfo{}, inspectErr
	}
	return sink.written, hex.EncodeToString(hasher.Sum(nil)), info, nil
}

func (s *V2UploadService) Complete(ctx context.Context, userID uint64, uploadKey string) (*V2UploadResponse, *errcode.AppError) {
	prevalidatedTargets := s.prevalidateCompleteTargets(ctx, userID, uploadKey)
	defer closeImmutableTargetPrevalidations(prevalidatedTargets)

	releaseCapacity, admitted := s.acquireCapacityGate(ctx)
	if !admitted {
		return nil, errcode.ErrUploadBusy
	}
	defer releaseCapacity()

	var resultSession model.UploadSession
	promotedParts := make([]v2PromotedPart, 0, 3)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockV2Storage(tx); err != nil {
			return err
		}
		var locked model.UploadSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("upload_key = ? AND user_id = ?", uploadKey, userID).
			First(&locked).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errV2SessionExpired
			}
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("upload_session_id = ?", locked.ID).
			Order("id ASC").Find(&locked.Parts).Error; err != nil {
			return err
		}

		if locked.Status == model.UploadSessionStatusProcessing ||
			locked.Status == model.UploadSessionStatusCompleted ||
			(locked.Status == model.UploadSessionStatusCleanupPending && locked.ImageID != nil) {
			if locked.ImageID != nil {
				var image model.Image
				if err := tx.First(&image, *locked.ImageID).Error; err != nil {
					return err
				}
				locked.Image = &image
			}
			resultSession = locked
			return nil
		}
		if locked.Status != model.UploadSessionStatusUploading && locked.Status != model.UploadSessionStatusInitiated {
			return errV2StateConflict
		}
		now := time.Now()
		if !locked.ExpiresAt.After(now) {
			return errV2SessionExpired
		}
		for _, kind := range v2RequiredParts {
			part := findV2Part(locked.Parts, kind)
			if part == nil || part.Status != model.UploadPartStatusReceived {
				return errV2UploadIncomplete
			}
		}

		snapshot, err := s.watermarkSnapshot(tx)
		if err != nil {
			return err
		}
		finalPaths := make(map[string]string, 3)
		for _, kind := range []string{model.ImageVariantKindMaster, model.ImageVariantKindGallery, model.ImageVariantKindAdmin} {
			part := findV2Part(locked.Parts, kind)
			if len(part.ActualHash) != sha256.Size*2 {
				return fmt.Errorf("%w: invalid %s hash", errV2PromotionFailed, kind)
			}
			relativePath := filepath.Join("v2", kind, part.ActualHash[:2], part.ActualHash+".webp")
			promotedParts = append(promotedParts, v2PromotedPart{
				SourcePath: part.StagingPath, RelativePath: relativePath,
				Size: part.ActualSize, Hash: part.ActualHash,
			})
			targetPath := filepath.Join(s.cfg.Storage.BasePath, relativePath)
			if err := promoteImmutablePartWithPrevalidation(
				part.StagingPath,
				targetPath,
				part.ActualSize,
				part.ActualHash,
				prevalidatedTargets[targetPath],
			); err != nil {
				return fmt.Errorf("%w: %v", errV2PromotionFailed, err)
			}
			finalPaths[kind] = relativePath
		}

		master := findV2Part(locked.Parts, model.ImageVariantKindMaster)
		publishSource := findV2Part(locked.Parts, model.ImageVariantKindPublishSource)
		var imageFile model.ImageFile
		findResult := tx.Where("file_hash = ?", master.ActualHash).Limit(1).Find(&imageFile)
		if findResult.Error != nil {
			return findResult.Error
		}
		if findResult.RowsAffected == 0 {
			imageFile = model.ImageFile{
				FileHash:       master.ActualHash,
				FileSize:       master.ActualSize,
				MimeType:       "image/webp",
				Width:          master.ActualWidth,
				Height:         master.ActualHeight,
				ReferenceCount: 1,
				OriginalPath:   finalPaths[model.ImageVariantKindMaster],
				ThumbnailPath:  finalPaths[model.ImageVariantKindGallery],
			}
			if err := tx.Create(&imageFile).Error; err != nil {
				return err
			}
		} else if err := tx.Model(&imageFile).UpdateColumn("reference_count", gorm.Expr("reference_count + 1")).Error; err != nil {
			return err
		}

		uniqueLink, err := uniqueV2Link(tx)
		if err != nil {
			return err
		}
		assetLink := uniqueLink
		if locked.Visibility == "private" {
			assetLink += "S"
		}
		image := model.Image{
			UserID:           userID,
			ImageFileID:      imageFile.ID,
			UniqueLink:       uniqueLink,
			OriginAlias:      &assetLink,
			Title:            locked.Filename,
			Filename:         locked.Filename,
			Visibility:       locked.Visibility,
			PipelineVersion:  model.ImagePipelineVersionV2,
			ProcessingStatus: model.ImageProcessingStatusProcessing,
			Width:            locked.SourceWidth,
			Height:           locked.SourceHeight,
			FileSize:         master.ActualSize,
		}
		if err := tx.Create(&image).Error; err != nil {
			return err
		}

		for _, kind := range []string{model.ImageVariantKindMaster, model.ImageVariantKindGallery, model.ImageVariantKindAdmin} {
			part := findV2Part(locked.Parts, kind)
			readyAt := now
			variant := model.ImageVariant{
				ImageID:         image.ID,
				Kind:            kind,
				Revision:        1,
				PipelineVersion: model.ImagePipelineVersionV2,
				Status:          model.ImageVariantStatusReady,
				FileHash:        part.ActualHash,
				FileSize:        part.ActualSize,
				MimeType:        "image/webp",
				Width:           part.ActualWidth,
				Height:          part.ActualHeight,
				Quality:         v2PartQuality(kind),
				StoragePath:     finalPaths[kind],
				IsActive:        true,
				ReadyAt:         &readyAt,
			}
			if err := tx.Create(&variant).Error; err != nil {
				return err
			}
			finalPath := finalPaths[kind]
			if err := tx.Model(&model.UploadPart{}).Where("id = ?", part.ID).Updates(map[string]interface{}{
				"status":       model.UploadPartStatusFinalized,
				"final_path":   finalPath,
				"finalized_at": now,
			}).Error; err != nil {
				return err
			}
		}
		publishVariant := model.ImageVariant{
			ImageID:         image.ID,
			Kind:            model.ImageVariantKindPublishSource,
			Revision:        1,
			PipelineVersion: model.ImagePipelineVersionV2,
			Status:          model.ImageVariantStatusPending,
			FileHash:        publishSource.ActualHash,
			FileSize:        publishSource.ActualSize,
			MimeType:        "image/webp",
			Width:           publishSource.ActualWidth,
			Height:          publishSource.ActualHeight,
			Quality:         80,
			StoragePath:     publishSource.StagingPath,
			IsActive:        false,
		}
		if err := tx.Create(&publishVariant).Error; err != nil {
			return err
		}

		targetPath := filepath.Join("v2", model.ImageVariantKindPublish, fmt.Sprintf("%d", image.ID), "v1.webp")
		payload := v2PublishJobPayload{
			ImageID:           image.ID,
			UploadSessionID:   locked.ID,
			SourcePath:        publishSource.StagingPath,
			TargetPath:        targetPath,
			Width:             publishSource.ActualWidth,
			Height:            publishSource.ActualHeight,
			WatermarkEnabled:  snapshot.Enabled,
			WatermarkText:     snapshot.Text,
			WatermarkOpacity:  snapshot.Opacity,
			WatermarkPosition: snapshot.Position,
			WatermarkSize:     snapshot.Size,
			WatermarkColor:    snapshot.Color,
			WatermarkVersion:  snapshot.Version,
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		job := model.ProcessingJob{
			JobType:         v2PublishJobType,
			DedupeKey:       fmt.Sprintf("image:%d:publish:1", image.ID),
			ImageID:         &image.ID,
			UploadSessionID: &locked.ID,
			Status:          model.ProcessingJobStatusQueued,
			Priority:        100,
			Payload:         string(payloadJSON),
			MaxAttempts:     5,
			AvailableAt:     now,
		}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		eventPayload, _ := json.Marshal(map[string]interface{}{"image_id": image.ID, "upload_id": locked.UploadKey})
		event := model.OutboxEvent{
			AggregateType: "image",
			AggregateID:   fmt.Sprint(image.ID),
			EventType:     "image.processing.started",
			DedupeKey:     fmt.Sprintf("image:%d:processing:started", image.ID),
			Status:        model.OutboxEventStatusPending,
			Payload:       string(eventPayload),
			AvailableAt:   now,
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		if err := tx.Model(&locked).Updates(map[string]interface{}{
			"status":        model.UploadSessionStatusProcessing,
			"image_id":      image.ID,
			"processing_at": now,
		}).Error; err != nil {
			return err
		}
		locked.ImageID = &image.ID
		locked.Image = &image
		locked.Status = model.UploadSessionStatusProcessing
		locked.ProcessingAt = &now
		for i := range locked.Parts {
			switch locked.Parts[i].Kind {
			case model.ImageVariantKindMaster, model.ImageVariantKindGallery, model.ImageVariantKindAdmin:
				locked.Parts[i].Status = model.UploadPartStatusFinalized
				finalPath := finalPaths[locked.Parts[i].Kind]
				locked.Parts[i].FinalPath = &finalPath
				locked.Parts[i].FinalizedAt = &now
			}
		}
		resultSession = locked
		return nil
	})
	if err != nil && len(promotedParts) > 0 {
		cleanupPaths, restoreErr := restorePromotedV2Parts(
			s.db, s.cfg.Storage.BasePath, s.cfg.Storage.StagingPath, promotedParts,
		)
		if restoreErr != nil {
			log.Printf("failed to restore V2 staging files after Complete rollback: %v", restoreErr)
		}
		if len(cleanupPaths) > 0 {
			if cleanupErr := cleanupUnreferencedV2Files(s.db, s.cfg.Storage.BasePath, cleanupPaths); cleanupErr != nil {
				log.Printf("failed to clean V2 persistent files after Complete rollback: %v", cleanupErr)
			}
		}
	}
	if errors.Is(err, errV2SessionExpired) {
		return nil, errcode.ErrUploadSessionMissing
	}
	if errors.Is(err, errV2UploadIncomplete) {
		return nil, errcode.ErrUploadIncomplete
	}
	if errors.Is(err, errV2StateConflict) {
		return nil, errcode.ErrUploadConflict
	}
	if errors.Is(err, errV2PromotionFailed) {
		return nil, errcode.ErrInternal
	}
	if err != nil {
		return nil, errcode.ErrDatabase
	}
	return buildV2UploadResponse(&resultSession), nil
}

// restorePromotedV2Parts preserves the retryable staging copy before rollback
// compensation removes an unreferenced persistent target. The hard-link fast
// path adds no image read/write; copying is only a fallback for filesystems
// that reject hard links despite supporting same-filesystem atomic rename.
func restorePromotedV2Parts(db *gorm.DB, storageRoot, stagingRoot string, parts []v2PromotedPart) ([]string, error) {
	cleanupPaths := make([]string, 0, len(parts))
	var restoreErr error
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := lockV2Storage(tx); err != nil {
			return err
		}
		for _, part := range parts {
			targetPath, err := pathInside(storageRoot, filepath.Join(storageRoot, part.RelativePath))
			if err != nil {
				restoreErr = errors.Join(restoreErr, err)
				continue
			}
			sourcePath, err := pathInside(stagingRoot, part.SourcePath)
			if err != nil {
				restoreErr = errors.Join(restoreErr, err)
				continue
			}
			if err := verifyImmutableTarget(sourcePath, part.Size, part.Hash); err == nil {
				cleanupPaths = append(cleanupPaths, part.RelativePath)
				continue
			} else if !os.IsNotExist(err) {
				restoreErr = errors.Join(restoreErr, err)
				continue
			}
			if err := verifyImmutableTarget(targetPath, part.Size, part.Hash); err != nil {
				restoreErr = errors.Join(restoreErr, err)
				continue
			}
			if err := os.MkdirAll(filepath.Dir(sourcePath), 0750); err != nil {
				restoreErr = errors.Join(restoreErr, err)
				continue
			}
			if err := syncV2DirectoryAncestors(filepath.Dir(sourcePath), 2); err != nil {
				restoreErr = errors.Join(restoreErr, err)
				continue
			}
			if err := os.Link(targetPath, sourcePath); err != nil {
				if err := copyImmutableRecoveryFile(targetPath, sourcePath, part.Size, part.Hash); err != nil {
					restoreErr = errors.Join(restoreErr, err)
					continue
				}
			} else if err := syncV2Directory(filepath.Dir(sourcePath)); err != nil {
				restoreErr = errors.Join(restoreErr, err)
				continue
			}
			cleanupPaths = append(cleanupPaths, part.RelativePath)
		}
		return nil
	})
	return cleanupPaths, errors.Join(err, restoreErr)
}

func copyImmutableRecoveryFile(sourcePath, targetPath string, expectedSize int64, expectedHash string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if errors.Is(err, os.ErrExist) {
		if err := verifyImmutableTarget(targetPath, expectedSize, expectedHash); err != nil {
			return err
		}
		return syncV2Directory(filepath.Dir(targetPath))
	}
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		_ = target.Close()
		if !keep {
			_ = os.Remove(targetPath)
		}
	}()
	if _, err := io.Copy(target, source); err != nil {
		return err
	}
	if err := target.Sync(); err != nil {
		return err
	}
	if err := target.Close(); err != nil {
		return err
	}
	if err := verifyImmutableTarget(targetPath, expectedSize, expectedHash); err != nil {
		return err
	}
	if err := syncV2Directory(filepath.Dir(targetPath)); err != nil {
		return err
	}
	keep = true
	return nil
}

func lockV2Storage(tx *gorm.DB) error {
	var capacityLock v2CapacityLock
	return tx.Table("v2_capacity_locks").Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", 1).Take(&capacityLock).Error
}

func (s *V2UploadService) acquireCapacityGate(ctx context.Context) (func(), bool) {
	s.capacityGateOnce.Do(func() {
		s.capacityGate = make(chan struct{}, 1)
	})
	return acquireBoundedV2Gate(ctx, s.capacityGate, v2CapacityAdmissionTimeout)
}

func acquireBoundedV2Gate(ctx context.Context, gate chan struct{}, timeout time.Duration) (func(), bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case gate <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() { <-gate })
		}, true
	case <-ctx.Done():
		return nil, false
	case <-timer.C:
		return nil, false
	}
}

func (s *V2UploadService) Cancel(ctx context.Context, userID uint64, uploadKey string) (*V2UploadResponse, *errcode.AppError) {
	session, appErr := s.findSession(ctx, userID, uploadKey)
	if appErr != nil {
		return nil, appErr
	}
	if session.Status != model.UploadSessionStatusInitiated && session.Status != model.UploadSessionStatusUploading {
		return nil, errcode.ErrUploadConflict
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if result := tx.Model(&model.UploadSession{}).Where("id = ? AND status IN ?", session.ID, []string{
			model.UploadSessionStatusInitiated, model.UploadSessionStatusUploading,
		}).Updates(map[string]interface{}{"status": model.UploadSessionStatusCancelled, "cancelled_at": now}); result.Error != nil {
			return result.Error
		} else if result.RowsAffected != 1 {
			return errV2StateConflict
		}
		return tx.Model(&model.UploadPart{}).Where("upload_session_id = ?", session.ID).
			Updates(map[string]interface{}{"status": model.UploadPartStatusCleaned, "cleaned_at": now}).Error
	}); err != nil {
		if errors.Is(err, errV2StateConflict) {
			return nil, errcode.ErrUploadConflict
		}
		return nil, errcode.ErrDatabase
	}
	if err := os.RemoveAll(session.StagingPath); err != nil {
		_ = s.db.WithContext(ctx).Model(&model.UploadSession{}).Where("id = ?", session.ID).
			Update("status", model.UploadSessionStatusCleanupPending).Error
		session.Status = model.UploadSessionStatusCleanupPending
	} else {
		session.Status = model.UploadSessionStatusCancelled
		session.CancelledAt = &now
	}
	return buildV2UploadResponse(session), nil
}

func (s *V2UploadService) findSession(ctx context.Context, userID uint64, uploadKey string) (*model.UploadSession, *errcode.AppError) {
	var session model.UploadSession
	err := s.db.WithContext(ctx).Preload("Parts").Preload("Image").
		Where("upload_key = ? AND user_id = ?", uploadKey, userID).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrUploadSessionMissing
	}
	if err != nil {
		return nil, errcode.ErrDatabase
	}
	return &session, nil
}

func (s *V2UploadService) checkStoragePressure(incoming int64) *errcode.AppError {
	return s.checkStoragePressureAt(incoming, s.cfg.Storage.DiskSoftPct)
}

func (s *V2UploadService) checkStorageHardPressure(incoming int64) *errcode.AppError {
	return s.checkStoragePressureAt(incoming, s.cfg.Storage.DiskHardPct)
}

func (s *V2UploadService) checkStoragePressureAt(incoming int64, limitPercent int) *errcode.AppError {
	if incoming < 0 {
		return errcode.ErrStoragePressure
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(s.cfg.Storage.BasePath, &stat); err != nil {
		return errcode.ErrStoragePressure
	}
	total := uint64(stat.Blocks) * uint64(stat.Bsize)
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	if total == 0 {
		return errcode.ErrStoragePressure
	}
	usedPct := int((total - available) * 100 / total)
	reserveFloor := uint64(256 << 20)
	if usedPct >= limitPercent || available < uint64(incoming)+reserveFloor {
		return errcode.ErrStoragePressure
	}
	return nil
}

func (s *V2UploadService) watermarkSnapshot(db *gorm.DB) (v2WatermarkSnapshot, error) {
	var configs []model.SystemConfig
	err := db.Find(&configs).Error
	if err != nil {
		return v2WatermarkSnapshot{}, err
	}
	values := make(map[string]string, len(configs))
	for _, item := range configs {
		values[item.ConfigKey] = item.ConfigValue
	}
	snapshot := v2WatermarkSnapshot{
		Enabled:  values["watermark_enabled"] == "true",
		Text:     values["watermark_text"],
		Opacity:  values["watermark_opacity"],
		Position: values["watermark_position"],
		Size:     values["watermark_size"],
		Color:    values["watermark_color"],
	}
	canonical := strings.Join([]string{
		fmt.Sprint(snapshot.Enabled), snapshot.Text, snapshot.Opacity,
		snapshot.Position, snapshot.Size, snapshot.Color,
	}, "\x00")
	sum := sha256.Sum256([]byte(canonical))
	snapshot.Version = hex.EncodeToString(sum[:])
	return snapshot, nil
}

func buildV2UploadResponse(session *model.UploadSession) *V2UploadResponse {
	resp := &V2UploadResponse{
		UploadID:  session.UploadKey,
		Status:    session.Status,
		ImageID:   session.ImageID,
		ExpiresAt: session.ExpiresAt,
		Parts:     make([]V2UploadPartResponse, 0, len(session.Parts)),
	}
	if session.Image != nil {
		resp.UniqueLink = session.Image.UniqueLink
		if session.Image.OriginAlias != nil {
			resp.AssetLink = *session.Image.OriginAlias
		}
	}
	for _, part := range session.Parts {
		resp.Parts = append(resp.Parts, v2PartResponse(session.UploadKey, part))
	}
	return resp
}

func buildV2BatchStatusResponse(uploadIDs []string, sessions []model.UploadSession) (*V2BatchStatusResponse, bool) {
	byUploadID := make(map[string]*model.UploadSession, len(sessions))
	for i := range sessions {
		byUploadID[sessions[i].UploadKey] = &sessions[i]
	}

	result := &V2BatchStatusResponse{Uploads: make([]V2UploadResponse, 0, len(uploadIDs))}
	for _, uploadID := range uploadIDs {
		session, exists := byUploadID[uploadID]
		if !exists {
			return nil, false
		}
		result.Uploads = append(result.Uploads, *buildV2UploadResponse(session))
	}
	return result, true
}

func v2PartResponse(uploadKey string, part model.UploadPart) V2UploadPartResponse {
	resp := V2UploadPartResponse{
		Kind:   part.Kind,
		Status: part.Status,
		Size:   part.ExpectedSize,
		SHA256: part.ExpectedHash,
		Width:  part.ExpectedWidth,
		Height: part.ExpectedHeight,
	}
	if part.Status == model.UploadPartStatusPending {
		resp.PutURL = fmt.Sprintf("/api/v1/uploads/%s/parts/%s", uploadKey, part.Kind)
	}
	return resp
}

func findV2Part(parts []model.UploadPart, kind string) *model.UploadPart {
	for i := range parts {
		if parts[i].Kind == kind {
			return &parts[i]
		}
	}
	return nil
}

func uploadManifestMatches(session *model.UploadSession, manifestHash string) bool {
	return session.ManifestHash != nil && *session.ManifestHash == manifestHash
}

func (s *V2UploadService) prevalidateCompleteTargets(ctx context.Context, userID uint64, uploadKey string) map[string]*immutableTargetPrevalidation {
	prevalidated := make(map[string]*immutableTargetPrevalidation, 3)
	var session model.UploadSession
	result := s.db.WithContext(ctx).
		Where("upload_key = ? AND user_id = ?", uploadKey, userID).
		Limit(1).
		Find(&session)
	if result.Error != nil || result.RowsAffected == 0 {
		return prevalidated
	}
	if err := s.db.WithContext(ctx).
		Where("upload_session_id = ? AND status = ?", session.ID, model.UploadPartStatusReceived).
		Find(&session.Parts).Error; err != nil {
		return prevalidated
	}
	for _, kind := range []string{model.ImageVariantKindMaster, model.ImageVariantKindGallery, model.ImageVariantKindAdmin} {
		part := findV2Part(session.Parts, kind)
		if part == nil || len(part.ActualHash) != sha256.Size*2 {
			continue
		}
		relativePath := filepath.Join("v2", kind, part.ActualHash[:2], part.ActualHash+".webp")
		targetPath := filepath.Join(s.cfg.Storage.BasePath, relativePath)
		snapshot, err := prevalidateImmutableTarget(targetPath, part.ActualSize, part.ActualHash)
		if err == nil && snapshot != nil {
			prevalidated[targetPath] = snapshot
		}
	}
	return prevalidated
}

func promoteImmutablePart(sourcePath, targetPath string, expectedSize int64, expectedHash string) error {
	return promoteImmutablePartWithPrevalidation(sourcePath, targetPath, expectedSize, expectedHash, nil)
}

func promoteImmutablePartWithPrevalidation(
	sourcePath, targetPath string,
	expectedSize int64,
	expectedHash string,
	prevalidated *immutableTargetPrevalidation,
) error {
	return promoteImmutablePartWithVerifier(
		sourcePath, targetPath, expectedSize, expectedHash, prevalidated, verifyImmutableTarget,
	)
}

func promoteImmutablePartWithVerifier(
	sourcePath, targetPath string,
	expectedSize int64,
	expectedHash string,
	prevalidated *immutableTargetPrevalidation,
	verifyTarget func(string, int64, string) error,
) error {
	if immutableTargetStillMatches(targetPath, expectedSize, prevalidated) {
		if err := syncV2Directory(filepath.Dir(targetPath)); err != nil {
			return err
		}
		return removePromotedSource(sourcePath)
	}
	if err := verifyTarget(targetPath, expectedSize, expectedHash); err == nil {
		if err := syncV2Directory(filepath.Dir(targetPath)); err != nil {
			return err
		}
		return removePromotedSource(sourcePath)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0750); err != nil {
		return err
	}
	if err := syncV2DirectoryAncestors(filepath.Dir(targetPath), 3); err != nil {
		return err
	}
	err := unix.Renameat2(unix.AT_FDCWD, sourcePath, unix.AT_FDCWD, targetPath, unix.RENAME_NOREPLACE)
	if err == nil {
		if err := syncV2Directory(filepath.Dir(targetPath)); err != nil {
			return err
		}
		return syncV2Directory(filepath.Dir(sourcePath))
	}
	if !errors.Is(err, unix.EEXIST) {
		return err
	}
	if err := verifyTarget(targetPath, expectedSize, expectedHash); err != nil {
		return err
	}
	if err := syncV2Directory(filepath.Dir(targetPath)); err != nil {
		return err
	}
	return removePromotedSource(sourcePath)
}

func prevalidateImmutableTarget(path string, expectedSize int64, expectedHash string) (*immutableTargetPrevalidation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	keepOpen := false
	defer func() {
		if !keepOpen {
			_ = file.Close()
		}
	}()

	before, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err := validateImmutableTargetInfo(path, before, expectedSize); err != nil {
		return nil, err
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	current, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !sameImmutableTargetFile(before, after) || !sameImmutableTargetFile(after, current) {
		return nil, fmt.Errorf("immutable target changed during prevalidation: %s", path)
	}
	if hex.EncodeToString(hasher.Sum(nil)) != expectedHash {
		return nil, fmt.Errorf("immutable target hash mismatch: %s", path)
	}
	keepOpen = true
	return &immutableTargetPrevalidation{path: filepath.Clean(path), file: file, info: current}, nil
}

func immutableTargetStillMatches(path string, expectedSize int64, prevalidated *immutableTargetPrevalidation) bool {
	if prevalidated == nil || prevalidated.file == nil || prevalidated.info == nil || filepath.Clean(path) != prevalidated.path {
		return false
	}
	held, err := prevalidated.file.Stat()
	if err != nil || validateImmutableTargetInfo(path, held, expectedSize) != nil ||
		!sameImmutableTargetFile(prevalidated.info, held) {
		return false
	}
	current, err := os.Stat(path)
	if err != nil || validateImmutableTargetInfo(path, current, expectedSize) != nil {
		return false
	}
	return sameImmutableTargetFile(held, current)
}

func closeImmutableTargetPrevalidations(prevalidated map[string]*immutableTargetPrevalidation) {
	for _, snapshot := range prevalidated {
		if snapshot != nil && snapshot.file != nil {
			_ = snapshot.file.Close()
			snapshot.file = nil
		}
	}
}

func sameImmutableTargetFile(left, right os.FileInfo) bool {
	return left != nil && right != nil &&
		os.SameFile(left, right) &&
		left.Mode() == right.Mode() &&
		left.Size() == right.Size() &&
		left.ModTime().Equal(right.ModTime())
}

func verifyImmutableTarget(path string, expectedSize int64, expectedHash string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if err := validateImmutableTargetInfo(path, info, expectedSize); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	hasher := sha256.New()
	_, copyErr := io.Copy(hasher, file)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if hex.EncodeToString(hasher.Sum(nil)) != expectedHash {
		return fmt.Errorf("immutable target hash mismatch: %s", path)
	}
	return nil
}

func validateImmutableTargetInfo(path string, info os.FileInfo, expectedSize int64) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("immutable target is not a regular file: %s", path)
	}
	if info.Size() != expectedSize {
		return fmt.Errorf("immutable target size mismatch: %s", path)
	}
	return nil
}

func removePromotedSource(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return syncV2Directory(filepath.Dir(path))
}

func uniqueV2Link(tx *gorm.DB) (string, error) {
	for attempt := 0; attempt < 8; attempt++ {
		link, err := randomHex(6)
		if err != nil {
			return "", err
		}
		var count int64
		if err := tx.Model(&model.Image{}).Where("unique_link = ? OR origin_alias = ? OR origin_alias = ?", link, link, link+"S").Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return link, nil
		}
	}
	return "", errors.New("failed to allocate unique image link")
}

type prefixWriter struct {
	data []byte
	max  int
}

func (w *prefixWriter) Write(data []byte) (int, error) {
	if len(w.data) < w.max {
		remaining := w.max - len(w.data)
		if remaining > len(data) {
			remaining = len(data)
		}
		w.data = append(w.data, data[:remaining]...)
	}
	return len(data), nil
}
