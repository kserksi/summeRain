// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

const (
	applicationShutdownTimeout = 30 * time.Second
	httpShutdownTimeout        = 18 * time.Second
	workerSoftDrainTimeout     = 5 * time.Second
	workerHardStopTimeout      = 5 * time.Second
	resourceCloseTimeout       = 2 * time.Second
)

type readinessGate struct {
	draining atomic.Bool
}

func (g *readinessGate) BeginDrain() {
	g.draining.Store(true)
}

func (g *readinessGate) IsDraining() bool {
	return g.draining.Load()
}

type lifecycleServer interface {
	Shutdown(context.Context) error
	Close() error
}

type lifecycleWorkers interface {
	BeginDrain()
}

type lifecycleTimeouts struct {
	total      time.Duration
	http       time.Duration
	workerSoft time.Duration
	workerHard time.Duration
	resources  time.Duration
}

type lifecycleCoordinator struct {
	gate       *readinessGate
	server     lifecycleServer
	workers    lifecycleWorkers
	workerDone <-chan struct{}
	hardCancel context.CancelFunc
	closeRedis func() error
	closeSQL   func() error
	timeouts   lifecycleTimeouts
}

func productionLifecycleTimeouts() lifecycleTimeouts {
	return lifecycleTimeouts{
		total:      applicationShutdownTimeout,
		http:       httpShutdownTimeout,
		workerSoft: workerSoftDrainTimeout,
		workerHard: workerHardStopTimeout,
		resources:  resourceCloseTimeout,
	}
}

func (c *lifecycleCoordinator) Shutdown(ctx context.Context) error {
	overallCtx, overallCancel := context.WithTimeout(ctx, c.timeouts.total)
	defer overallCancel()

	c.gate.BeginDrain()

	var shutdownErr error
	httpCtx, httpCancel := context.WithTimeout(overallCtx, c.timeouts.http)
	if err := c.server.Shutdown(httpCtx); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("shutdown HTTP server: %w", err))
		if closeErr := c.server.Close(); closeErr != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("force close HTTP server: %w", closeErr))
		}
	}
	httpCancel()

	// Producers must finish before consumers stop claiming. An in-flight upload
	// may enqueue durable publish or outbox work at the end of its HTTP request.
	c.workers.BeginDrain()
	softStopped := waitForWorkerStop(overallCtx, c.workerDone, c.timeouts.workerSoft)
	c.hardCancel()
	if !softStopped {
		if !waitForWorkerStop(overallCtx, c.workerDone, c.timeouts.workerHard) {
			shutdownErr = errors.Join(shutdownErr, errors.New("workers did not stop before the shutdown deadline"))
		}
	}

	resourceCtx, resourceCancel := context.WithTimeout(overallCtx, c.timeouts.resources)
	shutdownErr = errors.Join(shutdownErr, closeLifecycleResources(resourceCtx, c.closeRedis, c.closeSQL))
	resourceCancel()

	return shutdownErr
}

func closeLifecycleResources(ctx context.Context, closeRedis, closeSQL func() error) error {
	done := make(chan error, 1)
	go func() {
		var closeErr error
		if err := closeRedis(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close Redis: %w", err))
		}
		if err := closeSQL(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close SQL pool: %w", err))
		}
		done <- closeErr
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("close runtime resources: %w", ctx.Err())
	}
}

func waitForWorkerStop(ctx context.Context, done <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	case <-timer.C:
		return false
	}
}

func unexpectedHTTPServerError(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	if err == nil {
		return errors.New("HTTP server stopped unexpectedly")
	}
	return fmt.Errorf("HTTP server failed: %w", err)
}
