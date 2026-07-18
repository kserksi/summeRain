// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSyncStorageDeletionDirectoriesDeduplicatesAndOrders(t *testing.T) {
	root := t.TempDir()
	directoryA := filepath.Join(root, "a")
	directoryB := filepath.Join(root, "b")
	var synced []string

	err := syncStorageDeletionDirectoriesWith([]string{
		directoryB,
		filepath.Join(directoryA, "."),
		"",
		directoryB,
		directoryA,
	}, func(directory string) error {
		synced = append(synced, directory)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{directoryA, directoryB}; !reflect.DeepEqual(synced, want) {
		t.Fatalf("synced directories = %v, want %v", synced, want)
	}
}

func TestSyncStorageDeletionDirectoriesIgnoresUnsupportedFilesystems(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "missing directory", err: os.ErrNotExist},
		{name: "invalid operation", err: unix.EINVAL},
		{name: "not supported", err: unix.ENOTSUP},
		{name: "read only filesystem", err: unix.EROFS},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			err := syncStorageDeletionDirectoriesWith([]string{"/storage/b", "/storage/a"}, func(string) error {
				calls++
				return fmt.Errorf("wrapped directory sync: %w", test.err)
			})
			if err != nil {
				t.Fatalf("unsupported sync error was returned: %v", err)
			}
			if calls != 2 {
				t.Fatalf("sync calls = %d, want 2", calls)
			}
		})
	}
}

func TestSyncStorageDeletionDirectoriesReturnsIOError(t *testing.T) {
	wantErr := errors.New("directory I/O failure")
	var synced []string
	err := syncStorageDeletionDirectoriesWith([]string{"/storage/b", "/storage/a"}, func(directory string) error {
		synced = append(synced, directory)
		if directory == "/storage/a" {
			return wantErr
		}
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped I/O error", err)
	}
	if !strings.Contains(err.Error(), `/storage/a`) {
		t.Fatalf("error does not identify directory: %v", err)
	}
	if want := []string{"/storage/a"}; !reflect.DeepEqual(synced, want) {
		t.Fatalf("synced directories = %v, want stop on first failure %v", synced, want)
	}
}

func TestSyncStorageDeletionDirectoryAfterUnlink(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "orphan.webp")
	if err := os.WriteFile(path, []byte("image"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syncStorageDeletionDirectories([]string{directory}); err != nil {
		t.Fatalf("sync parent after unlink: %v", err)
	}
}
