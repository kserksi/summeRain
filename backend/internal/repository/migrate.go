// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var legacyAccessTokenColumns = []string{
	"token_hash",
	"token_prefix",
	"token_suffix",
	"description",
}

// BootstrapDatabase owns every startup schema mutation under one
// connection-scoped MySQL advisory lock. MySQL DDL commits implicitly, so each
// stage must remain idempotent and return errors without pretending to roll
// back an operation that the server has already committed.
func BootstrapDatabase(ctx context.Context, db *gorm.DB) error {
	if ctx == nil {
		return errors.New("database bootstrap: nil context")
	}
	if db == nil {
		return errors.New("database bootstrap: nil database")
	}
	if err := validateSchemaMigrationPlan(schemaMigrations); err != nil {
		return fmt.Errorf("database bootstrap: invalid migration plan: %w", err)
	}

	if err := withSchemaMigrationLock(ctx, db, func(conn *sql.Conn, bootstrapDB *gorm.DB) error {
		applied, err := inspectSchemaMigrationLedger(ctx, conn)
		if err != nil {
			return fmt.Errorf("validate migration ledger: %w", err)
		}
		if err := migrateLegacyTokens(ctx, conn); err != nil {
			return fmt.Errorf("migrate legacy access tokens: %w", err)
		}

		baselineDB := bootstrapDB.Session(&gorm.Session{NewDB: true, Context: ctx})
		if err := baselineDB.AutoMigrate(databaseBootstrapModels()...); err != nil {
			return fmt.Errorf("auto-migrate baseline schema: %w", err)
		}
		if err := applyPendingSchemaMigrations(ctx, conn, applied); err != nil {
			return fmt.Errorf("apply versioned migrations: %w", err)
		}
		seedDB := bootstrapDB.Session(&gorm.Session{NewDB: true, Context: ctx})
		if err := seedDefaultConfigs(seedDB); err != nil {
			return fmt.Errorf("seed default configuration: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("database bootstrap: %w", err)
	}
	return nil
}

func migrateLegacyTokens(ctx context.Context, conn *sql.Conn) error {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(legacyAccessTokenColumns)), ",")
	arguments := make([]any, len(legacyAccessTokenColumns))
	for index, column := range legacyAccessTokenColumns {
		arguments[index] = column
	}
	rows, err := conn.QueryContext(ctx, `SELECT column_name
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'image_access_tokens'
  AND column_name IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return fmt.Errorf("inspect legacy columns: %w", err)
	}

	present := make(map[string]bool, len(legacyAccessTokenColumns))
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			rows.Close()
			return fmt.Errorf("scan legacy columns: %w", err)
		}
		present[column] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate legacy columns: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close legacy column query: %w", err)
	}
	if present["token_hash"] {
		if _, err := conn.ExecContext(ctx, "DELETE FROM `image_access_tokens`"); err != nil {
			return fmt.Errorf("delete incompatible token rows: %w", err)
		}
	}
	drops := make([]string, 0, len(legacyAccessTokenColumns))
	for _, column := range legacyAccessTokenColumns {
		if present[column] {
			drops = append(drops, "DROP COLUMN `"+column+"`")
		}
	}
	if len(drops) == 0 {
		return nil
	}
	if _, err := conn.ExecContext(ctx,
		"ALTER TABLE `image_access_tokens` "+strings.Join(drops, ", "),
	); err != nil {
		return fmt.Errorf("drop legacy columns: %w", err)
	}
	return nil
}

func seedDefaultConfigs(db *gorm.DB) error {
	const (
		privateTokenTTLKey       = "private_token_ttl_default_ms"
		legacyPrivateTokenTTLKey = "image_token_default_ttl"
	)

	var legacyTTL model.SystemConfig
	if err := db.Where("config_key = ?", legacyPrivateTokenTTLKey).First(&legacyTTL).Error; err == nil {
		legacyTTL.ID = 0
		legacyTTL.ConfigKey = privateTokenTTLKey
		legacyTTL.Description = "Default private-image access token TTL (ms)"
		legacyTTL.UpdatedAt = time.Time{}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&legacyTTL).Error; err != nil {
			return fmt.Errorf("copy legacy %q: %w", legacyPrivateTokenTTLKey, err)
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("query legacy %q: %w", legacyPrivateTokenTTLKey, err)
	}

	defaults := []model.SystemConfig{
		{ConfigKey: privateTokenTTLKey, ConfigValue: "3600000", ConfigType: "int", Description: "Default private-image access token TTL (ms)"},
		{ConfigKey: "site_language", ConfigValue: "en-US", ConfigType: "string", Description: "Default site language"},
	}
	for index := range defaults {
		d := &defaults[index]
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(d).Error; err != nil {
			return fmt.Errorf("create %q: %w", d.ConfigKey, err)
		}
	}
	return nil
}

func databaseBootstrapModels() []any {
	return []any{
		&model.User{},
		&model.Session{},
		&model.CSRFToken{},
		&legacyImageFileSchema{},
		&legacyImageSchema{},
		&model.ImageAccessToken{},
		&model.Notification{},
		&model.SystemConfig{},
		&model.UploadQueue{},
		&model.AuditLog{},
	}
}

// legacyImageSchema bootstraps a fresh V1 database without letting GORM add V2
// columns outside the checksummed migration runner.
type legacyImageSchema struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	UserID      uint64    `gorm:"index;not null"`
	ImageFileID uint64    `gorm:"index;not null"`
	UniqueLink  string    `gorm:"size:32;uniqueIndex;not null"`
	Title       string    `gorm:"size:200"`
	Filename    string    `gorm:"size:255"`
	Description string    `gorm:"size:500"`
	Visibility  string    `gorm:"size:10;default:public;not null"`
	ViewCount   uint64    `gorm:"default:0;not null"`
	Width       int       `gorm:"default:0"`
	Height      int       `gorm:"default:0"`
	FileSize    int64     `gorm:"not null"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`

	User      *model.User            `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	ImageFile *legacyImageFileSchema `gorm:"foreignKey:ImageFileID"`
}

func (legacyImageSchema) TableName() string { return "images" }

// legacyImageFileSchema keeps object-storage lineage under the checksummed
// migration runner instead of letting AutoMigrate add it implicitly.
type legacyImageFileSchema struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	FileHash       string    `gorm:"size:64;uniqueIndex;not null"`
	FileSize       int64     `gorm:"not null"`
	MimeType       string    `gorm:"size:50;not null"`
	Width          int       `gorm:"default:0"`
	Height         int       `gorm:"default:0"`
	ReferenceCount int       `gorm:"default:1;not null"`
	OriginalPath   string    `gorm:"size:500;not null"`
	ThumbnailPath  string    `gorm:"size:500"`
	ProcessedPath  string    `gorm:"size:500"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

func (legacyImageFileSchema) TableName() string { return "image_files" }
