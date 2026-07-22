// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	schemaMigrationLockTimeoutSeconds = 60
	schemaMigrationLockPrefix         = "summerain:schema-migrations:"
)

type schemaMigrationOperationKind string

const (
	schemaMigrationSQL    schemaMigrationOperationKind = "sql"
	schemaMigrationColumn schemaMigrationOperationKind = "column"
	schemaMigrationIndex  schemaMigrationOperationKind = "index"
)

type schemaMigrationOperation struct {
	Kind      schemaMigrationOperationKind
	TableName string
	Name      string
	Statement string
}

type schemaMigration struct {
	Version    uint64
	Name       string
	Operations []schemaMigrationOperation
}

type appliedSchemaMigration struct {
	Version  uint64
	Name     string
	Checksum string
}

var schemaMigrations = []schemaMigration{
	{
		Version: 2026071501,
		Name:    "images_v2_pipeline_columns",
		Operations: []schemaMigrationOperation{
			{
				Kind:      schemaMigrationColumn,
				TableName: "images",
				Name:      "pipeline_version",
				Statement: "ALTER TABLE `images` ADD COLUMN `pipeline_version` SMALLINT UNSIGNED NOT NULL DEFAULT 1 AFTER `visibility`",
			},
			{
				Kind:      schemaMigrationColumn,
				TableName: "images",
				Name:      "processing_status",
				Statement: "ALTER TABLE `images` ADD COLUMN `processing_status` VARCHAR(24) NOT NULL DEFAULT 'completed' AFTER `pipeline_version`",
			},
			{
				Kind:      schemaMigrationColumn,
				TableName: "images",
				Name:      "origin_alias",
				Statement: "ALTER TABLE `images` ADD COLUMN `origin_alias` VARCHAR(255) NULL AFTER `processing_status`",
			},
			{
				Kind:      schemaMigrationIndex,
				TableName: "images",
				Name:      "idx_images_processing_status",
				Statement: "CREATE INDEX `idx_images_processing_status` ON `images` (`processing_status`)",
			},
			{
				Kind:      schemaMigrationIndex,
				TableName: "images",
				Name:      "idx_images_origin_alias",
				Statement: "CREATE INDEX `idx_images_origin_alias` ON `images` (`origin_alias`)",
			},
		},
	},
	{
		Version: 2026071502,
		Name:    "v2_image_pipeline_tables",
		Operations: []schemaMigrationOperation{
			{
				Kind: schemaMigrationSQL,
				Name: "image_variants",
				Statement: `CREATE TABLE IF NOT EXISTS ` + "`image_variants`" + ` (
  ` + "`id`" + ` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  ` + "`image_id`" + ` BIGINT UNSIGNED NOT NULL,
  ` + "`kind`" + ` VARCHAR(24) NOT NULL,
  ` + "`revision`" + ` INT UNSIGNED NOT NULL DEFAULT 1,
  ` + "`pipeline_version`" + ` SMALLINT UNSIGNED NOT NULL DEFAULT 2,
  ` + "`status`" + ` VARCHAR(20) NOT NULL DEFAULT 'pending',
  ` + "`file_hash`" + ` VARCHAR(64) NOT NULL,
  ` + "`file_size`" + ` BIGINT NOT NULL,
  ` + "`mime_type`" + ` VARCHAR(50) NOT NULL,
  ` + "`width`" + ` BIGINT NOT NULL,
  ` + "`height`" + ` BIGINT NOT NULL,
  ` + "`quality`" + ` TINYINT UNSIGNED NOT NULL DEFAULT 0,
  ` + "`storage_path`" + ` VARCHAR(500) NOT NULL,
  ` + "`watermark_version`" + ` VARCHAR(64) NULL,
  ` + "`is_active`" + ` TINYINT(1) NOT NULL DEFAULT 0,
  ` + "`ready_at`" + ` DATETIME(3) NULL,
  ` + "`created_at`" + ` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  ` + "`updated_at`" + ` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (` + "`id`" + `),
  UNIQUE KEY ` + "`uk_image_variant_revision`" + ` (` + "`image_id`" + `, ` + "`kind`" + `, ` + "`revision`" + `),
  KEY ` + "`idx_image_variants_image_id`" + ` (` + "`image_id`" + `),
  KEY ` + "`idx_image_variants_kind`" + ` (` + "`kind`" + `),
  KEY ` + "`idx_image_variants_status`" + ` (` + "`status`" + `),
  KEY ` + "`idx_image_variants_file_hash`" + ` (` + "`file_hash`" + `),
  KEY ` + "`idx_image_variants_watermark_version`" + ` (` + "`watermark_version`" + `),
  KEY ` + "`idx_image_variants_is_active`" + ` (` + "`is_active`" + `),
  KEY ` + "`idx_image_variants_ready_at`" + ` (` + "`ready_at`" + `),
  CONSTRAINT ` + "`fk_image_variants_image`" + ` FOREIGN KEY (` + "`image_id`" + `) REFERENCES ` + "`images`" + ` (` + "`id`" + `) ON UPDATE CASCADE ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,
			},
			{
				Kind: schemaMigrationSQL,
				Name: "upload_sessions",
				Statement: `CREATE TABLE IF NOT EXISTS ` + "`upload_sessions`" + ` (
  ` + "`id`" + ` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  ` + "`upload_key`" + ` VARCHAR(64) NOT NULL,
  ` + "`user_id`" + ` BIGINT UNSIGNED NOT NULL,
  ` + "`idempotency_key`" + ` VARCHAR(64) NULL,
  ` + "`status`" + ` VARCHAR(24) NOT NULL DEFAULT 'initiated',
  ` + "`pipeline_version`" + ` SMALLINT UNSIGNED NOT NULL DEFAULT 2,
  ` + "`visibility`" + ` VARCHAR(10) NOT NULL DEFAULT 'public',
  ` + "`filename`" + ` VARCHAR(255) NOT NULL,
  ` + "`source_mime_type`" + ` VARCHAR(50) NOT NULL,
  ` + "`source_width`" + ` BIGINT NOT NULL DEFAULT 0,
  ` + "`source_height`" + ` BIGINT NOT NULL DEFAULT 0,
  ` + "`processor_version`" + ` VARCHAR(64) NOT NULL,
  ` + "`recipe_version`" + ` VARCHAR(32) NOT NULL,
  ` + "`image_id`" + ` BIGINT UNSIGNED NULL,
  ` + "`expected_part_count`" + ` TINYINT UNSIGNED NOT NULL DEFAULT 0,
  ` + "`received_part_count`" + ` TINYINT UNSIGNED NOT NULL DEFAULT 0,
  ` + "`reserved_bytes`" + ` BIGINT NOT NULL DEFAULT 0,
  ` + "`actual_bytes`" + ` BIGINT NOT NULL DEFAULT 0,
  ` + "`staging_path`" + ` VARCHAR(500) NOT NULL,
  ` + "`manifest_hash`" + ` VARCHAR(64) NULL,
  ` + "`client_manifest`" + ` JSON NULL,
  ` + "`focal_x`" + ` DECIMAL(8,7) NULL,
  ` + "`focal_y`" + ` DECIMAL(8,7) NULL,
  ` + "`error_code`" + ` BIGINT NULL,
  ` + "`error_message`" + ` TEXT NULL,
  ` + "`expires_at`" + ` DATETIME(3) NOT NULL,
  ` + "`processing_at`" + ` DATETIME(3) NULL,
  ` + "`completed_at`" + ` DATETIME(3) NULL,
  ` + "`failed_at`" + ` DATETIME(3) NULL,
  ` + "`cancelled_at`" + ` DATETIME(3) NULL,
  ` + "`cleanup_after`" + ` DATETIME(3) NULL,
  ` + "`created_at`" + ` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  ` + "`updated_at`" + ` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (` + "`id`" + `),
  UNIQUE KEY ` + "`idx_upload_sessions_upload_key`" + ` (` + "`upload_key`" + `),
  UNIQUE KEY ` + "`uk_user_upload_idempotency`" + ` (` + "`user_id`" + `, ` + "`idempotency_key`" + `),
  KEY ` + "`idx_upload_sessions_user_id`" + ` (` + "`user_id`" + `),
  KEY ` + "`idx_upload_sessions_image_id`" + ` (` + "`image_id`" + `),
  KEY ` + "`idx_upload_session_expiry`" + ` (` + "`status`" + `, ` + "`expires_at`" + `),
  KEY ` + "`idx_upload_sessions_cleanup_after`" + ` (` + "`cleanup_after`" + `),
  CONSTRAINT ` + "`fk_upload_sessions_user`" + ` FOREIGN KEY (` + "`user_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT ` + "`fk_upload_sessions_image`" + ` FOREIGN KEY (` + "`image_id`" + `) REFERENCES ` + "`images`" + ` (` + "`id`" + `) ON UPDATE CASCADE ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,
			},
			{
				Kind: schemaMigrationSQL,
				Name: "upload_parts",
				Statement: `CREATE TABLE IF NOT EXISTS ` + "`upload_parts`" + ` (
  ` + "`id`" + ` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  ` + "`upload_session_id`" + ` BIGINT UNSIGNED NOT NULL,
  ` + "`kind`" + ` VARCHAR(24) NOT NULL,
  ` + "`status`" + ` VARCHAR(20) NOT NULL DEFAULT 'pending',
  ` + "`expected_size`" + ` BIGINT NOT NULL DEFAULT 0,
  ` + "`actual_size`" + ` BIGINT NOT NULL DEFAULT 0,
  ` + "`expected_hash`" + ` VARCHAR(64) NULL,
  ` + "`actual_hash`" + ` VARCHAR(64) NULL,
  ` + "`expected_width`" + ` BIGINT NOT NULL DEFAULT 0,
  ` + "`expected_height`" + ` BIGINT NOT NULL DEFAULT 0,
  ` + "`actual_width`" + ` BIGINT NOT NULL DEFAULT 0,
  ` + "`actual_height`" + ` BIGINT NOT NULL DEFAULT 0,
  ` + "`expected_mime_type`" + ` VARCHAR(50) NULL,
  ` + "`actual_mime_type`" + ` VARCHAR(50) NULL,
  ` + "`staging_path`" + ` VARCHAR(500) NOT NULL,
  ` + "`final_path`" + ` VARCHAR(500) NULL,
  ` + "`received_at`" + ` DATETIME(3) NULL,
  ` + "`finalized_at`" + ` DATETIME(3) NULL,
  ` + "`cleaned_at`" + ` DATETIME(3) NULL,
  ` + "`created_at`" + ` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  ` + "`updated_at`" + ` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (` + "`id`" + `),
  UNIQUE KEY ` + "`uk_upload_part_kind`" + ` (` + "`upload_session_id`" + `, ` + "`kind`" + `),
  KEY ` + "`idx_upload_parts_upload_session_id`" + ` (` + "`upload_session_id`" + `),
  KEY ` + "`idx_upload_parts_status`" + ` (` + "`status`" + `),
  KEY ` + "`idx_upload_parts_actual_hash`" + ` (` + "`actual_hash`" + `),
  CONSTRAINT ` + "`fk_upload_parts_upload_session`" + ` FOREIGN KEY (` + "`upload_session_id`" + `) REFERENCES ` + "`upload_sessions`" + ` (` + "`id`" + `) ON UPDATE CASCADE ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,
			},
			{
				Kind: schemaMigrationSQL,
				Name: "processing_jobs",
				Statement: `CREATE TABLE IF NOT EXISTS ` + "`processing_jobs`" + ` (
  ` + "`id`" + ` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  ` + "`job_type`" + ` VARCHAR(50) NOT NULL,
  ` + "`dedupe_key`" + ` VARCHAR(128) NOT NULL,
  ` + "`image_id`" + ` BIGINT UNSIGNED NULL,
  ` + "`upload_session_id`" + ` BIGINT UNSIGNED NULL,
  ` + "`status`" + ` VARCHAR(20) NOT NULL DEFAULT 'queued',
  ` + "`priority`" + ` BIGINT NOT NULL DEFAULT 0,
  ` + "`payload`" + ` JSON NOT NULL,
  ` + "`attempts`" + ` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  ` + "`max_attempts`" + ` BIGINT UNSIGNED NOT NULL DEFAULT 5,
  ` + "`available_at`" + ` DATETIME(3) NOT NULL,
  ` + "`lease_owner`" + ` VARCHAR(100) NULL,
  ` + "`lease_token`" + ` VARCHAR(64) NULL,
  ` + "`lease_expires_at`" + ` DATETIME(3) NULL,
  ` + "`started_at`" + ` DATETIME(3) NULL,
  ` + "`completed_at`" + ` DATETIME(3) NULL,
  ` + "`last_error`" + ` TEXT NULL,
  ` + "`created_at`" + ` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  ` + "`updated_at`" + ` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (` + "`id`" + `),
  UNIQUE KEY ` + "`uk_processing_job_dedupe`" + ` (` + "`job_type`" + `, ` + "`dedupe_key`" + `),
  KEY ` + "`idx_processing_jobs_job_type`" + ` (` + "`job_type`" + `),
  KEY ` + "`idx_processing_jobs_image_id`" + ` (` + "`image_id`" + `),
  KEY ` + "`idx_processing_jobs_upload_session_id`" + ` (` + "`upload_session_id`" + `),
  KEY ` + "`idx_processing_job_claim`" + ` (` + "`status`" + `, ` + "`available_at`" + `, ` + "`priority`" + `),
  KEY ` + "`idx_processing_jobs_lease_owner`" + ` (` + "`lease_owner`" + `),
  KEY ` + "`idx_processing_jobs_lease_token`" + ` (` + "`lease_token`" + `),
  KEY ` + "`idx_processing_jobs_lease_expires_at`" + ` (` + "`lease_expires_at`" + `),
  CONSTRAINT ` + "`fk_processing_jobs_image`" + ` FOREIGN KEY (` + "`image_id`" + `) REFERENCES ` + "`images`" + ` (` + "`id`" + `) ON UPDATE CASCADE ON DELETE SET NULL,
  CONSTRAINT ` + "`fk_processing_jobs_upload_session`" + ` FOREIGN KEY (` + "`upload_session_id`" + `) REFERENCES ` + "`upload_sessions`" + ` (` + "`id`" + `) ON UPDATE CASCADE ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,
			},
			{
				Kind: schemaMigrationSQL,
				Name: "outbox_events",
				Statement: `CREATE TABLE IF NOT EXISTS ` + "`outbox_events`" + ` (
  ` + "`id`" + ` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  ` + "`aggregate_type`" + ` VARCHAR(50) NOT NULL,
  ` + "`aggregate_id`" + ` VARCHAR(64) NOT NULL,
  ` + "`event_type`" + ` VARCHAR(100) NOT NULL,
  ` + "`dedupe_key`" + ` VARCHAR(128) NOT NULL,
  ` + "`status`" + ` VARCHAR(20) NOT NULL DEFAULT 'pending',
  ` + "`payload`" + ` JSON NOT NULL,
  ` + "`attempts`" + ` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  ` + "`max_attempts`" + ` BIGINT UNSIGNED NOT NULL DEFAULT 10,
  ` + "`available_at`" + ` DATETIME(3) NOT NULL,
  ` + "`lease_owner`" + ` VARCHAR(100) NULL,
  ` + "`lease_token`" + ` VARCHAR(64) NULL,
  ` + "`lease_expires_at`" + ` DATETIME(3) NULL,
  ` + "`published_at`" + ` DATETIME(3) NULL,
  ` + "`last_error`" + ` TEXT NULL,
  ` + "`created_at`" + ` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  ` + "`updated_at`" + ` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (` + "`id`" + `),
  UNIQUE KEY ` + "`uk_outbox_event_dedupe`" + ` (` + "`event_type`" + `, ` + "`dedupe_key`" + `),
  KEY ` + "`idx_outbox_events_aggregate_type`" + ` (` + "`aggregate_type`" + `),
  KEY ` + "`idx_outbox_events_aggregate_id`" + ` (` + "`aggregate_id`" + `),
  KEY ` + "`idx_outbox_events_event_type`" + ` (` + "`event_type`" + `),
  KEY ` + "`idx_outbox_event_claim`" + ` (` + "`status`" + `, ` + "`available_at`" + `),
  KEY ` + "`idx_outbox_events_lease_owner`" + ` (` + "`lease_owner`" + `),
  KEY ` + "`idx_outbox_events_lease_token`" + ` (` + "`lease_token`" + `),
  KEY ` + "`idx_outbox_events_lease_expires_at`" + ` (` + "`lease_expires_at`" + `),
  KEY ` + "`idx_outbox_events_published_at`" + ` (` + "`published_at`" + `)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,
			},
		},
	},
	{
		Version: 2026071601,
		Name:    "v2_capacity_lock",
		Operations: []schemaMigrationOperation{
			{
				Kind: schemaMigrationSQL,
				Name: "v2_capacity_locks",
				Statement: `CREATE TABLE IF NOT EXISTS ` + "`v2_capacity_locks`" + ` (
  ` + "`id`" + ` TINYINT UNSIGNED NOT NULL,
  ` + "`created_at`" + ` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (` + "`id`" + `)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,
			},
			{
				Kind:      schemaMigrationSQL,
				Name:      "v2_capacity_lock_seed",
				Statement: "INSERT IGNORE INTO `v2_capacity_locks` (`id`) VALUES (1)",
			},
		},
	},
	{
		Version: 2026071602,
		Name:    "v2_storage_gc_and_retention_indexes",
		Operations: []schemaMigrationOperation{
			{
				Kind:      schemaMigrationIndex,
				TableName: "image_variants",
				Name:      "idx_image_variants_storage_path",
				Statement: "CREATE INDEX `idx_image_variants_storage_path` ON `image_variants` (`storage_path`)",
			},
			{
				Kind:      schemaMigrationIndex,
				TableName: "image_files",
				Name:      "idx_image_files_original_path",
				Statement: "CREATE INDEX `idx_image_files_original_path` ON `image_files` (`original_path`)",
			},
			{
				Kind:      schemaMigrationIndex,
				TableName: "image_files",
				Name:      "idx_image_files_thumbnail_path",
				Statement: "CREATE INDEX `idx_image_files_thumbnail_path` ON `image_files` (`thumbnail_path`)",
			},
			{
				Kind:      schemaMigrationIndex,
				TableName: "image_files",
				Name:      "idx_image_files_processed_path",
				Statement: "CREATE INDEX `idx_image_files_processed_path` ON `image_files` (`processed_path`)",
			},
			{
				Kind:      schemaMigrationIndex,
				TableName: "upload_sessions",
				Name:      "idx_upload_sessions_user_status",
				Statement: "CREATE INDEX `idx_upload_sessions_user_status` ON `upload_sessions` (`user_id`, `status`, `expires_at`)",
			},
			{
				Kind:      schemaMigrationIndex,
				TableName: "upload_sessions",
				Name:      "idx_upload_sessions_retention",
				Statement: "CREATE INDEX `idx_upload_sessions_retention` ON `upload_sessions` (`status`, `updated_at`)",
			},
			{
				Kind:      schemaMigrationIndex,
				TableName: "processing_jobs",
				Name:      "idx_processing_jobs_retention",
				Statement: "CREATE INDEX `idx_processing_jobs_retention` ON `processing_jobs` (`status`, `updated_at`)",
			},
			{
				Kind:      schemaMigrationIndex,
				TableName: "outbox_events",
				Name:      "idx_outbox_events_retention",
				Statement: "CREATE INDEX `idx_outbox_events_retention` ON `outbox_events` (`status`, `published_at`)",
			},
		},
	},
	{
		Version: 2026071603,
		Name:    "image_file_remote_lineage",
		Operations: []schemaMigrationOperation{
			{
				Kind:      schemaMigrationColumn,
				TableName: "image_files",
				Name:      "remote_backend",
				Statement: "ALTER TABLE `image_files` ADD COLUMN `remote_backend` VARCHAR(16) NOT NULL DEFAULT '' AFTER `processed_path`",
			},
			{
				Kind:      schemaMigrationColumn,
				TableName: "image_files",
				Name:      "remote_endpoint",
				Statement: "ALTER TABLE `image_files` ADD COLUMN `remote_endpoint` VARCHAR(500) NOT NULL DEFAULT '' AFTER `remote_backend`",
			},
			{
				Kind:      schemaMigrationColumn,
				TableName: "image_files",
				Name:      "remote_bucket",
				Statement: "ALTER TABLE `image_files` ADD COLUMN `remote_bucket` VARCHAR(255) NOT NULL DEFAULT '' AFTER `remote_endpoint`",
			},
			{
				Kind:      schemaMigrationIndex,
				TableName: "image_files",
				Name:      "idx_image_files_remote_backend",
				Statement: "CREATE INDEX `idx_image_files_remote_backend` ON `image_files` (`remote_backend`)",
			},
		},
	},
}

const createSchemaMigrationsTableSQL = `CREATE TABLE IF NOT EXISTS ` + "`schema_migrations`" + ` (
  ` + "`version`" + ` BIGINT UNSIGNED NOT NULL,
  ` + "`name`" + ` VARCHAR(255) NOT NULL,
  ` + "`checksum`" + ` CHAR(64) NOT NULL,
  ` + "`applied_at`" + ` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (` + "`version`" + `)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`

// ApplySchemaMigrations applies all known, additive schema migrations. It uses
// a dedicated SQL connection because MySQL advisory locks are connection-scoped.
// Callers must treat a non-nil error as fatal and stop application startup.
func ApplySchemaMigrations(db *gorm.DB) error {
	if db == nil {
		return errors.New("schema migrations: nil database")
	}
	if err := validateSchemaMigrationPlan(schemaMigrations); err != nil {
		return fmt.Errorf("schema migrations: invalid plan: %w", err)
	}

	ctx := context.Background()
	return withSchemaMigrationLock(ctx, db, func(conn *sql.Conn, _ *gorm.DB) error {
		applied, err := inspectSchemaMigrationLedger(ctx, conn)
		if err != nil {
			return err
		}
		return applyPendingSchemaMigrations(ctx, conn, applied)
	})
}

func withSchemaMigrationLock(
	ctx context.Context,
	db *gorm.DB,
	operation func(*sql.Conn, *gorm.DB) error,
) error {
	if ctx == nil {
		return errors.New("schema migrations: nil context")
	}
	if db == nil {
		return errors.New("schema migrations: nil database")
	}
	if operation == nil {
		return errors.New("schema migrations: nil locked operation")
	}

	return db.WithContext(ctx).Connection(func(lockedDB *gorm.DB) (retErr error) {
		conn, ok := lockedDB.Statement.ConnPool.(*sql.Conn)
		if !ok {
			return fmt.Errorf("schema migrations: reserved connection has type %T", lockedDB.Statement.ConnPool)
		}
		databaseName, err := currentDatabaseName(ctx, conn)
		if err != nil {
			return err
		}
		lockName := schemaMigrationLockName(databaseName)
		if err := acquireSchemaMigrationLock(ctx, conn, lockName); err != nil {
			return err
		}
		defer func() {
			releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := releaseSchemaMigrationLock(releaseCtx, conn, lockName); err != nil {
				retErr = errors.Join(retErr, err)
			}
		}()
		return operation(conn, lockedDB)
	})
}

func inspectSchemaMigrationLedger(
	ctx context.Context,
	conn *sql.Conn,
) (map[uint64]appliedSchemaMigration, error) {
	if _, err := conn.ExecContext(ctx, createSchemaMigrationsTableSQL); err != nil {
		return nil, fmt.Errorf("schema migrations: create ledger: %w", err)
	}

	applied, err := loadAppliedSchemaMigrations(ctx, conn)
	if err != nil {
		return nil, err
	}
	if err := validateAppliedSchemaMigrations(schemaMigrations, applied); err != nil {
		return nil, fmt.Errorf("schema migrations: %w", err)
	}
	if err := validateAppliedSchemaObjects(ctx, conn, applied); err != nil {
		return nil, fmt.Errorf("schema migrations: %w", err)
	}
	return applied, nil
}

func validateAppliedSchemaObjects(
	ctx context.Context,
	conn *sql.Conn,
	applied map[uint64]appliedSchemaMigration,
) error {
	for _, migration := range schemaMigrations {
		if _, ok := applied[migration.Version]; !ok {
			continue
		}
		for _, operation := range migration.Operations {
			exists, err := appliedSchemaOperationExists(ctx, conn, operation)
			if err != nil {
				return fmt.Errorf("verify migration %d object %s: %w", migration.Version, operation.Name, err)
			}
			if !exists {
				return fmt.Errorf("migration %d is recorded but object %q is missing", migration.Version, operation.Name)
			}
		}
	}
	return nil
}

func appliedSchemaOperationExists(
	ctx context.Context,
	conn *sql.Conn,
	operation schemaMigrationOperation,
) (bool, error) {
	if operation.Kind != schemaMigrationSQL {
		return schemaMigrationOperationExists(ctx, conn, operation)
	}

	var count int64
	if operation.Name == "v2_capacity_lock_seed" {
		err := conn.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM `v2_capacity_locks` WHERE `id` = 1",
		).Scan(&count)
		return count == 1, err
	}
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*)
FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ?`, operation.Name).Scan(&count)
	return count == 1, err
}

func applyPendingSchemaMigrations(
	ctx context.Context,
	conn *sql.Conn,
	applied map[uint64]appliedSchemaMigration,
) error {
	for _, migration := range schemaMigrations {
		if _, ok := applied[migration.Version]; ok {
			continue
		}
		if err := applySchemaMigration(ctx, conn, migration); err != nil {
			return fmt.Errorf("schema migration %d (%s): %w", migration.Version, migration.Name, err)
		}
	}
	return nil
}

func currentDatabaseName(ctx context.Context, conn *sql.Conn) (string, error) {
	var name sql.NullString
	if err := conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&name); err != nil {
		return "", fmt.Errorf("schema migrations: resolve database name: %w", err)
	}
	if !name.Valid || strings.TrimSpace(name.String) == "" {
		return "", errors.New("schema migrations: no database selected")
	}
	return name.String, nil
}

func acquireSchemaMigrationLock(ctx context.Context, conn *sql.Conn, lockName string) error {
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", lockName, schemaMigrationLockTimeoutSeconds).Scan(&acquired); err != nil {
		return fmt.Errorf("schema migrations: acquire advisory lock: %w", err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return fmt.Errorf("schema migrations: advisory lock %q not acquired", lockName)
	}
	return nil
}

func releaseSchemaMigrationLock(ctx context.Context, conn *sql.Conn, lockName string) error {
	var released sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", lockName).Scan(&released); err != nil {
		return fmt.Errorf("schema migrations: release advisory lock: %w", err)
	}
	if !released.Valid || released.Int64 != 1 {
		return fmt.Errorf("schema migrations: advisory lock %q was not released", lockName)
	}
	return nil
}

func loadAppliedSchemaMigrations(ctx context.Context, conn *sql.Conn) (map[uint64]appliedSchemaMigration, error) {
	rows, err := conn.QueryContext(ctx, "SELECT version, name, checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("schema migrations: read ledger: %w", err)
	}
	defer rows.Close()

	applied := make(map[uint64]appliedSchemaMigration)
	for rows.Next() {
		var record appliedSchemaMigration
		if err := rows.Scan(&record.Version, &record.Name, &record.Checksum); err != nil {
			return nil, fmt.Errorf("schema migrations: scan ledger: %w", err)
		}
		applied[record.Version] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("schema migrations: iterate ledger: %w", err)
	}
	return applied, nil
}

func applySchemaMigration(ctx context.Context, conn *sql.Conn, migration schemaMigration) error {
	for _, operation := range migration.Operations {
		exists, err := schemaMigrationOperationExists(ctx, conn, operation)
		if err != nil {
			return fmt.Errorf("inspect %s %s: %w", operation.Kind, operation.Name, err)
		}
		if exists {
			continue
		}
		if _, err := conn.ExecContext(ctx, operation.Statement); err != nil {
			return fmt.Errorf("apply %s %s: %w", operation.Kind, operation.Name, err)
		}
	}

	checksum := schemaMigrationChecksum(migration)
	if _, err := conn.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, name, checksum) VALUES (?, ?, ?)",
		migration.Version, migration.Name, checksum,
	); err != nil {
		return fmt.Errorf("record ledger entry: %w", err)
	}
	return nil
}

func schemaMigrationOperationExists(ctx context.Context, conn *sql.Conn, operation schemaMigrationOperation) (bool, error) {
	var count int64
	switch operation.Kind {
	case schemaMigrationSQL:
		return false, nil
	case schemaMigrationColumn:
		err := conn.QueryRowContext(ctx, `SELECT COUNT(*)
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`, operation.TableName, operation.Name).Scan(&count)
		return count > 0, err
	case schemaMigrationIndex:
		err := conn.QueryRowContext(ctx, `SELECT COUNT(*)
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`, operation.TableName, operation.Name).Scan(&count)
		return count > 0, err
	default:
		return false, fmt.Errorf("unsupported operation kind %q", operation.Kind)
	}
}

func validateSchemaMigrationPlan(migrations []schemaMigration) error {
	if len(migrations) == 0 {
		return errors.New("migration plan is empty")
	}
	var previous uint64
	seenNames := make(map[string]struct{}, len(migrations))
	for i, migration := range migrations {
		if migration.Version == 0 {
			return fmt.Errorf("migration at index %d has zero version", i)
		}
		if i > 0 && migration.Version <= previous {
			return fmt.Errorf("migration version %d is not strictly increasing", migration.Version)
		}
		if strings.TrimSpace(migration.Name) == "" {
			return fmt.Errorf("migration %d has empty name", migration.Version)
		}
		if _, ok := seenNames[migration.Name]; ok {
			return fmt.Errorf("migration name %q is duplicated", migration.Name)
		}
		seenNames[migration.Name] = struct{}{}
		if len(migration.Operations) == 0 {
			return fmt.Errorf("migration %d has no operations", migration.Version)
		}
		for operationIndex, operation := range migration.Operations {
			if operation.Kind != schemaMigrationSQL && operation.Kind != schemaMigrationColumn && operation.Kind != schemaMigrationIndex {
				return fmt.Errorf("migration %d operation %d has invalid kind %q", migration.Version, operationIndex, operation.Kind)
			}
			if strings.TrimSpace(operation.Name) == "" || strings.TrimSpace(operation.Statement) == "" {
				return fmt.Errorf("migration %d operation %d is incomplete", migration.Version, operationIndex)
			}
			if operation.Kind != schemaMigrationSQL && strings.TrimSpace(operation.TableName) == "" {
				return fmt.Errorf("migration %d operation %d has empty table name", migration.Version, operationIndex)
			}
		}
		previous = migration.Version
	}
	return nil
}

func validateAppliedSchemaMigrations(migrations []schemaMigration, applied map[uint64]appliedSchemaMigration) error {
	known := make(map[uint64]schemaMigration, len(migrations))
	for _, migration := range migrations {
		known[migration.Version] = migration
	}

	versions := make([]uint64, 0, len(applied))
	for version := range applied {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })

	for _, version := range versions {
		record := applied[version]
		migration, ok := known[version]
		if !ok {
			return fmt.Errorf("database contains unknown migration version %d", version)
		}
		expectedChecksum := schemaMigrationChecksum(migration)
		if record.Name != migration.Name || !strings.EqualFold(record.Checksum, expectedChecksum) {
			return fmt.Errorf("migration %d checksum mismatch: database=%s binary=%s", version, record.Checksum, expectedChecksum)
		}
	}

	missingEarlierVersion := false
	for _, migration := range migrations {
		_, wasApplied := applied[migration.Version]
		if !wasApplied {
			missingEarlierVersion = true
			continue
		}
		if missingEarlierVersion {
			return fmt.Errorf("migration %d is applied after a missing earlier version", migration.Version)
		}
	}
	return nil
}

func schemaMigrationChecksum(migration schemaMigration) string {
	h := sha256.New()
	fmt.Fprintf(h, "version=%d\nname=%s\n", migration.Version, migration.Name)
	for _, operation := range migration.Operations {
		fmt.Fprintf(h, "kind=%s\ntable=%s\nname=%s\nstatement=%s\n", operation.Kind, operation.TableName, operation.Name, operation.Statement)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func schemaMigrationLockName(databaseName string) string {
	name := schemaMigrationLockPrefix + databaseName
	if len(name) <= 64 {
		return name
	}
	hash := sha256.Sum256([]byte(databaseName))
	return schemaMigrationLockPrefix + hex.EncodeToString(hash[:])[:32]
}
