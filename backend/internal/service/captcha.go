// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"github.com/summerain/image-gallery/internal/config"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
)

// CaptchaPayload carries the provider-agnostic human-verification payload from
// the client. Each verifier reads only the fields relevant to its provider; a
// payload for the wrong provider will have empty required fields and fail.
type CaptchaPayload struct {
	Provider       string `json:"provider"`
	Token          string `json:"token"`          // recaptcha / turnstile
	Action         string `json:"action"`         // recaptcha client-claimed action
	ExpectedAction string `json:"-"`              // recaptcha expected action (server-set)
	LotNumber      string `json:"lot_number"`     // geetest v4
	CaptchaOutput  string `json:"captcha_output"` // geetest v4
	PassToken      string `json:"pass_token"`     // geetest v4
	GenTime        string `json:"gen_time"`       // geetest v4
}

// CaptchaVerifier abstracts human-verification providers. Verify returns nil on
// success, ErrRecaptchaFailed (2009) on verification failure, or
// ErrRecaptchaUnavailable (1004) when the provider upstream is unreachable.
type CaptchaVerifier interface {
	Verify(ctx context.Context, payload CaptchaPayload, remoteIP string, requestHost string) *errcode.AppError
}

// NewCaptchaVerifier returns the verifier for the configured provider, or nil
// when provider is "none" (verification disabled / not configured).
func NewCaptchaVerifier(cfg config.CaptchaConfig) CaptchaVerifier {
	switch cfg.Provider {
	case "recaptcha":
		if cfg.Recaptcha.Enabled {
			return NewRecaptchaVerifier(cfg.Recaptcha)
		}
		return nil
	case "turnstile":
		return NewTurnstileVerifier(cfg.Turnstile)
	case "geetest_v4":
		return NewGeetestVerifier(cfg.Geetest)
	default:
		return nil
	}
}
