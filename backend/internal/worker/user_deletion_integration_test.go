// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestDeleteUserRecordsResolvesUnclassifiedV1ToExactR2Target(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT IGNORE INTO v2_capacity_locks (id) VALUES (1)").Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	suffix := fmt.Sprint(now.UnixNano())
	scheduledAt := now.Add(-time.Hour)
	user := model.User{
		Username: "legacy-r2-delete-" + suffix, Email: "legacy-r2-delete-" + suffix + "@example.test",
		PasswordHash: "test", Role: "user", Status: userDeletionStatusPending, DeletionScheduledAt: &scheduledAt,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	imageFile := model.ImageFile{
		FileHash: deletionTestHash(t.Name() + suffix), FileSize: 1, MimeType: "image/webp",
		OriginalPath: "original/" + suffix + ".webp", ThumbnailPath: "thumbnail/" + suffix + ".webp",
		ProcessedPath: "processed/" + suffix + ".webp",
	}
	if err := db.Create(&imageFile).Error; err != nil {
		t.Fatal(err)
	}
	image := model.Image{
		UserID: user.ID, ImageFileID: imageFile.ID, UniqueLink: "legacy-r2-" + suffix,
		Visibility: "private", PipelineVersion: 1, ProcessingStatus: model.ImageProcessingStatusCompleted, FileSize: 1,
	}
	if err := db.Create(&image).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Where("aggregate_type = ? AND aggregate_id = ?", "user", fmt.Sprint(user.ID)).Delete(&model.OutboxEvent{})
		db.Where("aggregate_type = ? AND aggregate_id = ?", "image", fmt.Sprint(image.ID)).Delete(&model.OutboxEvent{})
		db.Unscoped().Delete(&model.Image{}, image.ID)
		db.Unscoped().Delete(&model.ImageFile{}, imageFile.ID)
		db.Unscoped().Delete(&model.User{}, user.ID)
	})

	target := &fixedR2WorkerTarget{endpoint: "https://current-r2.example", bucket: "legacy-images"}
	manager := &Manager{
		DB: db, Config: &config.Config{Storage: config.StorageConfig{BasePath: t.TempDir()}}, R2: target,
	}
	cleanup, removed, err := manager.deleteUserRecords(user.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || cleanup == nil || len(cleanup.remoteObjects) != 3 {
		t.Fatalf("cleanup = %#v, removed=%d", cleanup, removed)
	}
	for _, object := range cleanup.remoteObjects {
		if object.Endpoint != target.endpoint || object.Bucket != target.bucket {
			t.Fatalf("remote cleanup target = %#v", object)
		}
	}
}

type fixedR2WorkerTarget struct {
	endpoint string
	bucket   string
}

func (t *fixedR2WorkerTarget) CurrentTarget() (string, string, bool) {
	return t.endpoint, t.bucket, t.endpoint != "" && t.bucket != ""
}

func (t *fixedR2WorkerTarget) CanDelete(endpoint, bucket string) bool {
	return endpoint == t.endpoint && bucket == t.bucket
}

func (t *fixedR2WorkerTarget) DeleteContext(context.Context, string, string, string) error {
	return nil
}

func TestDeleteUserRecordsWaitsForV2UploadLifecycle(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT IGNORE INTO v2_capacity_locks (id) VALUES (1)").Error; err != nil {
		t.Fatal(err)
	}

	for _, status := range []string{
		model.UploadSessionStatusInitiated,
		model.UploadSessionStatusCleanupPending,
		model.UploadSessionStatusCompleted,
	} {
		t.Run(status, func(t *testing.T) {
			now := time.Now()
			suffix := fmt.Sprintf("%d", now.UnixNano())
			scheduledAt := now.Add(-time.Hour)
			user := model.User{
				Username:            "deletion-v2-" + suffix,
				Email:               "deletion-v2-" + suffix + "@example.test",
				PasswordHash:        "test",
				Role:                "user",
				Status:              "pending_deletion",
				DeletionScheduledAt: &scheduledAt,
			}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			session := model.UploadSession{
				UploadKey:        "deletion-v2-" + suffix,
				UserID:           user.ID,
				Status:           status,
				PipelineVersion:  2,
				Visibility:       "private",
				Filename:         "photo.jpg",
				SourceMimeType:   "image/jpeg",
				SourceWidth:      1,
				SourceHeight:     1,
				ProcessorVersion: "test",
				RecipeVersion:    "2.0.0",
				StagingPath:      "/tmp/deletion-v2-" + suffix,
				ExpiresAt:        now.Add(time.Hour),
			}
			if err := db.Create(&session).Error; err != nil {
				db.Unscoped().Delete(&model.User{}, user.ID)
				t.Fatal(err)
			}
			t.Cleanup(func() {
				db.Unscoped().Delete(&model.UploadSession{}, session.ID)
				db.Unscoped().Delete(&model.User{}, user.ID)
			})

			manager := &Manager{DB: db}
			cleanup, _, err := manager.deleteUserRecords(user.ID, now)
			if !errors.Is(err, errUserDeletionBusy) {
				t.Fatalf("deleteUserRecords() error = %v, want %v", err, errUserDeletionBusy)
			}
			if cleanup != nil {
				t.Fatalf("deleteUserRecords() cleanup = %#v, want nil", cleanup)
			}

			var userCount, sessionCount int64
			if err := db.Model(&model.User{}).Where("id = ?", user.ID).Count(&userCount).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&model.UploadSession{}).Where("id = ?", session.ID).Count(&sessionCount).Error; err != nil {
				t.Fatal(err)
			}
			if userCount != 1 || sessionCount != 1 {
				t.Fatalf("rows after busy deletion: users=%d sessions=%d", userCount, sessionCount)
			}
			var unchanged model.User
			if err := db.First(&unchanged, user.ID).Error; err != nil {
				t.Fatal(err)
			}
			if unchanged.Status != userDeletionStatusPending {
				t.Fatalf("busy deletion changed status to %q; cancellation must remain available", unchanged.Status)
			}
		})
	}
}

func TestListPendingDeletionUsersIsBoundedAndOldestFirst(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	prefix := fmt.Sprintf("deletion-batch-%d-", time.Now().UnixNano())
	base := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	created := make([]model.User, 0, userDeletionBatchSize+1)
	for index := 0; index < userDeletionBatchSize+1; index++ {
		scheduledAt := base.Add(time.Duration(index) * time.Second)
		user := model.User{
			Username: prefix + fmt.Sprint(index), Email: prefix + fmt.Sprint(index) + "@example.test",
			PasswordHash: "test", Role: "user", Status: "pending_deletion",
			DeletionScheduledAt: &scheduledAt,
		}
		if err := db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
		created = append(created, user)
	}
	t.Cleanup(func() {
		ids := make([]uint64, 0, len(created))
		for _, user := range created {
			ids = append(ids, user.ID)
		}
		db.Unscoped().Where("id IN ?", ids).Delete(&model.User{})
	})

	users, err := listPendingDeletionUsers(db, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != userDeletionBatchSize {
		t.Fatalf("pending deletion batch size = %d, want %d", len(users), userDeletionBatchSize)
	}
	for index, user := range users {
		want := prefix + fmt.Sprint(index)
		if user.Username != want {
			t.Fatalf("users[%d] = %q, want oldest %q", index, user.Username, want)
		}
	}
}

func TestDeleteUserRecordsBatchesVariantsAndPreservesSharedImageFile(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT IGNORE INTO v2_capacity_locks (id) VALUES (1)").Error; err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	suffix := fmt.Sprintf("%d", now.UnixNano())
	scheduledAt := now.Add(-time.Hour)
	deletingUser := model.User{
		Username: "bounded-delete-" + suffix, Email: "bounded-delete-" + suffix + "@example.test",
		PasswordHash: "test", Role: "user", Status: userDeletionStatusPending, DeletionScheduledAt: &scheduledAt,
	}
	survivingUser := model.User{
		Username: "bounded-survivor-" + suffix, Email: "bounded-survivor-" + suffix + "@example.test",
		PasswordHash: "test", Role: "user", Status: "active",
	}
	if err := db.Create(&deletingUser).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&survivingUser).Error; err != nil {
		t.Fatal(err)
	}

	sharedFile := model.ImageFile{
		FileHash: deletionTestHash(suffix + "-shared"), FileSize: 10, MimeType: "image/webp", ReferenceCount: 2,
		OriginalPath: "original/" + suffix + "-shared.webp", ThumbnailPath: "thumbnail/" + suffix + "-shared.webp",
		ProcessedPath: "processed/" + suffix + "-shared.webp",
	}
	if err := db.Create(&sharedFile).Error; err != nil {
		t.Fatal(err)
	}
	survivingImage := model.Image{
		UserID: survivingUser.ID, ImageFileID: sharedFile.ID, UniqueLink: "survive-" + suffix,
		Filename: "survive.webp", Visibility: "public", PipelineVersion: 2,
		ProcessingStatus: model.ImageProcessingStatusCompleted, FileSize: 10,
	}
	if err := db.Create(&survivingImage).Error; err != nil {
		t.Fatal(err)
	}

	images := make([]model.Image, 0, userDeletionImageBatchSize+1)
	imageFiles := []model.ImageFile{sharedFile}
	for index := 0; index < userDeletionImageBatchSize+1; index++ {
		imageFileID := sharedFile.ID
		if index > 0 {
			imageFile := model.ImageFile{
				FileHash: deletionTestHash(fmt.Sprintf("%s-file-%d", suffix, index)), FileSize: 10, MimeType: "image/webp", ReferenceCount: 1,
				OriginalPath:  fmt.Sprintf("original/%s-%d.webp", suffix, index),
				ThumbnailPath: fmt.Sprintf("thumbnail/%s-%d.webp", suffix, index),
				ProcessedPath: fmt.Sprintf("processed/%s-%d.webp", suffix, index),
			}
			if index == 1 {
				imageFile.RemoteBackend = "r2"
				imageFile.RemoteEndpoint = "https://r2.example"
				imageFile.RemoteBucket = "images"
			}
			if err := db.Create(&imageFile).Error; err != nil {
				t.Fatal(err)
			}
			imageFiles = append(imageFiles, imageFile)
			imageFileID = imageFile.ID
		}
		alias := fmt.Sprintf("asset-%s-%d", suffix, index)
		image := model.Image{
			UserID: deletingUser.ID, ImageFileID: imageFileID, UniqueLink: fmt.Sprintf("delete-%s-%d", suffix, index),
			OriginAlias: &alias, Filename: "delete.webp", Visibility: "public", PipelineVersion: 2,
			ProcessingStatus: model.ImageProcessingStatusCompleted, FileSize: 10,
		}
		if err := db.Create(&image).Error; err != nil {
			t.Fatal(err)
		}
		images = append(images, image)
	}

	variants := make([]model.ImageVariant, 0, userDeletionRowBatchSize+1)
	for index := 0; index < userDeletionRowBatchSize+1; index++ {
		variants = append(variants, model.ImageVariant{
			ImageID: images[0].ID, Kind: model.ImageVariantKindGallery, Revision: uint32(index + 1),
			PipelineVersion: 2, Status: model.ImageVariantStatusReady,
			FileHash: deletionTestHash(fmt.Sprintf("%s-variant-%d", suffix, index)), FileSize: 1,
			MimeType: "image/webp", Width: 1, Height: 1, Quality: 60,
			StoragePath: fmt.Sprintf("v2/gallery/%s-%d.webp", suffix, index), IsActive: index == 0,
		})
	}
	if err := db.Create(&variants).Error; err != nil {
		t.Fatal(err)
	}

	imageIDs := make([]uint64, 0, len(images)+1)
	for _, image := range images {
		imageIDs = append(imageIDs, image.ID)
	}
	imageIDs = append(imageIDs, survivingImage.ID)
	t.Cleanup(func() {
		db.Where("aggregate_type = ? AND aggregate_id = ?", "user", fmt.Sprint(deletingUser.ID)).Delete(&model.OutboxEvent{})
		db.Where("aggregate_type = ? AND aggregate_id IN ?", "image", imageIDs).Delete(&model.OutboxEvent{})
		db.Unscoped().Where("image_id IN ?", imageIDs).Delete(&model.ImageVariant{})
		db.Unscoped().Where("id IN ?", imageIDs).Delete(&model.Image{})
		for _, imageFile := range imageFiles {
			db.Unscoped().Delete(&model.ImageFile{}, imageFile.ID)
		}
		db.Unscoped().Delete(&model.User{}, deletingUser.ID)
		db.Unscoped().Delete(&model.User{}, survivingUser.ID)
	})

	manager := &Manager{DB: db}
	cleanup, removed, err := manager.deleteUserRecords(deletingUser.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup.phase != userDeletionPhaseVariants || cleanup.rows != userDeletionRowBatchSize || removed != 0 || cleanup.completed {
		t.Fatalf("first batch = %#v removed=%d", cleanup, removed)
	}
	var remainingVariants, remainingImages int64
	if err := db.Model(&model.ImageVariant{}).Where("image_id = ?", images[0].ID).Count(&remainingVariants).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Image{}).Where("user_id = ?", deletingUser.ID).Count(&remainingImages).Error; err != nil {
		t.Fatal(err)
	}
	if remainingVariants != 1 || remainingImages != int64(userDeletionImageBatchSize+1) {
		t.Fatalf("after first batch variants=%d images=%d", remainingVariants, remainingImages)
	}
	var lockedUser model.User
	if err := db.First(&lockedUser, deletingUser.ID).Error; err != nil {
		t.Fatal(err)
	}
	if lockedUser.Status != userDeletionStatusDeleting {
		t.Fatalf("user status = %q, want %q", lockedUser.Status, userDeletionStatusDeleting)
	}

	completed := false
	for attempt := 0; attempt < 10; attempt++ {
		cleanup, _, err = manager.deleteUserRecords(deletingUser.ID, now.Add(time.Duration(attempt+1)*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		if cleanup != nil && cleanup.completed {
			completed = true
			break
		}
	}
	if !completed {
		t.Fatal("bounded user deletion did not complete")
	}

	var sharedCount int64
	if err := db.Model(&model.ImageFile{}).Where("id = ?", sharedFile.ID).Count(&sharedCount).Error; err != nil {
		t.Fatal(err)
	}
	if sharedCount != 1 {
		t.Fatalf("shared image file count = %d, want 1", sharedCount)
	}
	if err := db.First(&sharedFile, sharedFile.ID).Error; err != nil {
		t.Fatal(err)
	}
	if sharedFile.ReferenceCount != 1 {
		t.Fatalf("shared reference count = %d, want 1", sharedFile.ReferenceCount)
	}
	var survivingCount int64
	if err := db.Model(&model.Image{}).Where("id = ?", survivingImage.ID).Count(&survivingCount).Error; err != nil {
		t.Fatal(err)
	}
	if survivingCount != 1 {
		t.Fatalf("surviving shared image count = %d, want 1", survivingCount)
	}

	var storageEvents []model.OutboxEvent
	if err := db.Where("aggregate_type = ? AND aggregate_id = ? AND event_type = ?", "user", fmt.Sprint(deletingUser.ID), model.OutboxEventTypeStorageDelete).
		Find(&storageEvents).Error; err != nil {
		t.Fatal(err)
	}
	foundRemote := false
	for _, event := range storageEvents {
		var payload model.StorageDeletePayload
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			t.Fatal(err)
		}
		for _, path := range payload.Paths {
			if path == sharedFile.OriginalPath || path == sharedFile.ThumbnailPath || path == sharedFile.ProcessedPath {
				t.Fatalf("shared path was queued for deletion: %s", path)
			}
		}
		for _, object := range payload.RemoteObjects {
			if object.Endpoint == "https://r2.example" && object.Bucket == "images" {
				foundRemote = true
			}
		}
	}
	if !foundRemote {
		t.Fatal("unshared R2 image file cleanup was not queued with its lineage")
	}
}

func TestDeleteUserRecordsBatchesUploadSessions(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	suffix := fmt.Sprintf("%d", now.UnixNano())
	scheduledAt := now.Add(-time.Hour)
	user := model.User{
		Username: "session-delete-" + suffix, Email: "session-delete-" + suffix + "@example.test",
		PasswordHash: "test", Role: "user", Status: userDeletionStatusPending, DeletionScheduledAt: &scheduledAt,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessions := make([]model.UploadSession, 0, userDeletionRowBatchSize+1)
	for index := 0; index < userDeletionRowBatchSize+1; index++ {
		sessions = append(sessions, model.UploadSession{
			UploadKey: fmt.Sprintf("session-delete-%s-%d", suffix, index), UserID: user.ID,
			Status: model.UploadSessionStatusCompleted, PipelineVersion: 2, Visibility: "private",
			Filename: "photo.jpg", SourceMimeType: "image/jpeg", SourceWidth: 1, SourceHeight: 1,
			ProcessorVersion: "test", RecipeVersion: "2.0.0", StagingPath: "", ExpiresAt: now.Add(time.Hour),
		})
	}
	if err := db.Create(&sessions).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Unscoped().Where("user_id = ?", user.ID).Delete(&model.UploadSession{})
		db.Unscoped().Delete(&model.User{}, user.ID)
	})

	manager := &Manager{DB: db}
	cleanup, _, err := manager.deleteUserRecords(user.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup.phase != userDeletionPhaseUploads || cleanup.rows != userDeletionRowBatchSize || cleanup.completed {
		t.Fatalf("first upload-session batch = %#v", cleanup)
	}
	var remaining int64
	if err := db.Model(&model.UploadSession{}).Where("user_id = ?", user.ID).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("remaining upload sessions = %d, want 1", remaining)
	}
	cleanup, _, err = manager.deleteUserRecords(user.ID, now.Add(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if cleanup.phase != userDeletionPhaseUploads || cleanup.rows != 1 {
		t.Fatalf("second upload-session batch = %#v", cleanup)
	}
	cleanup, _, err = manager.deleteUserRecords(user.ID, now.Add(2*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if !cleanup.completed {
		t.Fatalf("final deletion batch = %#v, want completed", cleanup)
	}
}

func deletionTestHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
