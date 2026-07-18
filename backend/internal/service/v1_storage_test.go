// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kserksi/summerain/internal/model"
)

func TestV1PersistentVariantPathUsesStoredObjectName(t *testing.T) {
	tests := []struct {
		name, stored, format, want string
		ok                         bool
	}{
		{name: "legacy webp", stored: "processed/0123456789abcdef.webp", format: "webp", want: "processed/0123456789abcdef.webp", ok: true},
		{name: "lineage safe avif", stored: "processed/0123456789abcdef-a1b2c3d4.webp", format: "avif", want: "processed/0123456789abcdef-a1b2c3d4.avif", ok: true},
		{name: "absolute", stored: "/processed/image.webp", format: "avif"},
		{name: "traversal", stored: "processed/../image.webp", format: "avif"},
		{name: "nested", stored: "processed/nested/image.webp", format: "avif"},
		{name: "wrong directory", stored: "thumbnail/image.webp", format: "avif"},
		{name: "wrong source extension", stored: "processed/image.png", format: "avif"},
		{name: "unsupported target", stored: "processed/image.webp", format: "png"},
		{name: "backslash", stored: `processed\image.webp`, format: "webp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := V1PersistentVariantPath(test.stored, test.format)
			if ok != test.ok || got != test.want {
				t.Fatalf("V1PersistentVariantPath(%q, %q) = %q, %v; want %q, %v", test.stored, test.format, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestResolveV1StorageTargetPrefersSafeLocalOriginal(t *testing.T) {
	root := t.TempDir()
	originalPath := filepath.Join("original", "legacy.webp")
	if err := os.MkdirAll(filepath.Join(root, "original"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, originalPath), []byte("local"), 0640); err != nil {
		t.Fatal(err)
	}
	imageFile := &model.ImageFile{OriginalPath: originalPath}

	target, err := ResolveV1StorageTarget(root, 1, imageFile, "https://r2.example", "images", true)
	if err != nil {
		t.Fatal(err)
	}
	if target.Backend != V1StorageBackendLocal || target.Endpoint != "" || target.Bucket != "" {
		t.Fatalf("target = %#v, want safe local target", target)
	}
	if imageFile.RemoteBackend != "" || imageFile.RemoteEndpoint != "" || imageFile.RemoteBucket != "" {
		t.Fatalf("compatibility resolution wrote lineage back to the model: %#v", imageFile)
	}
}

func TestResolveV1StorageTargetFallsBackToCurrentExactR2Target(t *testing.T) {
	imageFile := &model.ImageFile{OriginalPath: filepath.Join("original", "missing.webp")}
	target, err := ResolveV1StorageTarget(t.TempDir(), 1, imageFile, " https://r2.example/ ", " images ", true)
	if err != nil {
		t.Fatal(err)
	}
	if target != (V1StorageTarget{Backend: V1StorageBackendR2, Endpoint: "https://r2.example", Bucket: "images"}) {
		t.Fatalf("target = %#v", target)
	}
	if imageFile.RemoteBackend != "" || imageFile.RemoteEndpoint != "" || imageFile.RemoteBucket != "" {
		t.Fatalf("compatibility resolution wrote lineage back to the model: %#v", imageFile)
	}
}

func TestResolveV1StorageTargetFailsClosedWhenLegacyTargetIsUnknown(t *testing.T) {
	tests := []struct {
		name string
		file *model.ImageFile
	}{
		{name: "missing local without R2", file: &model.ImageFile{OriginalPath: "original/missing.webp"}},
		{name: "partial lineage", file: &model.ImageFile{OriginalPath: "original/missing.webp", RemoteEndpoint: "https://r2.example"}},
		{name: "incomplete R2", file: &model.ImageFile{OriginalPath: "original/missing.webp", RemoteBackend: "r2", RemoteBucket: "images"}},
		{name: "unknown backend", file: &model.ImageFile{OriginalPath: "original/missing.webp", RemoteBackend: "s3"}},
		{name: "traversal", file: &model.ImageFile{OriginalPath: "../missing.webp"}},
		{name: "nil"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if target, err := ResolveV1StorageTarget(t.TempDir(), 1, test.file, "", "", false); err == nil {
				t.Fatalf("ResolveV1StorageTarget() = %#v, nil", target)
			}
		})
	}
}

func TestResolveV1StorageTargetRejectsEscapingLocalSymlinkInsteadOfFallingBack(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "legacy.webp"), []byte("outside"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "original")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	imageFile := &model.ImageFile{OriginalPath: filepath.Join("original", "legacy.webp")}
	if target, err := ResolveV1StorageTarget(root, 1, imageFile, "https://r2.example", "images", true); err == nil {
		t.Fatalf("ResolveV1StorageTarget() = %#v, nil", target)
	}
}

func TestResolveV1StorageTargetHonorsExplicitLineage(t *testing.T) {
	target, err := ResolveV1StorageTarget("", 1, &model.ImageFile{
		RemoteBackend: "r2", RemoteEndpoint: "https://old-r2.example/", RemoteBucket: "archive",
	}, "https://current-r2.example", "images", true)
	if err != nil {
		t.Fatal(err)
	}
	if target != (V1StorageTarget{Backend: V1StorageBackendR2, Endpoint: "https://old-r2.example", Bucket: "archive"}) {
		t.Fatalf("target = %#v, want explicit lineage", target)
	}
	local, err := ResolveV1StorageTarget("", 1, &model.ImageFile{RemoteBackend: V1StorageBackendLocal}, "https://current-r2.example", "images", true)
	if err != nil || local != (V1StorageTarget{Backend: V1StorageBackendLocal}) {
		t.Fatalf("explicit local target = %#v, %v", local, err)
	}
}
