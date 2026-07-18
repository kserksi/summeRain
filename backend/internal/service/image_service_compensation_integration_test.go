// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/repository"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestCompensateFailedImageCreateQueuesExactR2Lineage(t *testing.T) {
	db := openImageCompensationTestDB(t)
	suffix := fmt.Sprint(time.Now().UnixNano())
	hash := sha256.Sum256([]byte("failed-image-" + suffix))
	objectName := hex.EncodeToString(hash[:])[:16] + "-" + suffix
	imageFile := model.ImageFile{
		FileHash:       hex.EncodeToString(hash[:]),
		FileSize:       123,
		MimeType:       "image/jpeg",
		ReferenceCount: 99,
		OriginalPath:   "original/" + objectName + ".jpg",
		ThumbnailPath:  "thumbnail/" + objectName + ".webp",
		ProcessedPath:  "processed/" + objectName + ".webp",
		RemoteBackend:  "r2",
		RemoteEndpoint: "https://account.r2.cloudflarestorage.com",
		RemoteBucket:   "summerain-test",
	}
	if err := db.Create(&imageFile).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Where("aggregate_type = ? AND aggregate_id = ?", "image_file", fmt.Sprint(imageFile.ID)).Delete(&model.OutboxEvent{})
		db.Unscoped().Delete(&model.ImageFile{}, imageFile.ID)
	})

	svc := &ImageService{db: db}
	now := time.Now()
	if err := svc.compensateFailedImageCreate(imageFile.ID, now); err != nil {
		t.Fatalf("compensateFailedImageCreate() error = %v", err)
	}
	if err := db.First(&model.ImageFile{}, imageFile.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("image file lookup error = %v, want record not found", err)
	}

	var events []model.OutboxEvent
	if err := db.Where("aggregate_type = ? AND aggregate_id = ?", "image_file", fmt.Sprint(imageFile.ID)).Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("storage deletion events = %d, want one local and one R2 event", len(events))
	}
	wantPaths := map[string]bool{
		imageFile.OriginalPath:              true,
		imageFile.ThumbnailPath:             true,
		imageFile.ProcessedPath:             true,
		"processed/" + objectName + ".avif": true,
	}
	seenPaths := make(map[string]bool, len(wantPaths))
	remoteCount := 0
	for _, event := range events {
		var payload model.StorageDeletePayload
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			t.Fatal(err)
		}
		for _, path := range payload.Paths {
			if !wantPaths[path] {
				t.Fatalf("unexpected cleanup path %q", path)
			}
			seenPaths[path] = true
		}
		for _, object := range payload.RemoteObjects {
			remoteCount++
			if object.Backend != "r2" || object.Endpoint != imageFile.RemoteEndpoint || object.Bucket != imageFile.RemoteBucket {
				t.Fatalf("remote object target = %#v, want exact ImageFile lineage", object)
			}
			if object.Path == "processed/"+objectName+".avif" {
				t.Fatal("locally generated AVIF must not be treated as an uploaded R2 object")
			}
		}
	}
	if len(seenPaths) != len(wantPaths) {
		t.Fatalf("payload paths = %v, want %v", seenPaths, wantPaths)
	}
	if remoteCount != 3 {
		t.Fatalf("remote objects = %d, want 3", remoteCount)
	}
}

func TestCompensateFailedImageCreatePreservesConcurrentReference(t *testing.T) {
	db := openImageCompensationTestDB(t)
	suffix := fmt.Sprint(time.Now().UnixNano())
	user := model.User{
		Username:     "image-compensation-" + suffix,
		Email:        "image-compensation-" + suffix + "@example.test",
		PasswordHash: "test", Role: "user", Status: "active",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("referenced-image-" + suffix))
	imageFile := model.ImageFile{
		FileHash: hex.EncodeToString(hash[:]), FileSize: 123, MimeType: "image/webp",
		ReferenceCount: 99, OriginalPath: "original/" + suffix + ".webp",
	}
	if err := db.Create(&imageFile).Error; err != nil {
		t.Fatal(err)
	}
	image := model.Image{
		UserID: user.ID, ImageFileID: imageFile.ID, UniqueLink: "comp-" + suffix,
		Title: "referenced", Visibility: "public", FileSize: imageFile.FileSize,
	}
	if err := db.Create(&image).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Where("aggregate_type = ? AND aggregate_id = ?", "image_file", fmt.Sprint(imageFile.ID)).Delete(&model.OutboxEvent{})
		db.Unscoped().Delete(&model.Image{}, image.ID)
		db.Unscoped().Delete(&model.ImageFile{}, imageFile.ID)
		db.Unscoped().Delete(&model.User{}, user.ID)
	})

	svc := &ImageService{db: db}
	if err := svc.compensateFailedImageCreate(imageFile.ID, time.Now()); err != nil {
		t.Fatalf("compensateFailedImageCreate() error = %v", err)
	}
	var stored model.ImageFile
	if err := db.First(&stored, imageFile.ID).Error; err != nil {
		t.Fatalf("referenced image file was removed: %v", err)
	}
	if stored.ReferenceCount != 1 {
		t.Fatalf("reference_count = %d, want authoritative count 1", stored.ReferenceCount)
	}
	var events int64
	if err := db.Model(&model.OutboxEvent{}).
		Where("aggregate_type = ? AND aggregate_id = ?", "image_file", fmt.Sprint(imageFile.ID)).Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Fatalf("storage deletion events = %d, want 0 for a referenced file", events)
	}
}

func openImageCompensationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.ApplySchemaMigrations(db); err != nil {
		t.Fatalf("apply schema migrations: %v", err)
	}
	return db
}
