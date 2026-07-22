// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"
)

const (
	viewFlushShutdownTimeout = 3 * time.Second
	viewCountRestoreTimeout  = time.Second
)

func (m *Manager) runViewFlusher(ctx context.Context, drain <-chan struct{}) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			runBoundedFinalViewFlush(viewFlushShutdownTimeout, m.flushViewCounts)
			return
		case <-drain:
			runBoundedFinalViewFlush(viewFlushShutdownTimeout, m.flushViewCounts)
			return
		case <-ticker.C:
			m.flushViewCounts(ctx)
		}
	}
}

func (m *Manager) flushViewCounts(ctx context.Context) {
	var cursor uint64
	var flushed int

	for {
		keys, nextCursor, err := m.Redis.Scan(ctx, cursor, "views:*", 100).Result()
		if err != nil {
			log.Printf("[view_flusher] scan error: %v", err)
			return
		}

		for _, key := range keys {
			countStr, err := m.Redis.GetDel(ctx, key).Result()
			if err != nil {
				log.Printf("[view_flusher] getdel error for key %s: %v", key, err)
				continue
			}

			count, err := strconv.ParseInt(countStr, 10, 64)
			if err != nil || count <= 0 {
				continue
			}

			imageID := strings.TrimPrefix(key, "views:")

			updateErr, restoreErr := updateViewCountWithRestore(
				ctx,
				key,
				imageID,
				count,
				func(updateCtx context.Context, id string, delta int64) error {
					return m.DB.WithContext(updateCtx).Exec(
						"UPDATE images SET view_count = view_count + ? WHERE id = ?",
						delta, id,
					).Error
				},
				func(restoreCtx context.Context, redisKey string, delta int64) error {
					return m.Redis.IncrBy(restoreCtx, redisKey, delta).Err()
				},
			)
			if updateErr != nil {
				log.Printf("[view_flusher] db update error for image %s: %v", imageID, updateErr)
				if restoreErr != nil {
					log.Printf("[view_flusher] restore error for key %s: %v", key, restoreErr)
				}
				continue
			}

			flushed++
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if flushed > 0 {
		log.Printf("[view_flusher] flushed %d image view counts", flushed)
	}
}

func updateViewCountWithRestore(
	ctx context.Context,
	key string,
	imageID string,
	count int64,
	update func(context.Context, string, int64) error,
	restore func(context.Context, string, int64) error,
) (updateErr error, restoreErr error) {
	updateErr = update(ctx, imageID, count)
	if updateErr == nil {
		return nil, nil
	}
	restoreCtx, cancel := context.WithTimeout(context.Background(), viewCountRestoreTimeout)
	defer cancel()
	return updateErr, restore(restoreCtx, key, count)
}

func runBoundedFinalViewFlush(timeout time.Duration, flush func(context.Context)) {
	if timeout <= 0 || flush == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	flush(ctx)
}
