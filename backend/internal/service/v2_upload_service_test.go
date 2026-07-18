// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestUploadManifestMatchesRequiresSameHash(t *testing.T) {
	hash := "manifest-a"
	session := &model.UploadSession{ManifestHash: &hash}

	if !uploadManifestMatches(session, hash) {
		t.Fatal("same manifest hash should match")
	}
	if uploadManifestMatches(session, "manifest-b") {
		t.Fatal("different manifest hash must not match")
	}
	if uploadManifestMatches(&model.UploadSession{}, hash) {
		t.Fatal("missing stored manifest hash must not match")
	}
}

func TestV2CapacityGateWaitsOutsideDatabaseAndReleasesOnce(t *testing.T) {
	gate := make(chan struct{}, 1)
	release, ok := acquireBoundedV2Gate(context.Background(), gate, time.Second)
	if !ok {
		t.Fatal("first admission was rejected")
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, ok := acquireBoundedV2Gate(waitCtx, gate, time.Second); ok {
		t.Fatal("second admission entered an occupied gate")
	}

	release()
	release()
	secondRelease, ok := acquireBoundedV2Gate(context.Background(), gate, time.Second)
	if !ok {
		t.Fatal("gate was not released")
	}
	secondRelease()
}

func TestCompletePrevalidatesTargetsBeforeCapacityAdmission(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "summerain:summerain@tcp(127.0.0.1:1)/summerain?parseTime=true",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}

	var queryCount atomic.Int32
	if err := db.Callback().Query().Before("gorm:query").Register("test:count_prevalidation", func(*gorm.DB) {
		queryCount.Add(1)
	}); err != nil {
		t.Fatal(err)
	}

	svc := &V2UploadService{db: db}
	release, admitted := svc.acquireCapacityGate(context.Background())
	if !admitted {
		t.Fatal("failed to occupy capacity gate")
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, appErr := svc.Complete(ctx, 1, "upload-key"); appErr == nil || appErr.Code != errcode.ErrUploadBusy.Code {
		t.Fatalf("Complete() error = %#v, want upload busy", appErr)
	}
	if queryCount.Load() == 0 {
		t.Fatal("Complete() waited for capacity before prevalidating immutable targets")
	}
}

func TestStreamV2WebPPartWritesHashesAndValidatesInOnePass(t *testing.T) {
	payload := simpleLosslessWebP(400, 300)
	var destination bytes.Buffer

	written, hash, info, err := streamV2WebPPart(&destination, bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("streamV2WebPPart() error = %v", err)
	}
	if written != int64(len(payload)) || hash != testSHA256(payload) {
		t.Fatalf("stream result = (%d, %q), want (%d, %q)", written, hash, len(payload), testSHA256(payload))
	}
	if info.Width != 400 || info.Height != 300 || info.Animated {
		t.Fatalf("stream info = %+v", info)
	}
	if !bytes.Equal(destination.Bytes(), payload) {
		t.Fatal("streamed destination differs from request body")
	}
}

func TestStreamV2WebPPartRejectsShortAndTrailingBodies(t *testing.T) {
	payload := simpleLosslessWebP(32, 24)
	for name, body := range map[string][]byte{
		"short":    payload[:len(payload)-1],
		"trailing": append(append([]byte(nil), payload...), 0),
	} {
		t.Run(name, func(t *testing.T) {
			var destination bytes.Buffer
			_, _, _, err := streamV2WebPPart(&destination, bytes.NewReader(body), int64(len(payload)))
			if !errors.Is(err, errV2PartSize) {
				t.Fatalf("streamV2WebPPart() error = %v, want size mismatch", err)
			}
		})
	}
}

func TestStreamV2WebPPartPreservesInvalidAndTransportErrors(t *testing.T) {
	payload := simpleLosslessWebP(32, 24)
	invalid := append([]byte(nil), payload...)
	invalid[0] = 'X'
	if _, _, _, err := streamV2WebPPart(io.Discard, bytes.NewReader(invalid), int64(len(invalid))); !errors.Is(err, errInvalidWebP) {
		t.Fatalf("invalid container error = %v, want errInvalidWebP", err)
	}

	transportErr := errors.New("request body failed")
	body := io.MultiReader(bytes.NewReader(payload[:10]), iotest.ErrReader(transportErr))
	if _, _, _, err := streamV2WebPPart(io.Discard, body, int64(len(payload))); !errors.Is(err, errV2PartStream) {
		t.Fatalf("transport error = %v, want errV2PartStream", err)
	}
}

func TestPromoteImmutablePartMovesNewTarget(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "staging", "master.ready")
	target := filepath.Join(root, "v2", "master", "ab", "asset.webp")
	content := []byte("verified-webp-content")
	writeTestFile(t, source, content)

	if err := promoteImmutablePart(source, target, int64(len(content)), testSHA256(content)); err != nil {
		t.Fatalf("promoteImmutablePart() error = %v", err)
	}
	assertFileContent(t, target, content)
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source still exists after promotion: %v", err)
	}
}

func TestPromoteImmutablePartAcceptsVerifiedExistingTarget(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "staging", "master.ready")
	target := filepath.Join(root, "v2", "master", "ab", "asset.webp")
	content := []byte("verified-webp-content")
	writeTestFile(t, target, content)

	if err := promoteImmutablePart(source, target, int64(len(content)), testSHA256(content)); err != nil {
		t.Fatalf("idempotent promotion without source error = %v", err)
	}
	assertFileContent(t, target, content)

	writeTestFile(t, source, content)
	if err := promoteImmutablePart(source, target, int64(len(content)), testSHA256(content)); err != nil {
		t.Fatalf("idempotent promotion with duplicate source error = %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("duplicate source still exists after verified promotion: %v", err)
	}
}

func TestPromoteImmutablePartRejectsExistingTargetWithWrongHash(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "staging", "master.ready")
	target := filepath.Join(root, "v2", "master", "ab", "asset.webp")
	sourceContent := []byte("expected")
	targetContent := []byte("tampered")
	writeTestFile(t, source, sourceContent)
	writeTestFile(t, target, targetContent)

	err := promoteImmutablePart(source, target, int64(len(sourceContent)), testSHA256(sourceContent))
	if err == nil {
		t.Fatal("promotion accepted an existing target with the wrong hash")
	}
	assertFileContent(t, source, sourceContent)
	assertFileContent(t, target, targetContent)
}

func TestPromoteImmutablePartUsesMatchingPrevalidationWithoutRehash(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "staging", "master.ready")
	target := filepath.Join(root, "v2", "master", "ab", "asset.webp")
	content := []byte("verified-webp-content")
	writeTestFile(t, source, content)
	writeTestFile(t, target, content)

	prevalidated, err := prevalidateImmutableTarget(target, int64(len(content)), testSHA256(content))
	if err != nil {
		t.Fatalf("prevalidateImmutableTarget() error = %v", err)
	}
	t.Cleanup(func() {
		closeImmutableTargetPrevalidations(map[string]*immutableTargetPrevalidation{target: prevalidated})
	})
	verifyCalls := 0
	err = promoteImmutablePartWithVerifier(
		source, target, int64(len(content)), testSHA256(content), prevalidated,
		func(string, int64, string) error {
			verifyCalls++
			return errors.New("unexpected full verification")
		},
	)
	if err != nil {
		t.Fatalf("promoteImmutablePartWithVerifier() error = %v", err)
	}
	if verifyCalls != 0 {
		t.Fatalf("full verification calls = %d, want 0", verifyCalls)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("duplicate source still exists after fast-path promotion: %v", err)
	}
}

func TestPromoteImmutablePartFallsBackAfterTargetReplacement(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "staging", "master.ready")
	target := filepath.Join(root, "v2", "master", "ab", "asset.webp")
	expected := []byte("expected")
	writeTestFile(t, source, expected)
	writeTestFile(t, target, expected)

	prevalidated, err := prevalidateImmutableTarget(target, int64(len(expected)), testSHA256(expected))
	if err != nil {
		t.Fatalf("prevalidateImmutableTarget() error = %v", err)
	}
	t.Cleanup(func() {
		closeImmutableTargetPrevalidations(map[string]*immutableTargetPrevalidation{target: prevalidated})
	})
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove prevalidated target: %v", err)
	}
	writeTestFile(t, target, []byte("tampered"))

	verifyCalls := 0
	err = promoteImmutablePartWithVerifier(
		source, target, int64(len(expected)), testSHA256(expected), prevalidated,
		func(path string, size int64, hash string) error {
			verifyCalls++
			return verifyImmutableTarget(path, size, hash)
		},
	)
	if err == nil {
		t.Fatal("promotion accepted a replacement target with the wrong hash")
	}
	if verifyCalls != 1 {
		t.Fatalf("full verification calls = %d, want 1", verifyCalls)
	}
	assertFileContent(t, source, expected)
	assertFileContent(t, target, []byte("tampered"))
}

func TestPromoteImmutablePartFallsBackAfterTargetTampering(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "staging", "master.ready")
	target := filepath.Join(root, "v2", "master", "ab", "asset.webp")
	expected := []byte("expected")
	writeTestFile(t, source, expected)
	writeTestFile(t, target, expected)

	prevalidated, err := prevalidateImmutableTarget(target, int64(len(expected)), testSHA256(expected))
	if err != nil {
		t.Fatalf("prevalidateImmutableTarget() error = %v", err)
	}
	t.Cleanup(func() {
		closeImmutableTargetPrevalidations(map[string]*immutableTargetPrevalidation{target: prevalidated})
	})
	writeTestFile(t, target, []byte("tampered"))
	changedAt := prevalidated.info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(target, changedAt, changedAt); err != nil {
		t.Fatalf("change tampered target timestamps: %v", err)
	}

	verifyCalls := 0
	err = promoteImmutablePartWithVerifier(
		source, target, int64(len(expected)), testSHA256(expected), prevalidated,
		func(path string, size int64, hash string) error {
			verifyCalls++
			return verifyImmutableTarget(path, size, hash)
		},
	)
	if err == nil {
		t.Fatal("promotion accepted a tampered target")
	}
	if verifyCalls != 1 {
		t.Fatalf("full verification calls = %d, want 1", verifyCalls)
	}
	assertFileContent(t, source, expected)
	assertFileContent(t, target, []byte("tampered"))
}

func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		t.Fatalf("create test directory: %v", err)
	}
	if err := os.WriteFile(path, content, 0640); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}

func assertFileContent(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("content at %s = %q, want %q", path, got, want)
	}
}

func testSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
