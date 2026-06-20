package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/summerain/image-gallery/internal/config"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
)

type RecaptchaVerifier struct {
	cfg    config.RecaptchaConfig
	client *http.Client
}

func NewRecaptchaVerifier(cfg config.RecaptchaConfig) *RecaptchaVerifier {
	return &RecaptchaVerifier{cfg: cfg, client: &http.Client{Timeout: 2 * time.Second}}
}

func (v *RecaptchaVerifier) Verify(ctx context.Context, payload CaptchaPayload, remoteIP string, requestHost string) *errcode.AppError {
	if v == nil || !v.cfg.Enabled {
		return nil
	}
	if v.cfg.Secret == "" {
		return errcode.ErrRecaptchaUnavailable
	}
	if len(v.cfg.AllowedHostnames) == 0 {
		return errcode.ErrRecaptchaUnavailable
	}
	if payload.Token == "" || payload.Action != payload.ExpectedAction {
		return errcode.ErrRecaptchaFailed
	}

	form := url.Values{}
	form.Set("secret", v.cfg.Secret)
	form.Set("response", payload.Token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.cfg.VerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return errcode.ErrRecaptchaUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.client.Do(req)
	if err != nil {
		if v.cfg.FailClosed {
			return errcode.ErrRecaptchaUnavailable
		}
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if v.cfg.FailClosed {
			return errcode.ErrRecaptchaUnavailable
		}
		return nil
	}

	var result recaptchaVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		if v.cfg.FailClosed {
			return errcode.ErrRecaptchaUnavailable
		}
		return nil
	}
	if !result.Success || result.Action != payload.ExpectedAction || result.Score < v.cfg.MinScore || !v.hostnameAllowed(result.Hostname, requestHost) {
		return errcode.ErrRecaptchaFailed
	}
	return nil
}

type recaptchaVerifyResponse struct {
	Success  bool    `json:"success"`
	Score    float64 `json:"score"`
	Action   string  `json:"action"`
	Hostname string  `json:"hostname"`
}

func (v *RecaptchaVerifier) hostnameAllowed(hostname string, requestHost string) bool {
	if hostname == "" {
		return false
	}
	for _, allowed := range v.cfg.AllowedHostnames {
		if hostname == allowed {
			return true
		}
	}
	requestHost = strings.Split(requestHost, ":")[0]
	return false
}
