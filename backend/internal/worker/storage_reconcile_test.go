// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentV2ScannerBoundsPagesAndDoesNotRescan(t *testing.T) {
	root := t.TempDir()
	v2Root := filepath.Join(root, "v2")
	if err := os.MkdirAll(v2Root, 0700); err != nil {
		t.Fatal(err)
	}
	const fileCount = persistentReconcileScanSize*2 + 37
	old := time.Now().Add(-2 * time.Hour)
	for index := 0; index < fileCount; index++ {
		path := filepath.Join(v2Root, fmt.Sprintf("%04d.webp", index))
		if err := os.WriteFile(path, []byte("image"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}

	scanner := newPersistentV2Scanner(root)
	scanner.scanTime = time.Hour
	t.Cleanup(func() { _ = scanner.Close() })
	seen := make(map[string]struct{}, fileCount)
	visited := 0
	pages := 0
	for {
		page, err := scanner.NextPage(context.Background(), time.Now().Add(-time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if page.entriesVisited > persistentReconcileScanSize {
			t.Fatalf("page visited %d entries, limit %d", page.entriesVisited, persistentReconcileScanSize)
		}
		if len(page.paths) > persistentReconcileScanSize {
			t.Fatalf("page retained %d candidates, limit %d", len(page.paths), persistentReconcileScanSize)
		}
		visited += page.entriesVisited
		for _, path := range page.paths {
			if _, duplicate := seen[path]; duplicate {
				t.Fatalf("path was rescanned: %s", path)
			}
			seen[path] = struct{}{}
		}
		complete := page.complete
		scanner.CommitPage()
		if complete {
			break
		}
	}

	if pages != 3 {
		t.Fatalf("pages = %d, want 3", pages)
	}
	if visited != fileCount {
		t.Fatalf("visited entries = %d, want %d", visited, fileCount)
	}
	if len(seen) != fileCount {
		t.Fatalf("unique candidates = %d, want %d", len(seen), fileCount)
	}
}

func TestPersistentV2ScannerReplaysUncommittedPage(t *testing.T) {
	root := t.TempDir()
	v2Root := filepath.Join(root, "v2")
	if err := os.MkdirAll(v2Root, 0700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	for index := 0; index < persistentReconcileScanSize+1; index++ {
		path := filepath.Join(v2Root, fmt.Sprintf("%04d.webp", index))
		if err := os.WriteFile(path, []byte("image"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}

	scanner := newPersistentV2Scanner(root)
	scanner.scanTime = time.Hour
	t.Cleanup(func() { _ = scanner.Close() })
	first, err := scanner.NextPage(context.Background(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	second, err := scanner.NextPage(context.Background(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("uncommitted page was not replayed")
	}
	if first.complete || len(second.paths) != persistentReconcileScanSize {
		t.Fatalf("unexpected replayed page: %+v", second)
	}

	scanner.CommitPage()
	next, err := scanner.NextPage(context.Background(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !next.complete || len(next.paths) != 1 {
		t.Fatalf("scan did not continue after commit: %+v", next)
	}
}

func TestPersistentV2ScannerMissingRootCompletesWithoutWork(t *testing.T) {
	scanner := newPersistentV2Scanner(t.TempDir())
	t.Cleanup(func() { _ = scanner.Close() })
	page, err := scanner.NextPage(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !page.complete || page.entriesVisited != 0 || len(page.paths) != 0 {
		t.Fatalf("unexpected missing-root page: %+v", page)
	}
}

func TestPersistentV2ReconcilePageDrainsWithoutSkippingCandidates(t *testing.T) {
	const candidateCount = persistentReconcileScanSize + 37
	paths := make([]string, 0, candidateCount)
	for index := 0; index < candidateCount; index++ {
		paths = append(paths, filepath.Join("v2", fmt.Sprintf("%04d.webp", index)))
	}

	scanner := &persistentV2Scanner{pending: &persistentV2ScanPage{
		paths:    paths,
		complete: true,
	}}
	queued := make([]string, 0, candidateCount)
	completed := false
	for tick := 0; scanner.pending != nil; tick++ {
		page, err := scanner.NextPage(context.Background(), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		batch, remaining := nextPersistentV2OrphanBatch(page.paths, nil)
		queued = append(queued, batch...)
		scanner.CommitPageRemainder(remaining)

		complete := page.complete && len(remaining) == 0
		if len(batch) > persistentReconcileBatchSize {
			t.Fatalf("tick %d queued %d paths, limit %d", tick, len(batch), persistentReconcileBatchSize)
		}
		if scanner.pending != nil && complete {
			t.Fatalf("tick %d reported completion with %d candidates pending", tick, len(scanner.pending.paths))
		}
		completed = complete
	}

	if !completed {
		t.Fatal("final batch did not report completion")
	}
	if len(queued) != candidateCount {
		t.Fatalf("queued paths = %d, want %d", len(queued), candidateCount)
	}
	for index, path := range queued {
		if path != paths[index] {
			t.Fatalf("queued path %d = %q, want %q", index, path, paths[index])
		}
	}
}
