// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package imagegallery_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type requirementsLock struct {
	Toolchains struct {
		Go struct {
			Version string `json:"version"`
		} `json:"go"`
		Node struct {
			Version string `json:"version"`
		} `json:"node"`
	} `json:"toolchains"`
	Services struct {
		Alpine struct {
			Image string `json:"image"`
		} `json:"alpine"`
		MySQL struct {
			Image string `json:"image"`
		} `json:"mysql"`
		Redis struct {
			Image string `json:"image"`
		} `json:"redis"`
		Imgproxy struct {
			Image string `json:"image"`
		} `json:"imgproxy"`
	} `json:"services"`
}

func TestDockerfileUsesLockedToolchains(t *testing.T) {
	lock := readRequirementsLock(t)
	dockerfile := readTestFile(t, "../Dockerfile")

	for _, want := range []string{
		"FROM --platform=$BUILDPLATFORM golang:" + lock.Toolchains.Go.Version + "-alpine AS backend-builder",
		"FROM --platform=$BUILDPLATFORM node:" + lock.Toolchains.Node.Version + "-alpine AS frontend-builder",
		"FROM " + lock.Services.Alpine.Image,
	} {
		if !strings.Contains(dockerfile, want) {
			t.Fatalf("Dockerfile builder image mismatch: want line %q", want)
		}
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

func TestComposeMatchesRequirementsLock(t *testing.T) {
	lock := readRequirementsLock(t)

	for _, path := range []string{"docker-compose.yml", "docker-compose.deploy.yml"} {
		compose := readTestFile(t, path)
		for _, image := range []string{lock.Services.Alpine.Image, lock.Services.MySQL.Image, lock.Services.Redis.Image, lock.Services.Imgproxy.Image} {
			if !strings.Contains(compose, "image: "+image) {
				t.Errorf("%s does not use locked image %s", path, image)
			}
		}
		for _, want := range []string{
			"env_file:\n      - .env",
			"GOMEMLIMIT: 512MiB",
			"TEMP_PATH: /data/images/.staging",
			"image_storage:/data/images",
			"IMGPROXY_WORKERS: ${IMGPROXY_WORKERS:-2}",
			"IMGPROXY_REQUESTS_QUEUE_SIZE: \"4\"",
			"IMGPROXY_KEY: ${IMGPROXY_KEY:?Set IMGPROXY_KEY}",
			"IMGPROXY_SALT: ${IMGPROXY_SALT:?Set IMGPROXY_SALT}",
			"cpus:",
			"mem_limit:",
			"pids_limit:",
			"max-size: \"10m\"",
		} {
			if !strings.Contains(compose, want) {
				t.Errorf("%s is missing %q", path, want)
			}
		}
		if strings.Contains(compose, "temp_storage") {
			t.Errorf("%s must keep staging on image_storage", path)
		}
	}
}

func TestEnvExampleDocumentsResourceGuardrails(t *testing.T) {
	env := readTestFile(t, ".env.example")
	version := strings.TrimSpace(readTestFile(t, "../VERSION"))
	for _, want := range []string{
		"DOCKER_IMAGE=jaykserks/summerain:v" + version,
		"DB_PASSWORD=your_password_here",
		"IMGPROXY_KEY=replace_with_hex_key",
		"IMGPROXY_SALT=replace_with_hex_salt",
		"CROSS_ORIGIN_ISOLATION=true",
		"DB_MAX_OPEN_CONNS=8",
		"DB_MAX_IDLE_CONNS=4",
		"DB_CONN_MAX_LIFETIME=30m",
		"REDIS_POOL_SIZE=8",
		"DISK_SOFT_LIMIT_PERCENT=80",
		"DISK_HARD_LIMIT_PERCENT=90",
	} {
		if !strings.Contains(env, want) {
			t.Errorf(".env.example is missing %q", want)
		}
	}
}

func TestDefaultComposePullsApplicationImage(t *testing.T) {
	compose := readTestFile(t, "docker-compose.yml")
	if strings.Contains(compose, "\n    build:") {
		t.Fatal("default compose must pull the application image instead of building locally")
	}
	if !strings.Contains(compose, "image: ${DOCKER_IMAGE:-jaykserks/summerain:latest}") {
		t.Fatal("default compose must follow the stable latest image unless DOCKER_IMAGE is explicit")
	}
	if !strings.Contains(compose, "MYSQL_ROOT_PASSWORD: ${DB_PASSWORD:?Set DB_PASSWORD}") {
		t.Fatal("default compose must use the same DB_PASSWORD as the backend env file")
	}
	if strings.Contains(compose, "MYSQL_ROOT_PASSWORD_FILE") || strings.Contains(compose, "secrets:") {
		t.Fatal("default compose must not use a separate MySQL password source")
	}
}

func TestDevDependencyComposeMatchesRequirementsLock(t *testing.T) {
	lock := readRequirementsLock(t)
	compose := readTestFile(t, "docker-compose.dev-deps.yml")

	for _, want := range []string{
		"image: " + lock.Services.MySQL.Image,
		"image: " + lock.Services.Redis.Image,
		"image: " + lock.Services.Imgproxy.Image,
		"IMGPROXY_WORKERS: ${IMGPROXY_WORKERS:-2}",
		"IMGPROXY_REQUESTS_QUEUE_SIZE: \"4\"",
	} {
		if !strings.Contains(compose, want) {
			t.Errorf("development dependency compose is missing %q", want)
		}
	}
	if strings.Contains(compose, "\n    build:") {
		t.Fatal("development dependency compose must not build the application image")
	}
}

func TestDeployComposeRequiresExplicitApplicationImage(t *testing.T) {
	compose := readTestFile(t, "docker-compose.deploy.yml")
	if strings.Contains(compose, "\n    build:") {
		t.Fatal("production compose must not build images locally")
	}
	if strings.Contains(compose, ":latest") {
		t.Fatal("production compose must not use floating latest tags")
	}
	if !strings.Contains(compose, "DOCKER_IMAGE:?") {
		t.Fatal("production compose must require an explicit application image")
	}
}

func readRequirementsLock(t *testing.T) requirementsLock {
	t.Helper()
	content, err := os.ReadFile("../requirements.lock")
	if err != nil {
		t.Fatalf("read requirements.lock: %v", err)
	}
	var lock requirementsLock
	if err := json.Unmarshal(content, &lock); err != nil {
		t.Fatalf("parse requirements.lock: %v", err)
	}
	if lock.Toolchains.Go.Version == "" || lock.Toolchains.Node.Version == "" {
		t.Fatal("requirements.lock is missing toolchain versions")
	}
	return lock
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.ReplaceAll(string(content), "\r\n", "\n")
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
