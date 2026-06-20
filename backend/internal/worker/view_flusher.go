package worker

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"
)

func (m *Manager) runViewFlusher(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.flushViewCounts(ctx)
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[view_flusher] panic recovered: %v", r)
					}
				}()
				m.flushViewCounts(ctx)
			}()
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

			result := m.DB.Exec(
				"UPDATE images SET view_count = view_count + ? WHERE id = ?",
				count, imageID,
			)
			if result.Error != nil {
				log.Printf("[view_flusher] db update error for image %s: %v", imageID, result.Error)
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
