// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	storageDeleteBatchSize   = 100
	storageDeleteR2BatchSize = 5
	storageDeleteMaxAttempts = 100
)

// EnqueueStorageDelete records filesystem deletion intent in the same MySQL
// transaction that removes the owning rows. The outbox worker rechecks all
// references before deleting, so retries are safe for deduplicated files.
func EnqueueStorageDelete(tx *gorm.DB, aggregateType, aggregateID, dedupeBase string, paths []string, now time.Time) error {
	return EnqueueStorageDeleteWithRemote(tx, aggregateType, aggregateID, dedupeBase, paths, nil, now)
}

// EnqueueStorageDeleteWithRemote keeps local and object-store deletion intents
// durable. Remote paths must also be local targets so they share a reference
// check, while endpoint and bucket preserve the object's storage lineage.
func EnqueueStorageDeleteWithRemote(tx *gorm.DB, aggregateType, aggregateID, dedupeBase string, paths []string, remoteObjects []model.StorageDeleteRemoteObject, now time.Time) error {
	if tx == nil {
		return fmt.Errorf("enqueue storage delete: nil database")
	}
	seen := make(map[string]struct{}, len(paths))
	cleaned := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		cleaned = append(cleaned, path)
	}
	remoteByPath := make(map[string]model.StorageDeleteRemoteObject, len(remoteObjects))
	for _, object := range remoteObjects {
		object.Path = strings.TrimSpace(object.Path)
		object.Backend = strings.TrimSpace(object.Backend)
		object.Endpoint = strings.TrimRight(strings.TrimSpace(object.Endpoint), "/")
		object.Bucket = strings.TrimSpace(object.Bucket)
		if object.Path == "" {
			continue
		}
		if _, exists := seen[object.Path]; !exists {
			return fmt.Errorf("enqueue storage delete: remote path %q has no local deletion target", object.Path)
		}
		if object.Backend != "r2" || object.Endpoint == "" || object.Bucket == "" {
			return fmt.Errorf("enqueue storage delete: remote path %q has an invalid target", object.Path)
		}
		if existing, duplicate := remoteByPath[object.Path]; duplicate && existing != object {
			return fmt.Errorf("enqueue storage delete: remote path %q has conflicting targets", object.Path)
		}
		remoteByPath[object.Path] = object
	}
	type remoteTarget struct {
		backend  string
		endpoint string
		bucket   string
	}
	localOnly := make([]string, 0, len(cleaned))
	remoteGroups := make(map[remoteTarget][]model.StorageDeleteRemoteObject)
	remoteOrder := make([]remoteTarget, 0)
	for _, path := range cleaned {
		object, remote := remoteByPath[path]
		if !remote {
			localOnly = append(localOnly, path)
			continue
		}
		target := remoteTarget{backend: object.Backend, endpoint: object.Endpoint, bucket: object.Bucket}
		if _, exists := remoteGroups[target]; !exists {
			remoteOrder = append(remoteOrder, target)
		}
		remoteGroups[target] = append(remoteGroups[target], object)
	}

	payloads := make([]model.StorageDeletePayload, 0, 1+len(remoteOrder))
	for offset := 0; offset < len(localOnly); offset += storageDeleteBatchSize {
		end := minInt(offset+storageDeleteBatchSize, len(localOnly))
		payloads = append(payloads, model.StorageDeletePayload{Paths: localOnly[offset:end]})
	}
	for _, target := range remoteOrder {
		objects := remoteGroups[target]
		for offset := 0; offset < len(objects); offset += storageDeleteR2BatchSize {
			end := minInt(offset+storageDeleteR2BatchSize, len(objects))
			batchObjects := objects[offset:end]
			batchPaths := make([]string, 0, len(batchObjects))
			for _, object := range batchObjects {
				batchPaths = append(batchPaths, object.Path)
			}
			payloads = append(payloads, model.StorageDeletePayload{Paths: batchPaths, RemoteObjects: batchObjects})
		}
	}

	for batchIndex, deletePayload := range payloads {
		payload, err := json.Marshal(deletePayload)
		if err != nil {
			return err
		}
		dedupeKey := fmt.Sprintf("%s:%d", dedupeBase, batchIndex)
		if len(dedupeKey) > 128 {
			dedupeKey = dedupeKey[:128]
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.OutboxEvent{
			AggregateType: aggregateType,
			AggregateID:   aggregateID,
			EventType:     model.OutboxEventTypeStorageDelete,
			DedupeKey:     dedupeKey,
			Status:        model.OutboxEventStatusPending,
			Payload:       string(payload),
			MaxAttempts:   storageDeleteMaxAttempts,
			AvailableAt:   now,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
