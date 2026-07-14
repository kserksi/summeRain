// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/pkg/errcode"
)

func TestTurnstileAcceptsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("response") != "ts-token" {
			t.Fatalf("unexpected response field: %q", r.FormValue("response"))
		}
		w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()
	v := NewTurnstileVerifier(config.TurnstileConfig{Secret: "s", VerifyURL: server.URL})

	if appErr := v.Verify(context.Background(), CaptchaPayload{Token: "ts-token"}, "1.2.3.4", ""); appErr != nil {
		t.Fatalf("Verify error: %v", appErr)
	}
}

func TestTurnstileRejectsFailureResponse(t *testing.T) {
	server := newJSONServer(`{"success":false,"error-codes":["invalid-input-response"]}`)
	defer server.Close()
	v := NewTurnstileVerifier(config.TurnstileConfig{Secret: "s", VerifyURL: server.URL})

	appErr := v.Verify(context.Background(), CaptchaPayload{Token: "ts-token"}, "", "")
	if appErr == nil || appErr.Code != errcode.ErrRecaptchaFailed.Code {
		t.Fatalf("Verify error = %v, want 2009", appErr)
	}
}

func TestTurnstileMissingTokenFails(t *testing.T) {
	v := NewTurnstileVerifier(config.TurnstileConfig{Secret: "s", VerifyURL: "http://127.0.0.1"})
	if appErr := v.Verify(context.Background(), CaptchaPayload{}, "", ""); appErr == nil || appErr.Code != errcode.ErrRecaptchaFailed.Code {
		t.Fatalf("Verify error = %v, want 2009", appErr)
	}
}

func TestTurnstileMissingSecretUnavailable(t *testing.T) {
	v := NewTurnstileVerifier(config.TurnstileConfig{VerifyURL: "http://127.0.0.1"})
	if appErr := v.Verify(context.Background(), CaptchaPayload{Token: "x"}, "", ""); appErr == nil || appErr.Code != errcode.ErrRecaptchaUnavailable.Code {
		t.Fatalf("Verify error = %v, want 1004", appErr)
	}
}

func TestGeetestAcceptsSuccessAndSignsLotNumber(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["sign_token"] != geetestSign("gt-key", body["lot_number"]) {
			t.Fatalf("sign_token mismatch: got %q", body["sign_token"])
		}
		if body["captcha_id"] != "gt-id" {
			t.Fatalf("captcha_id = %q, want gt-id", body["captcha_id"])
		}
		w.Write([]byte(`{"status":"success","result":"success"}`))
	}))
	defer server.Close()
	v := NewGeetestVerifier(config.GeetestConfig{CaptchaID: "gt-id", CaptchaKey: "gt-key", VerifyURL: server.URL})

	appErr := v.Verify(context.Background(), CaptchaPayload{
		LotNumber: "lot123", CaptchaOutput: "out", PassToken: "pass", GenTime: "2026-06-19T00:00:00",
	}, "", "")
	if appErr != nil {
		t.Fatalf("Verify error: %v", appErr)
	}
}

func TestGeetestRejectsFailResult(t *testing.T) {
	server := newJSONServer(`{"status":"success","result":"fail","reason":"bad"}`)
	defer server.Close()
	v := NewGeetestVerifier(config.GeetestConfig{CaptchaID: "gt-id", CaptchaKey: "gt-key", VerifyURL: server.URL})

	appErr := v.Verify(context.Background(), CaptchaPayload{LotNumber: "l", CaptchaOutput: "o", PassToken: "p", GenTime: "g"}, "", "")
	if appErr == nil || appErr.Code != errcode.ErrRecaptchaFailed.Code {
		t.Fatalf("Verify error = %v, want 2009", appErr)
	}
}

func TestGeetestMissingFieldsFails(t *testing.T) {
	v := NewGeetestVerifier(config.GeetestConfig{CaptchaID: "gt-id", CaptchaKey: "gt-key", VerifyURL: "http://127.0.0.1"})
	if appErr := v.Verify(context.Background(), CaptchaPayload{LotNumber: "l"}, "", ""); appErr == nil || appErr.Code != errcode.ErrRecaptchaFailed.Code {
		t.Fatalf("Verify error = %v, want 2009", appErr)
	}
}

func TestNewCaptchaVerifierDispatchesByProvider(t *testing.T) {
	if v := NewCaptchaVerifier(config.CaptchaConfig{Provider: "none"}); v != nil {
		t.Fatalf("none provider should return nil, got %T", v)
	}
	if v := NewCaptchaVerifier(config.CaptchaConfig{Provider: "recaptcha"}); v != nil {
		t.Fatalf("recaptcha without Enabled should return nil, got %T", v)
	}
	if v := NewCaptchaVerifier(config.CaptchaConfig{Provider: "recaptcha", Recaptcha: config.RecaptchaConfig{Enabled: true}}); v == nil {
		t.Fatalf("enabled recaptcha should return verifier")
	}
	if v := NewCaptchaVerifier(config.CaptchaConfig{Provider: "turnstile"}); v == nil {
		t.Fatalf("turnstile should return verifier")
	}
	if v := NewCaptchaVerifier(config.CaptchaConfig{Provider: "geetest_v4"}); v == nil {
		t.Fatalf("geetest_v4 should return verifier")
	}
}

func newJSONServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
}
