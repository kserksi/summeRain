// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/summerain/image-gallery/internal/config"
)

func TestImgproxyServiceProcessTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	svc := NewImgproxyService(&config.ImgproxyConfig{})
	svc.client.Timeout = 5 * time.Millisecond

	if _, err := svc.Process(server.URL); err == nil {
		t.Fatalf("Process returned nil error for timed-out request")
	}
}

func TestSignPathUsesInsecureWithoutKey(t *testing.T) {
	svc := NewImgproxyService(&config.ImgproxyConfig{BaseURL: "http://imgproxy:8080"})
	url := svc.ThumbnailURL("original/test.jpg")
	if !strings.Contains(url, "/insecure/") {
		t.Fatalf("expected /insecure/ in URL, got %s", url)
	}
}

func TestSignPathUsesSignatureWithKey(t *testing.T) {
	svc := NewImgproxyService(&config.ImgproxyConfig{
		BaseURL: "http://imgproxy:8080",
		Key:     "8ddf4ab32a5dcd73b87329545c0fc19bf901d8e8d761d7561ae1a165bab1720e",
		Salt:    "f237c63b2c909f3aa8b6b1acadc1b80670207841b6da4a5eb78826a62ac45f84",
	})
	url := svc.ThumbnailURL("original/test.jpg")
	if strings.Contains(url, "/insecure/") {
		t.Fatalf("signed URL should not contain /insecure/, got %s", url)
	}
	if !strings.HasPrefix(url, "http://imgproxy:8080/") {
		t.Fatalf("URL should start with BaseURL, got %s", url)
	}
}

func TestProcessedURLWatermarkFormat(t *testing.T) {
	svc := NewImgproxyService(&config.ImgproxyConfig{
		BaseURL: "http://imgproxy:8080",
		Key:     "8ddf4ab32a5dcd73b87329545c0fc19bf901d8e8d761d7561ae1a165bab1720e",
		Salt:    "f237c63b2c909f3aa8b6b1acadc1b80670207841b6da4a5eb78826a62ac45f84",
	})
	url := svc.ProcessedURL("temp/test.jpg", true, "kserks", "0.7", "ce", "64", "66ccff")

	if strings.Contains(url, "/wmt:") {
		t.Fatalf("URL should not contain /wmt: (PRO-only), got %s", url)
	}
	if !strings.Contains(url, "/wm:0.7:ce") {
		t.Fatalf("URL should contain /wm:0.7:ce for watermark, got %s", url)
	}
}

func TestProcessedURLNoWatermark(t *testing.T) {
	svc := NewImgproxyService(&config.ImgproxyConfig{BaseURL: "http://imgproxy:8080"})
	url := svc.ProcessedURL("temp/test.jpg", false, "", "", "", "", "")
	if strings.Contains(url, "/wm:") {
		t.Fatalf("URL should not contain watermark options, got %s", url)
	}
}
