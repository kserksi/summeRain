// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"github.com/summerain/image-gallery/internal/model"
	"gorm.io/gorm"
)

// MigrateLegacyTokens clears obsolete hash-based access tokens (incompatible
// with the unified plaintext-token model: the old rows stored only a SHA256
// hash and cannot be matched by plaintext) and drops their columns. This is a
// one-time migration; a no-op after the first run (the token_hash column no
// longer exists). Owners re-issue tokens under the new model.
//
// Must run BEFORE AutoMigrate so the new unique-indexed Token column can be
// added to a clean (empty) table. Uses raw information_schema / ALTER instead
// of GORM's migrator to avoid nil-deref on raw table names and on columns no
// longer present in the model.
func MigrateLegacyTokens(db *gorm.DB) {
	var hasLegacy int64
	db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'image_access_tokens' AND column_name = 'token_hash'").Scan(&hasLegacy)
	if hasLegacy == 0 {
		return
	}
	db.Exec("DELETE FROM image_access_tokens")
	for _, col := range []string{"token_hash", "token_prefix", "token_suffix", "description"} {
		db.Exec("ALTER TABLE image_access_tokens DROP COLUMN " + col)
	}
}

// SeedDefaultConfigs seeds default system_config rows idempotently.
func SeedDefaultConfigs(db *gorm.DB) {
	defaults := []model.SystemConfig{
		{ConfigKey: "private_token_ttl_default_ms", ConfigValue: "3600000", ConfigType: "int", Description: "Default private-image access token TTL (ms)"},
	}
	for _, d := range defaults {
		var existing model.SystemConfig
		err := db.Where("config_key = ?", d.ConfigKey).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			db.Create(&d)
		}
	}
}
