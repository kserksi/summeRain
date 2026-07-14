// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

type Session struct {
	ID                    uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID                uint64     `gorm:"index;not null" json:"user_id"`
	TokenHash             string     `gorm:"size:64;uniqueIndex;not null" json:"-"`
	TokenType             string     `gorm:"size:10;index;not null" json:"token_type"`
	IdentityTokenID       *uint64    `gorm:"index" json:"identity_token_id,omitempty"`
	NonceHash             *string    `gorm:"size:64" json:"-"`
	LastHeartbeatAt       *time.Time `gorm:"index" json:"last_heartbeat_at,omitempty"`
	HeartbeatGraceSeconds uint       `gorm:"default:600;not null" json:"heartbeat_grace_seconds"`
	DeviceName            string     `gorm:"size:100" json:"device_name"`
	Platform              string     `gorm:"size:10;index;default:web;not null" json:"platform"`
	DeviceID              string     `gorm:"size:100;index" json:"device_id,omitempty"`
	IPAddress             string     `gorm:"size:45" json:"ip_address"`
	UserAgent             string     `gorm:"type:text" json:"user_agent,omitempty"`
	LastActiveAt          time.Time  `gorm:"index" json:"last_active_at"`
	ExpiresAt             time.Time  `gorm:"index;not null" json:"expires_at"`
	CreatedAt             time.Time  `gorm:"autoCreateTime" json:"created_at"`

	User          *User    `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	IdentityToken *Session `gorm:"foreignKey:IdentityTokenID" json:"-"`
}
