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
		Server:   ServerConfig{MaxJSONBodyBytes: 1 << 20},
		Database: DatabaseConfig{MaxOpenConns: 8, MaxIdleConns: 4},
		Redis:    RedisConfig{PoolSize: 8},
		Storage: StorageConfig{
			BasePath: base, StagingPath: filepath.Join(base, ".staging"),
			DiskSoftPct: 80, DiskHardPct: 90,
		},
		ImageV1: ImageV1Config{
			DynamicGenerationConcurrency: 2, DynamicGenerationQueueDepth: 4, BackgroundFormatConcurrency: 1,
			DynamicMaxResponseBytes: 96 << 20, BackgroundMaxResponseBytes: 32 << 20,
			GenerationTimeout: 35 * time.Second, ImgproxyRequestTimeout: 30 * time.Second,
			MultipartMemoryBytes: 8 << 20,
		},
		ImageV2: ImageV2Config{
			GlobalUploadConcurrency: 8, PerUserConcurrency: 4, WatermarkConcurrency: 1,
			MaxActiveSessionsPerUser: 8, InitMaxJSONBytes: 64 << 10, BatchStatusMaxJSONBytes: 16 << 10,
			SessionTTL: 30 * time.Minute, JobPollInterval: time.Second, JobLease: 2 * time.Minute,
			ClientPipelineConcurrency: 2, ClientActiveSessionConcurrency: 4,
			ClientMaxNativeConcurrency: 2, MaxSourceBytes: 15 << 20,
		},
		CDN: CDNConfig{
			CloudflareAPIBaseURL: "https://api.cloudflare.com/client/v4",
			OutboxBatchSize:      10, OutboxPollInterval: 2 * time.Second,
			OutboxLease: 3 * time.Minute, PurgeRequestsPerSecond: 4,
			PurgeRequestTimeout: 15 * time.Second,
		},
	}
}

func TestLoadStartupLimitDefaults(t *testing.T) {
	for _, key := range []string{
		"MAX_JSON_BODY_BYTES",
		"V2_INIT_MAX_JSON_BYTES",
		"V2_BATCH_STATUS_MAX_JSON_BYTES",
		"V2_MAX_ACTIVE_SESSIONS_PER_USER",
		"V1_DYNAMIC_GENERATION_CONCURRENCY",
		"V1_DYNAMIC_GENERATION_QUEUE_DEPTH",
		"V1_BACKGROUND_FORMAT_CONCURRENCY",
		"CLIENT_UPLOAD_PIPELINE_CONCURRENCY",
		"CLIENT_ACTIVE_SESSION_CONCURRENCY",
		"CLIENT_MAX_NATIVE_CONCURRENCY",
		"V2_MAX_SOURCE_BYTES",
	} {
		t.Setenv(key, "")
	}

	cfg := Load()

	if cfg.Server.MaxJSONBodyBytes != 1<<20 {
		t.Fatalf("MaxJSONBodyBytes = %d, want %d", cfg.Server.MaxJSONBodyBytes, 1<<20)
	}
	if cfg.ImageV2.InitMaxJSONBytes != 64<<10 {
		t.Fatalf("InitMaxJSONBytes = %d, want %d", cfg.ImageV2.InitMaxJSONBytes, 64<<10)
	}
	if cfg.ImageV2.BatchStatusMaxJSONBytes != 16<<10 {
		t.Fatalf("BatchStatusMaxJSONBytes = %d, want %d", cfg.ImageV2.BatchStatusMaxJSONBytes, 16<<10)
	}
	if cfg.ImageV2.MaxActiveSessionsPerUser != 8 {
		t.Fatalf("MaxActiveSessionsPerUser = %d, want 8", cfg.ImageV2.MaxActiveSessionsPerUser)
	}
	if cfg.ImageV1.DynamicGenerationConcurrency != 2 || cfg.ImageV1.DynamicGenerationQueueDepth != 4 || cfg.ImageV1.BackgroundFormatConcurrency != 1 {
		t.Fatalf("V1 defaults = %#v, want concurrency 2, queue depth 4, background 1", cfg.ImageV1)
	}
	if cfg.ImageV2.ClientPipelineConcurrency != 2 || cfg.ImageV2.ClientActiveSessionConcurrency != 4 ||
		cfg.ImageV2.ClientMaxNativeConcurrency != 2 || cfg.ImageV2.MaxSourceBytes != 15<<20 {
		t.Fatalf("client hint defaults = %d/%d/%d/%d, want 2/4/2/%d",
			cfg.ImageV2.ClientPipelineConcurrency, cfg.ImageV2.ClientActiveSessionConcurrency,
			cfg.ImageV2.ClientMaxNativeConcurrency, cfg.ImageV2.MaxSourceBytes, 15<<20)
	}
}

func TestLoadStartupLimitsFromEnvironment(t *testing.T) {
	t.Setenv("MAX_JSON_BODY_BYTES", "2097152")
	t.Setenv("V2_INIT_MAX_JSON_BYTES", "131072")
	t.Setenv("V2_BATCH_STATUS_MAX_JSON_BYTES", "32768")
	t.Setenv("V2_MAX_ACTIVE_SESSIONS_PER_USER", "3")
	t.Setenv("V1_DYNAMIC_GENERATION_CONCURRENCY", "5")
	t.Setenv("V1_DYNAMIC_GENERATION_QUEUE_DEPTH", "9")
	t.Setenv("V1_BACKGROUND_FORMAT_CONCURRENCY", "2")
	t.Setenv("CLIENT_UPLOAD_PIPELINE_CONCURRENCY", "3")
	t.Setenv("CLIENT_ACTIVE_SESSION_CONCURRENCY", "2")
	t.Setenv("CLIENT_MAX_NATIVE_CONCURRENCY", "4")
	t.Setenv("V2_MAX_SOURCE_BYTES", "8388608")

	cfg := Load()

	if cfg.Server.MaxJSONBodyBytes != 2097152 {
		t.Fatalf("MaxJSONBodyBytes = %d", cfg.Server.MaxJSONBodyBytes)
	}
	if cfg.ImageV2.InitMaxJSONBytes != 131072 || cfg.ImageV2.BatchStatusMaxJSONBytes != 32768 {
		t.Fatalf("V2 JSON limits = %d/%d", cfg.ImageV2.InitMaxJSONBytes, cfg.ImageV2.BatchStatusMaxJSONBytes)
	}
	if cfg.ImageV2.MaxActiveSessionsPerUser != 3 {
		t.Fatalf("MaxActiveSessionsPerUser = %d", cfg.ImageV2.MaxActiveSessionsPerUser)
	}
	if cfg.ImageV1.DynamicGenerationConcurrency != 5 || cfg.ImageV1.DynamicGenerationQueueDepth != 9 || cfg.ImageV1.BackgroundFormatConcurrency != 2 {
		t.Fatalf("V1 limits = %#v", cfg.ImageV1)
	}
	if cfg.ImageV2.ClientPipelineConcurrency != 3 || cfg.ImageV2.ClientActiveSessionConcurrency != 2 ||
		cfg.ImageV2.ClientMaxNativeConcurrency != 4 || cfg.ImageV2.MaxSourceBytes != 8388608 {
		t.Fatalf("client hint limits = %d/%d/%d/%d, want 3/2/4/8388608",
			cfg.ImageV2.ClientPipelineConcurrency, cfg.ImageV2.ClientActiveSessionConcurrency,
			cfg.ImageV2.ClientMaxNativeConcurrency, cfg.ImageV2.MaxSourceBytes)
	}
}

func TestValidateStartupLimitBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{name: "max json body lower bound accepted", mutate: func(cfg *Config) {
			cfg.Server.MaxJSONBodyBytes = 1 << 10
			cfg.ImageV2.InitMaxJSONBytes = 1 << 10
			cfg.ImageV2.BatchStatusMaxJSONBytes = 1 << 10
		}},
		{name: "max json body upper bound accepted", mutate: func(cfg *Config) { cfg.Server.MaxJSONBodyBytes = 16 << 20 }},
		{name: "max json body too small", mutate: func(cfg *Config) { cfg.Server.MaxJSONBodyBytes = (1 << 10) - 1 }, wantErr: "MAX_JSON_BODY_BYTES"},
		{name: "max json body too large", mutate: func(cfg *Config) { cfg.Server.MaxJSONBodyBytes = (16 << 20) + 1 }, wantErr: "MAX_JSON_BODY_BYTES"},

		{name: "init json lower bound accepted", mutate: func(cfg *Config) { cfg.ImageV2.InitMaxJSONBytes = 1 << 10 }},
		{name: "init json upper bound accepted", mutate: func(cfg *Config) { cfg.ImageV2.InitMaxJSONBytes = 1 << 20 }},
		{name: "init json too small", mutate: func(cfg *Config) { cfg.ImageV2.InitMaxJSONBytes = (1 << 10) - 1 }, wantErr: "V2_INIT_MAX_JSON_BYTES"},
		{name: "init json too large", mutate: func(cfg *Config) { cfg.ImageV2.InitMaxJSONBytes = (1 << 20) + 1 }, wantErr: "V2_INIT_MAX_JSON_BYTES"},
		{name: "init json exceeds global body limit", mutate: func(cfg *Config) {
			cfg.Server.MaxJSONBodyBytes = 1 << 10
			cfg.ImageV2.InitMaxJSONBytes = 2 << 10
		}, wantErr: "must not exceed MAX_JSON_BODY_BYTES"},

		{name: "batch status lower bound accepted", mutate: func(cfg *Config) { cfg.ImageV2.BatchStatusMaxJSONBytes = 1 << 10 }},
		{name: "batch status upper bound accepted", mutate: func(cfg *Config) { cfg.ImageV2.BatchStatusMaxJSONBytes = 256 << 10 }},
		{name: "batch status too small", mutate: func(cfg *Config) { cfg.ImageV2.BatchStatusMaxJSONBytes = (1 << 10) - 1 }, wantErr: "V2_BATCH_STATUS_MAX_JSON_BYTES"},
		{name: "batch status too large", mutate: func(cfg *Config) { cfg.ImageV2.BatchStatusMaxJSONBytes = (256 << 10) + 1 }, wantErr: "V2_BATCH_STATUS_MAX_JSON_BYTES"},
		{name: "batch status exceeds global body limit", mutate: func(cfg *Config) {
			cfg.Server.MaxJSONBodyBytes = 1 << 10
			cfg.ImageV2.BatchStatusMaxJSONBytes = 2 << 10
		}, wantErr: "must not exceed MAX_JSON_BODY_BYTES"},

		{name: "active sessions lower bound accepted", mutate: func(cfg *Config) {
			cfg.ImageV2.MaxActiveSessionsPerUser = 1
			cfg.ImageV2.ClientActiveSessionConcurrency = 1
		}},
		{name: "active sessions upper bound accepted", mutate: func(cfg *Config) { cfg.ImageV2.MaxActiveSessionsPerUser = 32 }},
		{name: "active sessions zero", mutate: func(cfg *Config) { cfg.ImageV2.MaxActiveSessionsPerUser = 0 }, wantErr: "V2_MAX_ACTIVE_SESSIONS_PER_USER"},
		{name: "active sessions too large", mutate: func(cfg *Config) { cfg.ImageV2.MaxActiveSessionsPerUser = 33 }, wantErr: "V2_MAX_ACTIVE_SESSIONS_PER_USER"},

		{name: "client pipeline lower bound accepted", mutate: func(cfg *Config) { cfg.ImageV2.ClientPipelineConcurrency = 1 }},
		{name: "client pipeline upper bound accepted", mutate: func(cfg *Config) { cfg.ImageV2.ClientPipelineConcurrency = 8 }},
		{name: "client pipeline zero", mutate: func(cfg *Config) { cfg.ImageV2.ClientPipelineConcurrency = 0 }, wantErr: "CLIENT_UPLOAD_PIPELINE_CONCURRENCY"},
		{name: "client pipeline too large", mutate: func(cfg *Config) { cfg.ImageV2.ClientPipelineConcurrency = 9 }, wantErr: "CLIENT_UPLOAD_PIPELINE_CONCURRENCY"},

		{name: "client active sessions lower bound accepted", mutate: func(cfg *Config) { cfg.ImageV2.ClientActiveSessionConcurrency = 1 }},
		{name: "client active sessions equal to server limit accepted", mutate: func(cfg *Config) {
			cfg.ImageV2.ClientActiveSessionConcurrency = cfg.ImageV2.MaxActiveSessionsPerUser
		}},
		{name: "client active sessions above server limit", mutate: func(cfg *Config) {
			cfg.ImageV2.ClientActiveSessionConcurrency = cfg.ImageV2.MaxActiveSessionsPerUser + 1
		}, wantErr: "CLIENT_ACTIVE_SESSION_CONCURRENCY"},
		{name: "client active sessions zero", mutate: func(cfg *Config) { cfg.ImageV2.ClientActiveSessionConcurrency = 0 }, wantErr: "CLIENT_ACTIVE_SESSION_CONCURRENCY"},

		{name: "client native lower bound accepted", mutate: func(cfg *Config) { cfg.ImageV2.ClientMaxNativeConcurrency = 1 }},
		{name: "client native upper bound accepted", mutate: func(cfg *Config) { cfg.ImageV2.ClientMaxNativeConcurrency = 4 }},
		{name: "client native zero", mutate: func(cfg *Config) { cfg.ImageV2.ClientMaxNativeConcurrency = 0 }, wantErr: "CLIENT_MAX_NATIVE_CONCURRENCY"},
		{name: "client native too large", mutate: func(cfg *Config) { cfg.ImageV2.ClientMaxNativeConcurrency = 5 }, wantErr: "CLIENT_MAX_NATIVE_CONCURRENCY"},

		{name: "max source bytes lower bound accepted", mutate: func(cfg *Config) { cfg.ImageV2.MaxSourceBytes = 1 << 20 }},
		{name: "max source bytes upper bound accepted", mutate: func(cfg *Config) { cfg.ImageV2.MaxSourceBytes = 64 << 20 }},
		{name: "max source bytes too small", mutate: func(cfg *Config) { cfg.ImageV2.MaxSourceBytes = (1 << 20) - 1 }, wantErr: "V2_MAX_SOURCE_BYTES"},
		{name: "max source bytes too large", mutate: func(cfg *Config) { cfg.ImageV2.MaxSourceBytes = (64 << 20) + 1 }, wantErr: "V2_MAX_SOURCE_BYTES"},

		{name: "v1 concurrency lower bound accepted", mutate: func(cfg *Config) { cfg.ImageV1.DynamicGenerationConcurrency = 1 }},
		{name: "v1 concurrency upper bound accepted", mutate: func(cfg *Config) { cfg.ImageV1.DynamicGenerationConcurrency = 16 }},
		{name: "v1 concurrency zero", mutate: func(cfg *Config) { cfg.ImageV1.DynamicGenerationConcurrency = 0 }, wantErr: "V1_DYNAMIC_GENERATION_CONCURRENCY"},
		{name: "v1 concurrency too large", mutate: func(cfg *Config) { cfg.ImageV1.DynamicGenerationConcurrency = 17 }, wantErr: "V1_DYNAMIC_GENERATION_CONCURRENCY"},

		{name: "v1 queue depth zero accepted", mutate: func(cfg *Config) { cfg.ImageV1.DynamicGenerationQueueDepth = 0 }},
		{name: "v1 queue depth upper bound accepted", mutate: func(cfg *Config) { cfg.ImageV1.DynamicGenerationQueueDepth = 64 }},
		{name: "v1 queue depth negative", mutate: func(cfg *Config) { cfg.ImageV1.DynamicGenerationQueueDepth = -1 }, wantErr: "V1_DYNAMIC_GENERATION_QUEUE_DEPTH"},
		{name: "v1 queue depth too large", mutate: func(cfg *Config) { cfg.ImageV1.DynamicGenerationQueueDepth = 65 }, wantErr: "V1_DYNAMIC_GENERATION_QUEUE_DEPTH"},

		{name: "v1 background lower bound accepted", mutate: func(cfg *Config) { cfg.ImageV1.BackgroundFormatConcurrency = 1 }},
		{name: "v1 background upper bound accepted", mutate: func(cfg *Config) { cfg.ImageV1.BackgroundFormatConcurrency = 8 }},
		{name: "v1 background zero", mutate: func(cfg *Config) { cfg.ImageV1.BackgroundFormatConcurrency = 0 }, wantErr: "V1_BACKGROUND_FORMAT_CONCURRENCY"},
		{name: "v1 background too large", mutate: func(cfg *Config) { cfg.ImageV1.BackgroundFormatConcurrency = 9 }, wantErr: "V1_BACKGROUND_FORMAT_CONCURRENCY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfigForTest(t)
			tt.mutate(cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error=%v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error=%v, want %s", err, tt.wantErr)
			}
		})
	}
}

func TestEffectiveSummaryOmitsSecrets(t *testing.T) {
	cfg := validConfigForTest(t)
	cfg.Server.CookieSecret = "super-secret-cookie"
	cfg.Database.Password = "super-secret-db"
	cfg.Imgproxy.Key = "super-secret-key"
	cfg.Imgproxy.Salt = "super-secret-salt"
	cfg.Captcha.Recaptcha.Secret = "super-secret-captcha"

	summary := strings.Join(cfg.EffectiveSummary(), "\n")

	for _, secret := range []string{
		cfg.Server.CookieSecret, cfg.Database.Password,
		cfg.Imgproxy.Key, cfg.Imgproxy.Salt, cfg.Captcha.Recaptcha.Secret,
	} {
		if strings.Contains(summary, secret) {
			t.Fatalf("effective summary leaked a secret")
		}
	}
	for _, want := range []string{
		"db_max_open_conns=8",
		"v1_dynamic_generation_concurrency=2",
		"v2_max_active_sessions_per_user=8",
		"max_json_body_bytes=1048576",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("effective summary missing %q", want)
		}
	}
}
