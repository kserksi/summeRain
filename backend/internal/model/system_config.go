// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

type SystemConfig struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ConfigKey   string    `gorm:"size:100;uniqueIndex;not null" json:"config_key"`
	ConfigValue string    `gorm:"type:text;not null" json:"config_value"`
	ConfigType  string    `gorm:"size:20;not null" json:"config_type"`
	Description string    `gorm:"type:text" json:"description"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
