// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"log"
	"time"

	"github.com/summerain/image-gallery/internal/model"
)

func (m *Manager) runHeartbeatMonitor(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[heartbeat] panic recovered: %v", r)
					}
				}()
				m.checkHeartbeats()
			}()
		}
	}
}

func (m *Manager) checkHeartbeats() {
	var staleIDs []uint64
	m.DB.Model(&model.Session{}).
		Where("token_type = 'session' AND expires_at > NOW() AND last_heartbeat_at IS NOT NULL AND last_heartbeat_at < NOW() - INTERVAL heartbeat_grace_seconds SECOND").
		Pluck("id", &staleIDs)

	if len(staleIDs) == 0 {
		return
	}

	result := m.DB.Model(&model.Session{}).Where("id IN ?", staleIDs).Update("expires_at", time.Now())
	if result.Error != nil {
		log.Printf("[heartbeat] error expiring stale sessions: %v", result.Error)
		return
	}

	if result.RowsAffected > 0 {
		log.Printf("[heartbeat] expired %d stale sessions", result.RowsAffected)
		for _, sid := range staleIDs {
			m.DB.Create(&model.AuditLog{
				Action:       "auth.heartbeat_gap_invalidated",
				ResourceType: "session",
				ResourceID:   sid,
			})
		}
	}
}
