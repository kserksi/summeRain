// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/config"
)

func TestRunControlPlaneCleanupDrainsFullBatchesRoundRobin(t *testing.T) {
	statements := []controlPlaneDelete{
		{name: "jobs", query: "jobs"},
		{name: "sessions", query: "sessions"},
	}
	responses := map[string][]int64{
		"jobs":     {100, 100, 17},
		"sessions": {0},
	}
	var calls []string
	run := runControlPlaneCleanup(
		context.Background(),
		statements,
		controlPlaneCleanupLimits{batchSize: 100, maxBatches: 5, timeBudget: time.Minute},
		time.Now,
		func(ctx context.Context, statement controlPlaneDelete, batchSize int) (int64, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("cleanup executor context has no time budget")
			}
			if batchSize != 100 {
				t.Fatalf("batch size = %d, want 100", batchSize)
			}
			calls = append(calls, statement.name)
			rows := responses[statement.name][0]
			responses[statement.name] = responses[statement.name][1:]
			return rows, nil
		},
	)

	if run.budgetExhausted {
		t.Fatal("cleanup unexpectedly exhausted its time budget")
	}
	if want := []string{"jobs", "sessions", "jobs", "jobs"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	if got := run.results[0]; got.batches != 3 || got.rowsAffected != 217 || got.capped || got.err != nil {
		t.Fatalf("jobs result = %+v", got)
	}
	if got := run.results[1]; got.batches != 1 || got.rowsAffected != 0 || got.capped || got.err != nil {
		t.Fatalf("sessions result = %+v", got)
	}
}

func TestRunControlPlaneCleanupCapsEveryStatement(t *testing.T) {
	statements := []controlPlaneDelete{{name: "first"}, {name: "second"}}
	calls := 0
	run := runControlPlaneCleanup(
		context.Background(),
		statements,
		controlPlaneCleanupLimits{batchSize: 25, maxBatches: 3, timeBudget: time.Minute},
		time.Now,
		func(context.Context, controlPlaneDelete, int) (int64, error) {
			calls++
			return 25, nil
		},
	)

	if calls != 6 {
		t.Fatalf("executor calls = %d, want 6", calls)
	}
	for _, result := range run.results {
		if result.batches != 3 || result.rowsAffected != 75 || !result.capped || result.err != nil {
			t.Fatalf("result = %+v", result)
		}
	}
}

func TestRunControlPlaneCleanupStopsAtTimeBudget(t *testing.T) {
	start := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	times := []time.Time{
		start,
		start,
		start.Add(time.Second),
		start.Add(2 * time.Second),
	}
	clockIndex := 0
	clock := func() time.Time {
		if clockIndex >= len(times) {
			return times[len(times)-1]
		}
		value := times[clockIndex]
		clockIndex++
		return value
	}
	var calls []string
	run := runControlPlaneCleanup(
		context.Background(),
		[]controlPlaneDelete{{name: "first"}, {name: "second"}},
		controlPlaneCleanupLimits{batchSize: 10, maxBatches: 10, timeBudget: 2 * time.Second},
		clock,
		func(_ context.Context, statement controlPlaneDelete, _ int) (int64, error) {
			calls = append(calls, statement.name)
			return 10, nil
		},
	)

	if !run.budgetExhausted {
		t.Fatal("cleanup did not report its exhausted time budget")
	}
	if want := []string{"first", "second"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestRunControlPlaneCleanupContinuesAfterStatementError(t *testing.T) {
	wantErr := errors.New("delete failed")
	var calls []string
	run := runControlPlaneCleanup(
		context.Background(),
		[]controlPlaneDelete{{name: "broken"}, {name: "healthy"}},
		controlPlaneCleanupLimits{batchSize: 10, maxBatches: 3, timeBudget: time.Minute},
		time.Now,
		func(_ context.Context, statement controlPlaneDelete, _ int) (int64, error) {
			calls = append(calls, statement.name)
			if statement.name == "broken" {
				return 0, wantErr
			}
			return 0, nil
		},
	)

	if want := []string{"broken", "healthy"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	if !errors.Is(run.results[0].err, wantErr) {
		t.Fatalf("broken result error = %v, want %v", run.results[0].err, wantErr)
	}
	if run.results[1].err != nil || run.results[1].batches != 1 {
		t.Fatalf("healthy result = %+v", run.results[1])
	}
}

func TestControlPlaneDeletesAreBoundedAndRetainDeadOutboxLonger(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	statements := controlPlaneDeletes(now)
	if len(statements) != 8 {
		t.Fatalf("statement count = %d, want 8", len(statements))
	}

	byName := make(map[string]controlPlaneDelete, len(statements))
	for _, statement := range statements {
		if !strings.Contains(statement.query, "ORDER BY id LIMIT ?") {
			t.Fatalf("cleanup query is not bounded: %q", statement.query)
		}
		byName[statement.name] = statement
	}
	accessTokens := byName["expired image access tokens"]
	if want := now.AddDate(0, 0, -7); !accessTokens.cutoff.Equal(want) {
		t.Fatalf("image token cutoff = %s, want %s", accessTokens.cutoff, want)
	}
	if !strings.Contains(accessTokens.query, "revoked_at IS NULL") {
		t.Fatalf("image token cleanup must retain revoked tokens: %q", accessTokens.query)
	}
	published := byName["published outbox events"]
	if want := now.AddDate(0, 0, -30); !published.cutoff.Equal(want) {
		t.Fatalf("published cutoff = %s, want %s", published.cutoff, want)
	}
	dead := byName["dead outbox events"]
	if want := now.AddDate(0, 0, -90); !dead.cutoff.Equal(want) {
		t.Fatalf("dead cutoff = %s, want %s", dead.cutoff, want)
	}
	if !strings.Contains(dead.query, "status = 'dead'") || !strings.Contains(dead.query, "available_at < ?") {
		t.Fatalf("dead outbox query does not use the indexed retention fields: %q", dead.query)
	}
	if !strings.Contains(dead.query, "event_type <> 'storage.file.delete'") {
		t.Fatalf("dead outbox cleanup can discard unresolved storage deletion intent: %q", dead.query)
	}
}

func TestCleanOrphanTempFilesCapsEachDirectoryScan(t *testing.T) {
	directory := t.TempDir()
	old := time.Now().Add(-2 * time.Hour)
	fileCount := orphanTempScanMaxEntries + 8
	for index := 0; index < fileCount; index++ {
		path := filepath.Join(directory, fmt.Sprintf("%04d.tmp", index))
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}

	manager := &Manager{Config: &config.Config{Storage: config.StorageConfig{TempPath: directory}}}
	manager.cleanOrphanTempFiles()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(entries), fileCount-orphanTempScanMaxEntries; got != want {
		t.Fatalf("remaining files after one bounded scan = %d, want %d", got, want)
	}

	manager.cleanOrphanTempFiles()
	entries, err = os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("remaining files after second scan = %d, want 0", len(entries))
	}
}
