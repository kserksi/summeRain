// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"gorm.io/gorm"
)

const (
	v2CleanupInterval  = time.Minute
	v2CleanupBatchSize = 100
)

type v2CleanupStore interface {
	ListCleanupSessions(context.Context, time.Time, int) ([]model.UploadSession, error)
	ClaimCleanup(context.Context, model.UploadSession, time.Time) (bool, error)
	ImageCompleted(context.Context, uint64) (bool, error)
	FinishCleanup(context.Context, uint64, string, time.Time) error
	ExistingUploadKeys(context.Context, []string) (map[string]struct{}, error)
}

type gormV2CleanupStore struct {
	db *gorm.DB
}

type v2CleanupStats struct {
	SessionsExamined int
	SessionsCleaned  int
	OrphansCleaned   int
}

type stagingGuard struct {
	rootPath       string
	canonicalRoot  string
	persistentV2   string
	canonicalV2Dir string
}

type stagingOrphanScanner struct {
	root      string
	directory *os.File
}

func (s *stagingOrphanScanner) Close() {
	if s.directory != nil {
		_ = s.directory.Close()
	}
	s.directory = nil
	s.root = ""
}

func (s *stagingOrphanScanner) Read(root string, limit int) ([]os.DirEntry, error) {
	if limit <= 0 {
		return nil, nil
	}
	if s.directory == nil || s.root != root {
		s.Close()
		directory, err := os.Open(root)
		if err != nil {
			return nil, err
		}
		s.root = root
		s.directory = directory
	}
	entries, err := s.directory.ReadDir(limit)
	if errors.Is(err, io.EOF) {
		s.Close()
		return entries, nil
	}
	if err != nil {
		s.Close()
		return nil, err
	}
	if len(entries) < limit {
		s.Close()
	}
	return entries, nil
}

func (m *Manager) runV2Cleanup(ctx context.Context) {
	nextPersistentReconcile := time.Time{}
	persistentReconciler := newPersistentV2Scanner(m.Config.Storage.BasePath)
	defer persistentReconciler.Close()
	stagingScanner := &stagingOrphanScanner{}
	defer stagingScanner.Close()
	run := func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[v2_cleanup] panic recovered: %v", recovered)
			}
		}()
		guard, err := newStagingGuard(m.Config.Storage.BasePath, m.Config.Storage.StagingPath)
		if err != nil {
			log.Printf("[v2_cleanup] unsafe staging configuration: %v", err)
			return
		}
		if m.Config.ImageV2.SessionTTL <= 0 {
			log.Printf("[v2_cleanup] invalid session TTL: %s", m.Config.ImageV2.SessionTTL)
			return
		}
		stats := cleanupV2BatchWithScanner(ctx, &gormV2CleanupStore{db: m.DB}, guard, stagingScanner, time.Now(), m.Config.ImageV2.SessionTTL)
		if stats.SessionsCleaned > 0 || stats.OrphansCleaned > 0 {
			log.Printf("[v2_cleanup] sessions=%d/%d orphans=%d", stats.SessionsCleaned, stats.SessionsExamined, stats.OrphansCleaned)
		}
		now := time.Now()
		if !now.Before(nextPersistentReconcile) {
			queued, complete, err := queuePersistentV2Orphans(
				ctx,
				m.DB,
				persistentReconciler,
				now,
				m.Config.ImageV2.SessionTTL,
			)
			if err != nil {
				log.Printf("[v2_cleanup] persistent reconciliation failed: %v", err)
				nextPersistentReconcile = now.Add(persistentReconcileContinuationInterval)
			} else {
				if queued > 0 {
					log.Printf("[v2_cleanup] queued %d persistent orphan files", queued)
				}
				nextPersistentReconcile = now.Add(persistentReconcileContinuationInterval)
				if complete {
					nextPersistentReconcile = now.Add(persistentReconcileInterval)
				}
			}
		}
	}

	run()
	ticker := time.NewTicker(v2CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func cleanupV2Batch(ctx context.Context, store v2CleanupStore, guard *stagingGuard, now time.Time, sessionTTL time.Duration) v2CleanupStats {
	scanner := &stagingOrphanScanner{}
	defer scanner.Close()
	return cleanupV2BatchWithScanner(ctx, store, guard, scanner, now, sessionTTL)
}

func cleanupV2BatchWithScanner(ctx context.Context, store v2CleanupStore, guard *stagingGuard, scanner *stagingOrphanScanner, now time.Time, sessionTTL time.Duration) v2CleanupStats {
	var stats v2CleanupStats
	if sessionTTL <= 0 {
		log.Printf("[v2_cleanup] refuse non-positive session TTL: %s", sessionTTL)
		return stats
	}
	sessions, err := store.ListCleanupSessions(ctx, now, v2CleanupBatchSize)
	if err != nil {
		log.Printf("[v2_cleanup] list sessions: %v", err)
	} else {
		if len(sessions) > v2CleanupBatchSize {
			sessions = sessions[:v2CleanupBatchSize]
		}
		stats.SessionsExamined = len(sessions)
		for _, session := range sessions {
			if ctx.Err() != nil {
				return stats
			}
			if cleanupV2Session(ctx, store, guard, session, now) {
				stats.SessionsCleaned++
			}
		}
	}

	remaining := v2CleanupBatchSize - stats.SessionsExamined
	if remaining <= 0 || ctx.Err() != nil {
		return stats
	}
	stats.OrphansCleaned = cleanupV2OrphansWithScanner(ctx, store, guard, scanner, now.Add(-sessionTTL), remaining)
	return stats
}

func cleanupV2Session(ctx context.Context, store v2CleanupStore, guard *stagingGuard, session model.UploadSession, now time.Time) bool {
	claimed, err := store.ClaimCleanup(ctx, session, now)
	if err != nil {
		log.Printf("[v2_cleanup] claim session=%s: %v", session.UploadKey, err)
		return false
	}
	if !claimed {
		return false
	}

	finalStatus, err := cleanupV2FinalStatus(ctx, store, session)
	if err != nil {
		log.Printf("[v2_cleanup] inspect image for session=%s: %v", session.UploadKey, err)
		return false
	}

	path, err := guard.sessionDir(session.StagingPath, session.UploadKey)
	if err != nil {
		log.Printf("[v2_cleanup] reject session=%s path=%q: %v", session.UploadKey, session.StagingPath, err)
		return false
	}
	if err := os.RemoveAll(path); err != nil {
		log.Printf("[v2_cleanup] remove session=%s path=%q: %v", session.UploadKey, path, err)
		return false
	}
	if err := store.FinishCleanup(ctx, session.ID, finalStatus, now); err != nil {
		log.Printf("[v2_cleanup] finish session=%s: %v", session.UploadKey, err)
		return false
	}
	return true
}

func cleanupV2FinalStatus(ctx context.Context, store v2CleanupStore, session model.UploadSession) (string, error) {
	switch session.Status {
	case model.UploadSessionStatusFailed:
		return model.UploadSessionStatusFailed, nil
	case model.UploadSessionStatusCompleted:
		return model.UploadSessionStatusCompleted, nil
	case model.UploadSessionStatusCancelled:
		return model.UploadSessionStatusCancelled, nil
	case model.UploadSessionStatusCleanupPending:
		// ClaimCleanup temporarily changes every terminal session to
		// cleanup_pending. These durable timestamps preserve the original state
		// when a filesystem or database failure makes cleanup retry later.
		if session.ImageID != nil {
			completed, err := store.ImageCompleted(ctx, *session.ImageID)
			if err != nil {
				return "", err
			}
			if completed {
				return model.UploadSessionStatusCompleted, nil
			}
		}
		if session.CompletedAt != nil {
			return model.UploadSessionStatusCompleted, nil
		}
		if session.FailedAt != nil || session.ErrorCode != nil {
			return model.UploadSessionStatusFailed, nil
		}
		return model.UploadSessionStatusCancelled, nil
	default:
		return model.UploadSessionStatusCancelled, nil
	}
}

func cleanupV2Orphans(ctx context.Context, store v2CleanupStore, guard *stagingGuard, cutoff time.Time, limit int) int {
	scanner := &stagingOrphanScanner{}
	defer scanner.Close()
	return cleanupV2OrphansWithScanner(ctx, store, guard, scanner, cutoff, limit)
}

func cleanupV2OrphansWithScanner(ctx context.Context, store v2CleanupStore, guard *stagingGuard, scanner *stagingOrphanScanner, cutoff time.Time, limit int) int {
	entries, err := scanner.Read(guard.rootPath, limit)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		log.Printf("[v2_cleanup] read staging root: %v", err)
		return 0
	}

	type orphanCandidate struct {
		name string
		path string
	}
	candidates := make([]orphanCandidate, 0, limit)
	keys := make([]string, 0, limit)
	for _, entry := range entries {
		if len(candidates) >= limit || ctx.Err() != nil {
			break
		}
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			log.Printf("[v2_cleanup] inspect orphan candidate=%q: %v", entry.Name(), err)
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}
		path, err := guard.sessionDir(filepath.Join(guard.rootPath, entry.Name()), entry.Name())
		if err != nil {
			log.Printf("[v2_cleanup] reject orphan candidate=%q: %v", entry.Name(), err)
			continue
		}
		candidates = append(candidates, orphanCandidate{name: entry.Name(), path: path})
		keys = append(keys, entry.Name())
	}
	if len(candidates) == 0 {
		return 0
	}

	existing, err := store.ExistingUploadKeys(ctx, keys)
	if err != nil {
		log.Printf("[v2_cleanup] resolve orphan sessions: %v", err)
		return 0
	}
	cleaned := 0
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		if _, found := existing[candidate.name]; found {
			continue
		}
		if err := os.RemoveAll(candidate.path); err != nil {
			log.Printf("[v2_cleanup] remove orphan=%q: %v", candidate.name, err)
			continue
		}
		cleaned++
	}
	return cleaned
}

func newStagingGuard(basePath, stagingPath string) (*stagingGuard, error) {
	base, err := filepath.Abs(filepath.Clean(basePath))
	if err != nil {
		return nil, fmt.Errorf("resolve storage root: %w", err)
	}
	root, err := filepath.Abs(filepath.Clean(stagingPath))
	if err != nil {
		return nil, fmt.Errorf("resolve staging root: %w", err)
	}
	if !strictlyWithin(base, root) {
		return nil, errors.New("staging root must be a strict child of storage root")
	}
	persistentV2 := filepath.Join(base, "v2")
	if sameOrWithin(persistentV2, root) {
		return nil, errors.New("staging root must not be the persistent v2 directory")
	}
	canonicalBase := canonicalPath(base)
	canonicalRoot := canonicalPath(root)
	if !strictlyWithin(canonicalBase, canonicalRoot) {
		return nil, errors.New("canonical staging root escapes storage root")
	}
	canonicalV2 := canonicalPath(filepath.Join(canonicalBase, "v2"))
	if sameOrWithin(canonicalV2, canonicalRoot) {
		return nil, errors.New("canonical staging root overlaps the persistent v2 directory")
	}
	return &stagingGuard{
		rootPath:       root,
		canonicalRoot:  canonicalRoot,
		persistentV2:   persistentV2,
		canonicalV2Dir: canonicalV2,
	}, nil
}

func (g *stagingGuard) sessionDir(candidate, expectedName string) (string, error) {
	if candidate == "" || expectedName == "" || filepath.Base(expectedName) != expectedName || expectedName == "." || expectedName == ".." {
		return "", errors.New("invalid session directory name")
	}
	absCandidate, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", fmt.Errorf("resolve candidate: %w", err)
	}
	rel, err := filepath.Rel(g.rootPath, absCandidate)
	if err != nil || rel != expectedName || filepath.Dir(rel) != "." {
		return "", errors.New("candidate is not a direct child of staging root")
	}
	if sameOrWithin(g.persistentV2, absCandidate) {
		return "", errors.New("candidate overlaps persistent v2 storage")
	}

	info, err := os.Lstat(absCandidate)
	if errors.Is(err, os.ErrNotExist) {
		return absCandidate, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect candidate: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("candidate is not a real directory")
	}
	canonicalCandidate, err := filepath.EvalSymlinks(absCandidate)
	if err != nil {
		return "", fmt.Errorf("resolve candidate symlinks: %w", err)
	}
	if filepath.Dir(canonicalCandidate) != g.canonicalRoot || sameOrWithin(g.canonicalV2Dir, canonicalCandidate) {
		return "", errors.New("canonical candidate escapes staging root")
	}
	return absCandidate, nil
}

func canonicalPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}
	parent, parentErr := filepath.EvalSymlinks(filepath.Dir(path))
	if parentErr == nil {
		return filepath.Join(parent, filepath.Base(path))
	}
	return path
}

func strictlyWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func sameOrWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (s *gormV2CleanupStore) ListCleanupSessions(ctx context.Context, now time.Time, limit int) ([]model.UploadSession, error) {
	if limit > v2CleanupBatchSize {
		limit = v2CleanupBatchSize
	}
	var sessions []model.UploadSession
	err := s.db.WithContext(ctx).
		Where("staging_path <> '' AND ((status IN ? AND expires_at <= ?) OR (status IN ? AND (cleanup_after IS NULL OR cleanup_after <= ?)))", []string{
			model.UploadSessionStatusInitiated,
			model.UploadSessionStatusUploading,
		}, now, []string{
			model.UploadSessionStatusCompleted,
			model.UploadSessionStatusFailed,
			model.UploadSessionStatusCancelled,
			model.UploadSessionStatusCleanupPending,
		}, now).
		Order("updated_at ASC, id ASC").
		Limit(limit).
		Find(&sessions).Error
	return sessions, err
}

func (s *gormV2CleanupStore) ClaimCleanup(ctx context.Context, session model.UploadSession, now time.Time) (bool, error) {
	query := s.db.WithContext(ctx).Model(&model.UploadSession{}).
		Where("id = ? AND status = ? AND staging_path <> ''", session.ID, session.Status)
	switch session.Status {
	case model.UploadSessionStatusInitiated, model.UploadSessionStatusUploading:
		query = query.Where("expires_at <= ?", now)
	case model.UploadSessionStatusCompleted, model.UploadSessionStatusFailed, model.UploadSessionStatusCancelled, model.UploadSessionStatusCleanupPending:
		query = query.Where("(cleanup_after IS NULL OR cleanup_after <= ?)", now)
	default:
		return false, nil
	}
	retryAt := now.Add(v2CleanupInterval)
	result := query.Updates(map[string]interface{}{
		"status":        model.UploadSessionStatusCleanupPending,
		"cleanup_after": retryAt,
	})
	return result.RowsAffected == 1, result.Error
}

func (s *gormV2CleanupStore) ImageCompleted(ctx context.Context, imageID uint64) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&model.Image{}).
		Where("id = ? AND processing_status = ?", imageID, model.ImageProcessingStatusCompleted).
		Count(&count).Error
	return count == 1, err
}

func (s *gormV2CleanupStore) FinishCleanup(ctx context.Context, sessionID uint64, status string, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.UploadPart{}).Where("upload_session_id = ?", sessionID).Updates(map[string]interface{}{
			"status":     model.UploadPartStatusCleaned,
			"cleaned_at": now,
		}).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{
			"status":        status,
			"cleanup_after": nil,
			"staging_path":  "",
		}
		switch status {
		case model.UploadSessionStatusCompleted:
			updates["completed_at"] = now
		case model.UploadSessionStatusFailed:
			updates["failed_at"] = now
		default:
			updates["cancelled_at"] = now
		}
		result := tx.Model(&model.UploadSession{}).
			Where("id = ? AND status = ?", sessionID, model.UploadSessionStatusCleanupPending).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (s *gormV2CleanupStore) ExistingUploadKeys(ctx context.Context, keys []string) (map[string]struct{}, error) {
	existing := make(map[string]struct{}, len(keys))
	if len(keys) == 0 {
		return existing, nil
	}
	var found []string
	if err := s.db.WithContext(ctx).Model(&model.UploadSession{}).
		Where("upload_key IN ? AND staging_path <> ''", keys).Pluck("upload_key", &found).Error; err != nil {
		return nil, err
	}
	for _, key := range found {
		existing[key] = struct{}{}
	}
	return existing, nil
}
