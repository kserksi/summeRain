// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kserksi/summerain/internal/model"
)

func TestValidateR2ConfigUpdate(t *testing.T) {
	current := []model.SystemConfig{
		{ConfigKey: "r2_endpoint", ConfigValue: "https://account.r2.example.com"},
		{ConfigKey: "r2_bucket", ConfigValue: "photos"},
		{ConfigKey: "r2_access_key", ConfigValue: "old-access"},
		{ConfigKey: "r2_secret_key", ConfigValue: "old-secret"},
		{ConfigKey: "r2_enabled", ConfigValue: "true"},
		{ConfigKey: "r2_public_url", ConfigValue: "https://cdn.example.com"},
	}
	lineage := r2StorageLineage{
		RemoteEndpoint: "https://account.r2.example.com",
		RemoteBucket:   "photos",
	}

	tests := []struct {
		name                 string
		current              []model.SystemConfig
		updates              []ConfigUpdateItem
		lineages             []r2StorageLineage
		unclassifiedHistory  int64
		pendingRemoteDeletes int64
		wantHTTP             int
		wantCode             int
		wantMessage          string
	}{
		{
			name: "target change allowed without remote lineage",
			updates: []ConfigUpdateItem{
				{Key: "r2_endpoint", Value: "https://new.r2.example.com"},
				{Key: "r2_bucket", Value: "new-photos"},
			},
		},
		{
			name:        "invalid endpoint rejected without remote lineage",
			updates:     []ConfigUpdateItem{{Key: "r2_endpoint", Value: "https://account.r2.example.com?token=secret"}},
			wantHTTP:    http.StatusBadRequest,
			wantCode:    3006,
			wantMessage: "r2_endpoint",
		},
		{
			name:        "invalid public URL rejected without remote lineage",
			updates:     []ConfigUpdateItem{{Key: "r2_public_url", Value: "javascript:alert(1)"}},
			wantHTTP:    http.StatusBadRequest,
			wantCode:    3006,
			wantMessage: "r2_public_url",
		},
		{
			name:     "endpoint change rejected",
			updates:  []ConfigUpdateItem{{Key: "r2_endpoint", Value: "https://new.r2.example.com"}},
			lineages: []r2StorageLineage{lineage},
			wantHTTP: http.StatusConflict,
			wantCode: 4094,
		},
		{
			name:                "endpoint change rejected with unclassified history",
			updates:             []ConfigUpdateItem{{Key: "r2_endpoint", Value: "https://new.r2.example.com"}},
			unclassifiedHistory: 1,
			wantHTTP:            http.StatusConflict,
			wantCode:            4094,
			wantMessage:         "未分类",
		},
		{
			name:                 "target change rejected with pending remote deletion",
			updates:              []ConfigUpdateItem{{Key: "r2_endpoint", Value: "https://new.r2.example.com"}},
			pendingRemoteDeletes: 1,
			wantHTTP:             http.StatusConflict,
			wantCode:             4094,
			wantMessage:          "清理",
		},
		{
			name:                 "equivalent target allowed with pending remote deletion",
			updates:              []ConfigUpdateItem{{Key: "r2_endpoint", Value: " https://account.r2.example.com/// "}},
			pendingRemoteDeletes: 1,
		},
		{
			name:                 "credentials cannot be cleared with pending remote deletion",
			updates:              []ConfigUpdateItem{{Key: "r2_secret_key", Value: ""}},
			pendingRemoteDeletes: 1,
			wantHTTP:             http.StatusBadRequest,
			wantCode:             3006,
			wantMessage:          "r2_secret_key",
		},
		{
			name:                "equivalent target allowed with unclassified history",
			updates:             []ConfigUpdateItem{{Key: "r2_endpoint", Value: " https://account.r2.example.com/// "}},
			unclassifiedHistory: 1,
		},
		{
			name:                "public URL change allowed with unclassified history",
			updates:             []ConfigUpdateItem{{Key: "r2_public_url", Value: "https://new-cdn.example.com"}},
			unclassifiedHistory: 1,
		},
		{
			name:                "credentials cannot be cleared with unclassified history",
			updates:             []ConfigUpdateItem{{Key: "r2_secret_key", Value: ""}},
			unclassifiedHistory: 1,
			wantHTTP:            http.StatusBadRequest,
			wantCode:            3006,
			wantMessage:         "r2_secret_key",
		},
		{
			name:     "bucket change rejected",
			updates:  []ConfigUpdateItem{{Key: "r2_bucket", Value: "archive"}},
			lineages: []r2StorageLineage{lineage},
			wantHTTP: http.StatusConflict,
			wantCode: 4094,
		},
		{
			name:        "access key cannot be cleared",
			updates:     []ConfigUpdateItem{{Key: "r2_access_key", Value: "  "}},
			lineages:    []r2StorageLineage{lineage},
			wantHTTP:    http.StatusBadRequest,
			wantCode:    3006,
			wantMessage: "r2_access_key",
		},
		{
			name:        "secret key cannot be cleared",
			updates:     []ConfigUpdateItem{{Key: "r2_secret_key", Value: ""}},
			lineages:    []r2StorageLineage{lineage},
			wantHTTP:    http.StatusBadRequest,
			wantCode:    3006,
			wantMessage: "r2_secret_key",
		},
		{
			name: "disable and rotate credentials on the same target",
			updates: []ConfigUpdateItem{
				{Key: "r2_enabled", Value: "false"},
				{Key: "r2_access_key", Value: "new-access"},
				{Key: "r2_secret_key", Value: "new-secret"},
				{Key: "r2_public_url", Value: "https://new-cdn.example.com"},
			},
			lineages: []r2StorageLineage{lineage},
		},
		{
			name: "equivalent target formatting",
			updates: []ConfigUpdateItem{
				{Key: "r2_endpoint", Value: " https://account.r2.example.com/// "},
				{Key: "r2_bucket", Value: " photos "},
			},
			lineages: []r2StorageLineage{lineage},
		},
		{
			name: "restore config to the lineage target",
			current: []model.SystemConfig{
				{ConfigKey: "r2_endpoint", ConfigValue: "https://drifted.r2.example.com"},
				{ConfigKey: "r2_bucket", ConfigValue: "drifted"},
			},
			updates: []ConfigUpdateItem{
				{Key: "r2_endpoint", Value: lineage.RemoteEndpoint},
				{Key: "r2_bucket", Value: lineage.RemoteBucket},
			},
			lineages: []r2StorageLineage{lineage},
		},
		{
			name: "all lineages must match the proposed target",
			updates: []ConfigUpdateItem{
				{Key: "r2_endpoint", Value: "https://new.r2.example.com"},
				{Key: "r2_bucket", Value: "new-photos"},
			},
			lineages: []r2StorageLineage{
				{RemoteEndpoint: "https://new.r2.example.com", RemoteBucket: "new-photos"},
				lineage,
			},
			wantHTTP: http.StatusConflict,
			wantCode: 4094,
		},
		{
			name: "last duplicate value is validated",
			updates: []ConfigUpdateItem{
				{Key: "r2_secret_key", Value: "replacement"},
				{Key: "r2_secret_key", Value: ""},
			},
			lineages:    []r2StorageLineage{lineage},
			wantHTTP:    http.StatusBadRequest,
			wantCode:    3006,
			wantMessage: "r2_secret_key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := tt.current
			if base == nil {
				base = current
			}
			appErr := validateR2ConfigUpdate(base, tt.updates, tt.lineages, tt.unclassifiedHistory, tt.pendingRemoteDeletes)
			if tt.wantHTTP == 0 {
				if appErr != nil {
					t.Fatalf("validateR2ConfigUpdate() error = %#v", appErr)
				}
				return
			}
			if appErr == nil {
				t.Fatal("validateR2ConfigUpdate() error = nil")
			}
			if appErr.HTTP != tt.wantHTTP || appErr.Code != tt.wantCode {
				t.Fatalf("validateR2ConfigUpdate() error = %#v, want HTTP %d code %d", appErr, tt.wantHTTP, tt.wantCode)
			}
			if tt.wantMessage != "" && !strings.Contains(appErr.Message, tt.wantMessage) {
				t.Fatalf("validateR2ConfigUpdate() message = %q, want it to contain %q", appErr.Message, tt.wantMessage)
			}
		})
	}
}
