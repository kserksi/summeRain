// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

var watermarkFileMu sync.RWMutex

const (
	watermarkLockFilename     = ".watermark.lock"
	watermarkRecoveryFilename = ".watermark.snapshot-recovery.json"
	watermarkLockPollInterval = 25 * time.Millisecond
)

func GenerateWatermarkSVG(text, color, size string) string {
	if size == "" {
		size = "64"
	}
	if color == "" {
		color = "ffffff"
	}
	width := int(float64(len(text)) * parseFloatDefault(size, 64) * 0.6)
	if width < 10 {
		width = 10
	}
	height := int(parseFloatDefault(size, 64) * 1.25)
	if height < 10 {
		height = 10
	}
	return fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d">`+
			`<text x="50%%" y="50%%" text-anchor="middle" dominant-baseline="central" `+
			`font-family="sans-serif" font-size="%s" fill="#%s">%s</text></svg>`,
		width, height, size, color, escapeXML(text),
	)
}

func SaveWatermarkFile(svg, storageBasePath string) error {
	return writeWatermarkFileAtomic(filepath.Join(storageBasePath, "watermark.svg"), []byte(svg), 0644)
}

func RegenerateWatermark(cfgMap map[string]string, storageBasePath string) (changed bool, err error) {
	watermarkFileMu.Lock()
	defer watermarkFileMu.Unlock()
	err = withWatermarkFileLock(context.Background(), storageBasePath, func() error {
		if err := recoverWatermarkSnapshot(storageBasePath); err != nil {
			return err
		}
		changed, err = regenerateWatermark(cfgMap, storageBasePath)
		return err
	})
	return changed, err
}

func regenerateWatermark(cfgMap map[string]string, storageBasePath string) (changed bool, err error) {
	if cfgMap["watermark_enabled"] != "true" {
		return false, nil
	}
	text := strings.TrimSpace(cfgMap["watermark_text"])
	if text == "" {
		return false, nil
	}
	svg := GenerateWatermarkSVG(text, cfgMap["watermark_color"], cfgMap["watermark_size"])

	path := filepath.Join(storageBasePath, "watermark.svg")
	if existing, err := os.ReadFile(path); err == nil && string(existing) == svg {
		return false, nil
	}
	return true, SaveWatermarkFile(svg, storageBasePath)
}

// WithCurrentWatermark serializes an imgproxy request that reads the shared
// watermark file with temporary V2 watermark snapshots. The filesystem lock
// extends this guarantee across overlapping backend processes.
func WithCurrentWatermark(ctx context.Context, storageBasePath string, process func() error) error {
	watermarkFileMu.RLock()
	defer watermarkFileMu.RUnlock()
	for {
		recoveryPending := false
		err := withWatermarkFileLockMode(ctx, storageBasePath, unix.LOCK_SH, func() error {
			_, err := os.Stat(filepath.Join(storageBasePath, watermarkRecoveryFilename))
			switch {
			case err == nil:
				recoveryPending = true
				return nil
			case errors.Is(err, os.ErrNotExist):
				return process()
			default:
				return fmt.Errorf("inspect watermark recovery state: %w", err)
			}
		})
		if err != nil || !recoveryPending {
			return err
		}
		// A recovery journal visible under a shared lock is stale: a live
		// snapshot holds the exclusive lock for the journal's full lifetime.
		if err := withWatermarkFileLock(ctx, storageBasePath, func() error {
			return recoverWatermarkSnapshot(storageBasePath)
		}); err != nil {
			return err
		}
	}
}

func withWatermarkSnapshot(ctx context.Context, cfgMap map[string]string, storageBasePath string, process func() error) (err error) {
	if expected, ok := expectedWatermarkSVG(cfgMap); ok {
		matched, fastPathErr := withMatchingWatermarkSnapshot(ctx, expected, storageBasePath, process)
		if matched || fastPathErr != nil {
			return fastPathErr
		}
	}

	watermarkFileMu.Lock()
	defer watermarkFileMu.Unlock()
	return withWatermarkFileLock(ctx, storageBasePath, func() (err error) {
		if err := recoverWatermarkSnapshot(storageBasePath); err != nil {
			return err
		}

		path := filepath.Join(storageBasePath, "watermark.svg")
		previous, err := readWatermarkFileState(path)
		if err != nil {
			return fmt.Errorf("snapshot current watermark: %w", err)
		}
		if err := writeWatermarkRecovery(storageBasePath, previous); err != nil {
			return fmt.Errorf("persist watermark recovery state: %w", err)
		}
		defer func() {
			if restoreErr := restoreAndClearWatermarkRecovery(storageBasePath, previous); restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("restore current watermark: %w", restoreErr))
			}
		}()
		if _, err := regenerateWatermark(cfgMap, storageBasePath); err != nil {
			return err
		}
		return process()
	})
}

func expectedWatermarkSVG(cfgMap map[string]string) ([]byte, bool) {
	if cfgMap["watermark_enabled"] != "true" {
		return nil, false
	}
	text := strings.TrimSpace(cfgMap["watermark_text"])
	if text == "" {
		return nil, false
	}
	return []byte(GenerateWatermarkSVG(text, cfgMap["watermark_color"], cfgMap["watermark_size"])), true
}

// withMatchingWatermarkSnapshot lets jobs that use the already-installed SVG
// share both the process lock and the cross-process file lock. A different or
// recovering snapshot falls back to the exclusive journaled swap below.
func withMatchingWatermarkSnapshot(ctx context.Context, expected []byte, storageBasePath string, process func() error) (matched bool, err error) {
	watermarkFileMu.RLock()
	defer watermarkFileMu.RUnlock()
	err = withWatermarkFileLockMode(ctx, storageBasePath, unix.LOCK_SH, func() error {
		if _, statErr := os.Stat(filepath.Join(storageBasePath, watermarkRecoveryFilename)); statErr == nil {
			return nil
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("inspect watermark recovery state: %w", statErr)
		}
		current, readErr := os.ReadFile(filepath.Join(storageBasePath, "watermark.svg"))
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("read current watermark: %w", readErr)
		}
		if !bytes.Equal(current, expected) {
			return nil
		}
		matched = true
		return process()
	})
	return matched, err
}

type watermarkFileState struct {
	data   []byte
	mode   os.FileMode
	exists bool
}

type watermarkRecoveryState struct {
	Data   []byte `json:"data,omitempty"`
	Mode   uint32 `json:"mode,omitempty"`
	Exists bool   `json:"exists"`
}

func readWatermarkFileState(path string) (watermarkFileState, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return watermarkFileState{}, nil
	}
	if err != nil {
		return watermarkFileState{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return watermarkFileState{}, err
	}
	return watermarkFileState{data: data, mode: info.Mode().Perm(), exists: true}, nil
}

func restoreWatermarkFile(path string, previous watermarkFileState) error {
	if !previous.exists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return writeWatermarkFileAtomic(path, previous.data, previous.mode)
}

func withWatermarkFileLock(ctx context.Context, storageBasePath string, process func() error) error {
	return withWatermarkFileLockMode(ctx, storageBasePath, unix.LOCK_EX, process)
}

func withWatermarkFileLockMode(ctx context.Context, storageBasePath string, mode int, process func() error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := os.MkdirAll(storageBasePath, 0750); err != nil {
		return fmt.Errorf("create watermark storage directory: %w", err)
	}
	lockPath := filepath.Join(storageBasePath, watermarkLockFilename)
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0640)
	if err != nil {
		return fmt.Errorf("open watermark lock: %w", err)
	}
	defer lockFile.Close()

	for {
		err = unix.Flock(int(lockFile.Fd()), mode|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return fmt.Errorf("lock watermark file: %w", err)
		}
		timer := time.NewTimer(watermarkLockPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	defer func() { _ = unix.Flock(int(lockFile.Fd()), unix.LOCK_UN) }()
	return process()
}

func writeWatermarkRecovery(storageBasePath string, previous watermarkFileState) error {
	recovery := watermarkRecoveryState{
		Data: previous.data, Mode: uint32(previous.mode.Perm()), Exists: previous.exists,
	}
	data, err := json.Marshal(recovery)
	if err != nil {
		return err
	}
	return writeAtomicWatermarkFile(
		filepath.Join(storageBasePath, watermarkRecoveryFilename), data, 0600, ".watermark-recovery.tmp-*",
	)
}

func recoverWatermarkSnapshot(storageBasePath string) error {
	recoveryPath := filepath.Join(storageBasePath, watermarkRecoveryFilename)
	data, err := os.ReadFile(recoveryPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read watermark recovery state: %w", err)
	}
	var recovery watermarkRecoveryState
	if err := json.Unmarshal(data, &recovery); err != nil {
		return fmt.Errorf("decode watermark recovery state: %w", err)
	}
	previous := watermarkFileState{
		data: recovery.Data, mode: os.FileMode(recovery.Mode), exists: recovery.Exists,
	}
	return restoreAndClearWatermarkRecovery(storageBasePath, previous)
}

func restoreAndClearWatermarkRecovery(storageBasePath string, previous watermarkFileState) error {
	if err := restoreWatermarkFile(filepath.Join(storageBasePath, "watermark.svg"), previous); err != nil {
		return err
	}
	if err := syncV2Directory(storageBasePath); err != nil {
		return err
	}
	recoveryPath := filepath.Join(storageBasePath, watermarkRecoveryFilename)
	if err := os.Remove(recoveryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncV2Directory(storageBasePath)
}

func writeWatermarkFileAtomic(path string, data []byte, mode os.FileMode) error {
	return writeAtomicWatermarkFile(path, data, mode, ".watermark.svg.tmp-*")
}

func writeAtomicWatermarkFile(path string, data []byte, mode os.FileMode, pattern string) error {
	if mode.Perm() == 0 {
		mode = 0644
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(mode.Perm()); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	closed = true
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return syncV2Directory(filepath.Dir(path))
}

func escapeXML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(s)
}

func parseFloatDefault(s string, def float64) float64 {
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil || f <= 0 {
		return def
	}
	return f
}
