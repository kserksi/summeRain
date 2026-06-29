// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/summerain/image-gallery/internal/config"
	"github.com/summerain/image-gallery/internal/model"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
)

type PublicConfigReader interface {
	FindAll() ([]model.SystemConfig, error)
}

type PublicConfigService struct {
	configReader PublicConfigReader
	captcha      config.CaptchaConfig
	rdb          *redis.Client
}

func NewPublicConfigService(configReader PublicConfigReader, captcha config.CaptchaConfig, rdb *redis.Client) *PublicConfigService {
	return &PublicConfigService{configReader: configReader, captcha: captcha, rdb: rdb}
}

type PublicConfigResult struct {
	CaptchaProvider string `json:"captcha_provider"`
	CaptchaSiteKey  string `json:"captcha_site_key"`
}

func (s *PublicConfigService) Get() (*PublicConfigResult, *errcode.AppError) {
	result := &PublicConfigResult{CaptchaProvider: "none", CaptchaSiteKey: ""}
	if s != nil {
		result.CaptchaProvider = s.captcha.Provider
		result.CaptchaSiteKey = providerSiteKey(s.captcha)
	}
	if s == nil || s.configReader == nil {
		return result, nil
	}
	configs, err := s.configReader.FindAll()
	if err != nil {
		return nil, errcode.ErrDatabase
	}
	for _, cfg := range configs {
		value := strings.TrimSpace(cfg.ConfigValue)
		switch cfg.ConfigKey {
		case "captcha_provider":
			if value != "" {
				result.CaptchaProvider = value
				result.CaptchaSiteKey = providerSiteKey(configForProvider(s.captcha, value))
			}
		case "recaptcha_site_key":
			if result.CaptchaProvider == "recaptcha" && value != "" {
				result.CaptchaSiteKey = value
			}
		case "turnstile_site_key":
			if result.CaptchaProvider == "turnstile" && value != "" {
				result.CaptchaSiteKey = value
			}
		case "geetest_captcha_id":
			if result.CaptchaProvider == "geetest_v4" && value != "" {
				result.CaptchaSiteKey = value
			}
		}
	}
	return result, nil
}

// providerSiteKey returns the client-side public key for the configured
// provider (recaptcha/turnstile site key, geetest captcha_id), or "" for none.
func providerSiteKey(c config.CaptchaConfig) string {
	switch c.Provider {
	case "recaptcha":
		return strings.TrimSpace(c.Recaptcha.SiteKey)
	case "turnstile":
		return strings.TrimSpace(c.Turnstile.SiteKey)
	case "geetest_v4":
		return strings.TrimSpace(c.Geetest.CaptchaID)
	}
	return ""
}

// configForProvider returns a copy of the config with the provider overridden,
// so providerSiteKey resolves the right key after an admin override.
func configForProvider(base config.CaptchaConfig, provider string) config.CaptchaConfig {
	out := base
	out.Provider = provider
	return out
}

type WatermarkConfig struct {
	Enabled  bool   `json:"enabled"`
	Opacity  string `json:"opacity"`
	Position string `json:"position"`
}

func (s *PublicConfigService) GetWatermark() *WatermarkConfig {
	if s == nil || s.configReader == nil {
		return &WatermarkConfig{}
	}
	if s.rdb != nil {
		if data, err := s.rdb.Get(context.Background(), "wm:config").Result(); err == nil {
			var wm WatermarkConfig
			if json.Unmarshal([]byte(data), &wm) == nil {
				return &wm
			}
		}
	}
	wm := &WatermarkConfig{Opacity: "0.5", Position: "soea"}
	configs, err := s.configReader.FindAll()
	if err != nil {
		return wm
	}
	for _, cfg := range configs {
		switch cfg.ConfigKey {
		case "watermark_enabled":
			wm.Enabled = strings.TrimSpace(cfg.ConfigValue) == "true"
		case "watermark_opacity":
			if v := strings.TrimSpace(cfg.ConfigValue); v != "" {
				wm.Opacity = v
			}
		case "watermark_position":
			if v := strings.TrimSpace(cfg.ConfigValue); v != "" {
				wm.Position = v
			}
		}
	}
	if s.rdb != nil {
		if data, err := json.Marshal(wm); err == nil {
			s.rdb.Set(context.Background(), "wm:config", data, 60*time.Second)
		}
	}
	return wm
}
