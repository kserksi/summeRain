// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kserksi/summerain/internal/config"
)

type ImgproxyService struct {
	cfg    *config.ImgproxyConfig
	client *http.Client
}

func NewImgproxyService(cfg *config.ImgproxyConfig) *ImgproxyService {
	return &ImgproxyService{cfg: cfg, client: &http.Client{Timeout: 90 * time.Second}}
}

func (s *ImgproxyService) signPath(path string) string {
	if s.cfg.Key == "" || s.cfg.Salt == "" {
		return s.cfg.BaseURL + "/insecure" + path
	}
	key, _ := hex.DecodeString(s.cfg.Key)
	salt, _ := hex.DecodeString(s.cfg.Salt)
	mac := hmac.New(sha256.New, key)
	mac.Write(salt)
	mac.Write([]byte(path))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return s.cfg.BaseURL + "/" + sig + path
}

func (s *ImgproxyService) ThumbnailURL(sourcePath string) string {
	sourcePath = strings.TrimPrefix(sourcePath, s.cfg.LocalFSRoot+"/")
	path := fmt.Sprintf("/rs:fill:300:300/quality:75/plain/local:///%s@webp", sourcePath)
	return s.signPath(path)
}

var imgproxyGravity = map[string]bool{
	"no": true, "so": true, "ea": true, "we": true,
	"noea": true, "nowe": true, "soea": true, "sowe": true,
	"ce": true,
}

func (s *ImgproxyService) ProcessedURL(sourcePath string, watermarkEnabled bool, watermarkText string, watermarkOpacity string, watermarkPosition string, watermarkSize string, watermarkColor string) string {
	path := "/quality:80"
	if watermarkEnabled && watermarkText != "" {
		if !validOpacity(watermarkOpacity) {
			watermarkOpacity = "0.5"
		}
		if !imgproxyGravity[watermarkPosition] {
			watermarkPosition = "soea"
		}
		path += fmt.Sprintf("/wm:%s:%s", watermarkOpacity, watermarkPosition)
	}
	sourcePath = strings.TrimPrefix(sourcePath, s.cfg.LocalFSRoot+"/")
	path += fmt.Sprintf("/plain/local:///%s@webp", sourcePath)
	return s.signPath(path)
}

func validOpacity(s string) bool {
	f, err := strconv.ParseFloat(s, 64)
	return err == nil && f >= 0 && f <= 1
}

func sanitizeWatermarkText(s string) string {
	s = strings.TrimSpace(s)
	for _, r := range s {
		if r == '/' || r < 0x20 {
			return ""
		}
	}
	return s
}

func (s *ImgproxyService) Process(url string) ([]byte, error) {
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imgproxy returned status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// ProcessToFile is the bounded streaming path used by V2 publish jobs. V1
// keeps Process for compatibility, while large V2 payloads never accumulate in
// a single Go byte slice.
func (s *ImgproxyService) ProcessToFile(ctx context.Context, url, destination string, maxBytes int64) (string, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("imgproxy returned status %d", resp.StatusCode)
	}

	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return "", 0, err
	}
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(destination)
		}
	}()

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", 0, err
	}
	if written > maxBytes {
		return "", 0, fmt.Errorf("imgproxy response exceeds %d bytes", maxBytes)
	}
	if err := file.Sync(); err != nil {
		return "", 0, err
	}
	if err := file.Close(); err != nil {
		return "", 0, err
	}
	keep = true
	return hex.EncodeToString(hasher.Sum(nil)), written, nil
}
