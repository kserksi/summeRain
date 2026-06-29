// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/summerain/image-gallery/internal/config"
	"github.com/summerain/image-gallery/internal/model"
	"github.com/summerain/image-gallery/internal/service"
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

func TestApplyImageCacheHeadersForPublicImagesRequiresRevalidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	applyImageCacheHeaders(c, false)

	headers := w.Header()
	if got := headers.Get("Cache-Control"); got != "no-cache, must-revalidate" {
		t.Fatalf("Cache-Control = %q, want no-cache, must-revalidate", got)
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
}

type staticConfigReader struct {
	configs []model.SystemConfig
}

func (r staticConfigReader) FindAll() ([]model.SystemConfig, error) {
	return r.configs, nil
}
