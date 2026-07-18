// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/service"
	"gorm.io/gorm"
)

const (
	persistentReconcileInterval             = 10 * time.Minute
	persistentReconcileContinuationInterval = time.Minute
	persistentReconcileBatchSize            = 100
	persistentReconcileScanSize             = 500
	persistentReconcileRunBudget            = 5 * time.Second
	persistentReconcileScanBudget           = time.Second
	persistentReconcileReadSize             = 32
	persistentReconcileMaxDepth             = 64
)

type persistentV2ScanPage struct {
	paths          []string
	entriesVisited int
	complete       bool
}

type persistentV2ScanFrame struct {
	path     string
	relative string
	dir      *os.File
	entries  []os.DirEntry
	next     int
	eof      bool
}

// persistentV2Scanner keeps directory streams open between bounded scans. A
// filepath.WalkDir cursor still walks from the root on every call and ReadDir
// materializes an entire high-fanout directory, so it becomes both quadratic
// over a full pass and unbounded in memory as storage grows.
type persistentV2Scanner struct {
	rootPath string
	stack    []*persistentV2ScanFrame
	pending  *persistentV2ScanPage
	scanTime time.Duration
}

func newPersistentV2Scanner(storageRoot string) *persistentV2Scanner {
	if strings.TrimSpace(storageRoot) == "" {
		return &persistentV2Scanner{}
	}
	return &persistentV2Scanner{
		rootPath: filepath.Join(storageRoot, "v2"),
		scanTime: persistentReconcileScanBudget,
	}
}

func (s *persistentV2Scanner) Close() error {
	var closeErr error
	for index := len(s.stack) - 1; index >= 0; index-- {
		if err := s.stack[index].dir.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	s.stack = nil
	s.pending = nil
	return closeErr
}

func (s *persistentV2Scanner) CommitPage() {
	s.pending = nil
}

func (s *persistentV2Scanner) CommitPageRemainder(remaining []string) {
	if s.pending == nil || len(remaining) == 0 {
		s.CommitPage()
		return
	}
	s.pending.paths = remaining
}

func (s *persistentV2Scanner) NextPage(ctx context.Context, cutoff time.Time) (*persistentV2ScanPage, error) {
	if s.pending != nil {
		return s.pending, nil
	}
	page := &persistentV2ScanPage{paths: make([]string, 0, persistentReconcileScanSize)}
	if err := s.ensureRoot(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			page.complete = true
			s.pending = page
			return page, nil
		}
		return nil, err
	}

	scanTime := s.scanTime
	if scanTime <= 0 {
		scanTime = persistentReconcileScanBudget
	}
	scanDeadline := time.Now().Add(scanTime)
	for page.entriesVisited < persistentReconcileScanSize && time.Now().Before(scanDeadline) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(s.stack) == 0 {
			page.complete = true
			break
		}

		frame := s.stack[len(s.stack)-1]
		entry, err := frame.nextEntry()
		if errors.Is(err, io.EOF) {
			if closeErr := frame.dir.Close(); closeErr != nil {
				return nil, closeErr
			}
			s.stack = s.stack[:len(s.stack)-1]
			continue
		}
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				_ = frame.dir.Close()
				s.stack = s.stack[:len(s.stack)-1]
				continue
			}
			return nil, err
		}
		page.entriesVisited++

		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(frame.path, entry.Name())
		relative := filepath.Join(frame.relative, entry.Name())
		if entry.IsDir() {
			// Production V2 paths are at most three directories below v2. The
			// cap prevents malformed trees from consuming unbounded descriptors.
			if len(s.stack) >= persistentReconcileMaxDepth {
				continue
			}
			child, openErr := openPersistentV2ScanFrame(path, relative)
			if openErr != nil {
				if errors.Is(openErr, os.ErrNotExist) {
					continue
				}
				return nil, openErr
			}
			s.stack = append(s.stack, child)
			continue
		}
		if !entry.Type().IsRegular() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			if errors.Is(infoErr, os.ErrNotExist) {
				continue
			}
			return nil, infoErr
		}
		if !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
			continue
		}
		page.paths = append(page.paths, filepath.Clean(relative))
	}

	s.pending = page
	return page, nil
}

func (s *persistentV2Scanner) ensureRoot() error {
	if len(s.stack) > 0 {
		return nil
	}
	frame, err := openPersistentV2ScanFrame(s.rootPath, "v2")
	if err != nil {
		return err
	}
	s.stack = append(s.stack, frame)
	return nil
}

func openPersistentV2ScanFrame(path, relative string) (*persistentV2ScanFrame, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, os.ErrNotExist
	}
	dir, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &persistentV2ScanFrame{path: path, relative: relative, dir: dir}, nil
}

func (f *persistentV2ScanFrame) nextEntry() (os.DirEntry, error) {
	if f.next < len(f.entries) {
		entry := f.entries[f.next]
		f.next++
		return entry, nil
	}
	if f.eof {
		return nil, io.EOF
	}

	entries, err := f.dir.ReadDir(persistentReconcileReadSize)
	f.entries = entries
	f.next = 0
	f.eof = errors.Is(err, io.EOF)
	if err != nil && !f.eof {
		return nil, err
	}
	if len(f.entries) == 0 {
		return nil, io.EOF
	}
	entry := f.entries[0]
	f.next = 1
	return entry, nil
}

// queuePersistentV2Orphans closes the crash window between an atomic rename
// and the following database commit. It only queues old, currently
// unreferenced files; the outbox worker performs a second locked reference
// check before removal.
func queuePersistentV2Orphans(ctx context.Context, db *gorm.DB, scanner *persistentV2Scanner, now time.Time, sessionTTL time.Duration) (int, bool, error) {
	if db == nil || scanner == nil || strings.TrimSpace(scanner.rootPath) == "" {
		return 0, true, nil
	}
	runCtx, cancel := context.WithTimeout(ctx, persistentReconcileRunBudget)
	defer cancel()

	minimumAge := 2 * sessionTTL
	if minimumAge < time.Hour {
		minimumAge = time.Hour
	}
	page, err := scanner.NextPage(runCtx, now.Add(-minimumAge))
	if err != nil {
		return 0, false, err
	}
	candidates := page.paths
	if len(candidates) == 0 {
		scanner.CommitPage()
		return 0, page.complete, nil
	}

	referenced := make(map[string]struct{}, len(candidates))
	var variantPaths []string
	if err := db.WithContext(runCtx).Model(&model.ImageVariant{}).
		Where("storage_path IN ?", candidates).Pluck("storage_path", &variantPaths).Error; err != nil {
		return 0, false, err
	}
	for _, path := range variantPaths {
		referenced[filepath.Clean(path)] = struct{}{}
	}
	for _, column := range []string{"original_path", "thumbnail_path", "processed_path"} {
		var paths []string
		if err := db.WithContext(runCtx).Model(&model.ImageFile{}).
			Where(column+" IN ?", candidates).Pluck(column, &paths).Error; err != nil {
			return 0, false, err
		}
		for _, path := range paths {
			referenced[filepath.Clean(path)] = struct{}{}
		}
	}

	paths, remaining := nextPersistentV2OrphanBatch(candidates, referenced)
	if len(paths) == 0 {
		scanner.CommitPage()
		return 0, page.complete, nil
	}
	hasher := sha256.New()
	for _, path := range paths {
		hasher.Write([]byte(path))
		hasher.Write([]byte{0})
	}
	dedupe := "storage-reconcile:" + now.UTC().Format("20060102") + ":" + hex.EncodeToString(hasher.Sum(nil))[:16]
	err = db.WithContext(runCtx).Transaction(func(tx *gorm.DB) error {
		return service.EnqueueStorageDelete(tx, "storage", "v2", dedupe, paths, now)
	})
	if err != nil {
		return 0, false, err
	}
	scanner.CommitPageRemainder(remaining)
	return len(paths), page.complete && len(remaining) == 0, nil
}

func nextPersistentV2OrphanBatch(candidates []string, referenced map[string]struct{}) ([]string, []string) {
	paths := make([]string, 0, persistentReconcileBatchSize)
	remaining := make([]string, 0, max(0, len(candidates)-persistentReconcileBatchSize))
	for _, candidate := range candidates {
		if _, exists := referenced[candidate]; exists {
			continue
		}
		if len(paths) < persistentReconcileBatchSize {
			paths = append(paths, candidate)
		} else {
			remaining = append(remaining, candidate)
		}
	}
	return paths, remaining
}
