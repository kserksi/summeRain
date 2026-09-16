// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"net/http"
	"testing"
)

func TestValidateAdminWritableSystemConfigKeysAllowsBusinessConfig(t *testing.T) {
	err := validateAdminWritableSystemConfigKeys([]ConfigUpdateItem{
		{Key: "site_language", Value: "zh-CN"},
		{Key: "watermark_text", Value: "summeRain"},
		{Key: "r2_public_url", Value: "https://images.example.test"},
	})
	if err != nil {
		t.Fatalf("unexpected validation error: %#v", err)
	}
}

func TestValidateAdminWritableSystemConfigKeysRejectsDeploymentAndRecipeConfig(t *testing.T) {
	for _, key := range []string{
		"DB_MAX_OPEN_CONNS",
		"V2_GLOBAL_UPLOAD_CONCURRENCY",
		"IMGPROXY_WORKERS",
		"MAX_JSON_BODY_BYTES",
		"V2_MAX_ACTIVE_SESSIONS_PER_USER",
		"V2_INIT_MAX_JSON_BYTES",
		"V2_BATCH_STATUS_MAX_JSON_BYTES",
		"V1_DYNAMIC_GENERATION_CONCURRENCY",
		"V1_DYNAMIC_GENERATION_QUEUE_DEPTH",
		"V1_BACKGROUND_FORMAT_CONCURRENCY",
		"recipe_version",
		"image_recipe",
		"unknown_key",
	} {
		t.Run(key, func(t *testing.T) {
			err := validateAdminWritableSystemConfigKeys([]ConfigUpdateItem{{Key: key, Value: "1"}})
			if err == nil || err.Code != 3006 || err.HTTP != http.StatusBadRequest {
				t.Fatalf("error = %#v, want rejected config key", err)
			}
		})
	}
}

func TestValidateAdminWritableSystemConfigKeysRejectsEmptyKey(t *testing.T) {
	err := validateAdminWritableSystemConfigKeys([]ConfigUpdateItem{{Key: "  ", Value: "x"}})
	if err == nil || err.Code != 3001 || err.HTTP != http.StatusBadRequest {
		t.Fatalf("error = %#v, want empty-key validation error", err)
	}
}
