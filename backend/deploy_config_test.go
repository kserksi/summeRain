// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package imagegallery_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestDockerfileUsesGoVersionFromModule(t *testing.T) {
	goMod := readTestFile(t, "go.mod")
	dockerfile := readTestFile(t, "Dockerfile")

	version := regexp.MustCompile(`(?m)^go\s+(\d+)\.(\d+)\.\d+`).FindStringSubmatch(goMod)
	if len(version) != 3 {
		t.Fatalf("go.mod does not declare a full Go version")
	}
	wantBuilder := "FROM golang:" + version[1] + "." + version[2] + "-alpine AS builder"

	if !strings.Contains(dockerfile, wantBuilder) {
		t.Fatalf("Dockerfile builder image mismatch: want line %q", wantBuilder)
	}
}

func TestComposeBackendHasHealthcheck(t *testing.T) {
	compose := readTestFile(t, "docker-compose.yml")
	backendBlock := serviceBlock(t, compose, "backend", "mysql")

	for _, want := range []string{"healthcheck:", "http://localhost:8080/health", "interval:", "timeout:", "retries:"} {
		if !strings.Contains(backendBlock, want) {
			t.Fatalf("backend service block missing %q", want)
		}
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func serviceBlock(t *testing.T, compose string, service string, nextService string) string {
	t.Helper()
	startMarker := "\n  " + service + ":\n"
	endMarker := "\n  " + nextService + ":\n"
	compose = "\n" + compose
	start := strings.Index(compose, startMarker)
	if start == -1 {
		t.Fatalf("compose missing service %s", service)
	}
	end := strings.Index(compose[start+len(startMarker):], endMarker)
	if end == -1 {
		t.Fatalf("compose missing next service %s", nextService)
	}
	return compose[start : start+len(startMarker)+end]
}
