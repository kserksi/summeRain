// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/service"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errUserDeletionBusy = errors.New("user still has active or pending-cleanup image work")

const (
	userDeletionBatchSize           = 25
	userDeletionImageBatchSize      = 25
	userDeletionRowBatchSize        = 100
	userDeletionMaxBatchesPerPass   = 4
	userDeletionStatusPending       = "pending_deletion"
	userDeletionStatusDeleting      = "deleting"
	userDeletionPhaseProcessingJobs = "processing_jobs"
	userDeletionPhaseAccessTokens   = "image_access_tokens"
	userDeletionPhaseVariants       = "image_variants"
	userDeletionPhaseImages         = "images"
	userDeletionPhaseCSRF           = "csrf_tokens"
	userDeletionPhaseUploadParts    = "upload_parts"
	userDeletionPhaseUploads        = "upload_sessions"
	userDeletionPhaseSessions       = "sessions"
	userDeletionPhaseNotifications  = "notifications"
	userDeletionPhaseAuditLogs      = "audit_logs"
	userDeletionPhaseUploadQueues   = "upload_queues"
)

type userDeletionCleanup struct {
	paths         []string
	remoteObjects []model.StorageDeleteRemoteObject
	phase         string
	rows          int
	progressed    bool
	completed     bool
}

type userDeletionCapacityLock struct {
	ID uint8
}

func (m *Manager) runUserDeletion(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[user-deletion] panic recovered: %v", r)
					}
				}()
				m.processPendingDeletions()
			}()
		}
	}
}

func (m *Manager) processPendingDeletions() {
	users, err := listPendingDeletionUsers(m.DB, time.Now())
	if err != nil {
		log.Printf("[user-deletion] query error: %v", err)
		return
	}

	for _, user := range users {
		log.Printf("[user-deletion] processing permanent deletion for user %s (id=%d)", user.Username, user.ID)
		m.permanentlyDeleteUser(user.ID)
	}
}

func listPendingDeletionUsers(db *gorm.DB, now time.Time) ([]model.User, error) {
	var users []model.User
	err := db.Where("(status = ? AND deletion_scheduled_at < ?) OR status = ?", userDeletionStatusPending, now, userDeletionStatusDeleting).
		Order("deletion_scheduled_at ASC, id ASC").
		Limit(userDeletionBatchSize).
		Find(&users).Error
	return users, err
}

func (m *Manager) permanentlyDeleteUser(userID uint64) {
	removedImages := 0
	for batch := 0; batch < userDeletionMaxBatchesPerPass; batch++ {
		cleanup, imageCount, err := m.deleteUserRecords(userID, time.Now())
		if err != nil {
			if errors.Is(err, errUserDeletionBusy) {
				log.Printf("[user-deletion] user %d still has active image processing; retrying later", userID)
				return
			}
			log.Printf("[user-deletion] failed to delete user %d: %v", userID, err)
			return
		}
		if cleanup == nil {
			return
		}
		removedImages += imageCount
		if cleanup.completed {
			log.Printf("[user-deletion] user %d permanently deleted (%d images removed in this pass; storage cleanup queued)", userID, removedImages)
			return
		}
		if !cleanup.progressed {
			log.Printf("[user-deletion] user %d made no deletion progress; retrying later", userID)
			return
		}
		if batch == userDeletionMaxBatchesPerPass-1 {
			log.Printf("[user-deletion] user %d deletion progressed through %s (%d rows); continuing next pass", userID, cleanup.phase, cleanup.rows)
		}
	}
}

// deleteUserRecords owns the reference-count transition. No filesystem object
// is removed until this transaction commits successfully.
func (m *Manager) deleteUserRecords(userID uint64, now time.Time) (*userDeletionCleanup, int, error) {
	cleanup := &userDeletionCleanup{}
	imageCount := 0
	err := m.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				cleanup = nil
				return nil
			}
			return err
		}
		// Recheck under the row lock so an administrator cancelling deletion
		// cannot race the worker's earlier candidate query.
		if user.Status != userDeletionStatusDeleting &&
			(user.Status != userDeletionStatusPending || user.DeletionScheduledAt == nil || user.DeletionScheduledAt.After(now)) {
			cleanup = nil
			return nil
		}

		if busy, err := userDeletionHasBlockingUpload(tx, userID); err != nil {
			return err
		} else if busy {
			return errUserDeletionBusy
		}
		if busy, err := userDeletionHasBlockingJob(tx, userID); err != nil {
			return err
		} else if busy {
			return errUserDeletionBusy
		}

		if ids, err := userDeletionProcessingJobIDs(tx, userID); err != nil {
			return err
		} else if len(ids) > 0 {
			if err := beginUserDeletion(tx, &user); err != nil {
				return err
			}
			if err := deleteUserDeletionRows(tx, &model.ProcessingJob{}, ids, false); err != nil {
				return err
			}
			setUserDeletionProgress(cleanup, userDeletionPhaseProcessingJobs, len(ids))
			return nil
		}

		if ids, err := userDeletionSimpleIDs(tx, &model.ImageAccessToken{}, "user_id = ?", userID); err != nil {
			return err
		} else if len(ids) > 0 {
			if err := beginUserDeletion(tx, &user); err != nil {
				return err
			}
			if err := deleteUserDeletionRows(tx, &model.ImageAccessToken{}, ids, false); err != nil {
				return err
			}
			setUserDeletionProgress(cleanup, userDeletionPhaseAccessTokens, len(ids))
			return nil
		}

		if exists, err := userDeletionJoinedRowExists(tx, "image_variants", "JOIN images ON images.id = image_variants.image_id", "images.user_id = ?", userID); err != nil {
			return err
		} else if exists {
			if err := lockUserDeletionStorage(tx); err != nil {
				return err
			}
			return deleteUserVariantBatch(tx, &user, cleanup, now)
		}

		if exists, err := userDeletionSimpleRowExists(tx, &model.Image{}, "user_id = ?", userID); err != nil {
			return err
		} else if exists {
			if err := lockUserDeletionStorage(tx); err != nil {
				return err
			}
			var err error
			imageCount, err = m.deleteUserImageBatch(tx, &user, cleanup, now)
			return err
		}

		if ids, err := userDeletionJoinedIDs(tx, "csrf_tokens", "JOIN sessions ON sessions.id = csrf_tokens.session_id", "sessions.user_id = ?", userID); err != nil {
			return err
		} else if len(ids) > 0 {
			return deleteUserPhaseRows(tx, &user, cleanup, &model.CSRFToken{}, ids, false, userDeletionPhaseCSRF)
		}
		if ids, err := userDeletionJoinedIDs(tx, "upload_parts", "JOIN upload_sessions ON upload_sessions.id = upload_parts.upload_session_id", "upload_sessions.user_id = ?", userID); err != nil {
			return err
		} else if len(ids) > 0 {
			return deleteUserPhaseRows(tx, &user, cleanup, &model.UploadPart{}, ids, false, userDeletionPhaseUploadParts)
		}
		if ids, err := userDeletionSimpleIDs(tx, &model.UploadSession{}, "user_id = ?", userID); err != nil {
			return err
		} else if len(ids) > 0 {
			return deleteUserPhaseRows(tx, &user, cleanup, &model.UploadSession{}, ids, false, userDeletionPhaseUploads)
		}
		if ids, err := userDeletionSessionIDs(tx, userID); err != nil {
			return err
		} else if len(ids) > 0 {
			return deleteUserPhaseRows(tx, &user, cleanup, &model.Session{}, ids, false, userDeletionPhaseSessions)
		}
		if ids, err := userDeletionNotificationIDs(tx, userID); err != nil {
			return err
		} else if len(ids) > 0 {
			return deleteUserPhaseRows(tx, &user, cleanup, &model.Notification{}, ids, true, userDeletionPhaseNotifications)
		}
		if ids, err := userDeletionSimpleIDs(tx, &model.AuditLog{}, "user_id = ?", userID); err != nil {
			return err
		} else if len(ids) > 0 {
			return deleteUserPhaseRows(tx, &user, cleanup, &model.AuditLog{}, ids, false, userDeletionPhaseAuditLogs)
		}
		if ids, err := userDeletionSimpleIDs(tx, &model.UploadQueue{}, "user_id = ?", userID); err != nil {
			return err
		} else if len(ids) > 0 {
			return deleteUserPhaseRows(tx, &user, cleanup, &model.UploadQueue{}, ids, false, userDeletionPhaseUploadQueues)
		}

		if err := beginUserDeletion(tx, &user); err != nil {
			return err
		}
		if err := tx.Delete(&model.User{}, userID).Error; err != nil {
			return err
		}
		cleanup.progressed = true
		cleanup.completed = true
		cleanup.phase = "user"
		cleanup.rows = 1
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return cleanup, imageCount, nil
}

type userDeletionIDRow struct {
	ID uint64
}

type userDeletionVariantRow struct {
	ID          uint64
	StoragePath string
}

func userDeletionHasBlockingUpload(tx *gorm.DB, userID uint64) (bool, error) {
	var row userDeletionIDRow
	result := tx.Model(&model.UploadSession{}).Select("id").
		Where("user_id = ? AND (status NOT IN ? OR staging_path <> '')", userID, []string{
			model.UploadSessionStatusCompleted,
			model.UploadSessionStatusFailed,
			model.UploadSessionStatusCancelled,
		}).Clauses(clause.Locking{Strength: "UPDATE"}).Order("id ASC").Limit(1).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return result.Error == nil, result.Error
}

func userDeletionJobQuery(tx *gorm.DB, userID uint64) *gorm.DB {
	return tx.Model(&model.ProcessingJob{}).Where(`
		EXISTS (SELECT 1 FROM images WHERE images.id = processing_jobs.image_id AND images.user_id = ?)
		OR EXISTS (SELECT 1 FROM upload_sessions WHERE upload_sessions.id = processing_jobs.upload_session_id AND upload_sessions.user_id = ?)`,
		userID, userID,
	)
}

func userDeletionHasBlockingJob(tx *gorm.DB, userID uint64) (bool, error) {
	var row userDeletionIDRow
	result := userDeletionJobQuery(tx, userID).Select("processing_jobs.id").
		Where("processing_jobs.status NOT IN ?", []string{model.ProcessingJobStatusCompleted, model.ProcessingJobStatusDead}).
		Clauses(clause.Locking{Strength: "UPDATE"}).Order("processing_jobs.id ASC").Limit(1).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return result.Error == nil, result.Error
}

func userDeletionProcessingJobIDs(tx *gorm.DB, userID uint64) ([]uint64, error) {
	var rows []userDeletionIDRow
	err := userDeletionJobQuery(tx, userID).Select("processing_jobs.id").
		Where("processing_jobs.status IN ?", []string{model.ProcessingJobStatusCompleted, model.ProcessingJobStatusDead}).
		Clauses(clause.Locking{Strength: "UPDATE"}).Order("processing_jobs.id ASC").
		Limit(userDeletionRowBatchSize).Find(&rows).Error
	return userDeletionIDs(rows), err
}

func userDeletionSimpleIDs(tx *gorm.DB, value interface{}, condition string, args ...interface{}) ([]uint64, error) {
	var rows []userDeletionIDRow
	err := tx.Model(value).Select("id").Where(condition, args...).
		Clauses(clause.Locking{Strength: "UPDATE"}).Order("id ASC").
		Limit(userDeletionRowBatchSize).Find(&rows).Error
	return userDeletionIDs(rows), err
}

func userDeletionNotificationIDs(tx *gorm.DB, userID uint64) ([]uint64, error) {
	var rows []userDeletionIDRow
	err := tx.Unscoped().Model(&model.Notification{}).Select("id").Where("user_id = ?", userID).
		Clauses(clause.Locking{Strength: "UPDATE"}).Order("id ASC").
		Limit(userDeletionRowBatchSize).Find(&rows).Error
	return userDeletionIDs(rows), err
}

func userDeletionJoinedIDs(tx *gorm.DB, table, join, condition string, args ...interface{}) ([]uint64, error) {
	var rows []userDeletionIDRow
	err := tx.Table(table).Select(table+".id").Joins(join).Where(condition, args...).
		Clauses(clause.Locking{Strength: "UPDATE"}).Order(table + ".id ASC").
		Limit(userDeletionRowBatchSize).Find(&rows).Error
	return userDeletionIDs(rows), err
}

func userDeletionIDs(rows []userDeletionIDRow) []uint64 {
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func userDeletionSimpleRowExists(tx *gorm.DB, value interface{}, condition string, args ...interface{}) (bool, error) {
	var row userDeletionIDRow
	result := tx.Model(value).Select("id").Where(condition, args...).Order("id ASC").Limit(1).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return result.Error == nil, result.Error
}

func userDeletionJoinedRowExists(tx *gorm.DB, table, join, condition string, args ...interface{}) (bool, error) {
	var row userDeletionIDRow
	result := tx.Table(table).Select(table+".id").Joins(join).Where(condition, args...).
		Order(table + ".id ASC").Limit(1).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return result.Error == nil, result.Error
}

func userDeletionSessionIDs(tx *gorm.DB, userID uint64) ([]uint64, error) {
	ids, err := userDeletionSimpleIDs(tx, &model.Session{}, "user_id = ? AND token_type <> ?", userID, "identity")
	if err != nil || len(ids) > 0 {
		return ids, err
	}
	var rows []userDeletionIDRow
	err = tx.Model(&model.Session{}).Select("sessions.id").
		Where("sessions.user_id = ? AND sessions.token_type = ?", userID, "identity").
		Where("NOT EXISTS (SELECT 1 FROM sessions AS children WHERE children.identity_token_id = sessions.id)").
		Clauses(clause.Locking{Strength: "UPDATE"}).Order("sessions.id ASC").
		Limit(userDeletionRowBatchSize).Find(&rows).Error
	return userDeletionIDs(rows), err
}

func beginUserDeletion(tx *gorm.DB, user *model.User) error {
	if user.Status == userDeletionStatusDeleting {
		return nil
	}
	result := tx.Model(user).Where("status = ?", userDeletionStatusPending).Update("status", userDeletionStatusDeleting)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("user deletion state changed while starting deletion")
	}
	user.Status = userDeletionStatusDeleting
	return nil
}

func deleteUserDeletionRows(tx *gorm.DB, value interface{}, ids []uint64, unscoped bool) error {
	query := tx
	if unscoped {
		query = query.Unscoped()
	}
	result := query.Where("id IN ?", ids).Delete(value)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != int64(len(ids)) {
		return fmt.Errorf("deleted %d of %d user child rows", result.RowsAffected, len(ids))
	}
	return nil
}

func deleteUserPhaseRows(tx *gorm.DB, user *model.User, cleanup *userDeletionCleanup, value interface{}, ids []uint64, unscoped bool, phase string) error {
	if err := beginUserDeletion(tx, user); err != nil {
		return err
	}
	if err := deleteUserDeletionRows(tx, value, ids, unscoped); err != nil {
		return err
	}
	setUserDeletionProgress(cleanup, phase, len(ids))
	return nil
}

func setUserDeletionProgress(cleanup *userDeletionCleanup, phase string, rows int) {
	cleanup.phase = phase
	cleanup.rows = rows
	cleanup.progressed = rows > 0
}

func deleteUserVariantBatch(tx *gorm.DB, user *model.User, cleanup *userDeletionCleanup, now time.Time) error {
	var variants []userDeletionVariantRow
	if err := tx.Model(&model.ImageVariant{}).
		Select("image_variants.id, image_variants.storage_path").
		Joins("JOIN images ON images.id = image_variants.image_id").
		Where("images.user_id = ?", user.ID).
		Clauses(clause.Locking{Strength: "UPDATE"}).Order("image_variants.id ASC").
		Limit(userDeletionRowBatchSize).Find(&variants).Error; err != nil {
		return err
	}
	if len(variants) == 0 {
		return errors.New("user image variants changed while claiming deletion batch")
	}
	if err := beginUserDeletion(tx, user); err != nil {
		return err
	}
	ids := make([]uint64, 0, len(variants))
	for _, variant := range variants {
		ids = append(ids, variant.ID)
		cleanup.paths = append(cleanup.paths, variant.StoragePath)
	}
	if err := deleteUserDeletionRows(tx, &model.ImageVariant{}, ids, false); err != nil {
		return err
	}
	if err := service.EnqueueStorageDelete(tx, "user", fmt.Sprint(user.ID),
		fmt.Sprintf("user:%d:variants:%d:%d:%d", user.ID, ids[0], ids[len(ids)-1], now.UnixNano()),
		cleanup.paths, now); err != nil {
		return err
	}
	setUserDeletionProgress(cleanup, userDeletionPhaseVariants, len(ids))
	return nil
}

func (m *Manager) deleteUserImageBatch(tx *gorm.DB, user *model.User, cleanup *userDeletionCleanup, now time.Time) (int, error) {
	var images []model.Image
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", user.ID).
		Order("id ASC").Limit(userDeletionImageBatchSize).Find(&images).Error; err != nil {
		return 0, err
	}
	if len(images) == 0 {
		return 0, errors.New("user images changed while claiming deletion batch")
	}
	if err := beginUserDeletion(tx, user); err != nil {
		return 0, err
	}

	imageIDs := make([]uint64, 0, len(images))
	imageFileSet := make(map[uint64]struct{}, len(images))
	imageFilePipeline := make(map[uint64]uint16, len(images))
	for _, image := range images {
		imageIDs = append(imageIDs, image.ID)
		imageFileSet[image.ImageFileID] = struct{}{}
		if current, exists := imageFilePipeline[image.ImageFileID]; !exists || image.PipelineVersion < current {
			imageFilePipeline[image.ImageFileID] = image.PipelineVersion
		}
	}
	imageFileIDs := make([]uint64, 0, len(imageFileSet))
	for id := range imageFileSet {
		imageFileIDs = append(imageFileIDs, id)
	}
	sort.Slice(imageFileIDs, func(left, right int) bool { return imageFileIDs[left] < imageFileIDs[right] })
	var imageFiles []model.ImageFile
	if len(imageFileIDs) > 0 {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", imageFileIDs).
			Order("id ASC").Find(&imageFiles).Error; err != nil {
			return 0, err
		}
	}
	if err := deleteUserDeletionRows(tx, &model.Image{}, imageIDs, false); err != nil {
		return 0, err
	}

	for _, image := range images {
		assetLink := image.UniqueLink
		if image.OriginAlias != nil {
			assetLink = *image.OriginAlias
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"image_id": image.ID, "old_asset_link": image.UniqueLink, "asset_link": assetLink,
		})
		event := model.OutboxEvent{
			AggregateType: "image", AggregateID: fmt.Sprint(image.ID), EventType: "image.cdn.purge",
			DedupeKey: fmt.Sprintf("image:%d:user-delete:%d", image.ID, now.UnixNano()),
			Status:    model.OutboxEventStatusPending, Payload: string(payload), AvailableAt: now,
		}
		if err := tx.Create(&event).Error; err != nil {
			return 0, err
		}
	}

	for _, imageFile := range imageFiles {
		var remaining int64
		if err := tx.Model(&model.Image{}).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("image_file_id = ?", imageFile.ID).Count(&remaining).Error; err != nil {
			return 0, err
		}
		if remaining > 0 {
			if err := tx.Model(&imageFile).UpdateColumn("reference_count", remaining).Error; err != nil {
				return 0, err
			}
			continue
		}
		storageRoot := ""
		if m.Config != nil {
			storageRoot = m.Config.Storage.BasePath
		}
		currentEndpoint, currentBucket, currentConfigured := "", "", false
		if m.R2 != nil {
			currentEndpoint, currentBucket, currentConfigured = m.R2.CurrentTarget()
		}
		storageTarget, err := service.ResolveV1StorageTarget(
			storageRoot,
			imageFilePipeline[imageFile.ID],
			&imageFile,
			currentEndpoint,
			currentBucket,
			currentConfigured,
		)
		if err != nil {
			return 0, fmt.Errorf("resolve image file %d storage target: %w", imageFile.ID, err)
		}
		cleanup.paths = append(cleanup.paths, imageFile.OriginalPath, imageFile.ThumbnailPath, imageFile.ProcessedPath)
		if avifPath, ok := service.V1PersistentVariantPath(imageFile.ProcessedPath, "avif"); ok {
			cleanup.paths = append(cleanup.paths, avifPath)
		}
		if storageTarget.Backend == service.V1StorageBackendR2 {
			for _, path := range []string{imageFile.OriginalPath, imageFile.ThumbnailPath, imageFile.ProcessedPath} {
				if path != "" {
					cleanup.remoteObjects = append(cleanup.remoteObjects, model.StorageDeleteRemoteObject{
						Path: path, Backend: service.V1StorageBackendR2, Endpoint: storageTarget.Endpoint, Bucket: storageTarget.Bucket,
					})
				}
			}
		}
		if err := tx.Delete(&imageFile).Error; err != nil {
			return 0, err
		}
	}
	if err := service.EnqueueStorageDeleteWithRemote(tx, "user", fmt.Sprint(user.ID),
		fmt.Sprintf("user:%d:images:%d:%d:%d", user.ID, imageIDs[0], imageIDs[len(imageIDs)-1], now.UnixNano()),
		cleanup.paths, cleanup.remoteObjects, now); err != nil {
		return 0, err
	}
	setUserDeletionProgress(cleanup, userDeletionPhaseImages, len(images))
	return len(images), nil
}

func uploadSessionBlocksUserDeletion(session model.UploadSession) bool {
	switch session.Status {
	case model.UploadSessionStatusCompleted:
		return session.StagingPath != ""
	case model.UploadSessionStatusFailed, model.UploadSessionStatusCancelled:
		return session.StagingPath != ""
	case model.UploadSessionStatusInitiated,
		model.UploadSessionStatusUploading,
		model.UploadSessionStatusProcessing,
		model.UploadSessionStatusCleanupPending:
		return true
	default:
		// Unknown future states fail closed so user deletion cannot orphan staging.
		return true
	}
}

func lockUserDeletionStorage(tx *gorm.DB) error {
	var capacityLock userDeletionCapacityLock
	return tx.Table("v2_capacity_locks").Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", 1).Take(&capacityLock).Error
}

func deletionPathInside(root, storedPath string) (string, error) {
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	candidate := storedPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootAbs, candidate)
	}
	candidateAbs, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return "", err
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes storage root")
	}
	if _, err := os.Lstat(candidateAbs); err != nil {
		if os.IsNotExist(err) {
			return candidateAbs, nil
		}
		return "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", err
	}
	resolvedCandidate, err := filepath.EvalSymlinks(candidateAbs)
	if err != nil {
		return "", err
	}
	resolvedRelative, err := filepath.Rel(resolvedRoot, resolvedCandidate)
	if err != nil {
		return "", err
	}
	if resolvedRelative == "." || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path resolves outside storage root")
	}
	return candidateAbs, nil
}
