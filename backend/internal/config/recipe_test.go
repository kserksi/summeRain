// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func validImageRecipe() ImageRecipe {
	return ImageRecipe{
		PipelineVersion: ImageRecipePipelineVersion,
		RecipeVersion:   "2.0.0",
		MaxPixels:       50_000_000,
		MaxPartBytes:    64 << 20,
		SourceMIMETypes: []string{"image/jpeg", "image/png", "image/bmp", "image/webp", "image/avif"},
		Variants: []ImageRecipeVariant{
			{Kind: "master", Quality: 80, Fit: "original"},
			{Kind: "gallery", Width: 400, Height: 400, Quality: 60, Fit: "cover"},
			{Kind: "admin", Width: 120, Height: 160, Quality: 60, Fit: "cover"},
			{Kind: "publish_source", LongEdge: 2048, Quality: 80, Fit: "contain"},
		},
	}
}

func writeImageRecipe(t *testing.T, recipe ImageRecipe) string {
	t.Helper()
	data, err := json.Marshal(recipe)
	if err != nil {
		t.Fatalf("marshal recipe: %v", err)
	}
	path := filepath.Join(t.TempDir(), "image-recipe.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write recipe: %v", err)
	}
	return path
}

// TestEmbeddedImageRecipeMatchesDocumentedDefaults pins the shipped recipe to
// the behavior of the constants it replaced in v2.0.7.
func TestEmbeddedImageRecipeMatchesDocumentedDefaults(t *testing.T) {
	var recipe ImageRecipe
	if err := json.Unmarshal(embeddedImageRecipe, &recipe); err != nil {
		t.Fatalf("parse embedded recipe: %v", err)
	}
	want := validImageRecipe()
	if !reflect.DeepEqual(recipe, want) {
		t.Fatalf("embedded recipe = %#v, want %#v", recipe, want)
	}
}

func TestLoadImageRecipeFallsBackToEmbeddedDefault(t *testing.T) {
	t.Setenv(imageRecipeFileEnv, filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv(imageRecipeRequiredEnv, "false")

	recipe, err := LoadImageRecipe()
	if err != nil {
		t.Fatalf("LoadImageRecipe() error = %v", err)
	}
	if recipe.Origin != imageRecipeEmbeddedOrigin {
		t.Fatalf("origin = %q, want %q", recipe.Origin, imageRecipeEmbeddedOrigin)
	}
	if recipe.PipelineVersion != 2 || recipe.RecipeVersion != "2.0.0" {
		t.Fatalf("pipeline/version = %d/%q, want 2/2.0.0", recipe.PipelineVersion, recipe.RecipeVersion)
	}
	if recipe.MaxPixels != 50_000_000 || recipe.MaxPartBytes != 64<<20 {
		t.Fatalf("limits = %d/%d, want 50000000/%d", recipe.MaxPixels, recipe.MaxPartBytes, 64<<20)
	}
}

func TestLoadImageRecipeUsesExternalFile(t *testing.T) {
	custom := ImageRecipe{
		PipelineVersion: ImageRecipePipelineVersion,
		RecipeVersion:   "2.1.0",
		MaxPixels:       30_000_000,
		MaxPartBytes:    32 << 20,
		SourceMIMETypes: []string{"image/jpeg", "image/png"},
		Variants: []ImageRecipeVariant{
			{Kind: "master", Quality: 70, Fit: "original"},
			{Kind: "gallery", Width: 320, Height: 320, Quality: 50, Fit: "cover"},
			{Kind: "admin", Width: 96, Height: 128, Quality: 50, Fit: "cover"},
			{Kind: "publish_source", LongEdge: 1600, Quality: 70, Fit: "contain"},
		},
	}
	path := writeImageRecipe(t, custom)
	t.Setenv(imageRecipeFileEnv, path)

	recipe, err := LoadImageRecipe()
	if err != nil {
		t.Fatalf("LoadImageRecipe() error = %v", err)
	}
	if recipe.Origin != path {
		t.Fatalf("origin = %q, want %q", recipe.Origin, path)
	}
	if recipe.RecipeVersion != "2.1.0" || recipe.MaxPartBytes != 32<<20 || recipe.MaxPixels != 30_000_000 {
		t.Fatalf("external recipe ignored: %#v", recipe)
	}
	if recipe.AllowsSourceMIME("image/avif") {
		t.Fatal("external recipe MIME subset ignored")
	}
	variant, ok := recipe.Variant("gallery")
	if !ok || variant.Width != 320 || variant.Quality != 50 {
		t.Fatalf("gallery variant = %#v, want the external values", variant)
	}
}

func TestLoadImageRecipeRequiresFileWhenConfigured(t *testing.T) {
	t.Setenv(imageRecipeFileEnv, filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv(imageRecipeRequiredEnv, "true")

	if _, err := LoadImageRecipe(); err == nil || !strings.Contains(err.Error(), imageRecipeRequiredEnv) {
		t.Fatalf("LoadImageRecipe() error = %v, want a missing-file error naming %s", err, imageRecipeRequiredEnv)
	}
}

func TestLoadImageRecipeRejectsMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image-recipe.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatalf("write recipe: %v", err)
	}
	t.Setenv(imageRecipeFileEnv, path)

	if _, err := LoadImageRecipe(); err == nil || !strings.Contains(err.Error(), "parse image recipe") {
		t.Fatalf("LoadImageRecipe() error = %v, want a parse error", err)
	}
}

func TestImageRecipeValidateRejectsInvalidRecipes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ImageRecipe)
		want   string
	}{
		{name: "pipeline version", mutate: func(r *ImageRecipe) { r.PipelineVersion = 3 }, want: "pipeline_version"},
		{name: "recipe version", mutate: func(r *ImageRecipe) { r.RecipeVersion = "two" }, want: "recipe_version"},
		{name: "max pixels", mutate: func(r *ImageRecipe) { r.MaxPixels = 200_000_000 }, want: "max_pixels"},
		{name: "max part bytes", mutate: func(r *ImageRecipe) { r.MaxPartBytes = 1 << 30 }, want: "max_part_bytes"},
		{name: "empty mime list", mutate: func(r *ImageRecipe) { r.SourceMIMETypes = nil }, want: "must not be empty"},
		{name: "unsupported mime", mutate: func(r *ImageRecipe) { r.SourceMIMETypes = []string{"image/gif"} }, want: "unsupported type"},
		{name: "duplicate mime", mutate: func(r *ImageRecipe) { r.SourceMIMETypes = []string{"image/webp", "image/webp"} }, want: "duplicate"},
		{name: "missing variant", mutate: func(r *ImageRecipe) { r.Variants = r.Variants[:3] }, want: "exactly"},
		{name: "unknown kind", mutate: func(r *ImageRecipe) { r.Variants[0].Kind = "poster" }, want: "unknown variant kind"},
		{name: "master geometry", mutate: func(r *ImageRecipe) { r.Variants[0].Width = 10 }, want: "must not declare geometry"},
		{name: "gallery fit", mutate: func(r *ImageRecipe) { r.Variants[1].Fit = "contain" }, want: "fit must be"},
		{name: "admin dimensions", mutate: func(r *ImageRecipe) { r.Variants[2].Width = 9000 }, want: "width must be"},
		{name: "publish edge", mutate: func(r *ImageRecipe) { r.Variants[3].LongEdge = 0 }, want: "long_edge must be"},
		{name: "quality", mutate: func(r *ImageRecipe) { r.Variants[1].Quality = 0 }, want: "quality must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipe := validImageRecipe()
			tt.mutate(&recipe)
			if err := recipe.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestImageRecipeAllowsSourceMIMECaseInsensitively(t *testing.T) {
	recipe := validImageRecipe()
	if !recipe.AllowsSourceMIME("  IMAGE/JPEG ") {
		t.Fatal("normalized source MIME should be accepted")
	}
	if recipe.AllowsSourceMIME("image/gif") {
		t.Fatal("unsupported source MIME should be rejected")
	}
}

func TestImageRecipeVariantKindsPreserveOrder(t *testing.T) {
	recipe := validImageRecipe()
	want := []string{"master", "gallery", "admin", "publish_source"}
	if !reflect.DeepEqual(recipe.VariantKinds(), want) {
		t.Fatalf("VariantKinds() = %v, want %v", recipe.VariantKinds(), want)
	}
}

func TestImageRecipeSummaryIsSingleLine(t *testing.T) {
	recipe := validImageRecipe()
	recipe.Origin = imageRecipeEmbeddedOrigin
	summary := recipe.Summary()
	if strings.Contains(summary, "\n") {
		t.Fatalf("summary must be one line: %q", summary)
	}
	for _, fragment := range []string{
		"image_recipe_source=embedded",
		"recipe_version=2.0.0",
		"pipeline_version=2",
		"max_pixels=50000000",
		"max_part_bytes=67108864",
		"variants=master:original:q80,gallery:400x400:q60,admin:120x160:q60,publish_source:long_edge=2048:q80",
	} {
		if !strings.Contains(summary, fragment) {
			t.Fatalf("summary %q is missing %q", summary, fragment)
		}
	}
}
