// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/repository"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestR2UploadIntentFirstMakesAdminRejectTargetSwitch(t *testing.T) {
	db := openR2UploadBarrierTestDB(t)
	putStarted := make(chan struct{})
	releasePut := make(chan struct{})
	var putStartedOnce sync.Once
	var releasePutOnce sync.Once
	defer releasePutOnce.Do(func() { close(releasePut) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		putStartedOnce.Do(func() { close(putStarted) })
		<-releasePut
		w.Header().Set("ETag", `"test"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	suffix := fmt.Sprint(time.Now().UnixNano())
	oldEndpoint := server.URL
	oldBucket := "old-bucket-" + suffix
	installR2UploadBarrierConfigs(t, db, oldEndpoint, oldBucket)
	target := newR2UploadBarrierSnapshot(oldEndpoint, oldBucket, server.Client())
	imageFile := newR2UploadBarrierImageFile(t.Name()+suffix, oldEndpoint, oldBucket)
	cleanupR2UploadBarrierFixture(t, db, imageFile, target)

	imageSvc := &ImageService{db: db, imageFileRepo: repository.NewImageFileRepo(db)}
	uploadDone := make(chan error, 1)
	go func() {
		uploadDone <- imageSvc.persistR2UploadedImageFile(
			&imageFile, target, []byte("original"), "image/webp", nil, false, nil, false,
		)
	}()
	select {
	case <-putStarted:
	case err := <-uploadDone:
		t.Fatalf("upload returned before PUT started: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("R2 PUT did not start")
	}
	activeIntent := findR2UploadBarrierIntent(t, db, imageFile)
	if activeIntent.Status != model.OutboxEventStatusPublishing || activeIntent.LeaseToken == nil || activeIntent.LeaseExpiresAt == nil ||
		activeIntent.AvailableAt.Before(*activeIntent.LeaseExpiresAt) {
		t.Fatalf("active upload intent = %#v, want fenced publishing lease through its availability deadline", activeIntent)
	}

	adminDone := make(chan *errcode.AppError, 1)
	adminSvc := &AdminService{db: db, configRepo: repository.NewSystemConfigRepo(db)}
	go func() {
		_, appErr := adminSvc.UpdateConfigs([]ConfigUpdateItem{
			{Key: "r2_endpoint", Value: "https://new-r2-" + suffix + ".example"},
			{Key: "r2_bucket", Value: "new-bucket-" + suffix},
		})
		adminDone <- appErr
	}()

	select {
	case appErr := <-adminDone:
		if appErr == nil || appErr.Code != 4094 {
			t.Fatalf("UpdateConfigs() error = %#v, want active upload-intent conflict", appErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("UpdateConfigs() did not observe the durable upload intent")
	}
	releasePutOnce.Do(func() { close(releasePut) })
	select {
	case err := <-uploadDone:
		if err != nil {
			t.Fatalf("persistR2UploadedImageFile() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("upload remained blocked after R2 PUT completed")
	}

	var stored model.ImageFile
	if err := db.Where("file_hash = ?", imageFile.FileHash).First(&stored).Error; err != nil {
		t.Fatalf("committed upload lineage missing: %v", err)
	}
	if stored.RemoteBackend != V1StorageBackendR2 || stored.RemoteEndpoint != oldEndpoint || stored.RemoteBucket != oldBucket {
		t.Fatalf("stored upload lineage = %#v, want exact old R2 target", stored)
	}
	intent := findR2UploadBarrierIntent(t, db, imageFile)
	if intent.Status != model.OutboxEventStatusPublished || intent.PublishedAt == nil {
		t.Fatalf("successful upload cleanup intent = %#v, want published cancellation", intent)
	}
	assertR2UploadBarrierIntentPayload(t, intent, imageFile, target)
	if _, appErr := adminSvc.UpdateConfigs([]ConfigUpdateItem{
		{Key: "r2_endpoint", Value: "https://new-r2-" + suffix + ".example"},
		{Key: "r2_bucket", Value: "new-bucket-" + suffix},
	}); appErr == nil || appErr.Code != 4094 {
		t.Fatalf("UpdateConfigs() after upload error = %#v, want committed lineage conflict", appErr)
	}
	if got := r2UploadBarrierConfigValue(t, db, "r2_endpoint"); got != oldEndpoint {
		t.Fatalf("r2_endpoint = %q, want rejected switch to preserve %q", got, oldEndpoint)
	}
}

func TestAdminTargetSwitchFirstMakesR2UploadFailBeforePUT(t *testing.T) {
	db := openR2UploadBarrierTestDB(t)
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	suffix := fmt.Sprint(time.Now().UnixNano())
	oldEndpoint := server.URL
	oldBucket := "old-bucket-" + suffix
	installR2UploadBarrierConfigs(t, db, oldEndpoint, oldBucket)
	target := newR2UploadBarrierSnapshot(oldEndpoint, oldBucket, server.Client())
	imageFile := newR2UploadBarrierImageFile(t.Name()+suffix, oldEndpoint, oldBucket)
	cleanupR2UploadBarrierFixture(t, db, imageFile, target)

	adminTx := db.Begin()
	if adminTx.Error != nil {
		t.Fatal(adminTx.Error)
	}
	adminCommitted := false
	defer func() {
		if !adminCommitted {
			adminTx.Rollback()
		}
	}()
	if err := lockV2Storage(adminTx); err != nil {
		t.Fatal(err)
	}
	newEndpoint := "https://new-r2-" + suffix + ".example"
	newBucket := "new-bucket-" + suffix
	for key, value := range map[string]string{"r2_endpoint": newEndpoint, "r2_bucket": newBucket} {
		if err := adminTx.Model(&model.SystemConfig{}).Where("config_key = ?", key).
			UpdateColumn("config_value", value).Error; err != nil {
			t.Fatal(err)
		}
	}

	imageSvc := &ImageService{db: db, imageFileRepo: repository.NewImageFileRepo(db)}
	uploadDone := make(chan error, 1)
	go func() {
		uploadDone <- imageSvc.persistR2UploadedImageFile(
			&imageFile, target, []byte("original"), "image/webp", nil, false, nil, false,
		)
	}()
	select {
	case err := <-uploadDone:
		t.Fatalf("upload returned before admin commit: %v", err)
	case request := <-requests:
		t.Fatalf("R2 request %q started before upload could validate the target", request)
	case <-time.After(150 * time.Millisecond):
	}
	if err := adminTx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	adminCommitted = true

	select {
	case err := <-uploadDone:
		if !errors.Is(err, errR2UploadTargetChanged) {
			t.Fatalf("persistR2UploadedImageFile() error = %v, want target-changed sentinel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("upload remained blocked after admin commit")
	}
	select {
	case request := <-requests:
		t.Fatalf("target-mismatched upload issued R2 request %q", request)
	default:
	}
	var imageFileCount int64
	if err := db.Model(&model.ImageFile{}).Where("file_hash = ?", imageFile.FileHash).Count(&imageFileCount).Error; err != nil {
		t.Fatal(err)
	}
	if imageFileCount != 0 {
		t.Fatalf("image_files rows = %d, want no upload lineage", imageFileCount)
	}
	var intentCount int64
	if err := db.Model(&model.OutboxEvent{}).
		Where("aggregate_type = ? AND payload LIKE ?", v1R2UploadIntentAggregate, "%"+imageFile.OriginalPath+"%").
		Count(&intentCount).Error; err != nil {
		t.Fatal(err)
	}
	if intentCount != 0 {
		t.Fatalf("cleanup intents = %d, want none because no PUT could start", intentCount)
	}
}

func TestRemoteDeleteIntentCommitFirstMakesAdminRejectTargetSwitchIncludingDead(t *testing.T) {
	db := openR2UploadBarrierTestDB(t)
	suffix := fmt.Sprint(time.Now().UnixNano())
	oldEndpoint := "https://old-r2-" + suffix + ".example"
	oldBucket := "old-bucket-" + suffix
	installR2UploadBarrierConfigs(t, db, oldEndpoint, oldBucket)

	path := "original/delete-before-switch-" + suffix + ".webp"
	payload, err := json.Marshal(model.StorageDeletePayload{
		Paths: []string{path},
		RemoteObjects: []model.StorageDeleteRemoteObject{{
			Path: path, Backend: V1StorageBackendR2, Endpoint: oldEndpoint, Bucket: oldBucket,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	event := model.OutboxEvent{
		AggregateType: "test", AggregateID: suffix,
		EventType: model.OutboxEventTypeStorageDelete, DedupeKey: "delete-before-switch:" + suffix,
		Status: model.OutboxEventStatusDead, Payload: string(payload), MaxAttempts: 1, AvailableAt: time.Now(),
	}
	t.Cleanup(func() {
		db.Unscoped().Where("dedupe_key = ?", event.DedupeKey).Delete(&model.OutboxEvent{})
	})
	deleteTx := db.Begin()
	if deleteTx.Error != nil {
		t.Fatal(deleteTx.Error)
	}
	deleteCommitted := false
	defer func() {
		if !deleteCommitted {
			deleteTx.Rollback()
		}
	}()
	if err := lockV2Storage(deleteTx); err != nil {
		t.Fatal(err)
	}
	if err := deleteTx.Create(&event).Error; err != nil {
		t.Fatal(err)
	}

	adminDone := make(chan *errcode.AppError, 1)
	adminSvc := &AdminService{db: db, configRepo: repository.NewSystemConfigRepo(db)}
	go func() {
		_, appErr := adminSvc.UpdateConfigs([]ConfigUpdateItem{
			{Key: "r2_endpoint", Value: "https://new-r2-" + suffix + ".example"},
			{Key: "r2_bucket", Value: "new-bucket-" + suffix},
		})
		adminDone <- appErr
	}()
	select {
	case appErr := <-adminDone:
		t.Fatalf("UpdateConfigs() returned before remote delete intent commit: %v", appErr)
	case <-time.After(150 * time.Millisecond):
	}
	if err := deleteTx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	deleteCommitted = true

	select {
	case appErr := <-adminDone:
		if appErr == nil || appErr.Code != 4094 || !strings.Contains(appErr.Message, "清理") {
			t.Fatalf("UpdateConfigs() error = %#v, want unfinished-cleanup conflict", appErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("UpdateConfigs() remained blocked after remote delete intent commit")
	}
	if got := r2UploadBarrierConfigValue(t, db, "r2_endpoint"); got != oldEndpoint {
		t.Fatalf("r2_endpoint = %q, want dead cleanup to preserve %q", got, oldEndpoint)
	}
}

func TestR2UploadFailureActivatesDurableExactCleanup(t *testing.T) {
	db := openR2UploadBarrierTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rejected", http.StatusBadRequest)
	}))
	defer server.Close()

	suffix := fmt.Sprint(time.Now().UnixNano())
	endpoint := server.URL
	bucket := "cleanup-bucket-" + suffix
	installR2UploadBarrierConfigs(t, db, endpoint, bucket)
	target := newR2UploadBarrierSnapshot(endpoint, bucket, server.Client())
	imageFile := newR2UploadBarrierImageFile(t.Name()+suffix, endpoint, bucket)
	cleanupR2UploadBarrierFixture(t, db, imageFile, target)

	imageSvc := &ImageService{db: db, imageFileRepo: repository.NewImageFileRepo(db)}
	startedAt := time.Now()
	err := imageSvc.persistR2UploadedImageFile(
		&imageFile, target, []byte("original"), "image/webp", nil, false, nil, false,
	)
	if !errors.Is(err, errR2UploadFailed) {
		t.Fatalf("persistR2UploadedImageFile() error = %v, want upload-failed sentinel", err)
	}
	intent := findR2UploadBarrierIntent(t, db, imageFile)
	if intent.Status != model.OutboxEventStatusPending || intent.AvailableAt.After(time.Now()) || intent.AvailableAt.Before(startedAt.Add(-time.Second)) {
		t.Fatalf("failed upload cleanup intent = %#v, want immediately available pending event", intent)
	}
	assertR2UploadBarrierIntentPayload(t, intent, imageFile, target)
	var count int64
	if err := db.Model(&model.ImageFile{}).Where("file_hash = ?", imageFile.FileHash).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("image_files rows = %d, want failed upload rollback", count)
	}
}

func TestR2ImageFileCreateFailureActivatesDurableExactCleanup(t *testing.T) {
	db := openR2UploadBarrierTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"test"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	suffix := fmt.Sprint(time.Now().UnixNano())
	endpoint := server.URL
	bucket := "db-failure-bucket-" + suffix
	installR2UploadBarrierConfigs(t, db, endpoint, bucket)
	target := newR2UploadBarrierSnapshot(endpoint, bucket, server.Client())
	imageFile := newR2UploadBarrierImageFile(t.Name()+suffix, endpoint, bucket)
	cleanupR2UploadBarrierFixture(t, db, imageFile, target)
	existing := imageFile
	existing.OriginalPath = "original/existing-" + suffix + ".webp"
	existing.ThumbnailPath = ""
	existing.ProcessedPath = ""
	existing.RemoteBackend = V1StorageBackendLocal
	existing.RemoteEndpoint = ""
	existing.RemoteBucket = ""
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}

	imageSvc := &ImageService{db: db, imageFileRepo: repository.NewImageFileRepo(db)}
	startedAt := time.Now()
	err := imageSvc.persistR2UploadedImageFile(
		&imageFile, target, []byte("original"), "image/webp", nil, false, nil, false,
	)
	if err == nil || errors.Is(err, errR2UploadFailed) {
		t.Fatalf("persistR2UploadedImageFile() error = %v, want ImageFile insert failure", err)
	}
	intent := findR2UploadBarrierIntent(t, db, imageFile)
	if intent.Status != model.OutboxEventStatusPending || intent.AvailableAt.After(time.Now()) || intent.AvailableAt.Before(startedAt.Add(-time.Second)) {
		t.Fatalf("database-failed upload cleanup intent = %#v, want immediately available pending event", intent)
	}
	assertR2UploadBarrierIntentPayload(t, intent, imageFile, target)
	var rows []model.ImageFile
	if err := db.Where("file_hash = ?", imageFile.FileHash).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].OriginalPath != existing.OriginalPath {
		t.Fatalf("image file rows = %#v, want only pre-existing duplicate", rows)
	}
}

func TestFailedR2UploadCanStageASecondIntentForSameContent(t *testing.T) {
	db := openR2UploadBarrierTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rejected", http.StatusBadRequest)
	}))
	defer server.Close()

	suffix := fmt.Sprint(time.Now().UnixNano())
	endpoint := server.URL
	bucket := "retry-bucket-" + suffix
	installR2UploadBarrierConfigs(t, db, endpoint, bucket)
	target := newR2UploadBarrierSnapshot(endpoint, bucket, server.Client())
	imageFile := newR2UploadBarrierImageFile(t.Name()+suffix, endpoint, bucket)
	cleanupR2UploadBarrierFixture(t, db, imageFile, target)
	imageSvc := &ImageService{db: db, imageFileRepo: repository.NewImageFileRepo(db)}

	for attempt := 0; attempt < 2; attempt++ {
		err := imageSvc.persistR2UploadedImageFile(
			&imageFile, target, []byte("same-original"), "image/webp", nil, false, nil, false,
		)
		if !errors.Is(err, errR2UploadFailed) {
			t.Fatalf("attempt %d error = %v, want upload-failed sentinel", attempt+1, err)
		}
	}
	intents := findAllR2UploadBarrierIntents(t, db, imageFile)
	if len(intents) != 2 {
		t.Fatalf("cleanup intents = %d, want one unique intent per retry", len(intents))
	}
	if intents[0].AggregateID == intents[1].AggregateID || intents[0].DedupeKey == intents[1].DedupeKey {
		t.Fatalf("retry reused intent identity: %#v %#v", intents[0], intents[1])
	}
	for _, intent := range intents {
		if intent.Status != model.OutboxEventStatusPending {
			t.Fatalf("retry intent status = %q, want pending cleanup", intent.Status)
		}
		assertR2UploadBarrierIntentPayload(t, intent, imageFile, target)
	}
}

func openR2UploadBarrierTestDB(t *testing.T) *gorm.DB {
	t.Helper()
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
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(6)
	return db
}

type r2UploadBarrierConfigSnapshot struct {
	config model.SystemConfig
	exists bool
}

func installR2UploadBarrierConfigs(t *testing.T, db *gorm.DB, endpoint, bucket string) {
	t.Helper()
	values := map[string]string{
		"r2_enabled":  "true",
		"r2_endpoint": endpoint,
		"r2_bucket":   bucket,
	}
	originals := make(map[string]r2UploadBarrierConfigSnapshot, len(values))
	for key := range values {
		var item model.SystemConfig
		err := db.Where("config_key = ?", key).First(&item).Error
		switch {
		case err == nil:
			originals[key] = r2UploadBarrierConfigSnapshot{config: item, exists: true}
		case errors.Is(err, gorm.ErrRecordNotFound):
			originals[key] = r2UploadBarrierConfigSnapshot{}
		default:
			t.Fatal(err)
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		for key, value := range values {
			item := model.SystemConfig{ConfigKey: key, ConfigValue: value, ConfigType: "string"}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "config_key"}},
				DoUpdates: clause.AssignmentColumns([]string{"config_value"}),
			}).Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := db.Transaction(func(tx *gorm.DB) error {
			for key, snapshot := range originals {
				if !snapshot.exists {
					if err := tx.Where("config_key = ?", key).Delete(&model.SystemConfig{}).Error; err != nil {
						return err
					}
					continue
				}
				if err := tx.Model(&model.SystemConfig{}).Where("config_key = ?", key).Updates(map[string]interface{}{
					"config_value": snapshot.config.ConfigValue,
					"config_type":  snapshot.config.ConfigType,
					"description":  snapshot.config.Description,
					"updated_at":   snapshot.config.UpdatedAt,
				}).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Errorf("restore R2 config fixture: %v", err)
		}
	})
}

func cleanupR2UploadBarrierFixture(t *testing.T, db *gorm.DB, imageFile model.ImageFile, _ *r2UploadSnapshot) {
	t.Helper()
	t.Cleanup(func() {
		db.Unscoped().Where("file_hash = ?", imageFile.FileHash).Delete(&model.ImageFile{})
		db.Unscoped().Where("aggregate_type = ? AND payload LIKE ?", v1R2UploadIntentAggregate, "%"+imageFile.OriginalPath+"%").
			Delete(&model.OutboxEvent{})
	})
}

func r2UploadBarrierConfigValue(t *testing.T, db *gorm.DB, key string) string {
	t.Helper()
	var item model.SystemConfig
	if err := db.Where("config_key = ?", key).First(&item).Error; err != nil {
		t.Fatal(err)
	}
	return item.ConfigValue
}

func findR2UploadBarrierIntent(t *testing.T, db *gorm.DB, imageFile model.ImageFile) model.OutboxEvent {
	t.Helper()
	var event model.OutboxEvent
	if err := db.Where("aggregate_type = ? AND payload LIKE ?", v1R2UploadIntentAggregate, "%"+imageFile.OriginalPath+"%").
		First(&event).Error; err != nil {
		t.Fatal(err)
	}
	return event
}

func findAllR2UploadBarrierIntents(t *testing.T, db *gorm.DB, imageFile model.ImageFile) []model.OutboxEvent {
	t.Helper()
	var events []model.OutboxEvent
	if err := db.Where("aggregate_type = ? AND payload LIKE ?", v1R2UploadIntentAggregate, "%"+imageFile.OriginalPath+"%").
		Order("id ASC").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	return events
}

func assertR2UploadBarrierIntentPayload(t *testing.T, event model.OutboxEvent, imageFile model.ImageFile, target *r2UploadSnapshot) {
	t.Helper()
	var payload model.StorageDeletePayload
	if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
		t.Fatal(err)
	}
	wantPaths := map[string]bool{
		imageFile.OriginalPath:  true,
		imageFile.ThumbnailPath: true,
		imageFile.ProcessedPath: true,
	}
	if len(payload.Paths) != len(wantPaths) || len(payload.RemoteObjects) != len(wantPaths) {
		t.Fatalf("cleanup payload = %#v, want three exact paths and remote objects", payload)
	}
	for _, path := range payload.Paths {
		if !wantPaths[path] {
			t.Fatalf("unexpected cleanup path %q", path)
		}
	}
	for _, object := range payload.RemoteObjects {
		if !wantPaths[object.Path] || object.Backend != V1StorageBackendR2 || object.Endpoint != target.endpoint || object.Bucket != target.bucket {
			t.Fatalf("cleanup remote object = %#v, want exact upload snapshot", object)
		}
	}
}

func newR2UploadBarrierImageFile(seed, endpoint, bucket string) model.ImageFile {
	hash := sha256.Sum256([]byte(seed))
	fileHash := hex.EncodeToString(hash[:])
	objectName := fileHash[:16]
	return model.ImageFile{
		FileHash:       fileHash,
		FileSize:       1,
		MimeType:       "image/webp",
		OriginalPath:   "original/" + objectName + ".webp",
		ThumbnailPath:  "thumbnail/" + objectName + ".webp",
		ProcessedPath:  "processed/" + objectName + ".webp",
		RemoteBackend:  V1StorageBackendR2,
		RemoteEndpoint: endpoint,
		RemoteBucket:   bucket,
	}
}

func newR2UploadBarrierSnapshot(endpoint, bucket string, httpClient *http.Client) *r2UploadSnapshot {
	awsConfig := aws.Config{
		Region:      "auto",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("access", "secret", "")),
		HTTPClient:  httpClient,
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = &endpoint
		options.UsePathStyle = true
	})
	return &r2UploadSnapshot{client: client, endpoint: endpoint, bucket: bucket}
}
