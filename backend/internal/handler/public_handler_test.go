// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/imgproxy"
	"github.com/kserksi/summerain/internal/service"
)

func TestPublicConfigExposesProviderAndSiteKeyWithoutSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	handler := NewPublicHandler(nil, nil, nil, nil, "", service.NewPublicConfigService(staticConfigReader{
		configs: []model.SystemConfig{
			{ConfigKey: "recaptcha_site_key", ConfigValue: "public-site-key"},
			{ConfigKey: "recaptcha_secret_key", ConfigValue: "server-secret"},
		},
	}, config.CaptchaConfig{Provider: "recaptcha", Recaptcha: config.RecaptchaConfig{SiteKey: "public-site-key"}}, nil), nil, nil)

	handler.GetConfig(c)

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data := body["data"].(map[string]any)
	if data["captcha_provider"] != "recaptcha" {
		t.Fatalf("captcha_provider = %#v, want recaptcha", data["captcha_provider"])
	}
	if data["captcha_site_key"] != "public-site-key" {
		t.Fatalf("captcha_site_key = %#v, want public-site-key", data["captcha_site_key"])
	}
	if _, ok := data["recaptcha_secret_key"]; ok {
		t.Fatalf("public config leaked a secret")
	}
}

func TestApplyImageCacheHeadersForPublicImagesCapsCachingAtTenMinutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	applyImageCacheHeaders(c, false)

	headers := w.Header()
	if got := headers.Get("Cache-Control"); got != "public, max-age=600, s-maxage=600, must-revalidate" {
		t.Fatalf("Cache-Control = %q, want ten-minute public cache", got)
	}
	if got := headers.Get("Pragma"); got != "" {
		t.Fatalf("Pragma = %q, want empty for public images", got)
	}
	if got := headers.Get("Expires"); got != "" {
		t.Fatalf("Expires = %q, want empty for public images", got)
	}
	if got := headers.Get("X-Accel-Expires"); got != "600" {
		t.Fatalf("X-Accel-Expires = %q, want 600", got)
	}
	if got := headers.Get("Surrogate-Control"); got != "max-age=600" {
		t.Fatalf("Surrogate-Control = %q, want max-age=600", got)
	}
}

func TestApplyImageCacheHeadersForPrivateImagesDisablesStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	applyImageCacheHeaders(c, true)

	headers := w.Header()
	if got := headers.Get("Cache-Control"); got != "no-store, no-cache, must-revalidate, private" {
		t.Fatalf("Cache-Control = %q, want no-store, no-cache, must-revalidate, private", got)
	}
	if got := headers.Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", got)
	}
	if got := headers.Get("Expires"); got != "0" {
		t.Fatalf("Expires = %q, want 0", got)
	}
	if got := headers.Get("X-Accel-Expires"); got != "0" {
		t.Fatalf("X-Accel-Expires = %q, want 0", got)
	}
	if got := headers.Get("Surrogate-Control"); got != "no-store" {
		t.Fatalf("Surrogate-Control = %q, want no-store", got)
	}
}

func TestApplyV2ImageCacheHeadersCapsPublicCachingAtTenMinutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	applyV2ImageCacheHeaders(c, false)

	headers := w.Header()
	if got := headers.Get("Cache-Control"); got != "public, max-age=600, s-maxage=600, must-revalidate" {
		t.Fatalf("Cache-Control = %q, want ten-minute public cache", got)
	}
	if got := headers.Get("X-Accel-Expires"); got != "600" {
		t.Fatalf("X-Accel-Expires = %q, want 600", got)
	}
	if got := headers.Get("Surrogate-Control"); got != "max-age=600" {
		t.Fatalf("Surrogate-Control = %q, want max-age=600", got)
	}
}

func TestApplyV2ImageCacheHeadersKeepsPrivateImagesNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	applyV2ImageCacheHeaders(c, true)

	if got := w.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate, private" {
		t.Fatalf("Cache-Control = %q, want private no-store", got)
	}
}

func TestRestrictedV2VariantsAreNeverPubliclyCacheable(t *testing.T) {
	for _, kind := range []string{model.ImageVariantKindMaster, model.ImageVariantKindAdmin} {
		if !isRestrictedV2Variant(kind) {
			t.Fatalf("variant %q must require owner or admin access", kind)
		}
	}
	for _, kind := range []string{model.ImageVariantKindGallery, model.ImageVariantKindPublish} {
		if isRestrictedV2Variant(kind) {
			t.Fatalf("variant %q should remain available through the public alias", kind)
		}
	}
}

func TestCopyGeneratedImageRejectsResponsesOverTheLimit(t *testing.T) {
	var destination bytes.Buffer
	err := copyGeneratedImage(&destination, bytes.NewReader([]byte("12345")), 4)
	if !errors.Is(err, errGeneratedImageTooLarge) {
		t.Fatalf("copyGeneratedImage error = %v, want errGeneratedImageTooLarge", err)
	}
	if destination.Len() != 5 {
		t.Fatalf("copied bytes = %d, want limit plus one", destination.Len())
	}
}

func TestOpenStorageFileRejectsTraversalAndEscapingSymlinks(t *testing.T) {
	basePath := t.TempDir()
	insideDir := filepath.Join(basePath, "v2", "publish")
	if err := os.MkdirAll(insideDir, 0750); err != nil {
		t.Fatalf("create storage directory: %v", err)
	}
	insidePath := filepath.Join(insideDir, "image.webp")
	if err := os.WriteFile(insidePath, []byte("inside"), 0640); err != nil {
		t.Fatalf("write inside file: %v", err)
	}

	opened, err := openStorageFile(basePath, "v2/publish/image.webp")
	if err != nil {
		t.Fatalf("open inside file: %v", err)
	}
	data, readErr := io.ReadAll(opened.file)
	_ = opened.file.Close()
	if readErr != nil || string(data) != "inside" {
		t.Fatalf("read inside file = %q, %v", data, readErr)
	}
	if opened.relativePath != filepath.Join("v2", "publish", "image.webp") {
		t.Fatalf("resolved relative path = %q", opened.relativePath)
	}

	for _, storedPath := range []string{"../outside.webp", filepath.Join(basePath, "v2", "publish", "image.webp"), "."} {
		if _, err := openStorageFile(basePath, storedPath); !errors.Is(err, errUnsafeStoragePath) {
			t.Fatalf("openStorageFile(%q) error = %v, want unsafe path", storedPath, err)
		}
	}

	outsidePath := filepath.Join(t.TempDir(), "outside.webp")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0640); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	linkPath := filepath.Join(basePath, "escape.webp")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := openStorageFile(basePath, "escape.webp"); !errors.Is(err, errUnsafeStoragePath) {
		t.Fatalf("escaping symlink error = %v, want unsafe path", err)
	}
}

func TestDynamicImageSingleflightSharesFileUntilAllReadersRelease(t *testing.T) {
	gate := make(chan struct{})
	var gateOnce sync.Once
	releaseUpstream := func() { gateOnce.Do(func() { close(gate) }) }
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/webp")
		<-gate
		_, _ = w.Write([]byte("generated-image"))
	}))
	defer server.Close()
	defer releaseUpstream()
	handler := newDynamicTestHandler(t, server.URL)

	type loadResult struct {
		image   generatedImageFile
		release func()
		err     error
	}
	results := make(chan loadResult, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	path := "/q:80/f:webp/plain/local:///images/source.jpg"
	for range 2 {
		go func() {
			image, release, err := handler.loadDynamicImage(ctx, path)
			results <- loadResult{image: image, release: release, err: err}
		}()
	}
	waitForCondition(t, func() bool {
		return requests.Load() == 1 && dynamicReferenceCount(handler) == 2
	}, "two requests to join one generation")
	releaseUpstream()

	first := <-results
	second := <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("singleflight errors = %v, %v", first.err, second.err)
	}
	if requests.Load() != 1 {
		t.Fatalf("imgproxy requests = %d, want 1", requests.Load())
	}
	if first.image.path == "" || first.image.path != second.image.path {
		t.Fatalf("temporary paths = %q and %q, want the same file", first.image.path, second.image.path)
	}
	if first.image.contentType != "image/webp" {
		t.Fatalf("content type = %q, want image/webp", first.image.contentType)
	}
	first.release()
	if _, err := os.Stat(first.image.path); err != nil {
		t.Fatalf("temporary file removed while a reader remains: %v", err)
	}
	second.release()
	waitForCondition(t, func() bool {
		_, err := os.Stat(first.image.path)
		return errors.Is(err, os.ErrNotExist)
	}, "temporary file cleanup after the final reader")
}

func TestDynamicImageCancellationStopsUnobservedGeneration(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	handler := newDynamicTestHandler(t, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, err := handler.loadDynamicImage(ctx, "/q:80/f:webp/plain/local:///images/cancel.jpg")
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("imgproxy request did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("loadDynamicImage error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled request remained blocked")
	}
	waitForCondition(t, func() bool {
		return dynamicCallCount(handler) == 0 && len(handler.dynamicCapacity) == 0 && len(handler.dynamicSemaphore) == 0
	}, "canceled generation cleanup")
	waitForCondition(t, func() bool {
		matches, err := filepath.Glob(filepath.Join(handler.storageCfg.TempPath, "v1-dynamic-*.image"))
		return err == nil && len(matches) == 0
	}, "canceled temporary file cleanup")
}

func TestDynamicImageLeaderCancellationDoesNotCancelFollower(t *testing.T) {
	gate := make(chan struct{})
	var gateOnce sync.Once
	releaseUpstream := func() { gateOnce.Do(func() { close(gate) }) }
	started := make(chan struct{})
	upstreamCanceled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		close(started)
		w.Header().Set("Content-Type", "image/webp")
		select {
		case <-gate:
			_, _ = w.Write([]byte("generated-image"))
		case <-request.Context().Done():
			upstreamCanceled <- struct{}{}
		}
	}))
	defer server.Close()
	defer releaseUpstream()
	handler := newDynamicTestHandler(t, server.URL)
	path := "/q:80/f:webp/plain/local:///images/shared-cancel.jpg"

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderResult := make(chan error, 1)
	go func() {
		_, _, err := handler.loadDynamicImage(leaderCtx, path)
		leaderResult <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("leader did not start imgproxy request")
	}

	type followerResult struct {
		image   generatedImageFile
		release func()
		err     error
	}
	follower := make(chan followerResult, 1)
	go func() {
		image, release, err := handler.loadDynamicImage(context.Background(), path)
		follower <- followerResult{image: image, release: release, err: err}
	}()
	waitForCondition(t, func() bool { return dynamicReferenceCount(handler) == 2 }, "follower to join leader")
	cancelLeader()
	select {
	case err := <-leaderResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("leader error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled leader remained blocked")
	}
	select {
	case <-upstreamCanceled:
		t.Fatal("leader cancellation canceled generation needed by a follower")
	case <-time.After(50 * time.Millisecond):
	}

	releaseUpstream()
	select {
	case result := <-follower:
		if result.err != nil {
			t.Fatalf("follower failed after leader cancellation: %v", result.err)
		}
		if result.image.path == "" {
			t.Fatal("follower received no generated file")
		}
		result.release()
	case <-time.After(2 * time.Second):
		t.Fatal("follower did not receive shared generation")
	}
	waitForCondition(t, func() bool { return dynamicCallCount(handler) == 0 }, "shared call release")
}

func TestDynamicImageGenerationHasBoundedConcurrencyAndQueue(t *testing.T) {
	gate := make(chan struct{})
	var gateOnce sync.Once
	releaseUpstream := func() { gateOnce.Do(func() { close(gate) }) }
	var active atomic.Int32
	var maximum atomic.Int32
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		requests.Add(1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		w.Header().Set("Content-Type", "image/webp")
		<-gate
		_, _ = w.Write([]byte("generated-image"))
	}))
	defer server.Close()
	defer releaseUpstream()
	handler := newDynamicTestHandler(t, server.URL)

	type loadResult struct {
		release func()
		err     error
	}
	const admitted = v1DynamicGenerationConcurrency + v1DynamicGenerationQueueDepth
	results := make(chan loadResult, admitted)
	for index := range admitted {
		go func() {
			_, release, err := handler.loadDynamicImage(context.Background(), fmt.Sprintf("/q:80/f:webp/plain/local:///images/%d.jpg", index))
			results <- loadResult{release: release, err: err}
		}()
	}
	waitForCondition(t, func() bool {
		return dynamicCallCount(handler) == admitted && requests.Load() == v1DynamicGenerationConcurrency
	}, "generation workers and queue to fill")

	_, _, err := handler.loadDynamicImage(context.Background(), "/q:80/f:webp/plain/local:///images/overflow.jpg")
	if !errors.Is(err, errDynamicImageQueueFull) {
		t.Fatalf("overflow error = %v, want queue full", err)
	}
	if maximum.Load() > v1DynamicGenerationConcurrency {
		t.Fatalf("maximum concurrency = %d, limit = %d", maximum.Load(), v1DynamicGenerationConcurrency)
	}

	releaseUpstream()
	waitForCondition(t, func() bool { return len(results) == admitted }, "generated results to remain retained")
	if got := len(handler.dynamicCapacity); got != admitted {
		t.Fatalf("retained result capacity = %d, want %d", got, admitted)
	}
	_, _, err = handler.loadDynamicImage(context.Background(), "/q:80/f:webp/plain/local:///images/retained-overflow.jpg")
	if !errors.Is(err, errDynamicImageQueueFull) {
		t.Fatalf("retained overflow error = %v, want queue full", err)
	}
	for range admitted {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("admitted generation failed: %v", result.err)
			}
			result.release()
		case <-time.After(5 * time.Second):
			t.Fatal("admitted generation did not complete")
		}
	}
	waitForCondition(t, func() bool {
		return dynamicCallCount(handler) == 0 && len(handler.dynamicCapacity) == 0
	}, "all generated calls to release")
}

func newDynamicTestHandler(t *testing.T, imgproxyURL string) *PublicHandler {
	t.Helper()
	tempPath := filepath.Join(t.TempDir(), "dynamic")
	publicConfig := service.NewPublicConfigService(staticConfigReader{}, config.CaptchaConfig{}, nil)
	handler := NewPublicHandler(
		nil,
		&config.StorageConfig{TempPath: tempPath},
		nil,
		imgproxy.NewSigner("", "", ""),
		imgproxyURL,
		publicConfig,
		nil,
		nil,
	)
	handler.client = &http.Client{Timeout: 10 * time.Second}
	return handler
}

func dynamicCallCount(handler *PublicHandler) int {
	handler.dynamicImages.mu.Lock()
	defer handler.dynamicImages.mu.Unlock()
	return len(handler.dynamicImages.calls)
}

func dynamicReferenceCount(handler *PublicHandler) int {
	handler.dynamicImages.mu.Lock()
	defer handler.dynamicImages.mu.Unlock()
	for _, call := range handler.dynamicImages.calls {
		return call.references
	}
	return 0
}

func waitForCondition(t *testing.T, condition func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

type staticConfigReader struct {
	configs []model.SystemConfig
}

func (r staticConfigReader) FindAll() ([]model.SystemConfig, error) {
	return r.configs, nil
}
