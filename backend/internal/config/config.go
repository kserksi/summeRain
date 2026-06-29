// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"strconv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Imgproxy ImgproxyConfig
	Storage  StorageConfig
	Captcha  CaptchaConfig
}

type ServerConfig struct {
	Port         string
	Mode         string // debug, release
	CookieSecret string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type ImgproxyConfig struct {
	BaseURL   string
	Key       string
	Salt      string
	PublicURL string
}

type StorageConfig struct {
	BasePath string
	TempPath string
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
	return &Config{
		Server: ServerConfig{
			Port:         getEnv("SERVER_PORT", "8080"),
			Mode:         getEnv("GIN_MODE", "debug"),
			CookieSecret: getEnv("COOKIE_SECRET", "change-me-in-production"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "mysql"),
			Port:     getEnv("DB_PORT", "3306"),
			User:     getEnv("DB_USER", "root"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "image_gallery"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "redis:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Imgproxy: ImgproxyConfig{
			BaseURL:   getEnv("IMGPROXY_URL", "http://imgproxy:8080"),
			Key:       getEnv("IMGPROXY_KEY", ""),
			Salt:      getEnv("IMGPROXY_SALT", ""),
			PublicURL: getEnv("IMGPROXY_PUBLIC_URL", "/img"),
		},
		Storage: StorageConfig{
			BasePath: getEnv("STORAGE_PATH", "/data/images"),
			TempPath: getEnv("TEMP_PATH", "/data/temp"),
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
