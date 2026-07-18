// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestDeleteUnclassifiedV1ImageQueuesOnlyResolvedStorageTarget(t *testing.T) {
	db := openLineageCompatibilityTestDB(t)
	for _, test := range []struct {
		name       string
		local      bool
		configured bool
		wantRemote bool
		wantError  bool
		admin      bool
	}{
		{name: "safe local original wins", local: true, configured: true},
		{name: "admin delete missing local uses exact current R2", configured: true, wantRemote: true, admin: true},
		{name: "missing local without current R2 fails closed", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			suffix := fmt.Sprint(time.Now().UnixNano())
			user := model.User{
				Username: "lineage-delete-" + suffix, Email: "lineage-delete-" + suffix + "@example.test",
				PasswordHash: "test", Role: "user", Status: "active", ImageCount: 1, StorageUsed: 123,
			}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256([]byte(t.Name() + suffix))
			imageFile := model.ImageFile{
				FileHash: hex.EncodeToString(hash[:]), FileSize: 123, MimeType: "image/webp", ReferenceCount: 1,
				OriginalPath:  "original/" + suffix + ".webp",
				ThumbnailPath: "thumbnail/" + suffix + ".webp",
				ProcessedPath: "processed/" + suffix + ".webp",
			}
			if err := db.Create(&imageFile).Error; err != nil {
				t.Fatal(err)
			}
			image := model.Image{
				UserID: user.ID, ImageFileID: imageFile.ID, UniqueLink: "lineage-" + suffix,
				Filename: "legacy.webp", Visibility: "private", PipelineVersion: 1,
				ProcessingStatus: model.ImageProcessingStatusCompleted, FileSize: imageFile.FileSize,
			}
			if err := db.Create(&image).Error; err != nil {
				t.Fatal(err)
			}
			if test.local {
				fullPath := filepath.Join(root, imageFile.OriginalPath)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fullPath, []byte("local"), 0640); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() {
				db.Where("aggregate_type = ? AND aggregate_id = ?", "image", fmt.Sprint(image.ID)).Delete(&model.OutboxEvent{})
				db.Unscoped().Delete(&model.Image{}, image.ID)
				db.Unscoped().Delete(&model.ImageFile{}, imageFile.ID)
				db.Unscoped().Delete(&model.User{}, user.ID)
			})

			var r2 *R2Service
			if test.configured {
				r2 = &R2Service{configured: true, endpoint: "https://current-r2.example", bucket: "legacy-images"}
			}
			svc := &ImageService{db: db, storageCfg: &config.StorageConfig{BasePath: root}, r2: r2}
			var appErr *errcode.AppError
			if test.admin {
				_, appErr = svc.AdminDelete(image.ID)
			} else {
				_, appErr = svc.Delete(user.ID, image.ID)
			}
			if test.wantError {
				if appErr != errcode.ErrDatabase {
					t.Fatalf("Delete() error = %#v, want database failure", appErr)
				}
				var imageCount, fileCount int64
				db.Model(&model.Image{}).Where("id = ?", image.ID).Count(&imageCount)
				db.Model(&model.ImageFile{}).Where("id = ?", imageFile.ID).Count(&fileCount)
				if imageCount != 1 || fileCount != 1 {
					t.Fatalf("failed-closed delete changed rows: images=%d files=%d", imageCount, fileCount)
				}
				var eventCount int64
				if err := db.Model(&model.OutboxEvent{}).
					Where("aggregate_type = ? AND aggregate_id = ?", "image", fmt.Sprint(image.ID)).
					Count(&eventCount).Error; err != nil {
					t.Fatal(err)
				}
				if eventCount != 0 {
					t.Fatalf("failed-closed delete committed %d outbox events", eventCount)
				}
				return
			}
			if appErr != nil {
				t.Fatalf("Delete() error = %v", appErr)
			}

			var events []model.OutboxEvent
			if err := db.Where("aggregate_type = ? AND aggregate_id = ? AND event_type = ?", "image", fmt.Sprint(image.ID), model.OutboxEventTypeStorageDelete).
				Find(&events).Error; err != nil {
				t.Fatal(err)
			}
			if len(events) == 0 {
				t.Fatal("no storage delete event was queued")
			}
			var remoteObjects []model.StorageDeleteRemoteObject
			for _, event := range events {
				var payload model.StorageDeletePayload
				if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
					t.Fatal(err)
				}
				remoteObjects = append(remoteObjects, payload.RemoteObjects...)
			}
			if test.wantRemote {
				if len(remoteObjects) != 3 {
					t.Fatalf("remote objects = %#v, want 3 exact R2 objects", remoteObjects)
				}
				for _, object := range remoteObjects {
					if object.Endpoint != "https://current-r2.example" || object.Bucket != "legacy-images" {
						t.Fatalf("remote object target = %#v", object)
					}
				}
			} else if len(remoteObjects) != 0 {
				t.Fatalf("local legacy deletion queued remote objects: %#v", remoteObjects)
			}
		})
	}
}

func openLineageCompatibilityTestDB(t *testing.T) *gorm.DB {
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
	return db
}
