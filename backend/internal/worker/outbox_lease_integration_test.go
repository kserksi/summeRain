// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestOutboxReleaseClaimsFencesOwnerAndToken(t *testing.T) {
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
	owner := "outbox-release-owner-" + suffix
	token := "outbox-release-token-" + suffix
	expires := now.Add(time.Minute)
	event := model.OutboxEvent{
		AggregateType: "test", AggregateID: suffix,
		EventType: "test.outbox.release", DedupeKey: "outbox-release-" + suffix,
		Status: model.OutboxEventStatusPublishing, Payload: "{}",
		Attempts: 2, MaxAttempts: 10, AvailableAt: now.Add(time.Minute),
		LeaseOwner: &owner, LeaseToken: &token, LeaseExpiresAt: &expires,
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&model.OutboxEvent{}, event.ID) })
	store := &gormOutboxStore{db: db}

	wrongOwner := owner + "-stale"
	if err := store.ReleaseClaims(context.Background(), wrongOwner, []model.OutboxEvent{event}, now, true); err != nil {
		t.Fatal(err)
	}
	wrongToken := token + "-stale"
	staleTokenEvent := event
	staleTokenEvent.LeaseToken = &wrongToken
	if err := store.ReleaseClaims(context.Background(), owner, []model.OutboxEvent{staleTokenEvent}, now, true); err != nil {
		t.Fatal(err)
	}

	var stillOwned model.OutboxEvent
	if err := db.First(&stillOwned, event.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stillOwned.Status != model.OutboxEventStatusPublishing || stillOwned.Attempts != 2 || stillOwned.LeaseToken == nil {
		t.Fatalf("stale release changed event: status=%q attempts=%d token=%v", stillOwned.Status, stillOwned.Attempts, stillOwned.LeaseToken)
	}

	if err := store.ReleaseClaims(context.Background(), owner, []model.OutboxEvent{event}, now, true); err != nil {
		t.Fatal(err)
	}
	var released model.OutboxEvent
	if err := db.First(&released, event.ID).Error; err != nil {
		t.Fatal(err)
	}
	if released.Status != model.OutboxEventStatusPending || released.Attempts != 1 {
		t.Fatalf("released status=%q attempts=%d, want pending/1", released.Status, released.Attempts)
	}
	if released.LeaseOwner != nil || released.LeaseToken != nil || released.LeaseExpiresAt != nil {
		t.Fatalf("released lease was retained: owner=%v token=%v expiry=%v", released.LeaseOwner, released.LeaseToken, released.LeaseExpiresAt)
	}

	panicToken := token + "-panic"
	panicEvent := model.OutboxEvent{
		AggregateType: "test", AggregateID: suffix,
		EventType: "test.outbox.release", DedupeKey: "outbox-release-panic-" + suffix,
		Status: model.OutboxEventStatusPublishing, Payload: "{}",
		Attempts: 2, MaxAttempts: 10, AvailableAt: now.Add(time.Minute),
		LeaseOwner: &owner, LeaseToken: &panicToken, LeaseExpiresAt: &expires,
	}
	if err := db.Create(&panicEvent).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&model.OutboxEvent{}, panicEvent.ID) })
	if err := store.ReleaseClaims(context.Background(), owner, []model.OutboxEvent{panicEvent}, now, false); err != nil {
		t.Fatal(err)
	}
	var panicReleased model.OutboxEvent
	if err := db.First(&panicReleased, panicEvent.ID).Error; err != nil {
		t.Fatal(err)
	}
	if panicReleased.Status != model.OutboxEventStatusPending || panicReleased.Attempts != 2 {
		t.Fatalf("non-refunded release status=%q attempts=%d, want pending/2", panicReleased.Status, panicReleased.Attempts)
	}
}
