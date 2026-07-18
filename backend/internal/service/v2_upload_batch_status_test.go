// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/model"
)

func TestBuildV2BatchStatusResponseKeepsRequestedOrder(t *testing.T) {
	firstID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	secondID := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	alias := "privateS"
	sessions := []model.UploadSession{
		{
			UploadKey: secondID,
			Status:    model.UploadSessionStatusCompleted,
			ExpiresAt: time.Unix(2, 0),
			Image:     &model.Image{UniqueLink: "private", OriginAlias: &alias},
			Parts: []model.UploadPart{
				{Kind: model.ImageVariantKindGallery, Status: model.UploadPartStatusFinalized},
			},
		},
		{
			UploadKey: firstID,
			Status:    model.UploadSessionStatusUploading,
			ExpiresAt: time.Unix(1, 0),
			Parts: []model.UploadPart{
				{Kind: model.ImageVariantKindMaster, Status: model.UploadPartStatusReceived},
			},
		},
	}

	result, complete := buildV2BatchStatusResponse([]string{firstID, secondID}, sessions)
	if !complete {
		t.Fatal("complete result reported as missing")
	}
	if len(result.Uploads) != 2 || result.Uploads[0].UploadID != firstID || result.Uploads[1].UploadID != secondID {
		t.Fatalf("uploads = %#v, want request order", result.Uploads)
	}
	if len(result.Uploads[0].Parts) != 1 || result.Uploads[0].Parts[0].Kind != model.ImageVariantKindMaster {
		t.Fatalf("parts were not preserved: %#v", result.Uploads[0].Parts)
	}
	if result.Uploads[1].UniqueLink != "private" || result.Uploads[1].AssetLink != alias {
		t.Fatalf("preloaded image was not preserved: %#v", result.Uploads[1])
	}
}

func TestBuildV2BatchStatusResponseIsAllOrNothing(t *testing.T) {
	requested := []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	sessions := []model.UploadSession{{UploadKey: requested[0]}}

	result, complete := buildV2BatchStatusResponse(requested, sessions)
	if complete || result != nil {
		t.Fatalf("partial result = %#v, complete = %v; want no disclosure", result, complete)
	}
}
