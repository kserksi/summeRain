// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

type ImageFile struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	FileHash       string    `gorm:"size:64;uniqueIndex;not null" json:"file_hash"`
	FileSize       int64     `gorm:"not null" json:"file_size"`
	MimeType       string    `gorm:"size:50;not null" json:"mime_type"`
	Width          int       `gorm:"default:0" json:"width"`
	Height         int       `gorm:"default:0" json:"height"`
	ReferenceCount int       `gorm:"default:1;not null" json:"reference_count"`
	OriginalPath   string    `gorm:"size:500;not null" json:"original_path"`
	ThumbnailPath  string    `gorm:"size:500" json:"thumbnail_path"`
	ProcessedPath  string    `gorm:"size:500" json:"processed_path"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
}
