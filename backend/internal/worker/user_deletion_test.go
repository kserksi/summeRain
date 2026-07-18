// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kserksi/summerain/internal/model"
)

func TestUploadSessionBlocksUserDeletion(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		stagingPath string
		want        bool
	}{
		{name: "initiated", status: "initiated", want: true},
		{name: "uploading", status: "uploading", want: true},
		{name: "processing", status: "processing", want: true},
		{name: "cleanup pending", status: "cleanup_pending", want: true},
		{name: "failed awaiting cleanup", status: "failed", stagingPath: "/staging/failed", want: true},
		{name: "cancelled awaiting cleanup", status: "cancelled", stagingPath: "/staging/cancelled", want: true},
		{name: "failed and cleaned", status: "failed", want: false},
		{name: "cancelled and cleaned", status: "cancelled", want: false},
		{name: "completed awaiting cleanup", status: "completed", stagingPath: "/legacy/stale-value", want: true},
		{name: "completed and cleaned", status: "completed", want: false},
		{name: "unknown fails closed", status: "future_state", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := uploadSessionBlocksUserDeletion(model.UploadSession{
				Status: test.status, StagingPath: test.stagingPath,
			})
			if got != test.want {
				t.Fatalf("uploadSessionBlocksUserDeletion() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDeletionPathInsideRejectsStorageRootAndEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := deletionPathInside(root, "."); err == nil {
		t.Fatal("storage root accepted as a deletion candidate")
	}
	if _, err := deletionPathInside(root, filepath.Join("..", "outside.webp")); err == nil {
		t.Fatal("escaping path accepted as a deletion candidate")
	}
	inside, err := deletionPathInside(root, filepath.Join("original", "image.webp"))
	if err != nil {
		t.Fatal(err)
	}
	if inside != filepath.Join(root, "original", "image.webp") {
		t.Fatalf("resolved path = %q", inside)
	}
}

func TestDeletionPathInsideRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "image.webp"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := deletionPathInside(root, filepath.Join("linked", "image.webp")); err == nil {
		t.Fatal("symlink escape accepted as a deletion candidate")
	}
}
