// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"testing"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
)

func TestPublicConfigExposesProviderAndSiteKey(t *testing.T) {
	svc := NewPublicConfigService(staticPublicConfigReader{}, config.CaptchaConfig{
		Provider:  "recaptcha",
		Recaptcha: config.RecaptchaConfig{SiteKey: "env-recaptcha-key"},
	}, nil)

	result, appErr := svc.Get()

	if appErr != nil {
		t.Fatalf("Get returned error: %v", appErr)
	}
	if result.CaptchaProvider != "recaptcha" {
		t.Fatalf("CaptchaProvider = %q, want recaptcha", result.CaptchaProvider)
	}
	if result.CaptchaSiteKey != "env-recaptcha-key" {
		t.Fatalf("CaptchaSiteKey = %q, want env-recaptcha-key", result.CaptchaSiteKey)
	}
}

func TestPublicConfigAdminOverrideSwitchesProviderAndKey(t *testing.T) {
	svc := NewPublicConfigService(staticPublicConfigReader{
		configs: []model.SystemConfig{
			{ConfigKey: "captcha_provider", ConfigValue: "turnstile"},
			{ConfigKey: "turnstile_site_key", ConfigValue: "db-turnstile-key"},
		},
	}, config.CaptchaConfig{
		Provider:  "recaptcha",
		Recaptcha: config.RecaptchaConfig{SiteKey: "env-recaptcha-key"},
		Turnstile: config.TurnstileConfig{SiteKey: "env-turnstile-key"},
	}, nil)

	result, _ := svc.Get()

	if result.CaptchaProvider != "turnstile" {
		t.Fatalf("CaptchaProvider = %q, want turnstile", result.CaptchaProvider)
	}
	if result.CaptchaSiteKey != "db-turnstile-key" {
		t.Fatalf("CaptchaSiteKey = %q, want db-turnstile-key", result.CaptchaSiteKey)
	}
}

func TestPublicConfigNoneProviderHasEmptyKey(t *testing.T) {
	svc := NewPublicConfigService(staticPublicConfigReader{}, config.CaptchaConfig{Provider: "none"}, nil)

	result, _ := svc.Get()

	if result.CaptchaProvider != "none" || result.CaptchaSiteKey != "" {
		t.Fatalf("none provider result = %+v, want provider=none empty key", result)
	}
	if result.SiteLanguage != "en-US" {
		t.Fatalf("SiteLanguage = %q, want en-US", result.SiteLanguage)
	}
}

type staticPublicConfigReader struct {
	configs []model.SystemConfig
}

func (r staticPublicConfigReader) FindAll() ([]model.SystemConfig, error) {
	return r.configs, nil
}
