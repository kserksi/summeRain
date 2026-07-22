// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const lifecycleMySQLDSNEnv = "SUMMERAIN_LIFECYCLE_TEST_MYSQL_DSN"

// TestBootstrapDatabaseMySQLLifecycleSchema prepares the disposable CI schema
// used by the worker lease-fencing integration tests that run later in the job.
func TestBootstrapDatabaseMySQLLifecycleSchema(t *testing.T) {
	dsn := os.Getenv(lifecycleMySQLDSNEnv)
	if dsn == "" {
		t.Skipf("%s is not configured", lifecycleMySQLDSNEnv)
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open lifecycle integration database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("access lifecycle integration pool: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := BootstrapDatabase(ctx, db); err != nil {
		t.Fatalf("bootstrap lifecycle integration database: %v", err)
	}
}
