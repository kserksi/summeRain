// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"strings"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
)

var testV2RequiredParts = []string{
	model.ImageVariantKindMaster,
	model.ImageVariantKindGallery,
	model.ImageVariantKindAdmin,
	model.ImageVariantKindPublishSource,
}

// testV2Recipe mirrors the embedded default recipe with a smaller part limit so
// validation tests stay cheap.
func testV2Recipe() *config.ImageRecipe {
	return &config.ImageRecipe{
		PipelineVersion: config.ImageRecipePipelineVersion,
		RecipeVersion:   "2.0.0",
		MaxPixels:       50_000_000,
		MaxPartBytes:    20 << 20,
		SourceMIMETypes: []string{"image/jpeg", "image/png", "image/bmp", "image/webp", "image/avif"},
		Variants: []config.ImageRecipeVariant{
			{Kind: model.ImageVariantKindMaster, Quality: 80, Fit: "original"},
			{Kind: model.ImageVariantKindGallery, Width: 400, Height: 400, Quality: 60, Fit: "cover"},
			{Kind: model.ImageVariantKindAdmin, Width: 120, Height: 160, Quality: 60, Fit: "cover"},
			{Kind: model.ImageVariantKindPublishSource, LongEdge: 2048, Quality: 80, Fit: "contain"},
		},
	}
}

func validV2Manifest() V2InitUploadRequest {
	hash := strings.Repeat("a", 64)
	return V2InitUploadRequest{
		Filename:         "photo.jpg",
		Visibility:       "public",
		ProcessorVersion: "web-2.0.0",
		RecipeVersion:    "2.0.0",
		Source:           V2SourceManifest{MimeType: "image/jpeg", Width: 8000, Height: 6000},
		Parts: []V2PartManifest{
			{Kind: model.ImageVariantKindMaster, Size: 128, SHA256: hash, MimeType: "image/webp", Width: 8000, Height: 6000, Quality: 80},
			{Kind: model.ImageVariantKindGallery, Size: 128, SHA256: hash, MimeType: "image/webp", Width: 400, Height: 400, Quality: 60},
			{Kind: model.ImageVariantKindAdmin, Size: 128, SHA256: hash, MimeType: "image/webp", Width: 120, Height: 160, Quality: 60},
			{Kind: model.ImageVariantKindPublishSource, Size: 128, SHA256: hash, MimeType: "image/webp", Width: 2048, Height: 1536, Quality: 80},
		},
	}
}

func TestValidateV2ManifestRejectsImplausiblySmallPart(t *testing.T) {
	recipe := testV2Recipe()
	req := validV2Manifest()
	req.Parts[0].Size = v2MinimumPartBytes - 1
	if appErr := validateV2Manifest(&req, recipe); appErr == nil || appErr.HTTP != 400 {
		t.Fatalf("undersized part error = %#v, want HTTP 400", appErr)
	}
}

func TestValidateV2Manifest(t *testing.T) {
	recipe := testV2Recipe()
	req := validV2Manifest()
	if appErr := validateV2Manifest(&req, recipe); appErr != nil {
		t.Fatalf("valid manifest rejected: %v", appErr)
	}
}

func TestV2RecipeReportsWhetherNewSessionsAreEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		svc := &V2UploadService{
			cfg:    &config.Config{ImageV2: config.ImageV2Config{Enabled: enabled}},
			recipe: testV2Recipe(),
		}
		if got := svc.Recipe().V2Enabled; got != enabled {
			t.Fatalf("recipe V2Enabled = %v, want %v", got, enabled)
		}
	}
}

func TestV2RecipeResponseFollowsRecipeValues(t *testing.T) {
	recipe := testV2Recipe()
	svc := &V2UploadService{
		cfg:    &config.Config{ImageV2: config.ImageV2Config{Enabled: true, SessionTTL: 30 * time.Minute}},
		recipe: recipe,
	}
	response := svc.Recipe()
	if response.PipelineVersion != config.ImageRecipePipelineVersion {
		t.Fatalf("pipeline version = %d, want %d", response.PipelineVersion, config.ImageRecipePipelineVersion)
	}
	if response.RecipeVersion != recipe.RecipeVersion {
		t.Fatalf("recipe version = %q, want %q", response.RecipeVersion, recipe.RecipeVersion)
	}
	if response.MaxPartBytes != recipe.MaxPartBytes || response.MaxPixels != recipe.MaxPixels {
		t.Fatalf("limits = %d/%d, want %d/%d", response.MaxPartBytes, response.MaxPixels, recipe.MaxPartBytes, recipe.MaxPixels)
	}
	if response.SessionTTLMs != (30 * time.Minute).Milliseconds() {
		t.Fatalf("session ttl = %d, want 30m", response.SessionTTLMs)
	}
	if len(response.Variants) != len(recipe.Variants) {
		t.Fatalf("variant count = %d, want %d", len(response.Variants), len(recipe.Variants))
	}
	galleryFound := false
	for _, variant := range response.Variants {
		if variant.Kind != model.ImageVariantKindGallery {
			continue
		}
		galleryFound = variant.Width == 400 && variant.Height == 400 && variant.Quality == 60 && variant.Fit == "cover"
	}
	if !galleryFound {
		t.Fatalf("gallery variant does not mirror the recipe: %#v", response.Variants)
	}
}

func TestValidateV2ManifestRejectsAnimationAndBadRecipe(t *testing.T) {
	recipe := testV2Recipe()
	req := validV2Manifest()
	req.Source.Animated = true
	if appErr := validateV2Manifest(&req, recipe); appErr == nil {
		t.Fatal("animated source should be rejected")
	}

	req = validV2Manifest()
	req.RecipeVersion = "1.0.0"
	if appErr := validateV2Manifest(&req, recipe); appErr == nil || appErr.HTTP != 426 {
		t.Fatalf("bad recipe should require upgrade, got %#v", appErr)
	}
}

func TestValidateV2ManifestRejectsOverflowingPixelDimensions(t *testing.T) {
	recipe := testV2Recipe()
	req := validV2Manifest()
	maximumInt := int(^uint(0) >> 1)
	req.Source.Width = maximumInt
	req.Source.Height = maximumInt
	if appErr := validateV2Manifest(&req, recipe); appErr == nil || appErr != errcode.ErrDimensionExceeded {
		t.Fatalf("overflowing dimensions error = %#v, want dimension exceeded", appErr)
	}
}

func TestValidateV2ManifestUsesRecipeGeometry(t *testing.T) {
	recipe := testV2Recipe()
	recipe.Variants[1] = config.ImageRecipeVariant{
		Kind: model.ImageVariantKindGallery, Width: 200, Height: 200, Quality: 70, Fit: "cover",
	}

	req := validV2Manifest()
	req.Parts[1] = V2PartManifest{
		Kind: model.ImageVariantKindGallery, Size: 128, SHA256: strings.Repeat("a", 64),
		MimeType: "image/webp", Width: 200, Height: 200, Quality: 70,
	}
	if appErr := validateV2Manifest(&req, recipe); appErr != nil {
		t.Fatalf("manifest matching the custom recipe rejected: %v", appErr)
	}

	req = validV2Manifest()
	if appErr := validateV2Manifest(&req, recipe); appErr == nil || appErr.HTTP != 400 {
		t.Fatalf("default geometry accepted against a custom recipe: %#v", appErr)
	}
}

func TestValidateV2ManifestUsesRecipeSourceMIMETypes(t *testing.T) {
	recipe := testV2Recipe()
	recipe.SourceMIMETypes = []string{"image/jpeg", "image/png"}

	req := validV2Manifest()
	if appErr := validateV2Manifest(&req, recipe); appErr != nil {
		t.Fatalf("allowed source type rejected: %v", appErr)
	}

	req.Source.MimeType = "image/avif"
	if appErr := validateV2Manifest(&req, recipe); appErr != errcode.ErrUnsupportedType {
		t.Fatalf("recipe-excluded source type error = %#v, want unsupported type", appErr)
	}
}

func TestValidateV2ManifestUsesRecipeLimits(t *testing.T) {
	recipe := testV2Recipe()
	recipe.MaxPixels = 1_000_000
	req := validV2Manifest()
	if appErr := validateV2Manifest(&req, recipe); appErr != errcode.ErrDimensionExceeded {
		t.Fatalf("source above recipe max_pixels error = %#v, want dimension exceeded", appErr)
	}

	recipe = testV2Recipe()
	recipe.MaxPartBytes = 100
	req = validV2Manifest()
	if appErr := validateV2Manifest(&req, recipe); appErr != errcode.ErrFileTooLarge {
		t.Fatalf("part above recipe max_part_bytes error = %#v, want file too large", appErr)
	}
}

func TestFitWithin(t *testing.T) {
	width, height := fitWithin(8000, 6000, 2048)
	if width != 2048 || height != 1536 {
		t.Fatalf("unexpected fit: %dx%d", width, height)
	}
	width, height = fitWithin(640, 480, 2048)
	if width != 640 || height != 480 {
		t.Fatalf("small image changed: %dx%d", width, height)
	}
}

func TestNormalizeV2UploadIDsDeduplicatesInInputOrder(t *testing.T) {
	first := strings.Repeat("a", 32)
	second := strings.Repeat("B", 31) + "_"
	uploadIDs, appErr := normalizeV2UploadIDs(&V2BatchStatusRequest{
		UploadIDs: []string{first, second, first},
	})
	if appErr != nil {
		t.Fatalf("valid upload IDs rejected: %v", appErr)
	}
	if len(uploadIDs) != 2 || uploadIDs[0] != first || uploadIDs[1] != second {
		t.Fatalf("upload IDs = %#v, want first occurrences in input order", uploadIDs)
	}
}

func TestNormalizeV2UploadIDsRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		req  *V2BatchStatusRequest
	}{
		{name: "nil", req: nil},
		{name: "empty", req: &V2BatchStatusRequest{}},
		{name: "too many", req: &V2BatchStatusRequest{UploadIDs: make([]string, 101)}},
		{name: "invalid characters", req: &V2BatchStatusRequest{UploadIDs: []string{strings.Repeat("a", 31) + "/"}}},
		{name: "wrong length", req: &V2BatchStatusRequest{UploadIDs: []string{"short"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, appErr := normalizeV2UploadIDs(tt.req); appErr == nil || appErr.HTTP != 400 {
				t.Fatalf("normalizeV2UploadIDs() error = %#v, want HTTP 400", appErr)
			}
		})
	}
}
