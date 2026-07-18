// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kserksi/summerain/internal/model"
)

const (
	V1StorageBackendLocal = "local"
	V1StorageBackendR2    = "r2"
)

type V1StorageTarget struct {
	Backend  string
	Endpoint string
	Bucket   string
}

// ResolveV1StorageTarget provides read/delete compatibility for V1 rows that
// predate explicit storage lineage. It never persists the inferred target.
func ResolveV1StorageTarget(storageRoot string, pipelineVersion uint16, imageFile *model.ImageFile, currentEndpoint, currentBucket string, currentR2Configured bool) (V1StorageTarget, error) {
	if imageFile == nil {
		return V1StorageTarget{}, errors.New("image file is missing")
	}
	backend := strings.TrimSpace(imageFile.RemoteBackend)
	endpoint := strings.TrimRight(strings.TrimSpace(imageFile.RemoteEndpoint), "/")
	bucket := strings.TrimSpace(imageFile.RemoteBucket)
	switch backend {
	case V1StorageBackendR2:
		if endpoint == "" || bucket == "" {
			return V1StorageTarget{}, errors.New("R2 image file has incomplete storage lineage")
		}
		return V1StorageTarget{Backend: V1StorageBackendR2, Endpoint: endpoint, Bucket: bucket}, nil
	case V1StorageBackendLocal:
		if endpoint != "" || bucket != "" {
			return V1StorageTarget{}, errors.New("local image file has unexpected remote lineage")
		}
		return V1StorageTarget{Backend: V1StorageBackendLocal}, nil
	case "":
		if endpoint != "" || bucket != "" {
			return V1StorageTarget{}, errors.New("local image file has incomplete remote lineage")
		}
	default:
		return V1StorageTarget{}, fmt.Errorf("unsupported remote storage backend %q", backend)
	}

	if pipelineVersion >= model.ImagePipelineVersionV2 {
		return V1StorageTarget{Backend: V1StorageBackendLocal}, nil
	}
	localOriginalExists, err := safeV1LocalOriginalExists(storageRoot, imageFile.OriginalPath)
	if err != nil {
		return V1StorageTarget{}, err
	}
	if localOriginalExists {
		return V1StorageTarget{Backend: V1StorageBackendLocal}, nil
	}
	currentEndpoint = strings.TrimRight(strings.TrimSpace(currentEndpoint), "/")
	currentBucket = strings.TrimSpace(currentBucket)
	if !currentR2Configured || currentEndpoint == "" || currentBucket == "" {
		return V1StorageTarget{}, errors.New("legacy image storage is unclassified and no exact R2 target is configured")
	}
	return V1StorageTarget{Backend: V1StorageBackendR2, Endpoint: currentEndpoint, Bucket: currentBucket}, nil
}

func safeV1LocalOriginalExists(storageRoot, storedPath string) (bool, error) {
	storageRoot = strings.TrimSpace(storageRoot)
	storedPath = strings.TrimSpace(storedPath)
	if storageRoot == "" || storedPath == "" || filepath.IsAbs(storedPath) || strings.Contains(storedPath, `\`) {
		return false, errors.New("legacy image has an invalid local storage path")
	}
	cleanPath := filepath.Clean(filepath.FromSlash(storedPath))
	if cleanPath == "." || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return false, errors.New("legacy image path escapes storage root")
	}
	rootAbs, err := filepath.Abs(storageRoot)
	if err != nil {
		return false, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return false, err
	}
	candidate, err := pathInside(rootAbs, filepath.Join(rootAbs, cleanPath))
	if err != nil {
		return false, err
	}
	relative, err := filepath.Rel(rootAbs, candidate)
	if err != nil {
		return false, err
	}
	current := rootAbs
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			return false, nil
		}
		if statErr != nil {
			return false, statErr
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr != nil {
			return false, resolveErr
		}
		if err := requirePathWithin(resolvedRoot, resolved); err != nil {
			return false, err
		}
	}
	resolvedCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return false, err
	}
	if err := requirePathWithin(resolvedRoot, resolvedCandidate); err != nil {
		return false, err
	}
	info, err := os.Stat(resolvedCandidate)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("legacy image local source is not a regular file")
	}
	return true, nil
}

func requirePathWithin(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("legacy image path resolves outside storage root")
	}
	return nil
}

// V1PersistentVariantPath resolves the two persistent V1 processed variants.
// Both the original hash-only layout and the newer lineage-safe object names
// are represented by ImageFile.ProcessedPath, so callers must not reconstruct
// either path from FileHash.
func V1PersistentVariantPath(processedPath, format string) (string, bool) {
	processedPath = strings.TrimSpace(processedPath)
	if processedPath == "" || strings.Contains(processedPath, `\`) || filepath.IsAbs(processedPath) {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(processedPath))
	if filepath.Dir(clean) != "processed" || filepath.Ext(clean) != ".webp" {
		return "", false
	}
	name := strings.TrimSuffix(filepath.Base(clean), ".webp")
	if name == "" || name == "." || name == ".." {
		return "", false
	}
	switch format {
	case "webp":
		return clean, true
	case "avif":
		return filepath.Join("processed", name+".avif"), true
	default:
		return "", false
	}
}
