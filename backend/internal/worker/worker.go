// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"log"
	"sync"

	"github.com/go-redis/redis/v8"
	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/service"
	"gorm.io/gorm"
)

type Manager struct {
	DB     *gorm.DB
	Redis  *redis.Client
	Config *config.Config
	V2     *service.V2UploadService
	R2     r2WorkerService
}

type r2WorkerService interface {
	remoteObjectDeleter
	CurrentTarget() (endpoint, bucket string, ok bool)
}

func NewManager(db *gorm.DB, rdb *redis.Client, cfg *config.Config, v2 *service.V2UploadService, r2 *service.R2Service) *Manager {
	return &Manager{
		DB:     db,
		Redis:  rdb,
		Config: cfg,
		V2:     v2,
		R2:     r2,
	}
}

func (m *Manager) Start(ctx context.Context) {
	var wg sync.WaitGroup

	v2Workers := 0
	if m.V2 != nil && m.Config != nil && m.Config.ImageV2.Enabled {
		v2Workers = m.Config.ImageV2.WatermarkConcurrency
	}
	v2CleanupWorkers := 0
	if m.Config != nil && m.Config.Storage.StagingPath != "" {
		v2CleanupWorkers = 1
	}
	wg.Add(5 + v2Workers + v2CleanupWorkers)

	go func() {
		defer wg.Done()
		m.runHeartbeatMonitor(ctx)
	}()

	go func() {
		defer wg.Done()
		m.runOutbox(ctx)
	}()

	for index := 0; index < v2Workers; index++ {
		index := index
		go func() {
			defer wg.Done()
			m.runV2Publish(ctx, index)
		}()
	}

	if v2CleanupWorkers == 1 {
		go func() {
			defer wg.Done()
			m.runV2Cleanup(ctx)
		}()
	}

	go func() {
		defer wg.Done()
		m.runViewFlusher(ctx)
	}()

	go func() {
		defer wg.Done()
		m.runCleanup(ctx)
	}()

	go func() {
		defer wg.Done()
		m.runUserDeletion(ctx)
	}()

	log.Printf("[worker] all workers started")

	wg.Wait()
	log.Printf("[worker] all workers stopped")
}
