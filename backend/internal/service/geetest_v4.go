// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/pkg/errcode"
)

type GeetestVerifier struct {
	cfg    config.GeetestConfig
	client *http.Client
}

func NewGeetestVerifier(cfg config.GeetestConfig) *GeetestVerifier {
	return &GeetestVerifier{cfg: cfg, client: &http.Client{Timeout: 3 * time.Second}}
}

// Verify validates a GeeTest v4 challenge. sign_token = HMAC-SHA256(key=
// captcha_key, msg=lot_number) hex. The official verify endpoint is POSTed a
// JSON body and returns {"result":"success"|"fail", ...}.
func (v *GeetestVerifier) Verify(ctx context.Context, payload CaptchaPayload, remoteIP string, requestHost string) *errcode.AppError {
	if v == nil {
		return nil
	}
	if v.cfg.CaptchaID == "" || v.cfg.CaptchaKey == "" {
		return errcode.ErrRecaptchaUnavailable
	}
	if payload.LotNumber == "" || payload.CaptchaOutput == "" || payload.PassToken == "" || payload.GenTime == "" {
		return errcode.ErrRecaptchaFailed
	}

	body := map[string]string{
		"captcha_id":     v.cfg.CaptchaID,
		"lot_number":     payload.LotNumber,
		"captcha_output": payload.CaptchaOutput,
		"pass_token":     payload.PassToken,
		"gen_time":       payload.GenTime,
		"sign_token":     geetestSign(v.cfg.CaptchaKey, payload.LotNumber),
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.cfg.VerifyURL, bytes.NewReader(raw))
	if err != nil {
		return errcode.ErrRecaptchaUnavailable
	}
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")

	resp, err := v.client.Do(req)
	if err != nil {
		return errcode.ErrRecaptchaUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errcode.ErrRecaptchaUnavailable
	}

	var result struct {
		Status string `json:"status"`
		Result string `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return errcode.ErrRecaptchaUnavailable
	}
	if result.Result != "success" {
		return errcode.ErrRecaptchaFailed
	}
	return nil
}

// geetestSign = lowercase hex of HMAC-SHA256(key=captchaKey, msg=lotNumber).
func geetestSign(captchaKey, lotNumber string) string {
	mac := hmac.New(sha256.New, []byte(captchaKey))
	mac.Write([]byte(lotNumber))
	return hex.EncodeToString(mac.Sum(nil))
}
