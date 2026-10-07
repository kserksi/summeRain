// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func captureClientErrorLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buffer := &bytes.Buffer{}
	previous := log.Writer()
	log.SetOutput(buffer)
	t.Cleanup(func() { log.SetOutput(previous) })
	return buffer
}

func TestClientErrorServiceWritesOneBoundedLine(t *testing.T) {
	buffer := captureClientErrorLog(t)
	svc := NewClientErrorService()

	appErr := svc.Report(ClientErrorReport{
		Kind:           "Boundary",
		Message:        "  " + strings.Repeat("→", 400) + "  ",
		Stack:          strings.Repeat("s", 5000),
		ComponentStack: "at Upload",
		Source:         "app.js:10:5",
		Path:           "/upload",
	}, ClientErrorMeta{RequestID: "req_test", UserAgent: "test-agent"})
	if appErr != nil {
		t.Fatalf("Report() error = %#v, want nil", appErr)
	}

	line := strings.TrimSuffix(buffer.String(), "\n")
	if strings.Contains(line, "\n") {
		t.Fatalf("report must be a single line: %q", line)
	}
	for _, fragment := range []string{
		"request_id=req_test",
		"kind=boundary",
		`path="/upload"`,
		`component_stack="at Upload"`,
		`source="app.js:10:5"`,
		`user_agent="test-agent"`,
	} {
		if !strings.Contains(line, fragment) {
			t.Fatalf("log line %q is missing %q", line, fragment)
		}
	}
	if strings.Contains(line, strings.Repeat("→", clientErrorMessageBytes/3+1)) {
		t.Fatalf("message was not truncated to %d bytes: %q", clientErrorMessageBytes, line)
	}
	if strings.Contains(line, strings.Repeat("s", clientErrorStackBytes+1)) {
		t.Fatalf("stack was not truncated to %d bytes", clientErrorStackBytes)
	}
}

func TestClientErrorServiceRejectsUnusableReports(t *testing.T) {
	buffer := captureClientErrorLog(t)
	svc := NewClientErrorService()

	tests := []struct {
		name   string
		report ClientErrorReport
	}{
		{name: "unknown kind", report: ClientErrorReport{Kind: "poster", Message: "boom"}},
		{name: "missing kind", report: ClientErrorReport{Message: "boom"}},
		{name: "empty message", report: ClientErrorReport{Kind: "error", Message: "   "}},
		{name: "message beyond cap", report: ClientErrorReport{Kind: "error", Message: ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if appErr := svc.Report(tt.report, ClientErrorMeta{}); appErr == nil || appErr.HTTP != 400 {
				t.Fatalf("Report() error = %#v, want HTTP 400", appErr)
			}
		})
	}
	if buffer.Len() != 0 {
		t.Fatalf("rejected reports must not be logged: %q", buffer.String())
	}
}

func TestTruncateUTF8KeepsRuneBoundaries(t *testing.T) {
	value := strings.Repeat("a", 299) + "→"
	if got := truncateUTF8(value, 300); got != strings.Repeat("a", 299) {
		t.Fatalf("truncateUTF8() = %q, want the 299 ASCII prefix", got)
	}
	if got := truncateUTF8("short", 300); got != "short" {
		t.Fatalf("truncateUTF8() = %q, want the input unchanged", got)
	}
}

func TestClientErrorServiceKeepsRequestMetadataOutOfTheMessage(t *testing.T) {
	buffer := captureClientErrorLog(t)
	svc := NewClientErrorService()

	if appErr := svc.Report(ClientErrorReport{
		Kind:    "unhandledrejection",
		Message: "network request failed",
	}, ClientErrorMeta{RequestID: "req_meta"}); appErr != nil {
		t.Fatalf("Report() error = %#v, want nil", appErr)
	}

	line := buffer.String()
	for _, forbidden := range []string{"cookie", "token", "authorization", "password"} {
		if strings.Contains(strings.ToLower(line), forbidden) {
			t.Fatalf("log line must not contain %q: %q", forbidden, line)
		}
	}
}
