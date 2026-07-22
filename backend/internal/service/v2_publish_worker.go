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
	"os"
	"path/filepath"
	"time"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
	"golang.org/x/sys/unix"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const v2PublishJobType = "v2_publish"

const publishLeaseCleanupTimeout = 3 * time.Second

var (
	errV2PublishLeaseExhausted = errors.New("publish lease expired after maximum attempts")
	errV2PublishLeaseLost      = errors.New("publish job lease was lost")
	errV2PublishAlreadyDone    = errors.New("publish job already completed")
)

type v2WatermarkSnapshot struct {
	Enabled  bool
	Text     string
	Opacity  string
	Position string
	Size     string
	Color    string
	Version  string
}

type v2PublishJobPayload struct {
	ImageID           uint64 `json:"image_id"`
	UploadSessionID   uint64 `json:"upload_session_id"`
	SourcePath        string `json:"source_path"`
	TargetPath        string `json:"target_path"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	WatermarkEnabled  bool   `json:"watermark_enabled"`
	WatermarkText     string `json:"watermark_text"`
	WatermarkOpacity  string `json:"watermark_opacity"`
	WatermarkPosition string `json:"watermark_position"`
	WatermarkSize     string `json:"watermark_size"`
	WatermarkColor    string `json:"watermark_color"`
	WatermarkVersion  string `json:"watermark_version"`
}

// ProcessNextPublishJob claims at most one durable MySQL job. Multiple process
// instances may call it safely because the lease is acquired under row lock.
func (s *V2UploadService) ProcessNextPublishJob(ctx context.Context, workerID string) (processed bool, resultErr error) {
	var claimed *model.ProcessingJob
	refundAttempt := false
	defer func() {
		if recovered := recover(); recovered != nil {
			if claimed != nil {
				cause := fmt.Errorf("publish worker panic: %v", recovered)
				// A panic is a real failed attempt. Keep the increment so a
				// deterministic panic eventually reaches the attempt limit.
				if releaseErr := s.releaseInterruptedPublishJob(claimed, false, cause); releaseErr != nil {
					log.Printf("[v2_publish] release claim after panic: %v", releaseErr)
				}
			}
			panic(recovered)
		}
		if ctxErr := ctx.Err(); ctxErr != nil && claimed != nil {
			resultErr = errors.Join(
				resultErr,
				ctxErr,
				s.releaseInterruptedPublishJob(claimed, refundAttempt, ctxErr),
			)
		}
	}()

	exhausted, err := s.claimExhaustedPublishJob(ctx, workerID)
	if err == nil {
		claimed = exhausted
		return true, s.failPublishJob(ctx, exhausted, errV2PublishLeaseExhausted)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}

	job, err := s.claimPublishJob(ctx, workerID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	claimed = job
	refundAttempt = true
	if err := s.executePublishJobWithLease(ctx, job); err != nil {
		return true, s.failPublishJob(ctx, job, err)
	}
	return true, nil
}

// releaseInterruptedPublishJob uses a fresh context because the execution
// context is already canceled during a hard shutdown. Owner and token fencing
// prevent an old process from releasing a lease subsequently acquired by a
// replacement worker.
func (s *V2UploadService) releaseInterruptedPublishJob(job *model.ProcessingJob, refundAttempt bool, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), publishLeaseCleanupTimeout)
	defer cancel()
	return s.releasePublishJobLease(cleanupCtx, job, refundAttempt, time.Now(), cause)
}

func (s *V2UploadService) releasePublishJobLease(ctx context.Context, job *model.ProcessingJob, refundAttempt bool, now time.Time, cause error) error {
	if s == nil || s.db == nil || job == nil || job.LeaseOwner == nil || *job.LeaseOwner == "" || job.LeaseToken == nil || *job.LeaseToken == "" {
		return nil
	}
	lastError := "publish worker interrupted"
	if cause != nil {
		lastError += ": " + cause.Error()
	}
	if len(lastError) > 4096 {
		lastError = lastError[:4096]
	}
	updates := map[string]interface{}{
		"status":           model.ProcessingJobStatusRetry,
		"available_at":     now,
		"lease_owner":      nil,
		"lease_token":      nil,
		"lease_expires_at": nil,
		"last_error":       lastError,
	}
	if refundAttempt {
		updates["attempts"] = gorm.Expr("GREATEST(attempts - 1, 0)")
	}
	result := s.db.WithContext(ctx).Model(&model.ProcessingJob{}).
		Where(
			"id = ? AND status = ? AND lease_owner = ? AND lease_token = ?",
			job.ID,
			model.ProcessingJobStatusRunning,
			*job.LeaseOwner,
			*job.LeaseToken,
		).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	// Zero rows means the job completed or a replacement worker acquired a new
	// token first. The owner/token fence makes that a safe no-op.
	return nil
}

// claimExhaustedPublishJob fences a crashed worker before dead-job
// compensation. It also recovers retryable rows left at their attempt limit by
// older releases.
func (s *V2UploadService) claimExhaustedPublishJob(ctx context.Context, workerID string) (*model.ProcessingJob, error) {
	var claimed model.ProcessingJob
	now := time.Now()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("job_type = ? AND attempts >= max_attempts AND ((status = ? AND (lease_expires_at IS NULL OR lease_expires_at < ?)) OR (status IN ? AND available_at <= ?))",
				v2PublishJobType,
				model.ProcessingJobStatusRunning,
				now,
				[]string{model.ProcessingJobStatusQueued, model.ProcessingJobStatusRetry},
				now,
			).
			Order("priority DESC, id ASC").Limit(1).Find(&claimed)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		leaseToken, err := randomHex(16)
		if err != nil {
			return err
		}
		leaseExpires := now.Add(s.cfg.ImageV2.JobLease)
		if err := tx.Model(&claimed).Updates(map[string]interface{}{
			"status":           model.ProcessingJobStatusRunning,
			"lease_owner":      workerID,
			"lease_token":      leaseToken,
			"lease_expires_at": leaseExpires,
		}).Error; err != nil {
			return err
		}
		claimed.Status = model.ProcessingJobStatusRunning
		claimed.LeaseOwner = &workerID
		claimed.LeaseToken = &leaseToken
		claimed.LeaseExpiresAt = &leaseExpires
		return nil
	})
	return &claimed, err
}

func (s *V2UploadService) claimPublishJob(ctx context.Context, workerID string) (*model.ProcessingJob, error) {
	var claimed model.ProcessingJob
	now := time.Now()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ProcessingJob{}).
			Where("job_type = ? AND status = ? AND attempts < max_attempts AND (lease_expires_at IS NULL OR lease_expires_at < ?)", v2PublishJobType, model.ProcessingJobStatusRunning, now).
			Updates(map[string]interface{}{
				"status":           model.ProcessingJobStatusRetry,
				"available_at":     now,
				"lease_owner":      nil,
				"lease_token":      nil,
				"lease_expires_at": nil,
			}).Error; err != nil {
			return err
		}
		result := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("job_type = ? AND status IN ? AND attempts < max_attempts AND available_at <= ?", v2PublishJobType, []string{
				model.ProcessingJobStatusQueued, model.ProcessingJobStatusRetry,
			}, now).
			Order("priority DESC, id ASC").Limit(1).Find(&claimed)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			// Find keeps an idle queue on the normal path without GORM emitting a
			// record-not-found error log on every worker poll.
			return gorm.ErrRecordNotFound
		}
		leaseToken, err := randomHex(16)
		if err != nil {
			return err
		}
		leaseExpires := now.Add(s.cfg.ImageV2.JobLease)
		attempts := claimed.Attempts + 1
		if err := tx.Model(&claimed).Updates(map[string]interface{}{
			"status":           model.ProcessingJobStatusRunning,
			"attempts":         attempts,
			"lease_owner":      workerID,
			"lease_token":      leaseToken,
			"lease_expires_at": leaseExpires,
			"started_at":       now,
		}).Error; err != nil {
			return err
		}
		claimed.Status = model.ProcessingJobStatusRunning
		claimed.Attempts = attempts
		claimed.LeaseOwner = &workerID
		claimed.LeaseToken = &leaseToken
		claimed.LeaseExpiresAt = &leaseExpires
		return nil
	})
	return &claimed, err
}

func (s *V2UploadService) executePublishJobWithLease(ctx context.Context, job *model.ProcessingJob) error {
	executionCtx, cancel := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	go func() {
		heartbeatErr := s.maintainPublishJobLease(executionCtx, job)
		if heartbeatErr != nil {
			cancel()
		}
		heartbeatDone <- heartbeatErr
	}()

	executionErr := s.executePublishJob(executionCtx, job)
	cancel()
	heartbeatErr := <-heartbeatDone
	if errors.Is(heartbeatErr, errV2PublishAlreadyDone) {
		return nil
	}
	return errors.Join(executionErr, heartbeatErr)
}

func (s *V2UploadService) maintainPublishJobLease(ctx context.Context, job *model.ProcessingJob) error {
	interval := s.cfg.ImageV2.JobLease / 3
	if interval <= 0 {
		return errors.New("publish job lease interval is invalid")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			renewalTimeout := s.cfg.ImageV2.JobLease / 6
			if renewalTimeout <= 0 {
				renewalTimeout = time.Second
			}
			renewalCtx, cancel := context.WithTimeout(ctx, renewalTimeout)
			err := s.extendPublishJobLease(renewalCtx, job)
			cancel()
			if ctx.Err() != nil {
				return nil
			}
			if err != nil {
				return err
			}
		}
	}
}

func (s *V2UploadService) extendPublishJobLease(ctx context.Context, job *model.ProcessingJob) error {
	if job == nil || job.LeaseToken == nil || *job.LeaseToken == "" {
		return errV2PublishLeaseLost
	}
	leaseExpires := time.Now().Add(s.cfg.ImageV2.JobLease)
	result := s.db.WithContext(ctx).Model(&model.ProcessingJob{}).
		Where("id = ? AND status = ? AND lease_token = ?", job.ID, model.ProcessingJobStatusRunning, *job.LeaseToken).
		Update("lease_expires_at", leaseExpires)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var current model.ProcessingJob
	if err := s.db.WithContext(ctx).Select("status").First(&current, job.ID).Error; err != nil {
		return errors.Join(errV2PublishLeaseLost, err)
	}
	if current.Status == model.ProcessingJobStatusCompleted {
		return errV2PublishAlreadyDone
	}
	return errV2PublishLeaseLost
}

func (s *V2UploadService) executePublishJob(ctx context.Context, job *model.ProcessingJob) error {
	var payload v2PublishJobPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return fmt.Errorf("decode publish payload: %w", err)
	}
	if payload.ImageID == 0 || payload.UploadSessionID == 0 || payload.Width <= 0 || payload.Height <= 0 {
		return errors.New("publish payload is incomplete")
	}
	sourcePath, err := pathInside(s.cfg.Storage.StagingPath, payload.SourcePath)
	if err != nil {
		return fmt.Errorf("invalid publish source: %w", err)
	}
	targetPath, err := pathInside(s.cfg.Storage.BasePath, filepath.Join(s.cfg.Storage.BasePath, payload.TargetPath))
	if err != nil {
		return fmt.Errorf("invalid publish target: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0750); err != nil {
		return err
	}
	if err := syncV2DirectoryAncestors(filepath.Dir(targetPath), 3); err != nil {
		return err
	}

	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		if payload.WatermarkEnabled {
			if err := ensurePublishStorageCapacity(s.cfg.Storage, v2PublishOutputReserveBytes); err != nil {
				return err
			}
		}
		publishSourcePath := sourcePath
		if payload.WatermarkEnabled {
			watermarkConfig := map[string]string{
				"watermark_enabled":  "true",
				"watermark_text":     payload.WatermarkText,
				"watermark_opacity":  payload.WatermarkOpacity,
				"watermark_position": payload.WatermarkPosition,
				"watermark_size":     payload.WatermarkSize,
				"watermark_color":    payload.WatermarkColor,
			}
			tempPath := targetPath + "." + *job.LeaseToken + ".part"
			defer os.Remove(tempPath)
			url := s.imgproxy.ProcessedURL(sourcePath, true, payload.WatermarkText, payload.WatermarkOpacity, payload.WatermarkPosition, payload.WatermarkSize, payload.WatermarkColor)
			if err := withWatermarkSnapshot(ctx, watermarkConfig, s.cfg.Storage.BasePath, func() error {
				if _, _, err := s.imgproxy.ProcessToFile(ctx, url, tempPath, 32<<20); err != nil {
					return fmt.Errorf("stream watermarked publish: %w", err)
				}
				return nil
			}); err != nil {
				return fmt.Errorf("prepare and apply watermark: %w", err)
			}
			publishSourcePath = tempPath
		}
		if err := s.promotePublishTarget(ctx, job, publishSourcePath, targetPath); err != nil {
			return fmt.Errorf("promote publish target: %w", err)
		}
	} else if err != nil {
		return err
	}

	fileHash, fileSize, info, err := inspectWebPFile(targetPath)
	if err != nil {
		return err
	}
	if info.Animated || info.Width != payload.Width || info.Height != payload.Height {
		return errors.New("published image failed dimension or animation validation")
	}
	if err := os.Remove(sourcePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove publish source: %w", err)
	}
	stagingClean := true
	var session model.UploadSession
	if err := s.db.WithContext(ctx).First(&session, payload.UploadSessionID).Error; err != nil {
		return err
	}
	if err := os.Remove(session.StagingPath); err != nil && !os.IsNotExist(err) {
		stagingClean = false
	}

	now := time.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockV2Storage(tx); err != nil {
			return err
		}
		var lockedJob model.ProcessingJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ? AND lease_token = ?", job.ID, model.ProcessingJobStatusRunning, *job.LeaseToken).First(&lockedJob).Error; err != nil {
			return err
		}
		var image model.Image
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&image, payload.ImageID).Error; err != nil {
			return err
		}
		alreadyCompleted := image.ProcessingStatus == model.ImageProcessingStatusCompleted
		readyAt := now
		variant := model.ImageVariant{
			ImageID:          image.ID,
			Kind:             model.ImageVariantKindPublish,
			Revision:         1,
			PipelineVersion:  model.ImagePipelineVersionV2,
			Status:           model.ImageVariantStatusReady,
			FileHash:         fileHash,
			FileSize:         fileSize,
			MimeType:         "image/webp",
			Width:            info.Width,
			Height:           info.Height,
			Quality:          80,
			StoragePath:      payload.TargetPath,
			WatermarkVersion: &payload.WatermarkVersion,
			IsActive:         true,
			ReadyAt:          &readyAt,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "image_id"}, {Name: "kind"}, {Name: "revision"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "file_hash", "file_size", "mime_type", "width", "height", "storage_path", "watermark_version", "is_active", "ready_at"}),
		}).Create(&variant).Error; err != nil {
			return err
		}
		if err := tx.Where("image_id = ? AND kind = ?", image.ID, model.ImageVariantKindPublishSource).Delete(&model.ImageVariant{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.UploadPart{}).Where("upload_session_id = ? AND kind = ?", payload.UploadSessionID, model.ImageVariantKindPublishSource).
			Updates(map[string]interface{}{"status": model.UploadPartStatusCleaned, "cleaned_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&image).Updates(map[string]interface{}{"processing_status": model.ImageProcessingStatusCompleted}).Error; err != nil {
			return err
		}
		var actualBytes int64
		if err := tx.Model(&model.ImageVariant{}).Where("image_id = ? AND is_active = ?", image.ID, true).
			Select("COALESCE(SUM(file_size), 0)").Scan(&actualBytes).Error; err != nil {
			return err
		}
		if !alreadyCompleted {
			var user model.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, image.UserID).Error; err != nil {
				return err
			}
			var otherReserved int64
			if err := tx.Model(&model.UploadSession{}).
				Where("user_id = ? AND id <> ? AND "+v2ActiveReservationCondition, image.UserID, payload.UploadSessionID, []string{
					model.UploadSessionStatusInitiated,
					model.UploadSessionStatusUploading,
				}, now, model.UploadSessionStatusProcessing).
				Select("COALESCE(SUM(reserved_bytes), 0)").Scan(&otherReserved).Error; err != nil {
				return err
			}
			if user.StorageUsed+otherReserved+actualBytes > user.StorageQuota {
				return errV2QuotaExceeded
			}
		}
		sessionStatus := model.UploadSessionStatusCompleted
		updates := map[string]interface{}{
			"status": sessionStatus, "actual_bytes": actualBytes, "completed_at": now,
			"staging_path": "",
		}
		if !stagingClean {
			sessionStatus = model.UploadSessionStatusCleanupPending
			updates["status"] = sessionStatus
			updates["completed_at"] = nil
			updates["staging_path"] = session.StagingPath
			cleanupAfter := now.Add(5 * time.Minute)
			updates["cleanup_after"] = cleanupAfter
		}
		if err := tx.Model(&model.UploadSession{}).Where("id = ?", payload.UploadSessionID).Updates(updates).Error; err != nil {
			return err
		}
		if !alreadyCompleted {
			if err := tx.Model(&model.User{}).Where("id = ?", image.UserID).Updates(map[string]interface{}{
				"storage_used": gorm.Expr("storage_used + ?", actualBytes),
				"image_count":  gorm.Expr("image_count + 1"),
			}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&lockedJob).Updates(map[string]interface{}{
			"status":           model.ProcessingJobStatusCompleted,
			"completed_at":     now,
			"lease_owner":      nil,
			"lease_token":      nil,
			"lease_expires_at": nil,
			"last_error":       "",
		}).Error; err != nil {
			return err
		}
		eventPayload, _ := json.Marshal(map[string]interface{}{"image_id": image.ID, "asset_link": image.OriginAlias})
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.OutboxEvent{
			AggregateType: "image",
			AggregateID:   fmt.Sprint(image.ID),
			EventType:     "image.processing.completed",
			DedupeKey:     fmt.Sprintf("image:%d:processing:completed", image.ID),
			Status:        model.OutboxEventStatusPending,
			Payload:       string(eventPayload),
			AvailableAt:   now,
		}).Error
	})
}

// promotePublishTarget keeps expensive image processing outside MySQL while
// making the final filesystem publish linearizable with dead-job cleanup. A
// worker that lost its lease can never recreate a target after compensation.
func (s *V2UploadService) promotePublishTarget(ctx context.Context, job *model.ProcessingJob, sourcePath, targetPath string) error {
	if job == nil || job.LeaseToken == nil || *job.LeaseToken == "" {
		return errors.New("publish job has no lease token")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockV2Storage(tx); err != nil {
			return err
		}
		var lockedJob model.ProcessingJob
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ? AND lease_token = ?", job.ID, model.ProcessingJobStatusRunning, *job.LeaseToken).
			Limit(1).Find(&lockedJob)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if _, err := os.Stat(targetPath); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(sourcePath, targetPath); err != nil {
			return err
		}
		return syncV2Directory(filepath.Dir(targetPath))
	})
}

func (s *V2UploadService) failPublishJob(ctx context.Context, job *model.ProcessingJob, cause error) error {
	now := time.Now()
	status := model.ProcessingJobStatusRetry
	availableAt := now.Add(time.Duration(1<<minInt(int(job.Attempts), 8)) * time.Second)
	if job.Attempts >= job.MaxAttempts || errors.Is(cause, errV2QuotaExceeded) {
		status = model.ProcessingJobStatusDead
	}
	cleanupPaths := make([]string, 0, 8)
	if status == model.ProcessingJobStatusDead {
		var payload v2PublishJobPayload
		if json.Unmarshal([]byte(job.Payload), &payload) == nil && payload.TargetPath != "" {
			cleanupPaths = append(cleanupPaths, payload.TargetPath)
		}
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockV2Storage(tx); err != nil {
			return err
		}
		result := tx.Model(&model.ProcessingJob{}).Where("id = ? AND status = ? AND lease_token = ?", job.ID, model.ProcessingJobStatusRunning, *job.LeaseToken).
			Updates(map[string]interface{}{
				"status":           status,
				"available_at":     availableAt,
				"lease_owner":      nil,
				"lease_token":      nil,
				"lease_expires_at": nil,
				"last_error":       cause.Error(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if status == model.ProcessingJobStatusDead {
			if job.ImageID != nil {
				var image model.Image
				imageErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&image, *job.ImageID).Error
				if imageErr != nil && !errors.Is(imageErr, gorm.ErrRecordNotFound) {
					return imageErr
				}
				if imageErr == nil && image.ProcessingStatus != model.ImageProcessingStatusCompleted {
					var variants []model.ImageVariant
					if err := tx.Where("image_id = ?", image.ID).Find(&variants).Error; err != nil {
						return err
					}
					for _, variant := range variants {
						cleanupPaths = append(cleanupPaths, variant.StoragePath)
					}
					var imageFile model.ImageFile
					fileErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&imageFile, image.ImageFileID).Error
					if fileErr != nil && !errors.Is(fileErr, gorm.ErrRecordNotFound) {
						return fileErr
					}
					if err := tx.Where("image_id = ?", image.ID).Delete(&model.ImageVariant{}).Error; err != nil {
						return err
					}
					if err := tx.Where("image_id = ?", image.ID).Delete(&model.ImageAccessToken{}).Error; err != nil {
						return err
					}
					if err := tx.Delete(&image).Error; err != nil {
						return err
					}
					if fileErr == nil {
						var remainingReferences int64
						if err := tx.Model(&model.Image{}).Where("image_file_id = ?", imageFile.ID).Count(&remainingReferences).Error; err != nil {
							return err
						}
						if remainingReferences == 0 {
							cleanupPaths = append(cleanupPaths, imageFile.OriginalPath, imageFile.ThumbnailPath, imageFile.ProcessedPath)
							if err := tx.Delete(&imageFile).Error; err != nil {
								return err
							}
						} else if err := tx.Model(&imageFile).UpdateColumn("reference_count", remainingReferences).Error; err != nil {
							return err
						}
					}
				}
			}
			if job.UploadSessionID != nil {
				code := 1003
				if err := tx.Model(&model.UploadSession{}).Where("id = ?", *job.UploadSessionID).Updates(map[string]interface{}{
					"status": model.UploadSessionStatusFailed, "failed_at": now, "cleanup_after": now,
					"image_id": nil, "error_code": code, "error_message": cause.Error(),
				}).Error; err != nil {
					return err
				}
			}
			if err := EnqueueStorageDelete(
				tx,
				"processing_job",
				fmt.Sprint(job.ID),
				fmt.Sprintf("job:%d:storage-delete:%d", job.ID, now.UnixNano()),
				cleanupPaths,
				now,
			); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if status == model.ProcessingJobStatusDead {
		if cleanupErr := cleanupUnreferencedV2Files(s.db.WithContext(ctx), s.cfg.Storage.BasePath, cleanupPaths); cleanupErr != nil {
			return errors.Join(cause, cleanupErr)
		}
	}
	return cause
}

func ensurePublishStorageCapacity(cfg config.StorageConfig, incoming int64) error {
	var stat unix.Statfs_t
	if err := unix.Statfs(cfg.BasePath, &stat); err != nil {
		return fmt.Errorf("inspect storage capacity: %w", err)
	}
	total := uint64(stat.Blocks) * uint64(stat.Bsize)
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	if total == 0 || incoming < 0 {
		return errV2StoragePressure
	}
	usedPct := int((total - available) * 100 / total)
	const reserveFloor = uint64(256 << 20)
	if usedPct >= cfg.DiskHardPct || available < uint64(incoming)+reserveFloor {
		return errV2StoragePressure
	}
	return nil
}

func syncV2Directory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil &&
		!errors.Is(err, unix.EINVAL) &&
		!errors.Is(err, unix.ENOTSUP) &&
		!errors.Is(err, unix.EROFS) {
		return err
	}
	return nil
}

func syncV2DirectoryAncestors(path string, count int) error {
	current := filepath.Clean(path)
	for index := 0; index < count; index++ {
		if err := syncV2Directory(current); err != nil {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil
}

func inspectWebPFile(path string) (string, int64, webPInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, webPInfo{}, err
	}
	defer file.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", 0, webPInfo{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", 0, webPInfo{}, err
	}
	info, err := inspectCompleteWebP(file, size)
	if err != nil {
		return "", 0, webPInfo{}, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, info, nil
}

func pathInside(root, candidate string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil || relative == "." || relative == ".." || (len(relative) > 3 && relative[:3] == ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes storage root")
	}
	return candidateAbs, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
