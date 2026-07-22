// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestClaimPublishJobEmptyQueueDoesNotLogRecordNotFound(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}

	var logs bytes.Buffer
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.New(
		log.New(&logs, "", 0),
		logger.Config{LogLevel: logger.Info, IgnoreRecordNotFoundError: false},
	)})
	if err != nil {
		t.Fatal(err)
	}

	var active int64
	if err := db.Model(&model.ProcessingJob{}).Where("job_type = ? AND status IN ?", v2PublishJobType, []string{
		model.ProcessingJobStatusQueued,
		model.ProcessingJobStatusRetry,
		model.ProcessingJobStatusRunning,
	}).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Skipf("publish queue is not empty (%d active jobs)", active)
	}

	logs.Reset()
	svc := &V2UploadService{db: db, cfg: &config.Config{ImageV2: config.ImageV2Config{JobLease: time.Minute}}}
	_, err = svc.claimPublishJob(context.Background(), "idle-queue-test")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("claim error = %v, want gorm.ErrRecordNotFound", err)
	}
	if strings.Contains(strings.ToLower(logs.String()), "record not found") {
		t.Fatalf("idle claim emitted record-not-found log: %s", logs.String())
	}
}

func TestExtendPublishJobLeaseFencesTokenAndRecognizesCompletion(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	token := fmt.Sprintf("lease-%d", now.UnixNano())
	owner := "lease-test"
	expires := now.Add(time.Second)
	job := model.ProcessingJob{
		JobType: v2PublishJobType, DedupeKey: token,
		Status: model.ProcessingJobStatusRunning, Payload: "{}",
		Attempts: 1, MaxAttempts: 5, AvailableAt: now,
		LeaseOwner: &owner, LeaseToken: &token, LeaseExpiresAt: &expires,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&model.ProcessingJob{}, job.ID) })

	svc := &V2UploadService{db: db, cfg: &config.Config{ImageV2: config.ImageV2Config{JobLease: 2 * time.Minute}}}
	if err := svc.extendPublishJobLease(context.Background(), &job); err != nil {
		t.Fatal(err)
	}
	var renewed model.ProcessingJob
	if err := db.First(&renewed, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if renewed.LeaseExpiresAt == nil || !renewed.LeaseExpiresAt.After(expires) {
		t.Fatalf("lease expiry = %v, want after %v", renewed.LeaseExpiresAt, expires)
	}

	wrongToken := token + "-stale"
	job.LeaseToken = &wrongToken
	if err := svc.extendPublishJobLease(context.Background(), &job); !errors.Is(err, errV2PublishLeaseLost) {
		t.Fatalf("stale lease error = %v, want lease lost", err)
	}
	job.LeaseToken = &token
	if err := db.Model(&job).Update("status", model.ProcessingJobStatusCompleted).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.extendPublishJobLease(context.Background(), &job); !errors.Is(err, errV2PublishAlreadyDone) {
		t.Fatalf("completed lease error = %v, want already completed", err)
	}
}

func TestReleasePublishJobLeaseFencesOwnerAndToken(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	owner := fmt.Sprintf("release-owner-%d", now.UnixNano())
	token := fmt.Sprintf("release-token-%d", now.UnixNano())
	expires := now.Add(time.Minute)
	job := model.ProcessingJob{
		JobType: v2PublishJobType, DedupeKey: token,
		Status: model.ProcessingJobStatusRunning, Payload: "{}",
		Attempts: 2, MaxAttempts: 5, AvailableAt: now.Add(time.Minute),
		LeaseOwner: &owner, LeaseToken: &token, LeaseExpiresAt: &expires,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&model.ProcessingJob{}, job.ID) })

	svc := &V2UploadService{db: db}
	wrongOwner := owner + "-stale"
	staleOwnerJob := job
	staleOwnerJob.LeaseOwner = &wrongOwner
	if err := svc.releasePublishJobLease(context.Background(), &staleOwnerJob, true, now, context.Canceled); err != nil {
		t.Fatal(err)
	}
	wrongToken := token + "-stale"
	staleTokenJob := job
	staleTokenJob.LeaseToken = &wrongToken
	if err := svc.releasePublishJobLease(context.Background(), &staleTokenJob, true, now, context.Canceled); err != nil {
		t.Fatal(err)
	}

	var stillOwned model.ProcessingJob
	if err := db.First(&stillOwned, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stillOwned.Status != model.ProcessingJobStatusRunning || stillOwned.Attempts != 2 || stillOwned.LeaseToken == nil {
		t.Fatalf("stale release changed job: status=%q attempts=%d token=%v", stillOwned.Status, stillOwned.Attempts, stillOwned.LeaseToken)
	}

	if err := svc.releasePublishJobLease(context.Background(), &job, true, now, context.Canceled); err != nil {
		t.Fatal(err)
	}
	var released model.ProcessingJob
	if err := db.First(&released, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if released.Status != model.ProcessingJobStatusRetry || released.Attempts != 1 {
		t.Fatalf("released status=%q attempts=%d, want retry/1", released.Status, released.Attempts)
	}
	if released.LeaseOwner != nil || released.LeaseToken != nil || released.LeaseExpiresAt != nil {
		t.Fatalf("released lease was retained: owner=%v token=%v expiry=%v", released.LeaseOwner, released.LeaseToken, released.LeaseExpiresAt)
	}
}

func TestProcessNextPublishJobReleasesClaimAndRepanics(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	suffix := fmt.Sprint(now.UnixNano())
	root := t.TempDir()
	staging := filepath.Join(root, ".staging")
	if err := os.MkdirAll(staging, 0750); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(v2PublishJobPayload{
		ImageID: 1, UploadSessionID: 1,
		SourcePath: filepath.Join(staging, "publish_source.ready"),
		TargetPath: filepath.Join("v2", "publish", suffix, "v1.webp"),
		Width:      1, Height: 1, WatermarkEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	job := model.ProcessingJob{
		JobType: v2PublishJobType, DedupeKey: "panic-release-" + suffix,
		Status: model.ProcessingJobStatusQueued, Priority: 1_000_000_000,
		Payload: string(payload), Attempts: 0, MaxAttempts: 5, AvailableAt: now,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&model.ProcessingJob{}, job.ID) })

	svc := &V2UploadService{db: db, cfg: &config.Config{
		Storage: config.StorageConfig{
			BasePath: root, StagingPath: staging, DiskHardPct: 98,
		},
		ImageV2: config.ImageV2Config{JobLease: time.Minute},
	}}
	panicked := false
	func() {
		defer func() {
			panicked = recover() != nil
		}()
		_, _ = svc.ProcessNextPublishJob(context.Background(), "panic-release-worker")
	}()
	if !panicked {
		t.Fatal("ProcessNextPublishJob did not propagate the publish panic")
	}

	var released model.ProcessingJob
	if err := db.First(&released, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if released.Status != model.ProcessingJobStatusRetry || released.Attempts != 1 {
		t.Fatalf("panic release status=%q attempts=%d, want retry/1", released.Status, released.Attempts)
	}
	if released.LeaseOwner != nil || released.LeaseToken != nil || released.LeaseExpiresAt != nil {
		t.Fatalf("panic release retained lease: owner=%v token=%v expiry=%v", released.LeaseOwner, released.LeaseToken, released.LeaseExpiresAt)
	}
}

func TestExpiredExhaustedPublishJobIsFencedAndCompensated(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprint(time.Now().UnixNano())
	user := model.User{
		Username:     "dead-job-" + suffix,
		Email:        "dead-job-" + suffix + "@example.test",
		PasswordHash: "test", Role: "user", Status: "active",
		StorageQuota: 1 << 30,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	masterPath := filepath.Join("v2", "master", suffix+".webp")
	galleryPath := filepath.Join("v2", "gallery", suffix+".webp")
	targetPath := filepath.Join("v2", "publish", suffix+".webp")
	for _, relativePath := range []string{masterPath, galleryPath, targetPath} {
		fullPath := filepath.Join(root, relativePath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte("dead-job"), 0640); err != nil {
			t.Fatal(err)
		}
	}

	imageFile := model.ImageFile{
		FileHash: strings.Repeat("a", 48) + suffix[len(suffix)-16:],
		// Deliberately stale: compensation must recompute from images rather
		// than trust or decrement this denormalized counter.
		FileSize: 8, MimeType: "image/webp", ReferenceCount: 99,
		OriginalPath: masterPath, ThumbnailPath: galleryPath,
	}
	if err := db.Create(&imageFile).Error; err != nil {
		t.Fatal(err)
	}
	image := model.Image{
		UserID: user.ID, ImageFileID: imageFile.ID,
		UniqueLink: suffix[len(suffix)-12:], Title: "dead", Filename: "dead.webp",
		Visibility: "public", PipelineVersion: model.ImagePipelineVersionV2,
		ProcessingStatus: model.ImageProcessingStatusProcessing, FileSize: 8,
	}
	assetLink := image.UniqueLink
	image.OriginAlias = &assetLink
	if err := db.Create(&image).Error; err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	uploadKey := "dead-upload-" + suffix
	session := model.UploadSession{
		UploadKey: uploadKey, UserID: user.ID, Status: model.UploadSessionStatusProcessing,
		PipelineVersion: model.ImagePipelineVersionV2, Visibility: "public", Filename: "dead.webp",
		SourceMimeType: "image/webp", SourceWidth: 1, SourceHeight: 1,
		ProcessorVersion: "test", RecipeVersion: "test", ImageID: &image.ID,
		StagingPath: filepath.Join(root, ".staging", uploadKey), ExpiresAt: now.Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	for index, item := range []struct {
		kind string
		path string
	}{{model.ImageVariantKindMaster, masterPath}, {model.ImageVariantKindGallery, galleryPath}} {
		variant := model.ImageVariant{
			ImageID: image.ID, Kind: item.kind, Revision: 1,
			PipelineVersion: model.ImagePipelineVersionV2, Status: model.ImageVariantStatusReady,
			FileHash: fmt.Sprintf("%064d", index+1), FileSize: 8, MimeType: "image/webp",
			Width: 1, Height: 1, StoragePath: item.path, IsActive: true,
		}
		if err := db.Create(&variant).Error; err != nil {
			t.Fatal(err)
		}
	}

	payload, err := json.Marshal(v2PublishJobPayload{
		ImageID: image.ID, UploadSessionID: session.ID,
		SourcePath: filepath.Join(root, ".staging", uploadKey, "publish_source.ready"),
		TargetPath: targetPath, Width: 1, Height: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	leaseToken := "dead-lease-" + suffix
	leaseOwner := "test"
	leaseExpiry := now.Add(-time.Minute)
	job := model.ProcessingJob{
		JobType: v2PublishJobType, DedupeKey: "dead-job-" + suffix,
		ImageID: &image.ID, UploadSessionID: &session.ID,
		Status: model.ProcessingJobStatusRunning, Priority: 1_000_000, Payload: string(payload),
		Attempts: 1, MaxAttempts: 1, AvailableAt: now,
		LeaseOwner: &leaseOwner, LeaseToken: &leaseToken, LeaseExpiresAt: &leaseExpiry,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(&model.ProcessingJob{}, job.ID)
		db.Unscoped().Delete(&model.UploadSession{}, session.ID)
		db.Unscoped().Where("image_id = ?", image.ID).Delete(&model.ImageVariant{})
		db.Unscoped().Delete(&model.Image{}, image.ID)
		db.Unscoped().Delete(&model.ImageFile{}, imageFile.ID)
		db.Unscoped().Delete(&model.User{}, user.ID)
	})

	svc := &V2UploadService{db: db, cfg: &config.Config{
		Storage: config.StorageConfig{BasePath: root},
		ImageV2: config.ImageV2Config{JobLease: time.Minute},
	}}
	processed, err := svc.ProcessNextPublishJob(context.Background(), "replacement-worker")
	if !processed || !errors.Is(err, errV2PublishLeaseExhausted) {
		t.Fatalf("ProcessNextPublishJob() = processed %v, err %v; want exhausted compensation", processed, err)
	}

	if err := db.First(&job, job.ID).Error; err != nil || job.Status != model.ProcessingJobStatusDead {
		t.Fatalf("job status = %q, err=%v", job.Status, err)
	}
	if err := db.First(&session, session.ID).Error; err != nil || session.Status != model.UploadSessionStatusFailed || session.ImageID != nil {
		t.Fatalf("session status=%q image=%v err=%v", session.Status, session.ImageID, err)
	}
	for _, target := range []struct {
		label string
		model interface{}
		id    uint64
	}{{"image", &model.Image{}, image.ID}, {"image file", &model.ImageFile{}, imageFile.ID}} {
		var count int64
		if err := db.Model(target.model).Where("id = ?", target.id).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", target.label, count, err)
		}
	}
	for _, relativePath := range []string{masterPath, galleryPath, targetPath} {
		if _, err := os.Stat(filepath.Join(root, relativePath)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("compensated file %s still exists: %v", relativePath, err)
		}
	}

	// A worker holding the pre-compensation lease must not be able to publish
	// after the dead transition and recreate an orphaned persistent target.
	staleSource := filepath.Join(root, "stale.part")
	staleTarget := filepath.Join(root, "v2", "publish", "stale.webp")
	if err := os.WriteFile(staleSource, []byte("stale"), 0640); err != nil {
		t.Fatal(err)
	}
	job.LeaseToken = &leaseToken
	if err := svc.promotePublishTarget(context.Background(), &job, staleSource, staleTarget); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("stale publish error = %v, want lost lease", err)
	}
	if _, err := os.Stat(staleTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale worker recreated target: %v", err)
	}
	if _, err := os.Stat(staleSource); err != nil {
		t.Fatalf("stale source was moved despite lost lease: %v", err)
	}
}

func TestCompleteRollbackRestoresStagingAndCleansPersistentParts(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprint(time.Now().UnixNano())
	user := model.User{
		Username:     "complete-rollback-" + suffix,
		Email:        "complete-rollback-" + suffix + "@example.test",
		PasswordHash: "test", Role: "user", Status: "active", StorageQuota: 1 << 30,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	stagingRoot := filepath.Join(root, ".staging")
	uploadKey := "rollback-" + suffix
	sessionPath := filepath.Join(stagingRoot, uploadKey)
	if err := os.MkdirAll(sessionPath, 0750); err != nil {
		t.Fatal(err)
	}
	session := model.UploadSession{
		UploadKey: uploadKey, UserID: user.ID, Status: model.UploadSessionStatusUploading,
		PipelineVersion: model.ImagePipelineVersionV2, Visibility: "public", Filename: "rollback.webp",
		SourceMimeType: "image/webp", SourceWidth: 10, SourceHeight: 10,
		ProcessorVersion: "test", RecipeVersion: "test",
		ExpectedPartCount: uint8(len(v2RequiredParts)), ReceivedPartCount: uint8(len(v2RequiredParts)),
		StagingPath: sessionPath, ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}

	parts := make([]model.UploadPart, 0, len(v2RequiredParts))
	contentByKind := make(map[string][]byte, len(v2RequiredParts))
	finalPathByKind := make(map[string]string, 3)
	for index, kind := range v2RequiredParts {
		content := []byte(fmt.Sprintf("rollback-%s-%s-%d", suffix, kind, index))
		hash := testSHA256(content)
		sourcePath := filepath.Join(sessionPath, kind+".ready")
		if err := os.WriteFile(sourcePath, content, 0640); err != nil {
			t.Fatal(err)
		}
		contentByKind[kind] = content
		parts = append(parts, model.UploadPart{
			UploadSessionID: session.ID, Kind: kind, Status: model.UploadPartStatusReceived,
			ExpectedSize: int64(len(content)), ActualSize: int64(len(content)),
			ExpectedHash: hash, ActualHash: hash,
			ExpectedWidth: 10, ExpectedHeight: 10, ActualWidth: 10, ActualHeight: 10,
			ExpectedMimeType: "image/webp", ActualMimeType: "image/webp",
			StagingPath: sourcePath,
		})
		if kind != model.ImageVariantKindPublishSource {
			finalPathByKind[kind] = filepath.Join("v2", kind, hash[:2], hash+".webp")
		}
	}
	if err := db.Create(&parts).Error; err != nil {
		t.Fatal(err)
	}

	callbackName := "test:complete-rollback:" + suffix
	forcedErr := errors.New("forced processing job create failure")
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "processing_jobs" {
			tx.AddError(forcedErr)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Where("upload_session_id = ?", session.ID).Delete(&model.ProcessingJob{})
		db.Where("upload_session_id = ?", session.ID).Delete(&model.UploadPart{})
		db.Delete(&model.UploadSession{}, session.ID)
		db.Where("user_id = ?", user.ID).Delete(&model.Image{})
		for _, part := range parts {
			db.Where("file_hash = ?", part.ActualHash).Delete(&model.ImageFile{})
		}
		db.Delete(&model.User{}, user.ID)
	})

	svc := &V2UploadService{db: db, cfg: &config.Config{
		Storage: config.StorageConfig{BasePath: root, StagingPath: stagingRoot},
		ImageV2: config.ImageV2Config{Enabled: true},
	}}
	if _, appErr := svc.Complete(context.Background(), user.ID, uploadKey); appErr == nil {
		t.Fatal("Complete unexpectedly succeeded")
	}

	for kind, relativePath := range finalPathByKind {
		if _, err := os.Stat(filepath.Join(root, relativePath)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("persistent %s survived transaction rollback: %v", kind, err)
		}
		sourcePath := filepath.Join(sessionPath, kind+".ready")
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Errorf("retryable staging %s was not restored: %v", kind, err)
			continue
		}
		if string(content) != string(contentByKind[kind]) {
			t.Errorf("restored staging %s content mismatch", kind)
		}
	}

	var persisted model.UploadSession
	if err := db.First(&persisted, session.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status != model.UploadSessionStatusUploading || persisted.ImageID != nil {
		t.Fatalf("rolled-back session status=%q image=%v", persisted.Status, persisted.ImageID)
	}
	var images int64
	if err := db.Model(&model.Image{}).Where("user_id = ?", user.ID).Count(&images).Error; err != nil || images != 0 {
		t.Fatalf("rolled-back images=%d err=%v", images, err)
	}
}
