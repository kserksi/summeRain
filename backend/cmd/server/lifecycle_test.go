// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type lifecycleRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *lifecycleRecorder) add(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *lifecycleRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

type fakeLifecycleServer struct {
	recorder    *lifecycleRecorder
	shutdownErr error
	closeErr    error
}

func (s *fakeLifecycleServer) Shutdown(context.Context) error {
	s.recorder.add("http-shutdown")
	return s.shutdownErr
}

func (s *fakeLifecycleServer) Close() error {
	s.recorder.add("http-close")
	return s.closeErr
}

type fakeLifecycleWorkers struct {
	recorder *lifecycleRecorder
	onDrain  func()
}

func (w *fakeLifecycleWorkers) BeginDrain() {
	w.recorder.add("worker-drain")
	if w.onDrain != nil {
		w.onDrain()
	}
}

func TestLifecycleShutdownOrdersGracefulStages(t *testing.T) {
	recorder := &lifecycleRecorder{}
	gate := &readinessGate{}
	workerDone := make(chan struct{})
	server := &fakeLifecycleServer{recorder: recorder}
	workers := &fakeLifecycleWorkers{recorder: recorder, onDrain: func() {
		recorder.add("worker-done")
		close(workerDone)
	}}
	coordinator := lifecycleCoordinator{
		gate:       gate,
		server:     server,
		workers:    workers,
		workerDone: workerDone,
		hardCancel: func() { recorder.add("hard-cancel") },
		closeRedis: func() error { recorder.add("redis-close"); return nil },
		closeSQL:   func() error { recorder.add("sql-close"); return nil },
		timeouts: lifecycleTimeouts{
			total: time.Second, http: time.Second, workerSoft: time.Second, workerHard: time.Second, resources: time.Second,
		},
	}

	if err := coordinator.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if !gate.IsDraining() {
		t.Fatal("readiness gate was not marked draining")
	}
	want := []string{"http-shutdown", "worker-drain", "worker-done", "hard-cancel", "redis-close", "sql-close"}
	if got := recorder.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("shutdown events = %v, want %v", got, want)
	}
}

func TestLifecycleShutdownForceCancelsStuckWorkers(t *testing.T) {
	recorder := &lifecycleRecorder{}
	workerDone := make(chan struct{})
	coordinator := lifecycleCoordinator{
		gate:       &readinessGate{},
		server:     &fakeLifecycleServer{recorder: recorder},
		workers:    &fakeLifecycleWorkers{recorder: recorder},
		workerDone: workerDone,
		hardCancel: func() {
			recorder.add("hard-cancel")
			close(workerDone)
		},
		closeRedis: func() error { recorder.add("redis-close"); return nil },
		closeSQL:   func() error { recorder.add("sql-close"); return nil },
		timeouts: lifecycleTimeouts{
			total: time.Second, http: 100 * time.Millisecond, workerSoft: time.Millisecond, workerHard: 100 * time.Millisecond, resources: 100 * time.Millisecond,
		},
	}

	if err := coordinator.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	want := []string{"http-shutdown", "worker-drain", "hard-cancel", "redis-close", "sql-close"}
	if got := recorder.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("shutdown events = %v, want %v", got, want)
	}
}

func TestLifecycleShutdownBoundsHardStopAndStillClosesResources(t *testing.T) {
	recorder := &lifecycleRecorder{}
	coordinator := lifecycleCoordinator{
		gate:       &readinessGate{},
		server:     &fakeLifecycleServer{recorder: recorder},
		workers:    &fakeLifecycleWorkers{recorder: recorder},
		workerDone: make(chan struct{}),
		hardCancel: func() { recorder.add("hard-cancel") },
		closeRedis: func() error { recorder.add("redis-close"); return nil },
		closeSQL:   func() error { recorder.add("sql-close"); return nil },
		timeouts: lifecycleTimeouts{
			total: time.Second, http: 100 * time.Millisecond, workerSoft: time.Millisecond, workerHard: time.Millisecond, resources: 100 * time.Millisecond,
		},
	}

	err := coordinator.Shutdown(context.Background())
	if err == nil || !strings.Contains(err.Error(), "workers did not stop") {
		t.Fatalf("Shutdown() error = %v, want bounded worker-stop error", err)
	}
	want := []string{"http-shutdown", "worker-drain", "hard-cancel", "redis-close", "sql-close"}
	if got := recorder.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("shutdown events = %v, want %v", got, want)
	}
}

func TestLifecycleShutdownForceClosesHTTPServer(t *testing.T) {
	recorder := &lifecycleRecorder{}
	workerDone := make(chan struct{})
	close(workerDone)
	coordinator := lifecycleCoordinator{
		gate:       &readinessGate{},
		server:     &fakeLifecycleServer{recorder: recorder, shutdownErr: context.DeadlineExceeded},
		workers:    &fakeLifecycleWorkers{recorder: recorder},
		workerDone: workerDone,
		hardCancel: func() {},
		closeRedis: func() error { return nil },
		closeSQL:   func() error { return nil },
		timeouts: lifecycleTimeouts{
			total: time.Second, http: time.Second, workerSoft: time.Second, workerHard: time.Second, resources: time.Second,
		},
	}

	err := coordinator.Shutdown(context.Background())
	if err == nil || !strings.Contains(err.Error(), "shutdown HTTP server") {
		t.Fatalf("Shutdown() error = %v, want HTTP shutdown error", err)
	}
	if got := recorder.snapshot(); !reflect.DeepEqual(got, []string{"http-shutdown", "http-close", "worker-drain"}) {
		t.Fatalf("HTTP shutdown events = %v", got)
	}
}

func TestLifecycleShutdownBoundsResourceClose(t *testing.T) {
	recorder := &lifecycleRecorder{}
	workerDone := make(chan struct{})
	close(workerDone)
	releaseClose := make(chan struct{})
	t.Cleanup(func() { close(releaseClose) })
	coordinator := lifecycleCoordinator{
		gate:       &readinessGate{},
		server:     &fakeLifecycleServer{recorder: recorder},
		workers:    &fakeLifecycleWorkers{recorder: recorder},
		workerDone: workerDone,
		hardCancel: func() {},
		closeRedis: func() error {
			<-releaseClose
			return nil
		},
		closeSQL: func() error { return nil },
		timeouts: lifecycleTimeouts{
			total: 10 * time.Millisecond, http: time.Millisecond, workerSoft: time.Millisecond, workerHard: time.Millisecond, resources: 5 * time.Millisecond,
		},
	}

	err := coordinator.Shutdown(context.Background())
	if err == nil || !strings.Contains(err.Error(), "close runtime resources") {
		t.Fatalf("Shutdown() error = %v, want bounded resource-close error", err)
	}
}

func TestReadinessGateBecomesDraining(t *testing.T) {
	gate := &readinessGate{}
	if gate.IsDraining() {
		t.Fatal("new readiness gate must accept traffic")
	}
	gate.BeginDrain()
	gate.BeginDrain()
	if !gate.IsDraining() {
		t.Fatal("readiness gate did not remain draining")
	}
}

func TestUnexpectedHTTPServerError(t *testing.T) {
	if err := unexpectedHTTPServerError(http.ErrServerClosed); err != nil {
		t.Fatalf("ErrServerClosed = %v, want nil", err)
	}
	if err := unexpectedHTTPServerError(nil); err == nil {
		t.Fatal("nil listener result must be treated as unexpected")
	}
	want := errors.New("bind failed")
	if err := unexpectedHTTPServerError(want); !errors.Is(err, want) {
		t.Fatalf("listener error = %v, want wrapped bind failure", err)
	}
}

func TestProductionLifecycleTimeoutsFitApplicationBudget(t *testing.T) {
	timeouts := productionLifecycleTimeouts()
	if got := timeouts.http + timeouts.workerSoft + timeouts.workerHard + timeouts.resources; got != timeouts.total {
		t.Fatalf("stage budget = %s, total = %s", got, timeouts.total)
	}
}
