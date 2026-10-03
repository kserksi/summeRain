// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestGeneratedMigrationsMatchBootstrapSchema proves that every committed
// .up.sql file reproduces the schema that BootstrapDatabase builds, which is
// the production contract for applying explicit SQL instead of AutoMigrate.
func TestGeneratedMigrationsMatchBootstrapSchema(t *testing.T) {
	dsn := os.Getenv(dsnEnv)
	if dsn == "" {
		t.Skipf("%s is not configured", dsnEnv)
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no generated .up.sql migrations found")
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			if err := verifySchemaFile(context.Background(), dsn, file); err != nil {
				t.Fatalf("verifySchemaFile() error = %v", err)
			}
		})
	}
}
