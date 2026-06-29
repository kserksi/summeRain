// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/summerain/image-gallery/internal/config"
)

func recaptchaCfg(url string, failClosed bool) config.RecaptchaConfig {
	return config.RecaptchaConfig{Enabled: true, Secret: "secret", MinScore: 0.5, VerifyURL: url, FailClosed: failClosed, AllowedHostnames: []string{"example.com"}}
}

func TestRecaptchaDisabledBypassesVerification(t *testing.T) {
	verifier := NewRecaptchaVerifier(config.RecaptchaConfig{Enabled: false})
	if appErr := verifier.Verify(context.Background(), CaptchaPayload{Token: "t", Action: "login", ExpectedAction: "login"}, "", "example.com"); appErr != nil {
		t.Fatalf("Verify returned error while disabled: %v", appErr)
	}
}

func TestRecaptchaRejectsActionMismatchBeforeRemoteCall(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer server.Close()
	verifier := NewRecaptchaVerifier(recaptchaCfg(server.URL, true))

	appErr := verifier.Verify(context.Background(), CaptchaPayload{Token: "token", Action: "register", ExpectedAction: "login"}, "", "example.com")

	if appErr == nil || appErr.Code != 2009 {
		t.Fatalf("Verify error = %v, want 2009", appErr)
	}
	if called {
		t.Fatalf("remote verifier should not be called for client action mismatch")
	}
}

func TestRecaptchaAcceptsValidSiteVerifyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("secret") != "secret" || r.Form.Get("response") != "token" {
			t.Fatalf("unexpected form: %s", r.Form.Encode())
		}
		w.Write([]byte(`{"success":true,"score":0.9,"action":"login","hostname":"example.com"}`))
	}))
	defer server.Close()
	verifier := NewRecaptchaVerifier(recaptchaCfg(server.URL, true))

	if appErr := verifier.Verify(context.Background(), CaptchaPayload{Token: "token", Action: "login", ExpectedAction: "login"}, "", "example.com"); appErr != nil {
		t.Fatalf("Verify returned error: %v", appErr)
	}
}

func TestRecaptchaRejectsLowScore(t *testing.T) {
	server := newRecaptchaJSONServer(`{"success":true,"score":0.1,"action":"login","hostname":"example.com"}`)
	defer server.Close()
	verifier := NewRecaptchaVerifier(recaptchaCfg(server.URL, true))

	appErr := verifier.Verify(context.Background(), CaptchaPayload{Token: "token", Action: "login", ExpectedAction: "login"}, "", "example.com")

	if appErr == nil || appErr.Code != 2009 {
		t.Fatalf("Verify error = %v, want 2009", appErr)
	}
}

func TestRecaptchaTimeoutFailOpenAndClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	failClosed := NewRecaptchaVerifier(recaptchaCfg(server.URL, true))
	failClosed.client.Timeout = 5 * time.Millisecond
	if appErr := failClosed.Verify(context.Background(), CaptchaPayload{Token: "token", Action: "login", ExpectedAction: "login"}, "", "example.com"); appErr == nil || appErr.Code != 1004 {
		t.Fatalf("fail-closed timeout error = %v, want 1004", appErr)
	}

	failOpen := NewRecaptchaVerifier(recaptchaCfg(server.URL, false))
	failOpen.client.Timeout = 5 * time.Millisecond
	if appErr := failOpen.Verify(context.Background(), CaptchaPayload{Token: "token", Action: "login", ExpectedAction: "login"}, "", "example.com"); appErr != nil {
		t.Fatalf("fail-open timeout returned error: %v", appErr)
	}
}

func TestRecaptchaRequiresAllowedHostnamesWhenEnabled(t *testing.T) {
	verifier := NewRecaptchaVerifier(config.RecaptchaConfig{Enabled: true, Secret: "secret", MinScore: 0.5, VerifyURL: "http://127.0.0.1", FailClosed: true})
	if appErr := verifier.Verify(context.Background(), CaptchaPayload{Token: "token", Action: "login", ExpectedAction: "login"}, "", "example.com"); appErr == nil || appErr.Code != 1004 {
		t.Fatalf("Verify error = %v, want 1004", appErr)
	}
}

func newRecaptchaJSONServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.ReplaceAll(body, `\"`, `"`)))
	}))
}
