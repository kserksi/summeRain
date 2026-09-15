// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"net/http"
	"strings"

	"github.com/kserksi/summerain/internal/pkg/errcode"
)

// adminWritableSystemConfigKeys is the single allowlist for values persisted
// through the administrator system-config API. Deployment and protocol
// configuration must come from environment/files, not from this API.
var adminWritableSystemConfigKeys = map[string]struct{}{
	"site_language":                {},
	"captcha_provider":             {},
	"captcha_site_key":             {},
	"captcha_secret":               {},
	"private_token_ttl_default_ms": {},
	"watermark_enabled":            {},
	"watermark_text":               {},
	"watermark_opacity":            {},
	"watermark_position":           {},
	"watermark_size":               {},
	"watermark_color":              {},
	"r2_enabled":                   {},
	"r2_endpoint":                  {},
	"r2_access_key":                {},
	"r2_secret_key":                {},
	"r2_bucket":                    {},
	"r2_public_url":                {},
}

func validateAdminWritableSystemConfigKeys(items []ConfigUpdateItem) *errcode.AppError {
	for _, item := range items {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			return errcode.New(3001, "系统配置键不能为空", http.StatusBadRequest)
		}
		if _, ok := adminWritableSystemConfigKeys[key]; !ok {
			return errcode.New(3006, "系统配置键不允许通过管理员 API 修改: "+key, http.StatusBadRequest)
		}
	}
	return nil
}
