// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

const (
	UploadSessionStatusInitiated      = "initiated"
	UploadSessionStatusUploading      = "uploading"
	UploadSessionStatusProcessing     = "processing"
	UploadSessionStatusCompleted      = "completed"
	UploadSessionStatusFailed         = "failed"
	UploadSessionStatusCancelled      = "cancelled"
	UploadSessionStatusCleanupPending = "cleanup_pending"
)

const (
	UploadPartStatusPending   = "pending"
	UploadPartStatusReceived  = "received"
	UploadPartStatusFinalized = "finalized"
	UploadPartStatusCleaned   = "cleaned"
)

type UploadSession struct {
	ID                uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UploadKey         string     `gorm:"size:64;uniqueIndex;not null" json:"upload_id"`
	UserID            uint64     `gorm:"not null;index;uniqueIndex:uk_user_upload_idempotency,priority:1" json:"user_id"`
	IdempotencyKey    *string    `gorm:"size:64;uniqueIndex:uk_user_upload_idempotency,priority:2" json:"idempotency_key,omitempty"`
	Status            string     `gorm:"size:24;default:initiated;not null;index:idx_upload_session_expiry,priority:1" json:"status"`
	PipelineVersion   uint16     `gorm:"default:2;not null" json:"pipeline_version"`
	Visibility        string     `gorm:"size:10;default:public;not null" json:"visibility"`
	Filename          string     `gorm:"size:255;not null" json:"filename"`
	SourceMimeType    string     `gorm:"size:50;not null" json:"source_mime_type"`
	SourceWidth       int        `gorm:"default:0;not null" json:"source_width"`
	SourceHeight      int        `gorm:"default:0;not null" json:"source_height"`
	ProcessorVersion  string     `gorm:"size:64;not null" json:"processor_version"`
	RecipeVersion     string     `gorm:"size:32;not null" json:"recipe_version"`
	ImageID           *uint64    `gorm:"index" json:"image_id,omitempty"`
	ExpectedPartCount uint8      `gorm:"default:0;not null" json:"expected_part_count"`
	ReceivedPartCount uint8      `gorm:"default:0;not null" json:"received_part_count"`
	ReservedBytes     int64      `gorm:"default:0;not null" json:"reserved_bytes"`
	ActualBytes       int64      `gorm:"default:0;not null" json:"actual_bytes"`
	StagingPath       string     `gorm:"size:500;not null" json:"-"`
	ManifestHash      *string    `gorm:"size:64" json:"manifest_hash,omitempty"`
	ClientManifest    *string    `gorm:"type:json" json:"client_manifest,omitempty"`
	FocalX            *float64   `gorm:"type:decimal(8,7)" json:"focal_x,omitempty"`
	FocalY            *float64   `gorm:"type:decimal(8,7)" json:"focal_y,omitempty"`
	ErrorCode         *int       `json:"error_code,omitempty"`
	ErrorMessage      string     `gorm:"type:text" json:"error_message,omitempty"`
	ExpiresAt         time.Time  `gorm:"not null;index:idx_upload_session_expiry,priority:2" json:"expires_at"`
	ProcessingAt      *time.Time `json:"processing_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	FailedAt          *time.Time `json:"failed_at,omitempty"`
	CancelledAt       *time.Time `json:"cancelled_at,omitempty"`
	CleanupAfter      *time.Time `gorm:"index" json:"cleanup_after,omitempty"`
	CreatedAt         time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	User  *User        `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	Image *Image       `gorm:"foreignKey:ImageID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"-"`
	Parts []UploadPart `gorm:"foreignKey:UploadSessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"parts,omitempty"`
}

type UploadPart struct {
	ID               uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UploadSessionID  uint64     `gorm:"not null;index;uniqueIndex:uk_upload_part_kind,priority:1" json:"upload_session_id"`
	Kind             string     `gorm:"size:24;not null;uniqueIndex:uk_upload_part_kind,priority:2" json:"kind"`
	Status           string     `gorm:"size:20;default:pending;not null;index" json:"status"`
	ExpectedSize     int64      `gorm:"default:0;not null" json:"expected_size"`
	ActualSize       int64      `gorm:"default:0;not null" json:"actual_size"`
	ExpectedHash     string     `gorm:"size:64" json:"expected_hash,omitempty"`
	ActualHash       string     `gorm:"size:64;index" json:"actual_hash,omitempty"`
	ExpectedWidth    int        `gorm:"default:0;not null" json:"expected_width"`
	ExpectedHeight   int        `gorm:"default:0;not null" json:"expected_height"`
	ActualWidth      int        `gorm:"default:0;not null" json:"actual_width"`
	ActualHeight     int        `gorm:"default:0;not null" json:"actual_height"`
	ExpectedMimeType string     `gorm:"size:50" json:"expected_mime_type,omitempty"`
	ActualMimeType   string     `gorm:"size:50" json:"actual_mime_type,omitempty"`
	StagingPath      string     `gorm:"size:500;not null" json:"-"`
	FinalPath        *string    `gorm:"size:500" json:"final_path,omitempty"`
	ReceivedAt       *time.Time `json:"received_at,omitempty"`
	FinalizedAt      *time.Time `json:"finalized_at,omitempty"`
	CleanedAt        *time.Time `json:"cleaned_at,omitempty"`
	CreatedAt        time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	UploadSession *UploadSession `gorm:"foreignKey:UploadSessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
}
