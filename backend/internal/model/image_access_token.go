// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

// ImageAccessToken is the single unified share token for a private image.
// At most one active (RevokedAt IS NULL AND ExpiresAt > now) token exists per
// image, enforced at the service layer. The Token string is stored in plaintext
// so the owner can retrieve it for sharing; it is immutable until revoked +
// re-issued.
type ImageAccessToken struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ImageID    uint64     `gorm:"index;not null" json:"image_id"`
	UserID     uint64     `gorm:"index;not null" json:"user_id"`
	Token      string     `gorm:"size:64;uniqueIndex;not null" json:"-"`
	ExpiresAt  time.Time  `gorm:"not null" json:"expires_at"`
	RevokedAt  *time.Time `gorm:"index" json:"revoked_at,omitempty"`
	UsageCount uint       `gorm:"default:0;not null" json:"usage_count"`
	CreatedAt  time.Time  `gorm:"autoCreateTime" json:"created_at"`

	Image *Image `gorm:"foreignKey:ImageID;constraint:OnDelete:CASCADE" json:"-"`
	User  *User  `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}
