// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeR2BaseURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "endpoint", raw: " https://account.r2.example/// ", want: "https://account.r2.example"},
		{name: "cdn path", raw: "https://cdn.example/images/", want: "https://cdn.example/images"},
		{name: "relative", raw: "cdn.example/images", wantErr: true},
		{name: "unsupported scheme", raw: "javascript:alert(1)", wantErr: true},
		{name: "credentials", raw: "https://user:pass@cdn.example", wantErr: true},
		{name: "query", raw: "https://cdn.example/images?token=secret", wantErr: true},
		{name: "empty query", raw: "https://cdn.example/images?", wantErr: true},
		{name: "fragment", raw: "https://cdn.example/images#fragment", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeR2BaseURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeR2BaseURL(%q) = %q, nil", tt.raw, got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("normalizeR2BaseURL(%q) = %q, %v; want %q", tt.raw, got, err, tt.want)
			}
		})
	}
}

func TestR2HTTPClientBoundsConnectionsAndHeaders(t *testing.T) {
	client, transport := newR2HTTPClient()
	defer transport.CloseIdleConnections()
	if client.Transport != transport {
		t.Fatal("R2 HTTP client does not use the bounded transport")
	}
	if transport.MaxConnsPerHost != 16 || transport.MaxIdleConnsPerHost != 8 {
		t.Fatalf("unexpected connection limits: max=%d idle=%d", transport.MaxConnsPerHost, transport.MaxIdleConnsPerHost)
	}
	if transport.ResponseHeaderTimeout <= 0 || transport.MaxResponseHeaderBytes != 1<<20 {
		t.Fatalf("unexpected response bounds: timeout=%s headers=%d", transport.ResponseHeaderTimeout, transport.MaxResponseHeaderBytes)
	}
}

func TestR2TargetMatchingRemainsAvailableWhenUploadsAreDisabled(t *testing.T) {
	svc := &R2Service{
		configured: true,
		enabled:    false,
		endpoint:   "https://account.r2.example",
		bucket:     "images",
		publicURL:  "https://cdn.example",
	}

	if svc.IsEnabled() {
		t.Fatal("uploads should remain disabled")
	}
	if !svc.CanDelete("https://account.r2.example/", "images") {
		t.Fatal("matching historical target should remain deletable")
	}
	if svc.CanDelete("https://other.r2.example", "images") || svc.CanDelete("https://account.r2.example", "other") {
		t.Fatal("a mismatched target must fail closed")
	}
	got, err := svc.PublicURLForTarget("https://account.r2.example", "images", "original/image.webp")
	if err != nil || got != "https://cdn.example/original/image.webp" {
		t.Fatalf("PublicURLForTarget() = %q, %v", got, err)
	}
	if _, err := svc.PublicURLForTarget("https://other.r2.example", "images", "original/image.webp"); err == nil {
		t.Fatal("mismatched public target was accepted")
	}
}

func TestR2DeleteTreatsMissingObjectAsIdempotentSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()
	target := newR2UploadBarrierSnapshot(server.URL, "images", server.Client())

	if err := target.delete(context.Background(), "original/missing.webp"); err != nil {
		t.Fatalf("delete missing R2 object: %v", err)
	}
}
