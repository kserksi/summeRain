// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/pkg/errcode"
)

type TurnstileVerifier struct {
	cfg    config.TurnstileConfig
	client *http.Client
}

func NewTurnstileVerifier(cfg config.TurnstileConfig) *TurnstileVerifier {
	return &TurnstileVerifier{cfg: cfg, client: &http.Client{Timeout: 3 * time.Second}}
}

// Verify POSTs the Cloudflare Turnstile siteverify. Transport/HTTP failures
// fail closed (ErrRecaptchaUnavailable).
func (v *TurnstileVerifier) Verify(ctx context.Context, payload CaptchaPayload, remoteIP string, requestHost string) *errcode.AppError {
	if v == nil {
		return nil
	}
	if v.cfg.Secret == "" {
		return errcode.ErrRecaptchaUnavailable
	}
	if payload.Token == "" {
		return errcode.ErrRecaptchaFailed
	}

	form := url.Values{}
	form.Set("secret", v.cfg.Secret)
	form.Set("response", payload.Token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.cfg.VerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return errcode.ErrRecaptchaUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.client.Do(req)
	if err != nil {
		return errcode.ErrRecaptchaUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errcode.ErrRecaptchaUnavailable
	}

	var result struct {
		Success    bool     `json:"success"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return errcode.ErrRecaptchaUnavailable
	}
	if !result.Success {
		return errcode.ErrRecaptchaFailed
	}
	return nil
}
