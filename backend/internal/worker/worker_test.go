// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"bytes"
	"context"
	"log"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/service"
)

func TestManagerWorkersIncludesEveryConfiguredWorker(t *testing.T) {
	manager := &Manager{
		Config: &config.Config{
			Storage: config.StorageConfig{StagingPath: "/data/.staging"},
			ImageV2: config.ImageV2Config{
				Enabled:              true,
				WatermarkConcurrency: 2,
			},
		},
		V2: &service.V2UploadService{},
	}

	workers := manager.workers(make(chan struct{}))
	names := make([]string, 0, len(workers))
	for _, worker := range workers {
		names = append(names, worker.name)
	}
	want := []string{
		"heartbeat",
		"outbox",
		"v2-publish-0",
		"v2-publish-1",
		"v2-cleanup",
		"view-flusher",
		"cleanup",
		"user-deletion",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("worker names = %v, want %v", names, want)
	}
}

func TestSuperviseWorkerRestartsUnexpectedReturn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runs := 0
	waits := 0
	worker := managedWorker{
		name: "returning-worker",
		run: func(context.Context) {
			runs++
			if runs == 2 {
				cancel()
			}
		},
	}
	wait := func(_ context.Context, _ <-chan struct{}, delay time.Duration) bool {
		waits++
		if delay != workerRestartInitialBackoff {
			t.Fatalf("restart delay = %s, want %s", delay, workerRestartInitialBackoff)
		}
		return true
	}

	superviseWorker(ctx, nil, worker, workerRestartInitialBackoff, wait)

	if runs != 2 {
		t.Fatalf("worker runs = %d, want 2", runs)
	}
	if waits != 1 {
		t.Fatalf("restart waits = %d, want 1", waits)
	}
}

func TestSuperviseWorkerRecoversPanicAndLogsWorkerName(t *testing.T) {
	var logs bytes.Buffer
	originalOutput := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()
	log.SetOutput(&logs)
	log.SetFlags(0)
	log.SetPrefix("")
	defer func() {
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runs := 0
	worker := managedWorker{
		name: "panic-worker",
		run: func(context.Context) {
			runs++
			if runs == 1 {
				panic("test panic")
			}
			cancel()
		},
	}

	superviseWorker(
		ctx,
		nil,
		worker,
		workerRestartInitialBackoff,
		func(context.Context, <-chan struct{}, time.Duration) bool { return true },
	)

	if runs != 2 {
		t.Fatalf("worker runs = %d, want 2", runs)
	}
	if output := logs.String(); !strings.Contains(output, "[worker] panic-worker panicked: test panic") {
		t.Fatalf("panic log does not identify worker: %q", output)
	}
}

func TestSuperviseWorkerBackoffSequenceAndCap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runs := 0
	var delays []time.Duration
	worker := managedWorker{
		name: "deterministic-panic-worker",
		run: func(context.Context) {
			runs++
			if runs == 8 {
				cancel()
				return
			}
			panic("deterministic panic")
		},
	}

	superviseWorker(
		ctx,
		nil,
		worker,
		workerRestartInitialBackoff,
		func(_ context.Context, _ <-chan struct{}, delay time.Duration) bool {
			delays = append(delays, delay)
			return true
		},
	)

	want := []time.Duration{
		time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second,
		30 * time.Second,
	}
	if !reflect.DeepEqual(delays, want) {
		t.Fatalf("restart delays = %v, want %v", delays, want)
	}
}

func TestSuperviseWorkerDoesNotRestartAfterDrain(t *testing.T) {
	drain := make(chan struct{})
	runs := 0
	waits := 0
	worker := managedWorker{
		name: "draining-worker",
		run: func(context.Context) {
			runs++
			close(drain)
		},
	}

	superviseWorker(
		context.Background(),
		drain,
		worker,
		workerRestartInitialBackoff,
		func(context.Context, <-chan struct{}, time.Duration) bool {
			waits++
			return true
		},
	)

	if runs != 1 {
		t.Fatalf("worker runs = %d, want 1", runs)
	}
	if waits != 0 {
		t.Fatalf("restart waits = %d, want 0", waits)
	}
}

func TestWaitForWorkerRestartStopsOnDrain(t *testing.T) {
	drain := make(chan struct{})
	close(drain)
	if waitForWorkerRestart(context.Background(), drain, time.Hour) {
		t.Fatal("restart wait continued after drain")
	}
}

func TestRunSupervisedWorkersWaitsForEveryWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	entered := make(chan string, 2)
	firstRelease := make(chan struct{})
	secondRelease := make(chan struct{})
	workers := []managedWorker{
		{name: "first", run: func(context.Context) {
			entered <- "first"
			<-firstRelease
		}},
		{name: "second", run: func(context.Context) {
			entered <- "second"
			<-secondRelease
		}},
	}
	done := make(chan struct{})
	go func() {
		runSupervisedWorkers(
			ctx,
			nil,
			workers,
			workerRestartInitialBackoff,
			func(context.Context, <-chan struct{}, time.Duration) bool { return true },
		)
		close(done)
	}()

	for index := 0; index < len(workers); index++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("worker did not start")
		}
	}
	cancel()
	close(firstRelease)
	select {
	case <-done:
		t.Fatal("manager returned before every worker stopped")
	default:
	}
	close(secondRelease)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("manager did not return after every worker stopped")
	}
}

func TestManagerBeginDrainIsConcurrentAndIdempotent(t *testing.T) {
	manager := &Manager{}
	const callers = 16
	var wg sync.WaitGroup
	wg.Add(callers)
	for index := 0; index < callers; index++ {
		go func() {
			defer wg.Done()
			manager.BeginDrain()
		}()
	}
	wg.Wait()

	select {
	case <-manager.drainSignal():
	default:
		t.Fatal("drain signal is still open")
	}
}
