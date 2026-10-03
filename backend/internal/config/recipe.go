// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/kserksi/summerain/internal/model"
)

const (
	// ImageRecipePipelineVersion is the only pipeline version this server
	// understands. The browser processor and the part validators are written
	// against it, so it is a code-level constant rather than recipe data.
	ImageRecipePipelineVersion uint16 = 2
	// ImageRecipeDefaultPath is where the container image bakes the default
	// recipe. IMAGE_RECIPE_FILE overrides it for other deployments.
	ImageRecipeDefaultPath = "/app/config/image-recipe.json"

	imageRecipeFileEnv     = "IMAGE_RECIPE_FILE"
	imageRecipeRequiredEnv = "IMAGE_RECIPE_REQUIRED"

	imageRecipeFitOriginal = "original"
	imageRecipeFitCover    = "cover"
	imageRecipeFitContain  = "contain"

	imageRecipeEmbeddedOrigin = "embedded"

	imageRecipeMinPixels    = 1_000_000
	imageRecipeMaxPixels    = 100_000_000
	imageRecipeMinPartBytes = 1 << 20
	imageRecipeMaxPartBytes = 64 << 20
	imageRecipeMinDimension = 1
	imageRecipeMaxDimension = 4096
	imageRecipeMinQuality   = 1
	imageRecipeMaxQuality   = 100
)

var imageRecipeVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// imageRecipeSourceMIMETypes is the code-level allowlist of source formats the
// browser processors can decode. A recipe may select any subset of it.
var imageRecipeSourceMIMETypes = []string{
	"image/jpeg",
	"image/png",
	"image/bmp",
	"image/webp",
	"image/avif",
}

// ImageRecipe is the fixed image policy of this server: which source formats
// are accepted, which pixel and byte limits apply, and which variants the
// browser must produce. It is read once at startup and requires a restart to
// change. Variant geometry is data, but the pipeline version, the shape of
// each variant kind, and the absolute limits stay in code so a recipe can only
// tighten them.
type ImageRecipe struct {
	PipelineVersion uint16               `json:"pipeline_version"`
	RecipeVersion   string               `json:"recipe_version"`
	MaxPixels       int64                `json:"max_pixels"`
	MaxPartBytes    int64                `json:"max_part_bytes"`
	SourceMIMETypes []string             `json:"supported_source_mime_types"`
	Variants        []ImageRecipeVariant `json:"variants"`
	// Origin reports where this recipe was loaded from for the startup log. It
	// is never serialized into API responses.
	Origin string `json:"-"`
}

// ImageRecipeVariant describes one persisted asset. Master keeps the oriented
// source dimensions, cover crops to a fixed box, and contain fits the longest
// edge into a fixed bound.
type ImageRecipeVariant struct {
	Kind     string `json:"kind"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	LongEdge int    `json:"long_edge,omitempty"`
	Quality  uint8  `json:"quality"`
	Fit      string `json:"fit"`
}

//go:embed image-recipe.json
var embeddedImageRecipe []byte

// LoadImageRecipe resolves the image policy for this process. An external file
// at IMAGE_RECIPE_FILE (default ImageRecipeDefaultPath) wins when it exists;
// otherwise the embedded default is used unless IMAGE_RECIPE_REQUIRED=true, in
// which case a missing file aborts startup. Any parse or validation error is
// fatal for the caller.
func LoadImageRecipe() (*ImageRecipe, error) {
	path := getEnv(imageRecipeFileEnv, ImageRecipeDefaultPath)
	required := getEnvBool(imageRecipeRequiredEnv, false)

	origin := path
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
	case errors.Is(err, os.ErrNotExist):
		if required {
			return nil, fmt.Errorf("image recipe %q is missing and %s=true", path, imageRecipeRequiredEnv)
		}
		data = embeddedImageRecipe
		origin = imageRecipeEmbeddedOrigin
	default:
		return nil, fmt.Errorf("read image recipe %q: %w", path, err)
	}

	recipe := &ImageRecipe{}
	if err := json.Unmarshal(data, recipe); err != nil {
		return nil, fmt.Errorf("parse image recipe %s: %w", origin, err)
	}
	if err := recipe.Validate(); err != nil {
		return nil, fmt.Errorf("invalid image recipe %s: %w", origin, err)
	}
	recipe.Origin = origin
	return recipe, nil
}

// Validate enforces the code-level absolute limits and the required shape of
// every variant kind. A recipe can tighten a limit but never exceed it, and it
// must declare exactly the four persisted kinds.
func (r *ImageRecipe) Validate() error {
	if r == nil {
		return errors.New("image recipe is nil")
	}
	if r.PipelineVersion != ImageRecipePipelineVersion {
		return fmt.Errorf("pipeline_version %d is not supported (want %d)", r.PipelineVersion, ImageRecipePipelineVersion)
	}
	if !imageRecipeVersionPattern.MatchString(r.RecipeVersion) {
		return fmt.Errorf("recipe_version %q must be a semantic version such as 2.0.0", r.RecipeVersion)
	}
	if r.MaxPixels < imageRecipeMinPixels || r.MaxPixels > imageRecipeMaxPixels {
		return fmt.Errorf("max_pixels must be between %d and %d", imageRecipeMinPixels, imageRecipeMaxPixels)
	}
	if r.MaxPartBytes < imageRecipeMinPartBytes || r.MaxPartBytes > imageRecipeMaxPartBytes {
		return fmt.Errorf("max_part_bytes must be between %d and %d", imageRecipeMinPartBytes, imageRecipeMaxPartBytes)
	}
	if err := r.validateSourceMIMETypes(); err != nil {
		return err
	}
	return r.validateVariants()
}

func (r *ImageRecipe) validateSourceMIMETypes() error {
	if len(r.SourceMIMETypes) == 0 {
		return errors.New("supported_source_mime_types must not be empty")
	}
	seen := make(map[string]struct{}, len(r.SourceMIMETypes))
	for _, mimeType := range r.SourceMIMETypes {
		normalized := strings.ToLower(strings.TrimSpace(mimeType))
		if !isSupportedRecipeSourceMIME(normalized) {
			return fmt.Errorf("supported_source_mime_types contains unsupported type %q", mimeType)
		}
		if _, exists := seen[normalized]; exists {
			return fmt.Errorf("supported_source_mime_types contains duplicate %q", mimeType)
		}
		seen[normalized] = struct{}{}
	}
	return nil
}

func (r *ImageRecipe) validateVariants() error {
	required := []string{
		model.ImageVariantKindMaster,
		model.ImageVariantKindGallery,
		model.ImageVariantKindAdmin,
		model.ImageVariantKindPublishSource,
	}
	if len(r.Variants) != len(required) {
		return fmt.Errorf("variants must declare exactly %s", strings.Join(required, ", "))
	}
	seen := make(map[string]struct{}, len(r.Variants))
	for index, variant := range r.Variants {
		if _, exists := seen[variant.Kind]; exists {
			return fmt.Errorf("variants contains duplicate kind %q", variant.Kind)
		}
		seen[variant.Kind] = struct{}{}
		if err := variant.validate(); err != nil {
			return fmt.Errorf("variant %d: %w", index, err)
		}
	}
	for _, kind := range required {
		if _, exists := seen[kind]; !exists {
			return fmt.Errorf("variants must include %q", kind)
		}
	}
	return nil
}

func (v ImageRecipeVariant) validate() error {
	if v.Quality < imageRecipeMinQuality || v.Quality > imageRecipeMaxQuality {
		return fmt.Errorf("%s quality must be between %d and %d", v.Kind, imageRecipeMinQuality, imageRecipeMaxQuality)
	}
	switch v.Kind {
	case model.ImageVariantKindMaster:
		if v.Fit != imageRecipeFitOriginal {
			return fmt.Errorf("master fit must be %q", imageRecipeFitOriginal)
		}
		if v.Width != 0 || v.Height != 0 || v.LongEdge != 0 {
			return errors.New("master must not declare geometry")
		}
	case model.ImageVariantKindGallery, model.ImageVariantKindAdmin:
		if v.Fit != imageRecipeFitCover {
			return fmt.Errorf("%s fit must be %q", v.Kind, imageRecipeFitCover)
		}
		if err := validateRecipeDimension(v.Kind, "width", v.Width); err != nil {
			return err
		}
		if err := validateRecipeDimension(v.Kind, "height", v.Height); err != nil {
			return err
		}
		if v.LongEdge != 0 {
			return fmt.Errorf("%s must not declare long_edge", v.Kind)
		}
	case model.ImageVariantKindPublishSource:
		if v.Fit != imageRecipeFitContain {
			return fmt.Errorf("publish_source fit must be %q", imageRecipeFitContain)
		}
		if v.Width != 0 || v.Height != 0 {
			return errors.New("publish_source must not declare width or height")
		}
		if err := validateRecipeDimension(v.Kind, "long_edge", v.LongEdge); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown variant kind %q", v.Kind)
	}
	return nil
}

func validateRecipeDimension(kind, field string, value int) error {
	if value < imageRecipeMinDimension || value > imageRecipeMaxDimension {
		return fmt.Errorf("%s %s must be between %d and %d", kind, field, imageRecipeMinDimension, imageRecipeMaxDimension)
	}
	return nil
}

func isSupportedRecipeSourceMIME(mimeType string) bool {
	for _, supported := range imageRecipeSourceMIMETypes {
		if supported == mimeType {
			return true
		}
	}
	return false
}

// Variant returns the declared variant of the given kind.
func (r *ImageRecipe) Variant(kind string) (ImageRecipeVariant, bool) {
	for _, variant := range r.Variants {
		if variant.Kind == kind {
			return variant, true
		}
	}
	return ImageRecipeVariant{}, false
}

// VariantKinds lists the declared variant kinds in recipe order.
func (r *ImageRecipe) VariantKinds() []string {
	kinds := make([]string, 0, len(r.Variants))
	for _, variant := range r.Variants {
		kinds = append(kinds, variant.Kind)
	}
	return kinds
}

// AllowsSourceMIME reports whether the recipe accepts the source format.
func (r *ImageRecipe) AllowsSourceMIME(mimeType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(mimeType))
	for _, allowed := range r.SourceMIMETypes {
		if strings.ToLower(strings.TrimSpace(allowed)) == normalized {
			return true
		}
	}
	return false
}

// Summary returns one non-sensitive startup log line.
func (r *ImageRecipe) Summary() string {
	variants := make([]string, 0, len(r.Variants))
	for _, variant := range r.Variants {
		switch {
		case variant.LongEdge != 0:
			variants = append(variants, fmt.Sprintf("%s:long_edge=%d:q%d", variant.Kind, variant.LongEdge, variant.Quality))
		case variant.Width != 0 || variant.Height != 0:
			variants = append(variants, fmt.Sprintf("%s:%dx%d:q%d", variant.Kind, variant.Width, variant.Height, variant.Quality))
		default:
			variants = append(variants, fmt.Sprintf("%s:%s:q%d", variant.Kind, variant.Fit, variant.Quality))
		}
	}
	return fmt.Sprintf(
		"image_recipe_source=%s recipe_version=%s pipeline_version=%d max_pixels=%d max_part_bytes=%d source_mime_types=%d variants=%s",
		r.Origin, r.RecipeVersion, r.PipelineVersion, r.MaxPixels, r.MaxPartBytes, len(r.SourceMIMETypes), strings.Join(variants, ","),
	)
}
