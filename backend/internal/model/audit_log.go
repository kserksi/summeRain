// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"time"

	"gorm.io/gorm"
)

type AuditLog struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID       uint64    `gorm:"index;not null" json:"user_id"`
	Action       string    `gorm:"size:100;index;not null" json:"action"`
	ResourceType string    `gorm:"size:50" json:"resource_type,omitempty"`
	ResourceID   uint64    `gorm:"default:0" json:"resource_id,omitempty"`
	IPAddress    string    `gorm:"size:45" json:"ip_address"`
	Metadata     string    `gorm:"type:json" json:"metadata,omitempty"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`

	User *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (a *AuditLog) BeforeCreate(tx *gorm.DB) error {
	if a.Metadata == "" {
		a.Metadata = "{}"
	}
	return nil
}
