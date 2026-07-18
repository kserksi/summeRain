// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	OutboxEventStatusPending    = "pending"
	OutboxEventStatusPublishing = "publishing"
	OutboxEventStatusPublished  = "published"
	OutboxEventStatusDead       = "dead"
)

const OutboxEventTypeStorageDelete = "storage.file.delete"

type StorageDeletePayload struct {
	Paths         []string                    `json:"paths"`
	RemoteObjects []StorageDeleteRemoteObject `json:"remote_objects,omitempty"`
}

type StorageDeleteRemoteObject struct {
	Path     string `json:"path"`
	Backend  string `json:"backend"`
	Endpoint string `json:"endpoint"`
	Bucket   string `json:"bucket"`
}

type OutboxEvent struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	AggregateType  string     `gorm:"size:50;not null;index" json:"aggregate_type"`
	AggregateID    string     `gorm:"size:64;not null;index" json:"aggregate_id"`
	EventType      string     `gorm:"size:100;not null;index;uniqueIndex:uk_outbox_event_dedupe,priority:1" json:"event_type"`
	DedupeKey      string     `gorm:"size:128;not null;uniqueIndex:uk_outbox_event_dedupe,priority:2" json:"dedupe_key"`
	Status         string     `gorm:"size:20;default:pending;not null;index:idx_outbox_event_claim,priority:1" json:"status"`
	Payload        string     `gorm:"type:json;not null" json:"payload"`
	Attempts       uint       `gorm:"default:0;not null" json:"attempts"`
	MaxAttempts    uint       `gorm:"default:10;not null" json:"max_attempts"`
	AvailableAt    time.Time  `gorm:"not null;index:idx_outbox_event_claim,priority:2" json:"available_at"`
	LeaseOwner     *string    `gorm:"size:100;index" json:"lease_owner,omitempty"`
	LeaseToken     *string    `gorm:"size:64;index" json:"lease_token,omitempty"`
	LeaseExpiresAt *time.Time `gorm:"index" json:"lease_expires_at,omitempty"`
	PublishedAt    *time.Time `gorm:"index" json:"published_at,omitempty"`
	LastError      string     `gorm:"type:text" json:"last_error,omitempty"`
	CreatedAt      time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (e *OutboxEvent) BeforeCreate(tx *gorm.DB) error {
	if e.Payload == "" {
		e.Payload = "{}"
	}
	if e.AvailableAt.IsZero() {
		e.AvailableAt = time.Now()
	}
	return nil
}
