// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// fakeRateLimitStore counts INCR calls per key in memory, so the limiters can
// be tested without a live Redis.
type fakeRateLimitStore struct {
	counts map[string]int64
	err    error
}

func newFakeRateLimitStore() *fakeRateLimitStore {
	return &fakeRateLimitStore{counts: make(map[string]int64)}
}

func (f *fakeRateLimitStore) Incr(_ context.Context, key string) *redis.IntCmd {
	if f.err != nil {
		return redis.NewIntResult(0, f.err)
	}
	f.counts[key]++
	return redis.NewIntResult(f.counts[key], nil)
}

func (f *fakeRateLimitStore) Expire(_ context.Context, _ string, _ time.Duration) *redis.BoolCmd {
	return redis.NewBoolResult(true, nil)
}

func runRateLimiter(t *testing.T, limiter gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/public/client-errors", nil)
	ctx.Request.RemoteAddr = "192.0.2.7:1234"
	limiter(ctx)
	return recorder
}

func decodeRateLimitCode(t *testing.T, recorder *httptest.ResponseRecorder) int {
	t.Helper()
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	return body.Code
}

func TestClientErrorLimitAllowsTwentyReportsPerWindow(t *testing.T) {
	store := newFakeRateLimitStore()
	limiter := newRateLimitMiddlewareWithStore(store)

	for attempt := 1; attempt <= clientErrorRateLimit; attempt++ {
		if recorder := runRateLimiter(t, limiter.ClientErrorLimit()); recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want 200", attempt, recorder.Code)
		}
	}

	recorder := runRateLimiter(t, limiter.ClientErrorLimit())
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want 60", got)
	}
	if code := decodeRateLimitCode(t, recorder); code != 2091 {
		t.Fatalf("code = %d, want 2091", code)
	}
	if _, ok := store.counts["client-error:ip:192.0.2.7"]; !ok {
		t.Fatalf("rate-limit key is not namespaced per IP: %#v", store.counts)
	}
}

func TestClientErrorLimitFailsClosedWhenRedisIsUnavailable(t *testing.T) {
	store := newFakeRateLimitStore()
	store.err = errors.New("redis down")
	limiter := newRateLimitMiddlewareWithStore(store)

	recorder := runRateLimiter(t, limiter.ClientErrorLimit())
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", recorder.Code, recorder.Body.String())
	}
	if code := decodeRateLimitCode(t, recorder); code != 1002 {
		t.Fatalf("code = %d, want 1002", code)
	}
}

func TestLoginLimitAllowsFiveAttemptsPerWindow(t *testing.T) {
	store := newFakeRateLimitStore()
	limiter := newRateLimitMiddlewareWithStore(store)

	for attempt := 1; attempt <= 5; attempt++ {
		if recorder := runRateLimiter(t, limiter.LoginLimit()); recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want 200", attempt, recorder.Code)
		}
	}

	recorder := runRateLimiter(t, limiter.LoginLimit())
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", recorder.Code)
	}
	if code := decodeRateLimitCode(t, recorder); code != 2008 {
		t.Fatalf("code = %d, want 2008", code)
	}
	if _, ok := store.counts["login:ip:192.0.2.7"]; !ok {
		t.Fatalf("login rate-limit key missing: %#v", store.counts)
	}
}

func TestBootstrapLimitAllowsTenAttemptsPerWindow(t *testing.T) {
	store := newFakeRateLimitStore()
	limiter := newRateLimitMiddlewareWithStore(store)

	for attempt := 1; attempt <= 10; attempt++ {
		if recorder := runRateLimiter(t, limiter.BootstrapLimit()); recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want 200", attempt, recorder.Code)
		}
	}

	recorder := runRateLimiter(t, limiter.BootstrapLimit())
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", recorder.Code)
	}
	if got := recorder.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want 60", got)
	}
	if code := decodeRateLimitCode(t, recorder); code != 2090 {
		t.Fatalf("code = %d, want 2090", code)
	}
}
