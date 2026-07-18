// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	ProcessingJobStatusQueued    = "queued"
	ProcessingJobStatusRunning   = "running"
	ProcessingJobStatusRetry     = "retry"
	ProcessingJobStatusCompleted = "completed"
	ProcessingJobStatusDead      = "dead"
)

type ProcessingJob struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	JobType         string     `gorm:"size:50;not null;index;uniqueIndex:uk_processing_job_dedupe,priority:1" json:"job_type"`
	DedupeKey       string     `gorm:"size:128;not null;uniqueIndex:uk_processing_job_dedupe,priority:2" json:"dedupe_key"`
	ImageID         *uint64    `gorm:"index" json:"image_id,omitempty"`
	UploadSessionID *uint64    `gorm:"index" json:"upload_session_id,omitempty"`
	Status          string     `gorm:"size:20;default:queued;not null;index:idx_processing_job_claim,priority:1" json:"status"`
	Priority        int        `gorm:"default:0;not null;index:idx_processing_job_claim,priority:3" json:"priority"`
	Payload         string     `gorm:"type:json;not null" json:"payload"`
	Attempts        uint       `gorm:"default:0;not null" json:"attempts"`
	MaxAttempts     uint       `gorm:"default:5;not null" json:"max_attempts"`
	AvailableAt     time.Time  `gorm:"not null;index:idx_processing_job_claim,priority:2" json:"available_at"`
	LeaseOwner      *string    `gorm:"size:100;index" json:"lease_owner,omitempty"`
	LeaseToken      *string    `gorm:"size:64;index" json:"lease_token,omitempty"`
	LeaseExpiresAt  *time.Time `gorm:"index" json:"lease_expires_at,omitempty"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	LastError       string     `gorm:"type:text" json:"last_error,omitempty"`
	CreatedAt       time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	Image         *Image         `gorm:"foreignKey:ImageID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"-"`
	UploadSession *UploadSession `gorm:"foreignKey:UploadSessionID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"-"`
}

func (j *ProcessingJob) BeforeCreate(tx *gorm.DB) error {
	if j.Payload == "" {
		j.Payload = "{}"
	}
	if j.AvailableAt.IsZero() {
		j.AvailableAt = time.Now()
	}
	return nil
}
