// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Imgproxy ImgproxyConfig
	Storage  StorageConfig
	ImageV2  ImageV2Config
	CDN      CDNConfig
	Captcha  CaptchaConfig
}

type ServerConfig struct {
	Port                 string
	Mode                 string // debug, release
	CookieSecret         string
	CrossOriginIsolation bool
}

type DatabaseConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	DBName          string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	PoolSize int
}

type ImgproxyConfig struct {
	BaseURL     string
	Key         string
	Salt        string
	PublicURL   string
	LocalFSRoot string
}

type StorageConfig struct {
	BasePath    string
	TempPath    string
	StagingPath string
	DiskSoftPct int
	DiskHardPct int
}

// ImageV2Config bounds the new client-processed upload pipeline. The values are
// intentionally conservative because summeRain commonly shares a small host
// with MySQL, Redis, and other applications.
type ImageV2Config struct {
	Enabled                 bool
	RecipeVersion           string
	MaxPartBytes            int64
	MaxPixels               int64
	SessionTTL              time.Duration
	GlobalUploadConcurrency int
	PerUserConcurrency      int
	WatermarkConcurrency    int
	JobPollInterval         time.Duration
	JobLease                time.Duration
}

// CDNConfig controls durable outbox delivery. Cloudflare is preferred when
// configured; PurgeWebhookURL is a portable fallback for another CDN.
type CDNConfig struct {
	PublicBaseURL          string
	CloudflareZoneID       string
	CloudflareAPIToken     string
	CloudflareAPIBaseURL   string
	PurgeWebhookURL        string
	PurgeWebhookToken      string
	OutboxBatchSize        int
	OutboxPollInterval     time.Duration
	OutboxLease            time.Duration
	PurgeRequestsPerSecond int
	PurgeRequestTimeout    time.Duration
}

type RecaptchaConfig struct {
	Enabled          bool
	SiteKey          string
	Secret           string
	MinScore         float64
	VerifyURL        string
	FailClosed       bool
	AllowedHostnames []string
}

type TurnstileConfig struct {
	SiteKey   string
	Secret    string
	VerifyURL string
}

type GeetestConfig struct {
	CaptchaID  string
	CaptchaKey string
	VerifyURL  string
}

// CaptchaConfig is the pluggable human-verification configuration. Provider is
// one of: none (default, no verification) | recaptcha | turnstile | geetest_v4.
type CaptchaConfig struct {
	Provider  string
	Recaptcha RecaptchaConfig
	Turnstile TurnstileConfig
	Geetest   GeetestConfig
}

func Load() *Config {
	storageBase := getEnv("STORAGE_PATH", "/data/images")
	return &Config{
		Server: ServerConfig{
			Port:                 getEnv("SERVER_PORT", "8080"),
			Mode:                 getEnv("GIN_MODE", "debug"),
			CookieSecret:         getEnv("COOKIE_SECRET", "change-me-in-production"),
			CrossOriginIsolation: getEnvBool("CROSS_ORIGIN_ISOLATION", true),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "mysql"),
			Port:            getEnv("DB_PORT", "3306"),
			User:            getEnv("DB_USER", "root"),
			Password:        getEnv("DB_PASSWORD", ""),
			DBName:          getEnv("DB_NAME", "summerain"),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 8),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 4),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "redis:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
			PoolSize: getEnvInt("REDIS_POOL_SIZE", 8),
		},
		Imgproxy: ImgproxyConfig{
			BaseURL:     getEnv("IMGPROXY_URL", "http://imgproxy:8080"),
			Key:         getEnv("IMGPROXY_KEY", ""),
			Salt:        getEnv("IMGPROXY_SALT", ""),
			PublicURL:   getEnv("IMGPROXY_PUBLIC_URL", "/img"),
			LocalFSRoot: getEnv("IMGPROXY_LOCAL_FILESYSTEM_ROOT", "/data"),
		},
		Storage: StorageConfig{
			BasePath:    storageBase,
			TempPath:    getEnv("TEMP_PATH", "/data/temp"),
			StagingPath: getEnv("V2_STAGING_PATH", filepath.Join(storageBase, ".staging")),
			DiskSoftPct: getEnvInt("DISK_SOFT_LIMIT_PERCENT", 80),
			DiskHardPct: getEnvInt("DISK_HARD_LIMIT_PERCENT", 90),
		},
		ImageV2: ImageV2Config{
			Enabled:                 getEnvBool("V2_UPLOAD_ENABLED", true),
			RecipeVersion:           getEnv("V2_RECIPE_VERSION", "2.0.0"),
			MaxPartBytes:            getEnvInt64("V2_MAX_PART_BYTES", 64<<20),
			MaxPixels:               getEnvInt64("V2_MAX_PIXELS", 50_000_000),
			SessionTTL:              getEnvDuration("V2_SESSION_TTL", 30*time.Minute),
			GlobalUploadConcurrency: getEnvInt("V2_GLOBAL_UPLOAD_CONCURRENCY", 8),
			PerUserConcurrency:      getEnvInt("V2_PER_USER_UPLOAD_CONCURRENCY", 4),
			WatermarkConcurrency:    getEnvInt("V2_WATERMARK_CONCURRENCY", 2),
			JobPollInterval:         getEnvDuration("V2_JOB_POLL_INTERVAL", time.Second),
			JobLease:                getEnvDuration("V2_JOB_LEASE", 2*time.Minute),
		},
		CDN: CDNConfig{
			PublicBaseURL:          getEnv("CDN_PUBLIC_BASE_URL", ""),
			CloudflareZoneID:       getEnv("CLOUDFLARE_ZONE_ID", ""),
			CloudflareAPIToken:     getEnv("CLOUDFLARE_API_TOKEN", ""),
			CloudflareAPIBaseURL:   getEnv("CLOUDFLARE_API_BASE_URL", "https://api.cloudflare.com/client/v4"),
			PurgeWebhookURL:        getEnv("CDN_PURGE_WEBHOOK_URL", ""),
			PurgeWebhookToken:      getEnv("CDN_PURGE_WEBHOOK_TOKEN", ""),
			OutboxBatchSize:        getEnvInt("OUTBOX_BATCH_SIZE", 10),
			OutboxPollInterval:     getEnvDuration("OUTBOX_POLL_INTERVAL", 2*time.Second),
			OutboxLease:            getEnvDuration("OUTBOX_LEASE", 3*time.Minute),
			PurgeRequestsPerSecond: getEnvInt("CDN_PURGE_REQUESTS_PER_SECOND", 4),
			PurgeRequestTimeout:    getEnvDuration("CDN_PURGE_REQUEST_TIMEOUT", 15*time.Second),
		},
		Captcha: func() CaptchaConfig {
			cc := CaptchaConfig{
				Provider: getEnv("CAPTCHA_PROVIDER", ""),
				Recaptcha: RecaptchaConfig{
					Enabled:          getEnvBool("RECAPTCHA_ENABLED", false),
					SiteKey:          getEnv("RECAPTCHA_SITE_KEY", ""),
					Secret:           getEnv("RECAPTCHA_SECRET", ""),
					MinScore:         getEnvFloat("RECAPTCHA_MIN_SCORE", 0.5),
					VerifyURL:        getEnv("RECAPTCHA_VERIFY_URL", "https://www.recaptcha.net/recaptcha/api/siteverify"),
					FailClosed:       getEnvBool("RECAPTCHA_FAIL_CLOSED", true),
					AllowedHostnames: splitCSV(getEnv("RECAPTCHA_ALLOWED_HOSTNAMES", "")),
				},
				Turnstile: TurnstileConfig{
					SiteKey:   getEnv("TURNSTILE_SITE_KEY", ""),
					Secret:    getEnv("TURNSTILE_SECRET", ""),
					VerifyURL: getEnv("TURNSTILE_VERIFY_URL", "https://challenges.cloudflare.com/turnstile/v0/siteverify"),
				},
				Geetest: GeetestConfig{
					CaptchaID:  getEnv("GEETEST_CAPTCHA_ID", ""),
					CaptchaKey: getEnv("GEETEST_CAPTCHA_KEY", ""),
					VerifyURL:  getEnv("GEETEST_VERIFY_URL", "https://gcaptcha4.geetest.com/verify"),
				},
			}
			// Backward compatibility: when CAPTCHA_PROVIDER is unset, derive from
			// RECAPTCHA_ENABLED so legacy deployments keep working.
			if cc.Provider == "" {
				if cc.Recaptcha.Enabled {
					cc.Provider = "recaptcha"
				} else {
					cc.Provider = "none"
				}
			}
			return cc
		}(),
	}
}

func (c *Config) Validate() error {
	if err := ValidateCaptchaCrossOriginIsolation(c.Captcha.Provider, c.Server.CrossOriginIsolation); err != nil {
		return err
	}
	if c.Database.MaxOpenConns < 1 || c.Database.MaxOpenConns > 64 {
		return fmt.Errorf("DB_MAX_OPEN_CONNS must be between 1 and 64")
	}
	if c.Database.MaxIdleConns < 0 || c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		return fmt.Errorf("DB_MAX_IDLE_CONNS must be between 0 and DB_MAX_OPEN_CONNS")
	}
	if c.Redis.PoolSize < 1 || c.Redis.PoolSize > 64 {
		return fmt.Errorf("REDIS_POOL_SIZE must be between 1 and 64")
	}
	if c.ImageV2.MaxPartBytes < 1<<20 || c.ImageV2.MaxPartBytes > 64<<20 {
		return fmt.Errorf("V2_MAX_PART_BYTES must be between 1 MiB and 64 MiB")
	}
	if c.ImageV2.MaxPixels < 1_000_000 || c.ImageV2.MaxPixels > 100_000_000 {
		return fmt.Errorf("V2_MAX_PIXELS must be between 1MP and 100MP")
	}
	if c.ImageV2.SessionTTL < time.Minute || c.ImageV2.SessionTTL > 24*time.Hour {
		return fmt.Errorf("V2_SESSION_TTL must be between 1m and 24h")
	}
	if c.ImageV2.GlobalUploadConcurrency < 1 || c.ImageV2.GlobalUploadConcurrency > 32 {
		return fmt.Errorf("V2_GLOBAL_UPLOAD_CONCURRENCY must be between 1 and 32")
	}
	if c.ImageV2.PerUserConcurrency < 1 || c.ImageV2.PerUserConcurrency > c.ImageV2.GlobalUploadConcurrency {
		return fmt.Errorf("V2_PER_USER_UPLOAD_CONCURRENCY must be between 1 and the global limit")
	}
	if c.ImageV2.WatermarkConcurrency < 1 || c.ImageV2.WatermarkConcurrency > 2 {
		return fmt.Errorf("V2_WATERMARK_CONCURRENCY must be 1 or 2")
	}
	if c.ImageV2.JobPollInterval < 100*time.Millisecond || c.ImageV2.JobPollInterval > time.Minute {
		return fmt.Errorf("V2_JOB_POLL_INTERVAL must be between 100ms and 1m")
	}
	if c.ImageV2.JobLease < 2*time.Minute || c.ImageV2.JobLease > 10*time.Minute {
		return fmt.Errorf("V2_JOB_LEASE must be between 2m and 10m")
	}
	if c.Storage.DiskSoftPct < 50 || c.Storage.DiskHardPct > 98 || c.Storage.DiskSoftPct >= c.Storage.DiskHardPct {
		return fmt.Errorf("disk thresholds must satisfy 50 <= soft < hard <= 98")
	}
	if c.CDN.OutboxBatchSize < 1 || c.CDN.OutboxBatchSize > 50 {
		return fmt.Errorf("OUTBOX_BATCH_SIZE must be between 1 and 50")
	}
	if c.CDN.OutboxPollInterval < 100*time.Millisecond || c.CDN.OutboxPollInterval > time.Minute {
		return fmt.Errorf("OUTBOX_POLL_INTERVAL must be between 100ms and 1m")
	}
	if c.CDN.OutboxLease < 10*time.Second || c.CDN.OutboxLease > 10*time.Minute {
		return fmt.Errorf("OUTBOX_LEASE must be between 10s and 10m")
	}
	if c.CDN.PurgeRequestsPerSecond < 1 || c.CDN.PurgeRequestsPerSecond > 20 {
		return fmt.Errorf("CDN_PURGE_REQUESTS_PER_SECOND must be between 1 and 20")
	}
	if c.CDN.PurgeRequestTimeout < time.Second || c.CDN.PurgeRequestTimeout > time.Minute {
		return fmt.Errorf("CDN_PURGE_REQUEST_TIMEOUT must be between 1s and 1m")
	}
	if c.CDN.OutboxLease < c.CDN.PurgeRequestTimeout+5*time.Second {
		return fmt.Errorf("OUTBOX_LEASE must exceed CDN_PURGE_REQUEST_TIMEOUT by at least 5s")
	}
	cloudflareConfigured := c.CDN.CloudflareZoneID != "" || c.CDN.CloudflareAPIToken != ""
	if cloudflareConfigured && (c.CDN.CloudflareZoneID == "" || c.CDN.CloudflareAPIToken == "") {
		return fmt.Errorf("CLOUDFLARE_ZONE_ID and CLOUDFLARE_API_TOKEN must be configured together")
	}
	if c.CDN.PurgeWebhookToken != "" && c.CDN.PurgeWebhookURL == "" {
		return fmt.Errorf("CDN_PURGE_WEBHOOK_TOKEN requires CDN_PURGE_WEBHOOK_URL")
	}
	cdnDeliveryConfigured := cloudflareConfigured || c.CDN.PurgeWebhookURL != ""
	if cdnDeliveryConfigured && c.CDN.PublicBaseURL == "" {
		return fmt.Errorf("CDN_PUBLIC_BASE_URL is required when CDN purge delivery is configured")
	}
	if c.CDN.PublicBaseURL != "" {
		if err := validateHTTPBaseURL(c.CDN.PublicBaseURL); err != nil {
			return fmt.Errorf("CDN_PUBLIC_BASE_URL: %w", err)
		}
	}
	if cloudflareConfigured {
		if err := validateHTTPBaseURL(c.CDN.CloudflareAPIBaseURL); err != nil {
			return fmt.Errorf("CLOUDFLARE_API_BASE_URL: %w", err)
		}
	}
	if c.CDN.PurgeWebhookURL != "" {
		if err := validateHTTPBaseURL(c.CDN.PurgeWebhookURL); err != nil {
			return fmt.Errorf("CDN_PURGE_WEBHOOK_URL: %w", err)
		}
	}
	base, err := filepath.Abs(c.Storage.BasePath)
	if err != nil {
		return fmt.Errorf("resolve STORAGE_PATH: %w", err)
	}
	staging, err := filepath.Abs(c.Storage.StagingPath)
	if err != nil {
		return fmt.Errorf("resolve V2_STAGING_PATH: %w", err)
	}
	rel, err := filepath.Rel(base, staging)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("V2_STAGING_PATH must be a child of STORAGE_PATH for atomic promotion")
	}
	return nil
}

// ValidateCaptchaCrossOriginIsolation rejects providers whose client script
// cannot be embedded under COEP require-corp. The V2 browser pipeline keeps
// cross-origin isolation enabled by default for large wasm-vips workloads.
func ValidateCaptchaCrossOriginIsolation(provider string, enabled bool) error {
	if enabled && strings.EqualFold(strings.TrimSpace(provider), "geetest_v4") {
		return fmt.Errorf("geetest_v4 is incompatible with CROSS_ORIGIN_ISOLATION=true; choose another CAPTCHA provider or disable cross-origin isolation")
	}
	return nil
}

func validateHTTPBaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("must be an absolute http(s) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("must not contain credentials, query, or fragment")
	}
	return nil
}

func (c *Config) DSN() string {
	return c.Database.User + ":" + c.Database.Password +
		"@tcp(" + c.Database.Host + ":" + c.Database.Port + ")/" +
		c.Database.DBName + "?charset=utf8mb4&parseTime=True&loc=Local"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if duration, err := time.ParseDuration(v); err == nil {
			return duration
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i <= len(value); i++ {
		if i == len(value) || value[i] == ',' {
			if item := trimSpace(value[start:i]); item != "" {
				out = append(out, item)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(value string) string {
	start := 0
	for start < len(value) && (value[start] == ' ' || value[start] == '\t' || value[start] == '\n' || value[start] == '\r') {
		start++
	}
	end := len(value)
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t' || value[end-1] == '\n' || value[end-1] == '\r') {
		end--
	}
	return value[start:end]
}
