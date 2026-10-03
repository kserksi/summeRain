// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
)

const (
	v2MinimumPartBytes          = 64
	v2PublishOutputReserveBytes = int64(32 << 20)
	// Terminal sessions may still wait for staging cleanup, but their files are
	// already reflected by disk pressure and must not reserve quota a second time.
	v2ActiveReservationCondition = "((status IN ? AND expires_at > ?) OR status = ?)"
)

var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var uploadIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{32}$`)

type V2SourceManifest struct {
	MimeType string `json:"mime_type" binding:"required"`
	Width    int    `json:"width" binding:"required"`
	Height   int    `json:"height" binding:"required"`
	Animated bool   `json:"animated"`
}

type V2PartManifest struct {
	Kind     string `json:"kind" binding:"required"`
	Size     int64  `json:"size" binding:"required"`
	SHA256   string `json:"sha256" binding:"required"`
	MimeType string `json:"mime_type" binding:"required"`
	Width    int    `json:"width" binding:"required"`
	Height   int    `json:"height" binding:"required"`
	Quality  uint8  `json:"quality" binding:"required"`
}

type V2InitUploadRequest struct {
	Filename         string           `json:"filename" binding:"required"`
	Visibility       string           `json:"visibility"`
	ProcessorVersion string           `json:"processor_version" binding:"required"`
	RecipeVersion    string           `json:"recipe_version" binding:"required"`
	Source           V2SourceManifest `json:"source" binding:"required"`
	Parts            []V2PartManifest `json:"parts" binding:"required"`
	FocalX           *float64         `json:"focal_x,omitempty"`
	FocalY           *float64         `json:"focal_y,omitempty"`
}

type V2UploadPartResponse struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
	PutURL string `json:"put_url,omitempty"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type V2UploadResponse struct {
	UploadID   string                 `json:"upload_id"`
	Status     string                 `json:"status"`
	ImageID    *uint64                `json:"image_id,omitempty"`
	UniqueLink string                 `json:"unique_link,omitempty"`
	AssetLink  string                 `json:"asset_link,omitempty"`
	ExpiresAt  time.Time              `json:"expires_at"`
	Parts      []V2UploadPartResponse `json:"parts"`
}

type V2BatchStatusRequest struct {
	UploadIDs []string `json:"upload_ids" binding:"required"`
}

type V2BatchStatusResponse struct {
	Uploads []V2UploadResponse `json:"uploads"`
}

type V2RecipeResponse struct {
	V2Enabled       bool   `json:"v2_enabled"`
	PipelineVersion uint16 `json:"pipeline_version"`
	RecipeVersion   string `json:"recipe_version"`
	MaxPartBytes    int64  `json:"max_part_bytes"`
	MaxPixels       int64  `json:"max_pixels"`
	SessionTTLMs    int64  `json:"session_ttl_ms"`
	Variants        []struct {
		Kind     string `json:"kind"`
		Width    int    `json:"width,omitempty"`
		Height   int    `json:"height,omitempty"`
		LongEdge int    `json:"long_edge,omitempty"`
		Quality  uint8  `json:"quality"`
		Fit      string `json:"fit"`
	} `json:"variants"`
}

func normalizeV2UploadIDs(req *V2BatchStatusRequest) ([]string, *errcode.AppError) {
	if req == nil || len(req.UploadIDs) < 1 || len(req.UploadIDs) > 100 {
		return nil, errcode.New(3005, "upload_ids 必须包含 1 到 100 个上传 ID", 400)
	}

	uploadIDs := make([]string, 0, len(req.UploadIDs))
	seen := make(map[string]struct{}, len(req.UploadIDs))
	for _, uploadID := range req.UploadIDs {
		if !uploadIDPattern.MatchString(uploadID) {
			return nil, errcode.New(3005, "upload_ids 包含无效的上传 ID", 400)
		}
		if _, exists := seen[uploadID]; exists {
			continue
		}
		seen[uploadID] = struct{}{}
		uploadIDs = append(uploadIDs, uploadID)
	}
	return uploadIDs, nil
}

func validateV2Manifest(req *V2InitUploadRequest, recipe *config.ImageRecipe) *errcode.AppError {
	req.Filename = filepath.Base(strings.TrimSpace(req.Filename))
	if req.Filename == "." || req.Filename == "" || len(req.Filename) > 255 {
		return errcode.New(3005, "无效的文件名", 400)
	}
	if req.Visibility == "" {
		req.Visibility = "public"
	}
	if req.Visibility != "public" && req.Visibility != "private" {
		return errcode.New(3005, "visibility 必须为 public 或 private", 400)
	}
	if req.RecipeVersion != recipe.RecipeVersion {
		return errcode.NewWithData(4261, "客户端图片配方版本不受支持", 426, map[string]string{"required_recipe_version": recipe.RecipeVersion})
	}
	if strings.TrimSpace(req.ProcessorVersion) == "" || len(req.ProcessorVersion) > 64 {
		return errcode.New(3005, "无效的客户端处理器版本", 400)
	}
	if !recipe.AllowsSourceMIME(req.Source.MimeType) || req.Source.Animated {
		return errcode.ErrUnsupportedType
	}
	if !v2DimensionsWithinLimit(req.Source.Width, req.Source.Height, recipe.MaxPixels) {
		return errcode.ErrDimensionExceeded
	}
	if (req.FocalX == nil) != (req.FocalY == nil) {
		return errcode.New(3005, "focal_x 和 focal_y 必须同时提供", 400)
	}
	if req.FocalX != nil && (*req.FocalX < 0 || *req.FocalX > 1 || *req.FocalY < 0 || *req.FocalY > 1) {
		return errcode.New(3005, "裁剪焦点必须位于 0 到 1 之间", 400)
	}
	requiredKinds := recipe.VariantKinds()
	if len(req.Parts) != len(requiredKinds) {
		return errcode.New(3005, "V2 上传必须包含 master、gallery、admin 和 publish_source", 400)
	}

	seen := make(map[string]bool, len(req.Parts))
	for i := range req.Parts {
		part := &req.Parts[i]
		part.SHA256 = strings.ToLower(strings.TrimSpace(part.SHA256))
		if seen[part.Kind] || !isRequiredV2Part(part.Kind, requiredKinds) {
			return errcode.New(3005, "上传部件类型重复或不受支持", 400)
		}
		seen[part.Kind] = true
		if part.MimeType != "image/webp" || !sha256Pattern.MatchString(part.SHA256) {
			return errcode.New(3005, "上传部件必须是带有效 SHA-256 的 WebP", 400)
		}
		if part.Size < v2MinimumPartBytes {
			return errcode.New(3005, "上传部件大小低于有效 WebP 的最小限制", 400)
		}
		if part.Size > recipe.MaxPartBytes {
			return errcode.ErrFileTooLarge
		}
		if !v2DimensionsWithinLimit(part.Width, part.Height, recipe.MaxPixels) {
			return errcode.ErrDimensionExceeded
		}
		if err := validateV2PartGeometry(*part, req.Source, recipe); err != nil {
			return errcode.New(3005, err.Error(), 400)
		}
	}
	return nil
}

func v2DimensionsWithinLimit(width, height int, maximumPixels int64) bool {
	if width <= 0 || height <= 0 || maximumPixels <= 0 {
		return false
	}
	// Division keeps hostile near-MaxInt JSON dimensions from overflowing the
	// multiplication and being mistaken for a small image.
	return int64(width) <= maximumPixels/int64(height)
}

func validateV2PartGeometry(part V2PartManifest, source V2SourceManifest, recipe *config.ImageRecipe) error {
	variant, ok := recipe.Variant(part.Kind)
	if !ok {
		return fmt.Errorf("未知上传部件")
	}
	switch part.Kind {
	case model.ImageVariantKindMaster:
		if part.Width != source.Width || part.Height != source.Height {
			return fmt.Errorf("master 尺寸必须与源图片一致")
		}
	case model.ImageVariantKindGallery:
		if part.Width != variant.Width || part.Height != variant.Height {
			return fmt.Errorf("gallery 尺寸必须为 %dx%d", variant.Width, variant.Height)
		}
	case model.ImageVariantKindAdmin:
		if part.Width != variant.Width || part.Height != variant.Height {
			return fmt.Errorf("admin 尺寸必须为 %dx%d", variant.Width, variant.Height)
		}
	case model.ImageVariantKindPublishSource:
		if part.Width > variant.LongEdge || part.Height > variant.LongEdge {
			return fmt.Errorf("publish_source 最长边不得超过 %d", variant.LongEdge)
		}
		wantWidth, wantHeight := fitWithin(source.Width, source.Height, variant.LongEdge)
		if part.Width != wantWidth || part.Height != wantHeight {
			return fmt.Errorf("publish_source 尺寸与固定配方不一致")
		}
	default:
		return fmt.Errorf("未知上传部件")
	}
	if part.Quality != variant.Quality {
		return fmt.Errorf("%s 质量参数与固定配方不一致", part.Kind)
	}
	return nil
}

func fitWithin(width, height, edge int) (int, int) {
	if width <= edge && height <= edge {
		return width, height
	}
	if width >= height {
		return edge, maxInt(1, int(float64(height)*float64(edge)/float64(width)+0.5))
	}
	return maxInt(1, int(float64(width)*float64(edge)/float64(height)+0.5)), edge
}

func isRequiredV2Part(kind string, required []string) bool {
	for _, expected := range required {
		if kind == expected {
			return true
		}
	}
	return false
}

// v2PartQuality reports the recipe quality for a persisted variant kind.
func v2PartQuality(kind string, recipe *config.ImageRecipe) uint8 {
	if variant, ok := recipe.Variant(kind); ok {
		return variant.Quality
	}
	return 0
}

func randomURLToken(byteCount int) (string, error) {
	data := make([]byte, byteCount)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func randomHex(byteCount int) (string, error) {
	data := make([]byte, byteCount)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type v2UploadLimiter struct {
	global  chan struct{}
	perUser int
	mu      sync.Mutex
	active  map[uint64]int
}

func newV2UploadLimiter(global, perUser int) *v2UploadLimiter {
	return &v2UploadLimiter{global: make(chan struct{}, global), perUser: perUser, active: make(map[uint64]int)}
}

func (l *v2UploadLimiter) acquire(userID uint64) (func(), bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active[userID] >= l.perUser {
		return nil, false
	}
	select {
	case l.global <- struct{}{}:
		l.active[userID]++
	default:
		return nil, false
	}
	return func() {
		<-l.global
		l.mu.Lock()
		l.active[userID]--
		if l.active[userID] == 0 {
			delete(l.active, userID)
		}
		l.mu.Unlock()
	}, true
}
