// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

const (
	ImagePipelineVersionLegacy uint16 = 1
	ImagePipelineVersionV2     uint16 = 2
)

const (
	ImageProcessingStatusPending    = "pending"
	ImageProcessingStatusProcessing = "processing"
	ImageProcessingStatusCompleted  = "completed"
	ImageProcessingStatusFailed     = "failed"
)

const (
	ImageVariantKindMaster        = "master"
	ImageVariantKindGallery       = "gallery"
	ImageVariantKindAdmin         = "admin"
	ImageVariantKindPublishSource = "publish_source"
	ImageVariantKindPublish       = "publish"
)

const (
	ImageVariantStatusPending    = "pending"
	ImageVariantStatusReady      = "ready"
	ImageVariantStatusFailed     = "failed"
	ImageVariantStatusSuperseded = "superseded"
)

// ImageVariant records immutable V2 outputs. publish_source is a temporary
// upload part and must not remain in this table after the publish job finishes.
type ImageVariant struct {
	ID               uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ImageID          uint64     `gorm:"not null;index;uniqueIndex:uk_image_variant_revision,priority:1" json:"image_id"`
	Kind             string     `gorm:"size:24;not null;index;uniqueIndex:uk_image_variant_revision,priority:2" json:"kind"`
	Revision         uint32     `gorm:"default:1;not null;uniqueIndex:uk_image_variant_revision,priority:3" json:"revision"`
	PipelineVersion  uint16     `gorm:"default:2;not null" json:"pipeline_version"`
	Status           string     `gorm:"size:20;default:pending;not null;index" json:"status"`
	FileHash         string     `gorm:"size:64;not null;index" json:"file_hash"`
	FileSize         int64      `gorm:"not null" json:"file_size"`
	MimeType         string     `gorm:"size:50;not null" json:"mime_type"`
	Width            int        `gorm:"not null" json:"width"`
	Height           int        `gorm:"not null" json:"height"`
	Quality          uint8      `gorm:"default:0;not null" json:"quality"`
	StoragePath      string     `gorm:"size:500;not null;index" json:"storage_path"`
	WatermarkVersion *string    `gorm:"size:64;index" json:"watermark_version,omitempty"`
	IsActive         bool       `gorm:"default:false;not null;index" json:"is_active"`
	ReadyAt          *time.Time `gorm:"index" json:"ready_at,omitempty"`
	CreatedAt        time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	Image *Image `gorm:"foreignKey:ImageID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
}
