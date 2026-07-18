// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestStorageDeleteOutboxRechecksReferences(t *testing.T) {
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

	root := t.TempDir()
	orphanPath := filepath.Join("v2", "orphan.webp")
	referencedPath := filepath.Join("v2", "referenced.webp")
	for _, storedPath := range []string{orphanPath, referencedPath} {
		fullPath := filepath.Join(root, storedPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(storedPath), 0640); err != nil {
			t.Fatal(err)
		}
	}

	hash := sha256.Sum256([]byte(t.Name()))
	imageFile := model.ImageFile{
		FileHash:       hex.EncodeToString(hash[:]),
		FileSize:       1,
		MimeType:       "image/webp",
		ReferenceCount: 1,
		OriginalPath:   referencedPath,
	}
	if err := db.Create(&imageFile).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&model.ImageFile{}, imageFile.ID) })

	payload, err := json.Marshal(model.StorageDeletePayload{
		Paths: []string{orphanPath, referencedPath},
		RemoteObjects: []model.StorageDeleteRemoteObject{
			{Path: orphanPath, Backend: "r2", Endpoint: "https://r2.example", Bucket: "images"},
			{Path: referencedPath, Backend: "r2", Endpoint: "https://r2.example", Bucket: "images"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	delivery := newOutboxDeliveryForStorageTest(db, root)
	remote := &recordingRemoteDeleter{configured: true}
	delivery.r2 = remote
	if err := delivery.Deliver(context.Background(), model.OutboxEvent{
		EventType: model.OutboxEventTypeStorageDelete,
		Payload:   string(payload),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, orphanPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, referencedPath)); err != nil {
		t.Fatalf("referenced file was removed: %v", err)
	}
	if got := remote.paths(); !slices.Equal(got, []string{orphanPath}) {
		t.Fatalf("R2 deleted paths = %v, want only the unreferenced object", got)
	}
}

func TestStorageDeleteOutboxRejectsEscapingPath(t *testing.T) {
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
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.webp")
	if err := os.WriteFile(outside, []byte("keep"), 0640); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(model.StorageDeletePayload{Paths: []string{outside}})
	if err != nil {
		t.Fatal(err)
	}
	if err := newOutboxDeliveryForStorageTest(db, root).Deliver(context.Background(), model.OutboxEvent{
		EventType: model.OutboxEventTypeStorageDelete,
		Payload:   string(payload),
	}); err == nil {
		t.Fatal("escaping storage path was accepted")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file was touched: %v", err)
	}
}

func newOutboxDeliveryForStorageTest(db *gorm.DB, root string) *outboxDelivery {
	delivery := newOutboxDelivery(config.CDNConfig{PurgeRequestsPerSecond: 1})
	delivery.db = db
	delivery.storageRoot = root
	return delivery
}

type recordingRemoteDeleter struct {
	mu         sync.Mutex
	configured bool
	deleted    []string
}

func (d *recordingRemoteDeleter) CanDelete(endpoint, bucket string) bool {
	if endpoint != "https://r2.example" || bucket != "images" {
		return false
	}
	return d.configured
}

func (d *recordingRemoteDeleter) DeleteContext(_ context.Context, endpoint, bucket, path string) error {
	if endpoint != "https://r2.example" || bucket != "images" {
		return errors.New("unexpected remote target")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deleted = append(d.deleted, path)
	return nil
}

func (d *recordingRemoteDeleter) paths() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.deleted...)
}
