// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

const (
	UserStatusActive          = "active"
	UserStatusSuspended       = "suspended"
	UserStatusPendingDeletion = "pending_deletion"
	UserStatusDeleting        = "deleting"
)

// UserStatusAllowsAuthentication keeps internal/destructive states fail-closed.
func UserStatusAllowsAuthentication(status string) bool {
	return status == UserStatusActive || status == UserStatusPendingDeletion
}

type User struct {
	ID                  uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Username            string     `gorm:"size:50;uniqueIndex;not null" json:"username"`
	Email               string     `gorm:"size:100;uniqueIndex;not null" json:"email"`
	PasswordHash        string     `gorm:"size:255;not null" json:"-"`
	Role                string     `gorm:"size:10;default:user;not null" json:"role"`
	Status              string     `gorm:"size:20;default:active;not null" json:"status"`
	AvatarURL           *string    `gorm:"type:mediumtext" json:"avatar_url"`
	StorageUsed         int64      `gorm:"default:0;not null" json:"storage_used"`
	StorageQuota        int64      `gorm:"default:524288000;not null" json:"storage_quota"`
	ImageCount          int        `gorm:"default:0;not null" json:"image_count"`
	DeletionScheduledAt *time.Time `gorm:"index" json:"deletion_scheduled_at,omitempty"`
	DeletedByAdmin      string     `gorm:"size:50" json:"deleted_by_admin,omitempty"`
	BatchDownloadCount  int        `gorm:"default:0;not null" json:"batch_download_count"`
	CreatedAt           time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}
