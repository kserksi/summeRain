// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadUsesMainlandCompatibleRecaptchaVerifyURL(t *testing.T) {
	t.Setenv("RECAPTCHA_VERIFY_URL", "")

	cfg := Load()

	if cfg.Captcha.Recaptcha.VerifyURL != "https://www.recaptcha.net/recaptcha/api/siteverify" {
		t.Fatalf("VerifyURL = %q, want recaptcha.net mirror", cfg.Captcha.Recaptcha.VerifyURL)
	}
}

func TestLoadDefaultsProviderFromRecaptchaEnabled(t *testing.T) {
	t.Setenv("CAPTCHA_PROVIDER", "")
	t.Setenv("RECAPTCHA_ENABLED", "true")
	if cfg := Load(); cfg.Captcha.Provider != "recaptcha" {
		t.Fatalf("Provider = %q, want recaptcha (derived)", cfg.Captcha.Provider)
	}

	t.Setenv("RECAPTCHA_ENABLED", "false")
	if cfg := Load(); cfg.Captcha.Provider != "none" {
		t.Fatalf("Provider = %q, want none", cfg.Captcha.Provider)
	}

	t.Setenv("CAPTCHA_PROVIDER", "turnstile")
	if cfg := Load(); cfg.Captcha.Provider != "turnstile" {
		t.Fatalf("Provider = %q, want turnstile", cfg.Captcha.Provider)
	}
}

func TestValidateCDNDeliveryRequiresCompleteCloudflareCredentials(t *testing.T) {
	cfg := validConfigForTest(t)
	cfg.CDN.CloudflareZoneID = "zone"
	cfg.CDN.PublicBaseURL = "https://cdn.example.com"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "configured together") {
		t.Fatalf("Validate() error=%v, want incomplete Cloudflare credentials error", err)
	}
}

func TestValidateCDNDeliveryRequiresPublicBaseURL(t *testing.T) {
	cfg := validConfigForTest(t)
	cfg.CDN.PurgeWebhookURL = "https://purger.example.com/hook"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "CDN_PUBLIC_BASE_URL") {
		t.Fatalf("Validate() error=%v, want missing public base URL error", err)
	}
}

func TestValidateAcceptsBoundedCDNDeliveryConfiguration(t *testing.T) {
	cfg := validConfigForTest(t)
	cfg.CDN.PublicBaseURL = "https://cdn.example.com/images"
	cfg.CDN.CloudflareZoneID = "zone"
	cfg.CDN.CloudflareAPIToken = "token"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error=%v", err)
	}
}

func TestValidateRejectsGeetestWithCrossOriginIsolation(t *testing.T) {
	cfg := validConfigForTest(t)
	cfg.Server.CrossOriginIsolation = true
	cfg.Captcha.Provider = "geetest_v4"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "geetest_v4") {
		t.Fatalf("Validate() error=%v, want geetest incompatibility error", err)
	}

	cfg.Server.CrossOriginIsolation = false
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() with isolation disabled error=%v", err)
	}
}

func TestValidateRejectsUnsafeV2WorkerDurations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "session TTL", mutate: func(cfg *Config) { cfg.ImageV2.SessionTTL = 0 }, want: "V2_SESSION_TTL"},
		{name: "poll interval", mutate: func(cfg *Config) { cfg.ImageV2.JobPollInterval = 0 }, want: "V2_JOB_POLL_INTERVAL"},
		{name: "job lease", mutate: func(cfg *Config) { cfg.ImageV2.JobLease = time.Minute }, want: "V2_JOB_LEASE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfigForTest(t)
			tt.mutate(cfg)
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error=%v, want %s error", err, tt.want)
			}
		})
	}
}

func validConfigForTest(t *testing.T) *Config {
	t.Helper()
	base := t.TempDir()
	return &Config{
		Database: DatabaseConfig{MaxOpenConns: 8, MaxIdleConns: 4},
		Redis:    RedisConfig{PoolSize: 8},
		Storage: StorageConfig{
			BasePath: base, StagingPath: filepath.Join(base, ".staging"),
			DiskSoftPct: 80, DiskHardPct: 90,
		},
		ImageV2: ImageV2Config{
			MaxPartBytes: 20 << 20, MaxPixels: 50_000_000,
			GlobalUploadConcurrency: 8, PerUserConcurrency: 4, WatermarkConcurrency: 1,
			SessionTTL: 30 * time.Minute, JobPollInterval: time.Second, JobLease: 2 * time.Minute,
		},
		CDN: CDNConfig{
			CloudflareAPIBaseURL: "https://api.cloudflare.com/client/v4",
			OutboxBatchSize:      10, OutboxPollInterval: 2 * time.Second,
			OutboxLease: 3 * time.Minute, PurgeRequestsPerSecond: 4,
			PurgeRequestTimeout: 15 * time.Second,
		},
	}
}
