// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestGenerateWatermarkSVG(t *testing.T) {
	svg := GenerateWatermarkSVG("summeRain", "66ccff", "64")
	if !strings.Contains(svg, "<svg") {
		t.Fatal("missing <svg tag")
	}
	if !strings.Contains(svg, `fill="#66ccff"`) {
		t.Fatal("missing fill color")
	}
	if !strings.Contains(svg, `font-size="64"`) {
		t.Fatal("missing font-size")
	}
	if !strings.Contains(svg, ">summeRain<") {
		t.Fatal("missing text content")
	}
}

func TestGenerateWatermarkSVGXMLEscape(t *testing.T) {
	svg := GenerateWatermarkSVG(`<script>alert(1)</script>`, "fff", "32")
	if strings.Contains(svg, "<script>") {
		t.Fatal("XML injection: raw <script> tag found in SVG")
	}
	if !strings.Contains(svg, "&lt;script&gt;") {
		t.Fatal("text not XML-escaped")
	}
}

func TestGenerateWatermarkSVGDefaults(t *testing.T) {
	svg := GenerateWatermarkSVG("test", "", "")
	if !strings.Contains(svg, `fill="#ffffff"`) {
		t.Fatal("default color should be ffffff")
	}
	if !strings.Contains(svg, `font-size="64"`) {
		t.Fatal("default size should be 64")
	}
}

func TestRegenerateWatermarkDisabled(t *testing.T) {
	tmp := t.TempDir()
	changed, err := RegenerateWatermark(map[string]string{"watermark_enabled": "false"}, tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("should not change when disabled")
	}
	if _, err := os.Stat(filepath.Join(tmp, "watermark.svg")); !os.IsNotExist(err) {
		t.Fatal("file should not exist when disabled")
	}
}

func TestRegenerateWatermarkCreatesFile(t *testing.T) {
	tmp := t.TempDir()
	cfg := map[string]string{
		"watermark_enabled": "true",
		"watermark_text":    "hello",
		"watermark_color":   "ff0000",
		"watermark_size":    "32",
	}
	changed, err := RegenerateWatermark(cfg, tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("should report changed on first generation")
	}
	data, err := os.ReadFile(filepath.Join(tmp, "watermark.svg"))
	if err != nil {
		t.Fatal("file should exist")
	}
	if !strings.Contains(string(data), "hello") {
		t.Fatal("SVG should contain watermark text")
	}
}

func TestRegenerateWatermarkNoChange(t *testing.T) {
	tmp := t.TempDir()
	cfg := map[string]string{
		"watermark_enabled": "true",
		"watermark_text":    "hello",
		"watermark_size":    "32",
	}
	if _, err := RegenerateWatermark(cfg, tmp); err != nil {
		t.Fatal(err)
	}
	changed, err := RegenerateWatermark(cfg, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("should not report changed when content is identical")
	}
}

func TestWithWatermarkSnapshotRestoresExistingFileOnProcessError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "watermark.svg")
	current := []byte("<svg>current watermark</svg>")
	if err := os.WriteFile(path, current, 0600); err != nil {
		t.Fatal(err)
	}
	processErr := errors.New("processing failed")
	err := withWatermarkSnapshot(context.Background(), map[string]string{
		"watermark_enabled": "true",
		"watermark_text":    "upload snapshot",
		"watermark_color":   "00ff00",
		"watermark_size":    "24",
	}, tmp, func() error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(data), "upload snapshot") {
			return errors.New("snapshot watermark was not installed")
		}
		return processErr
	})
	if !errors.Is(err, processErr) {
		t.Fatalf("expected process error, got %v", err)
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(current) {
		t.Fatalf("current watermark was not restored: %q", restored)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("restored mode = %o, want 600", got)
	}
	if leftovers, err := filepath.Glob(filepath.Join(tmp, ".watermark.svg.tmp-*")); err != nil || len(leftovers) != 0 {
		t.Fatalf("atomic write left temporary files: %v, err=%v", leftovers, err)
	}
}

func TestWithWatermarkSnapshotRemovesFileWhenOriginallyMissing(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "watermark.svg")
	err := withWatermarkSnapshot(context.Background(), map[string]string{
		"watermark_enabled": "true",
		"watermark_text":    "temporary snapshot",
	}, tmp, func() error {
		if _, err := os.Stat(path); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temporary watermark should be removed, stat error = %v", err)
	}
}

func TestMatchingWatermarkSnapshotsProcessConcurrently(t *testing.T) {
	tmp := t.TempDir()
	cfg := map[string]string{
		"watermark_enabled": "true",
		"watermark_text":    "shared snapshot",
		"watermark_color":   "ffffff",
		"watermark_size":    "32",
	}
	if _, err := RegenerateWatermark(cfg, tmp); err != nil {
		t.Fatal(err)
	}

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 2)
	go func() {
		done <- withWatermarkSnapshot(context.Background(), cfg, tmp, func() error {
			close(firstStarted)
			<-release
			return nil
		})
	}()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first matching snapshot did not start")
	}
	go func() {
		done <- withWatermarkSnapshot(context.Background(), cfg, tmp, func() error {
			close(secondStarted)
			<-release
			return nil
		})
	}()
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("matching snapshot was serialized")
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func TestWithWatermarkSnapshotRestoresExistingFileAfterPanic(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "watermark.svg")
	current := []byte("<svg>current watermark</svg>")
	if err := os.WriteFile(path, current, 0644); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "processing panic" {
				t.Fatalf("recovered %v, want processing panic", recovered)
			}
		}()
		_ = withWatermarkSnapshot(context.Background(), map[string]string{
			"watermark_enabled": "true",
			"watermark_text":    "temporary snapshot",
		}, tmp, func() error {
			panic("processing panic")
		})
	}()
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(current) {
		t.Fatalf("current watermark was not restored after panic: %q", restored)
	}
}

func TestWatermarkSnapshotExcludesCurrentWatermarkRequest(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "watermark.svg")
	current := []byte("<svg>current watermark</svg>")
	if err := os.WriteFile(path, current, 0644); err != nil {
		t.Fatal(err)
	}

	snapshotStarted := make(chan struct{})
	releaseSnapshot := make(chan struct{})
	defer func() {
		select {
		case <-releaseSnapshot:
		default:
			close(releaseSnapshot)
		}
	}()
	snapshotDone := make(chan error, 1)
	go func() {
		snapshotDone <- withWatermarkSnapshot(context.Background(), map[string]string{
			"watermark_enabled": "true",
			"watermark_text":    "upload snapshot",
		}, tmp, func() error {
			close(snapshotStarted)
			<-releaseSnapshot
			return nil
		})
	}()

	select {
	case <-snapshotStarted:
	case <-time.After(time.Second):
		t.Fatal("snapshot did not start")
	}

	type watermarkResult struct {
		data []byte
		err  error
	}
	currentStarted := make(chan struct{})
	currentDone := make(chan watermarkResult, 1)
	go func() {
		close(currentStarted)
		var result watermarkResult
		result.err = WithCurrentWatermark(context.Background(), tmp, func() error {
			result.data, result.err = os.ReadFile(path)
			return result.err
		})
		currentDone <- result
	}()
	<-currentStarted

	select {
	case result := <-currentDone:
		close(releaseSnapshot)
		<-snapshotDone
		t.Fatalf("current request entered snapshot critical section: data=%q err=%v", result.data, result.err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseSnapshot)
	if err := <-snapshotDone; err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-currentDone:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if string(result.data) != string(current) {
			t.Fatalf("current request observed %q, want restored watermark", result.data)
		}
	case <-time.After(time.Second):
		t.Fatal("current watermark request remained blocked")
	}
}

func TestCurrentWatermarkHonorsCrossProcessLockCancellation(t *testing.T) {
	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, watermarkLockFilename)
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lockFile.Close()
	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	defer unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	err = WithCurrentWatermark(ctx, tmp, func() error {
		t.Fatal("watermark callback entered while filesystem lock was held")
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WithCurrentWatermark() error = %v, want deadline exceeded", err)
	}
}

func TestCurrentWatermarkAllowsConcurrentReaders(t *testing.T) {
	tmp := t.TempDir()
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- WithCurrentWatermark(context.Background(), tmp, func() error {
			close(firstEntered)
			<-releaseFirst
			return nil
		})
	}()

	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first watermark reader did not enter")
	}
	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- WithCurrentWatermark(context.Background(), tmp, func() error {
			close(secondEntered)
			return nil
		})
	}()
	select {
	case <-secondEntered:
	case <-time.After(time.Second):
		close(releaseFirst)
		<-firstDone
		t.Fatal("second watermark reader was serialized behind the first")
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestCurrentWatermarkRecoversInterruptedSnapshot(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "watermark.svg")
	original := []byte("<svg>canonical watermark</svg>")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	previous, err := readWatermarkFileState(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeWatermarkRecovery(tmp, previous); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("<svg>interrupted snapshot</svg>"), 0644); err != nil {
		t.Fatal(err)
	}

	err = WithCurrentWatermark(context.Background(), tmp, func() error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(data) != string(original) {
			return errors.New("interrupted snapshot was not recovered")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tmp, watermarkRecoveryFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("watermark recovery journal remains: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("recovered mode = %o, want 600", info.Mode().Perm())
	}
}
