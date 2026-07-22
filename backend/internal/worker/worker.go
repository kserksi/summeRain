// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/service"
	"gorm.io/gorm"
)

const (
	workerRestartInitialBackoff = time.Second
	workerRestartMaximumBackoff = 30 * time.Second
)

type managedWorker struct {
	name string
	run  func(context.Context)
}

type workerRestartWaiter func(context.Context, <-chan struct{}, time.Duration) bool

type Manager struct {
	DB     *gorm.DB
	Redis  *redis.Client
	Config *config.Config
	V2     *service.V2UploadService
	R2     r2WorkerService

	drainInit sync.Once
	drainOnce sync.Once
	drain     chan struct{}
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
		drain:  make(chan struct{}),
	}
}

func (m *Manager) Start(ctx context.Context) {
	drain := m.drainSignal()
	runSupervisedWorkers(ctx, drain, m.workers(drain), workerRestartInitialBackoff, waitForWorkerRestart)
}

func (m *Manager) BeginDrain() {
	m.drainSignal()
	m.drainOnce.Do(func() {
		close(m.drain)
	})
}

func (m *Manager) drainSignal() <-chan struct{} {
	m.drainInit.Do(func() {
		if m.drain == nil {
			m.drain = make(chan struct{})
		}
	})
	return m.drain
}

func (m *Manager) workers(drain <-chan struct{}) []managedWorker {
	v2Workers := 0
	if m.V2 != nil && m.Config != nil && m.Config.ImageV2.Enabled {
		v2Workers = m.Config.ImageV2.WatermarkConcurrency
	}

	workers := []managedWorker{
		{name: "heartbeat", run: func(ctx context.Context) {
			m.runHeartbeatMonitor(ctx, drain)
		}},
		{name: "outbox", run: func(ctx context.Context) {
			m.runOutbox(ctx, drain)
		}},
	}

	for index := 0; index < v2Workers; index++ {
		index := index
		workers = append(workers, managedWorker{
			name: fmt.Sprintf("v2-publish-%d", index),
			run: func(ctx context.Context) {
				m.runV2Publish(ctx, drain, index)
			},
		})
	}

	if m.Config != nil && m.Config.Storage.StagingPath != "" {
		workers = append(workers, managedWorker{name: "v2-cleanup", run: func(ctx context.Context) {
			m.runV2Cleanup(ctx, drain)
		}})
	}

	return append(workers,
		managedWorker{name: "view-flusher", run: func(ctx context.Context) {
			m.runViewFlusher(ctx, drain)
		}},
		managedWorker{name: "cleanup", run: func(ctx context.Context) {
			m.runCleanup(ctx, drain)
		}},
		managedWorker{name: "user-deletion", run: func(ctx context.Context) {
			m.runUserDeletion(ctx, drain)
		}},
	)
}

func runSupervisedWorkers(
	ctx context.Context,
	drain <-chan struct{},
	workers []managedWorker,
	initialRestartBackoff time.Duration,
	wait workerRestartWaiter,
) {
	var wg sync.WaitGroup
	wg.Add(len(workers))
	for _, worker := range workers {
		worker := worker
		go func() {
			defer wg.Done()
			superviseWorker(ctx, drain, worker, initialRestartBackoff, wait)
		}()
	}

	log.Printf("[worker] all workers started")
	wg.Wait()
	log.Printf("[worker] all workers stopped")
}

func superviseWorker(
	ctx context.Context,
	drain <-chan struct{},
	worker managedWorker,
	initialRestartBackoff time.Duration,
	wait workerRestartWaiter,
) {
	restartBackoff := initialRestartBackoff
	for !workerStopping(ctx, drain) {
		panicValue, panicked := runWorkerSafely(ctx, worker.run)
		if panicked {
			if workerStopping(ctx, drain) {
				log.Printf("[worker] %s panicked while stopping: %v", worker.name, panicValue)
				return
			}
			log.Printf(
				"[worker] %s panicked: %v; restarting in %s",
				worker.name,
				panicValue,
				restartBackoff,
			)
		} else {
			if workerStopping(ctx, drain) {
				return
			}
			log.Printf(
				"[worker] %s exited unexpectedly; restarting in %s",
				worker.name,
				restartBackoff,
			)
		}

		if !wait(ctx, drain, restartBackoff) {
			return
		}
		restartBackoff = nextWorkerRestartBackoff(restartBackoff)
	}
}

func nextWorkerRestartBackoff(current time.Duration) time.Duration {
	if current >= workerRestartMaximumBackoff {
		return workerRestartMaximumBackoff
	}
	next := current * 2
	if next <= current || next > workerRestartMaximumBackoff {
		return workerRestartMaximumBackoff
	}
	return next
}

func runWorkerSafely(ctx context.Context, run func(context.Context)) (panicValue interface{}, panicked bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			panicValue = recovered
			panicked = true
		}
	}()
	run(ctx)
	return nil, false
}

func waitForWorkerRestart(ctx context.Context, drain <-chan struct{}, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-drain:
		return false
	case <-timer.C:
		return true
	}
}

func workerStopping(ctx context.Context, drain <-chan struct{}) bool {
	if ctx.Err() != nil {
		return true
	}
	select {
	case <-drain:
		return true
	default:
		return false
	}
}
