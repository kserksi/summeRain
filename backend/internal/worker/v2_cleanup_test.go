// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/model"
)

type fakeV2CleanupStore struct {
	sessions    []model.UploadSession
	completed   map[uint64]bool
	existing    map[string]struct{}
	finished    map[uint64]string
	claims      int
	listLimit   int
	ignoreLimit bool
	rejectClaim bool
	finishFails int
	maxKeyBatch int
}

func (f *fakeV2CleanupStore) ListCleanupSessions(_ context.Context, _ time.Time, limit int) ([]model.UploadSession, error) {
	f.listLimit = limit
	sessions := f.sessions
	if !f.ignoreLimit && len(sessions) > limit {
		sessions = sessions[:limit]
	}
	return sessions, nil
}

func (f *fakeV2CleanupStore) ClaimCleanup(_ context.Context, _ model.UploadSession, _ time.Time) (bool, error) {
	f.claims++
	return !f.rejectClaim, nil
}

func (f *fakeV2CleanupStore) ImageCompleted(_ context.Context, imageID uint64) (bool, error) {
	return f.completed[imageID], nil
}

func (f *fakeV2CleanupStore) FinishCleanup(_ context.Context, sessionID uint64, status string, _ time.Time) error {
	if f.finishFails > 0 {
		f.finishFails--
		return errors.New("temporary finish failure")
	}
	if f.finished == nil {
		f.finished = make(map[uint64]string)
	}
	f.finished[sessionID] = status
	return nil
}

func TestCleanupV2RetryPreservesFailedFinalStatus(t *testing.T) {
	base := t.TempDir()
	staging := filepath.Join(base, ".staging")
	mustMkdir(t, staging)
	stagingPath := makeStagingDir(t, staging, "failed-retry")
	guard, err := newStagingGuard(base, staging)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Round(time.Second)
	failedAt := now.Add(-time.Minute)
	store := &fakeV2CleanupStore{finishFails: 1}
	session := model.UploadSession{
		ID: 99, UploadKey: "failed-retry", Status: model.UploadSessionStatusFailed,
		StagingPath: stagingPath, FailedAt: &failedAt,
	}

	if cleanupV2Session(context.Background(), store, guard, session, now) {
		t.Fatal("first cleanup unexpectedly succeeded")
	}
	assertNotExists(t, stagingPath)

	// ClaimCleanup persisted cleanup_pending before the first FinishCleanup
	// failed. The retry must recover the original failed terminal state.
	session.Status = model.UploadSessionStatusCleanupPending
	if !cleanupV2Session(context.Background(), store, guard, session, now.Add(time.Minute)) {
		t.Fatal("cleanup retry failed")
	}
	if got := store.finished[session.ID]; got != model.UploadSessionStatusFailed {
		t.Fatalf("retried session status = %q, want failed", got)
	}
}

func (f *fakeV2CleanupStore) ExistingUploadKeys(_ context.Context, keys []string) (map[string]struct{}, error) {
	if len(keys) > f.maxKeyBatch {
		f.maxKeyBatch = len(keys)
	}
	result := make(map[string]struct{})
	for _, key := range keys {
		if _, ok := f.existing[key]; ok {
			result[key] = struct{}{}
		}
	}
	return result, nil
}

func TestCleanupV2OrphanScannerBoundsAndContinuesDirectoryReads(t *testing.T) {
	base := t.TempDir()
	staging := filepath.Join(base, ".staging")
	mustMkdir(t, staging)
	now := time.Now().Round(time.Second)
	oldTime := now.Add(-2 * time.Hour)
	const total = 137
	for index := 0; index < total; index++ {
		path := makeStagingDir(t, staging, fmt.Sprintf("orphan-%03d", index))
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}
	guard, err := newStagingGuard(base, staging)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeV2CleanupStore{}
	scanner := &stagingOrphanScanner{}
	defer scanner.Close()

	cleaned := 0
	for attempt := 0; attempt < 20 && cleaned < total; attempt++ {
		cleaned += cleanupV2OrphansWithScanner(
			context.Background(), store, guard, scanner, now.Add(-time.Hour), 11,
		)
	}
	if cleaned != total {
		t.Fatalf("cleaned orphans = %d, want %d", cleaned, total)
	}
	if store.maxKeyBatch > 11 {
		t.Fatalf("orphan lookup batch = %d, want <= 11", store.maxKeyBatch)
	}
}

func TestStagingGuardRejectsPersistentAndEscapingPaths(t *testing.T) {
	base := t.TempDir()
	staging := filepath.Join(base, ".staging")
	persistent := filepath.Join(base, "v2")
	mustMkdir(t, staging)
	mustMkdir(t, persistent)
	marker := filepath.Join(persistent, "keep.webp")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}

	guard, err := newStagingGuard(base, staging)
	if err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(staging, "session-a")
	mustMkdir(t, valid)
	if _, err := guard.sessionDir(valid, "session-a"); err != nil {
		t.Fatalf("valid session directory rejected: %v", err)
	}
	if _, err := guard.sessionDir(persistent, "v2"); err == nil {
		t.Fatal("persistent v2 directory was accepted")
	}
	if _, err := guard.sessionDir(staging, ".staging"); err == nil {
		t.Fatal("staging root was accepted as a session directory")
	}
	if _, err := newStagingGuard(base, persistent); err == nil {
		t.Fatal("persistent v2 directory was accepted as staging root")
	}
	escapeTarget := t.TempDir()
	escapeLink := filepath.Join(base, "escape-staging")
	if err := os.Symlink(escapeTarget, escapeLink); err != nil {
		t.Fatal(err)
	}
	if _, err := newStagingGuard(base, escapeLink); err == nil {
		t.Fatal("staging symlink escaping storage root was accepted")
	}

	link := filepath.Join(staging, "session-link")
	if err := os.Symlink(persistent, link); err != nil {
		t.Fatal(err)
	}
	if _, err := guard.sessionDir(link, "session-link"); err == nil {
		t.Fatal("symlink session directory was accepted")
	}
	assertExists(t, marker)
}

func TestCleanupV2BatchFinalizesSessionsAndRemovesOnlyOldOrphans(t *testing.T) {
	base := t.TempDir()
	staging := filepath.Join(base, ".staging")
	persistent := filepath.Join(base, "v2")
	mustMkdir(t, staging)
	mustMkdir(t, persistent)
	persistentMarker := filepath.Join(persistent, "master.webp")
	if err := os.WriteFile(persistentMarker, []byte("master"), 0600); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Round(time.Second)
	completedImageID := uint64(42)
	sessions := []model.UploadSession{
		{ID: 1, UploadKey: "expired", Status: model.UploadSessionStatusInitiated, StagingPath: makeStagingDir(t, staging, "expired")},
		{ID: 2, UploadKey: "failed", Status: model.UploadSessionStatusFailed, StagingPath: makeStagingDir(t, staging, "failed")},
		{ID: 3, UploadKey: "cancelled", Status: model.UploadSessionStatusCancelled, StagingPath: makeStagingDir(t, staging, "cancelled")},
		{ID: 4, UploadKey: "completed", Status: model.UploadSessionStatusCleanupPending, ImageID: &completedImageID, StagingPath: makeStagingDir(t, staging, "completed")},
		{ID: 5, UploadKey: "legacy-completed", Status: model.UploadSessionStatusCompleted, ImageID: &completedImageID, StagingPath: makeStagingDir(t, staging, "legacy-completed")},
	}
	oldOrphan := makeStagingDir(t, staging, "old-orphan")
	knownOld := makeStagingDir(t, staging, "known-old")
	freshOrphan := makeStagingDir(t, staging, "fresh-orphan")
	oldTime := now.Add(-2 * time.Hour)
	for _, path := range []string{oldOrphan, knownOld} {
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}

	guard, err := newStagingGuard(base, staging)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeV2CleanupStore{
		sessions:  sessions,
		completed: map[uint64]bool{completedImageID: true},
		existing:  map[string]struct{}{"known-old": {}},
	}
	stats := cleanupV2Batch(context.Background(), store, guard, now, 30*time.Minute)

	if stats.SessionsExamined != 5 || stats.SessionsCleaned != 5 || stats.OrphansCleaned != 1 {
		t.Fatalf("unexpected cleanup stats: %+v", stats)
	}
	for _, session := range sessions {
		assertNotExists(t, session.StagingPath)
	}
	for _, id := range []uint64{1, 3} {
		if got := store.finished[id]; got != model.UploadSessionStatusCancelled {
			t.Errorf("session %d status = %q, want cancelled", id, got)
		}
	}
	if got := store.finished[2]; got != model.UploadSessionStatusFailed {
		t.Errorf("failed session status = %q, want failed", got)
	}
	if got := store.finished[4]; got != model.UploadSessionStatusCompleted {
		t.Errorf("completed image session status = %q, want completed", got)
	}
	if got := store.finished[5]; got != model.UploadSessionStatusCompleted {
		t.Errorf("legacy completed session status = %q, want completed", got)
	}
	assertNotExists(t, oldOrphan)
	assertExists(t, knownOld)
	assertExists(t, freshOrphan)
	assertExists(t, persistentMarker)
}

func TestCleanupV2BatchContinuesAfterRejectedSessionPath(t *testing.T) {
	base := t.TempDir()
	staging := filepath.Join(base, ".staging")
	mustMkdir(t, staging)
	validPath := makeStagingDir(t, staging, "valid")
	outsidePath := makeStagingDir(t, base, "outside")
	store := &fakeV2CleanupStore{sessions: []model.UploadSession{
		{ID: 1, UploadKey: "outside", Status: model.UploadSessionStatusFailed, StagingPath: outsidePath},
		{ID: 2, UploadKey: "valid", Status: model.UploadSessionStatusFailed, StagingPath: validPath},
	}}
	guard, err := newStagingGuard(base, staging)
	if err != nil {
		t.Fatal(err)
	}

	stats := cleanupV2Batch(context.Background(), store, guard, time.Now(), time.Hour)
	if stats.SessionsCleaned != 1 {
		t.Fatalf("cleaned sessions = %d, want 1", stats.SessionsCleaned)
	}
	assertExists(t, outsidePath)
	assertNotExists(t, validPath)
	if _, found := store.finished[1]; found {
		t.Fatal("unsafe session was finalized")
	}
	if got := store.finished[2]; got != model.UploadSessionStatusFailed {
		t.Fatalf("safe session status = %q, want failed", got)
	}
}

func TestCleanupV2BatchHardCapsSessionWork(t *testing.T) {
	sessions := make([]model.UploadSession, v2CleanupBatchSize+1)
	for i := range sessions {
		sessions[i] = model.UploadSession{ID: uint64(i + 1), Status: model.UploadSessionStatusFailed}
	}
	store := &fakeV2CleanupStore{sessions: sessions, ignoreLimit: true, rejectClaim: true}
	guard := &stagingGuard{rootPath: t.TempDir()}
	stats := cleanupV2Batch(context.Background(), store, guard, time.Now(), time.Hour)

	if store.listLimit != v2CleanupBatchSize {
		t.Fatalf("store limit = %d, want %d", store.listLimit, v2CleanupBatchSize)
	}
	if stats.SessionsExamined != v2CleanupBatchSize || store.claims != v2CleanupBatchSize {
		t.Fatalf("batch exceeded limit: stats=%+v claims=%d", stats, store.claims)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
}

func makeStagingDir(t *testing.T, parent, name string) string {
	t.Helper()
	path := filepath.Join(parent, name)
	mustMkdir(t, path)
	return path
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be absent, got %v", path, err)
	}
}
