// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestEnqueueStorageDeleteSeparatesRemoteTargets(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	aggregateID := "storage-group-" + suffix
	paths := []string{"original/a.webp", "processed/a.webp", "original/b.webp", "local/only.webp"}
	remote := []model.StorageDeleteRemoteObject{
		{Path: paths[0], Backend: "r2", Endpoint: "https://a.r2.example", Bucket: "a"},
		{Path: paths[1], Backend: "r2", Endpoint: "https://a.r2.example", Bucket: "a"},
		{Path: paths[2], Backend: "r2", Endpoint: "https://b.r2.example", Bucket: "b"},
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return EnqueueStorageDeleteWithRemote(tx, "test", aggregateID, "storage-group:"+suffix, paths, remote, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Where("aggregate_type = ? AND aggregate_id = ?", "test", aggregateID).Delete(&model.OutboxEvent{})
	})

	var events []model.OutboxEvent
	if err := db.Where("aggregate_type = ? AND aggregate_id = ?", "test", aggregateID).Order("id ASC").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %d, want one local and two target-specific events", len(events))
	}
	seenTargets := make(map[string]bool)
	localEvents := 0
	for _, event := range events {
		var payload model.StorageDeletePayload
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.RemoteObjects) == 0 {
			localEvents++
			continue
		}
		first := payload.RemoteObjects[0]
		for _, object := range payload.RemoteObjects[1:] {
			if object.Backend != first.Backend || object.Endpoint != first.Endpoint || object.Bucket != first.Bucket {
				t.Fatalf("event mixed remote targets: %#v", payload.RemoteObjects)
			}
		}
		seenTargets[first.Endpoint+"/"+first.Bucket] = true
	}
	if localEvents != 1 || !seenTargets["https://a.r2.example/a"] || !seenTargets["https://b.r2.example/b"] {
		t.Fatalf("localEvents=%d targets=%v", localEvents, seenTargets)
	}
}
