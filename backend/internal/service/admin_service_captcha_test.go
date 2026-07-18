// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"net/http"
	"testing"
)

func TestValidateCaptchaConfigUpdateRejectsGeetestWithCrossOriginIsolation(t *testing.T) {
	err := validateCaptchaConfigUpdate([]ConfigUpdateItem{{
		Key:   "captcha_provider",
		Value: " GeeTest_V4 ",
	}}, true)
	if err == nil {
		t.Fatal("expected geetest_v4 to be rejected while cross-origin isolation is enabled")
	}
	if err.Code != 3006 || err.HTTP != http.StatusBadRequest {
		t.Fatalf("unexpected application error: %#v", err)
	}
}

func TestValidateCaptchaConfigUpdateAllowsCompatibleChanges(t *testing.T) {
	tests := []struct {
		name      string
		items     []ConfigUpdateItem
		isolation bool
	}{
		{name: "turnstile with isolation", items: []ConfigUpdateItem{{Key: "captcha_provider", Value: "turnstile"}}, isolation: true},
		{name: "geetest without isolation", items: []ConfigUpdateItem{{Key: "captcha_provider", Value: "geetest_v4"}}, isolation: false},
		{name: "unrelated config", items: []ConfigUpdateItem{{Key: "site_name", Value: "summeRain"}}, isolation: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateCaptchaConfigUpdate(tt.items, tt.isolation); err != nil {
				t.Fatalf("unexpected error: %#v", err)
			}
		})
	}
}
