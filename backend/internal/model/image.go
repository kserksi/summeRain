// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

type Image struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID           uint64    `gorm:"index;not null" json:"user_id"`
	ImageFileID      uint64    `gorm:"index;not null" json:"image_file_id"`
	UniqueLink       string    `gorm:"size:32;uniqueIndex;not null" json:"unique_link"`
	Title            string    `gorm:"size:200" json:"title"`
	Filename         string    `gorm:"size:255" json:"filename"`
	Description      string    `gorm:"size:500" json:"description,omitempty"`
	Visibility       string    `gorm:"size:10;default:public;not null" json:"visibility"`
	PipelineVersion  uint16    `gorm:"default:1;not null" json:"pipeline_version"`
	ProcessingStatus string    `gorm:"size:24;default:completed;not null;index" json:"processing_status"`
	OriginAlias      *string   `gorm:"column:origin_alias;size:255;index" json:"asset_link,omitempty"`
	ViewCount        uint64    `gorm:"default:0;not null" json:"view_count"`
	Width            int       `gorm:"default:0" json:"width"`
	Height           int       `gorm:"default:0" json:"height"`
	FileSize         int64     `gorm:"not null" json:"file_size"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	User      *User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	ImageFile *ImageFile `gorm:"foreignKey:ImageFileID" json:"-"`
}
