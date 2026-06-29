// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

type CSRFToken struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID uint64    `gorm:"index;not null" json:"session_id"`
	TokenHash string    `gorm:"size:64;uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`

	Session *Session `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
}
