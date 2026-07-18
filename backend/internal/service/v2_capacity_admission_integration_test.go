// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/pkg/errcode"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestCompleteCapacityWaitDoesNotConsumeDatabaseConnections(t *testing.T) {
	dsn := os.Getenv("SUMMERAIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SUMMERAIN_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	previousMaxOpen := sqlDB.Stats().MaxOpenConnections
	sqlDB.SetMaxOpenConns(2)
	t.Cleanup(func() { sqlDB.SetMaxOpenConns(previousMaxOpen) })

	svc := &V2UploadService{db: db}
	release, admitted := svc.acquireCapacityGate(context.Background())
	if !admitted {
		t.Fatal("failed to occupy capacity gate")
	}
	defer release()

	contexts := make([]context.CancelFunc, 2)
	results := make(chan *errcode.AppError, len(contexts))
	var started sync.WaitGroup
	started.Add(len(contexts))
	for i := range contexts {
		ctx, cancel := context.WithCancel(context.Background())
		contexts[i] = cancel
		go func(uploadID string) {
			started.Done()
			_, appErr := svc.Complete(ctx, 1, uploadID)
			results <- appErr
		}(string(rune('a' + i)))
	}
	started.Wait()
	time.Sleep(25 * time.Millisecond)

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer queryCancel()
	var one int
	if err := db.WithContext(queryCtx).Raw("SELECT 1").Scan(&one).Error; err != nil {
		t.Fatalf("status query could not obtain a database connection: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 = %d", one)
	}

	for _, cancel := range contexts {
		cancel()
	}
	for range contexts {
		appErr := <-results
		if appErr == nil || appErr.Code != errcode.ErrUploadBusy.Code {
			t.Fatalf("capacity waiter error = %#v, want upload busy", appErr)
		}
	}
}
