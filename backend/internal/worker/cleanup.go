package worker

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/summerain/image-gallery/internal/model"
)

func (m *Manager) runCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[cleanup] panic recovered: %v", r)
					}
				}()
				m.cleanExpiredSessions()
				m.cleanExpiredImageAccessTokens()
				m.cleanExpiredCSRFTokens()
				m.cleanFailedUploads()
				m.cleanOrphanTempFiles()
			}()
		}
	}
}

func (m *Manager) cleanExpiredSessions() {
	result := m.DB.Where("expires_at < NOW()").Delete(&model.Session{})
	if result.Error != nil {
		log.Printf("[cleanup] error cleaning expired sessions: %v", result.Error)
		return
	}
	if result.RowsAffected > 0 {
		log.Printf("[cleanup] deleted %d expired sessions", result.RowsAffected)
	}
}

func (m *Manager) cleanExpiredImageAccessTokens() {
	// Keep revoked tokens (they must remain so /i/ returns 404 "revoked"); only
	// purge non-revoked tokens that expired more than 7 days ago.
	result := m.DB.Where("revoked_at IS NULL AND expires_at < ?", time.Now().AddDate(0, 0, -7)).Delete(&model.ImageAccessToken{})
	if result.Error != nil {
		log.Printf("[cleanup] error cleaning expired image access tokens: %v", result.Error)
		return
	}
	if result.RowsAffected > 0 {
		log.Printf("[cleanup] deleted %d expired image access tokens", result.RowsAffected)
	}
}

func (m *Manager) cleanExpiredCSRFTokens() {
	result := m.DB.Where("expires_at < NOW()").Delete(&model.CSRFToken{})
	if result.Error != nil {
		log.Printf("[cleanup] error cleaning expired csrf tokens: %v", result.Error)
		return
	}
	if result.RowsAffected > 0 {
		log.Printf("[cleanup] deleted %d expired csrf tokens", result.RowsAffected)
	}
}

func (m *Manager) cleanFailedUploads() {
	result := m.DB.Where("status = 'failed' AND updated_at < NOW() - INTERVAL 24 HOUR").Delete(&model.UploadQueue{})
	if result.Error != nil {
		log.Printf("[cleanup] error cleaning failed uploads: %v", result.Error)
		return
	}
	if result.RowsAffected > 0 {
		log.Printf("[cleanup] deleted %d failed upload entries", result.RowsAffected)
	}
}

func (m *Manager) cleanOrphanTempFiles() {
	tempPath := m.Config.Storage.TempPath
	cutoff := time.Now().Add(-1 * time.Hour)
	var cleaned int

	entries, err := os.ReadDir(tempPath)
	if err != nil {
		log.Printf("[cleanup] error reading temp directory: %v", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
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

	if cleaned > 0 {
		log.Printf("[cleanup] removed %d orphan temp files", cleaned)
	}
}
