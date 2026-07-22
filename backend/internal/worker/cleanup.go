// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	controlPlaneCleanupBatchSize  = 100
	controlPlaneCleanupMaxBatches = 20
	controlPlaneCleanupTimeBudget = 5 * time.Second
	orphanTempScanBatchSize       = 64
	orphanTempScanMaxEntries      = 512
)

type controlPlaneDelete struct {
	name   string
	query  string
	cutoff time.Time
}

type controlPlaneCleanupLimits struct {
	batchSize  int
	maxBatches int
	timeBudget time.Duration
}

type controlPlaneCleanupResult struct {
	name         string
	batches      int
	rowsAffected int64
	err          error
	capped       bool
}

type controlPlaneCleanupRun struct {
	results         []controlPlaneCleanupResult
	budgetExhausted bool
}

type controlPlaneDeleteExecutor func(context.Context, controlPlaneDelete, int) (int64, error)

func (m *Manager) runCleanup(ctx context.Context, drain <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-drain:
			return
		case <-ticker.C:
			m.cleanControlPlane(ctx)
			m.cleanOrphanTempFiles()
		}
	}
}

func (m *Manager) cleanControlPlane(ctx context.Context) {
	run := runControlPlaneCleanup(
		ctx,
		controlPlaneDeletes(time.Now()),
		controlPlaneCleanupLimits{
			batchSize:  controlPlaneCleanupBatchSize,
			maxBatches: controlPlaneCleanupMaxBatches,
			timeBudget: controlPlaneCleanupTimeBudget,
		},
		time.Now,
		func(ctx context.Context, statement controlPlaneDelete, batchSize int) (int64, error) {
			result := m.DB.WithContext(ctx).Exec(statement.query, statement.cutoff, batchSize)
			return result.RowsAffected, result.Error
		},
	)

	for _, result := range run.results {
		if result.err != nil {
			log.Printf("[cleanup] error cleaning %s after %d batches: %v", result.name, result.batches, result.err)
			continue
		}
		if result.rowsAffected > 0 {
			log.Printf("[cleanup] deleted %d old %s in %d batches", result.rowsAffected, result.name, result.batches)
		}
		if result.capped {
			log.Printf("[cleanup] %s reached the %d-batch cleanup limit", result.name, controlPlaneCleanupMaxBatches)
		}
	}
	if run.budgetExhausted {
		log.Printf("[cleanup] control-plane cleanup reached its %s time budget", controlPlaneCleanupTimeBudget)
	}
}

func controlPlaneDeletes(now time.Time) []controlPlaneDelete {
	standardCutoff := now.AddDate(0, 0, -30)
	deadOutboxCutoff := now.AddDate(0, 0, -90)
	return []controlPlaneDelete{
		{
			name:   "expired sessions",
			query:  "DELETE FROM sessions WHERE expires_at < ? ORDER BY id LIMIT ?",
			cutoff: now,
		},
		{
			name:   "expired image access tokens",
			query:  "DELETE FROM image_access_tokens WHERE revoked_at IS NULL AND expires_at < ? ORDER BY id LIMIT ?",
			cutoff: now.AddDate(0, 0, -7),
		},
		{
			name:   "expired CSRF tokens",
			query:  "DELETE FROM csrf_tokens WHERE expires_at < ? ORDER BY id LIMIT ?",
			cutoff: now,
		},
		{
			name:   "failed upload entries",
			query:  "DELETE FROM upload_queues WHERE status = 'failed' AND updated_at < ? ORDER BY id LIMIT ?",
			cutoff: now.Add(-24 * time.Hour),
		},
		{
			name:   "processing jobs",
			query:  "DELETE FROM processing_jobs WHERE status IN ('completed','dead') AND updated_at < ? ORDER BY id LIMIT ?",
			cutoff: standardCutoff,
		},
		{
			name:   "upload sessions",
			query:  "DELETE FROM upload_sessions WHERE status IN ('completed','failed','cancelled') AND updated_at < ? AND NOT EXISTS (SELECT 1 FROM processing_jobs WHERE processing_jobs.upload_session_id = upload_sessions.id AND processing_jobs.status IN ('queued','running','retry')) ORDER BY id LIMIT ?",
			cutoff: standardCutoff,
		},
		{
			name:   "published outbox events",
			query:  "DELETE FROM outbox_events WHERE status = 'published' AND published_at < ? ORDER BY id LIMIT ?",
			cutoff: standardCutoff,
		},
		// Dead events never get a published_at value. available_at records their
		// final delivery attempt and shares the indexed (status, available_at) key.
		{
			name:   "dead outbox events",
			query:  "DELETE FROM outbox_events WHERE status = 'dead' AND event_type <> 'storage.file.delete' AND available_at < ? ORDER BY id LIMIT ?",
			cutoff: deadOutboxCutoff,
		},
	}
}

func runControlPlaneCleanup(
	ctx context.Context,
	statements []controlPlaneDelete,
	limits controlPlaneCleanupLimits,
	now func() time.Time,
	execute controlPlaneDeleteExecutor,
) controlPlaneCleanupRun {
	run := controlPlaneCleanupRun{
		results: make([]controlPlaneCleanupResult, len(statements)),
	}
	for index, statement := range statements {
		run.results[index].name = statement.name
	}
	if len(statements) == 0 || limits.batchSize < 1 || limits.maxBatches < 1 || limits.timeBudget <= 0 || now == nil || execute == nil {
		return run
	}

	cleanupCtx, cancel := context.WithTimeout(ctx, limits.timeBudget)
	defer cancel()
	startedAt := now()
	deadline := startedAt.Add(limits.timeBudget)
	active := make([]bool, len(statements))
	for index := range active {
		active[index] = true
	}

	for batch := 0; batch < limits.maxBatches; batch++ {
		anyActive := false
		for index, statement := range statements {
			if !active[index] {
				continue
			}
			anyActive = true
			if cleanupCtx.Err() != nil {
				run.budgetExhausted = ctx.Err() == nil
				return run
			}
			if !now().Before(deadline) {
				run.budgetExhausted = true
				return run
			}

			rowsAffected, err := execute(cleanupCtx, statement, limits.batchSize)
			result := &run.results[index]
			result.batches++
			if err != nil {
				result.err = err
				active[index] = false
				if cleanupCtx.Err() != nil {
					run.budgetExhausted = ctx.Err() == nil
					return run
				}
				continue
			}
			result.rowsAffected += rowsAffected
			if rowsAffected < int64(limits.batchSize) {
				active[index] = false
			}
		}
		if !anyActive {
			return run
		}
	}

	for index, isActive := range active {
		if isActive {
			run.results[index].capped = true
		}
	}
	return run
}

func (m *Manager) cleanOrphanTempFiles() {
	tempPath := m.Config.Storage.TempPath
	cutoff := time.Now().Add(-1 * time.Hour)
	var cleaned int

	directory, err := os.Open(tempPath)
	if err != nil {
		log.Printf("[cleanup] error reading temp directory: %v", err)
		return
	}
	defer directory.Close()

	for scanned := 0; scanned < orphanTempScanMaxEntries; {
		remaining := orphanTempScanMaxEntries - scanned
		batchSize := orphanTempScanBatchSize
		if remaining < batchSize {
			batchSize = remaining
		}
		entries, readErr := directory.ReadDir(batchSize)
		scanned += len(entries)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			info, infoErr := entry.Info()
			if infoErr != nil {
				continue
			}

			if info.ModTime().Before(cutoff) {
				fullPath := filepath.Join(tempPath, entry.Name())
				if err := os.Remove(fullPath); err != nil {
					log.Printf("[cleanup] error removing temp file %s: %v", fullPath, err)
					continue
				}
				cleaned++
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			log.Printf("[cleanup] error scanning temp directory: %v", readErr)
			break
		}
	}

	if cleaned > 0 {
		log.Printf("[cleanup] removed %d orphan temp files", cleaned)
	}
}
