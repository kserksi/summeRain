// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
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

func TestApplyImageCacheHeadersForPublicImagesIsAggressivelyCached(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	applyImageCacheHeaders(c, false)

	headers := w.Header()
	// Public images are immutable (unique_link never changes, content never modified).
	// Cache aggressively at every layer: browser, nginx, Cloudflare/CDN.
	if got := headers.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q, want public, max-age=31536000, immutable", got)
	}
	// No legacy anti-cache headers for public images
	if got := headers.Get("Pragma"); got != "" {
		t.Fatalf("Pragma = %q, want empty for public images", got)
	}
	if got := headers.Get("Expires"); got != "" {
		t.Fatalf("Expires = %q, want empty for public images", got)
	}
	// nginx edge cache (1 year)
	if got := headers.Get("X-Accel-Expires"); got != "31536000" {
		t.Fatalf("X-Accel-Expires = %q, want 31536000", got)
	}
	// CDN / Cloudflare surrogate cache (1 year)
	if got := headers.Get("Surrogate-Control"); got != "max-age=31536000" {
		t.Fatalf("Surrogate-Control = %q, want max-age=31536000", got)
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

type staticConfigReader struct {
	configs []model.SystemConfig
}

func (r staticConfigReader) FindAll() ([]model.SystemConfig, error) {
	return r.configs, nil
}
