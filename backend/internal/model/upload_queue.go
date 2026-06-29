// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"time"

	"gorm.io/gorm"
)

type UploadQueue struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID         uint64    `gorm:"index;not null" json:"user_id"`
	Status         string    `gorm:"size:20;index;default:processing;not null" json:"status"`
	TotalCount     int       `gorm:"default:0;not null" json:"total_count"`
	ProcessedCount int       `gorm:"default:0;not null" json:"processed_count"`
	FileInfo       string    `gorm:"type:json" json:"file_info"`
	ErrorMessage   string    `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	User *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (u *UploadQueue) BeforeCreate(tx *gorm.DB) error {
	if u.FileInfo == "" {
		u.FileInfo = "[]"
	}
	return nil
}
