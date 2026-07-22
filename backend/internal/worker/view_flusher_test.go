// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunBoundedFinalViewFlushUsesFreshContext(t *testing.T) {
	timeout := 250 * time.Millisecond
	started := time.Now()
	called := false
	runBoundedFinalViewFlush(timeout, func(ctx context.Context) {
		called = true
		if err := ctx.Err(); err != nil {
			t.Fatalf("final flush context is already canceled: %v", err)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("final flush context has no deadline")
		}
		remaining := deadline.Sub(started)
		if remaining <= 0 || remaining > timeout+50*time.Millisecond {
			t.Fatalf("final flush deadline remaining=%s, want within %s", remaining, timeout)
		}
	})
	if !called {
		t.Fatal("final flush was not called")
	}
}

func TestRunBoundedFinalViewFlushRejectsInvalidInputs(t *testing.T) {
	called := false
	runBoundedFinalViewFlush(0, func(context.Context) { called = true })
	runBoundedFinalViewFlush(time.Second, nil)
	if called {
		t.Fatal("final flush ran with an invalid timeout")
	}
}

func TestViewFlushCleanupFitsWorkerHardStopBudget(t *testing.T) {
	// Keep this aligned with workerHardStopTimeout in cmd/server/lifecycle.go.
	const workerHardStopBudget = 5 * time.Second
	if got := viewFlushShutdownTimeout + viewCountRestoreTimeout; got >= workerHardStopBudget {
		t.Fatalf("view flush cleanup budget = %s, must be less than worker hard-stop budget %s", got, workerHardStopBudget)
	}
}

func TestUpdateViewCountWithRestoreReturnsCountAfterDBFailure(t *testing.T) {
	dbErr := errors.New("database unavailable")
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	var restoredKey string
	var restoredCount int64
	updateErr, restoreErr := updateViewCountWithRestore(
		parent,
		"views:42",
		"42",
		17,
		func(ctx context.Context, imageID string, count int64) error {
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("update context error=%v, want canceled", ctx.Err())
			}
			if imageID != "42" || count != 17 {
				t.Fatalf("update args=(%q, %d), want (42, 17)", imageID, count)
			}
			return dbErr
		},
		func(ctx context.Context, key string, count int64) error {
			if err := ctx.Err(); err != nil {
				t.Fatalf("restore inherited canceled context: %v", err)
			}
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > viewCountRestoreTimeout {
				t.Fatalf("restore deadline=%v, want within %s", deadline, viewCountRestoreTimeout)
			}
			restoredKey = key
			restoredCount = count
			return nil
		},
	)
	if !errors.Is(updateErr, dbErr) || restoreErr != nil {
		t.Fatalf("errors=(%v, %v), want (%v, nil)", updateErr, restoreErr, dbErr)
	}
	if restoredKey != "views:42" || restoredCount != 17 {
		t.Fatalf("restored=(%q, %d), want (views:42, 17)", restoredKey, restoredCount)
	}
}
