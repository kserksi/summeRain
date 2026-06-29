// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/summerain/image-gallery/internal/config"
	"github.com/summerain/image-gallery/internal/pkg/errcode"
	"github.com/summerain/image-gallery/internal/pkg/imgproxy"
	"github.com/summerain/image-gallery/internal/pkg/response"
	"github.com/summerain/image-gallery/internal/service"
)

type sessionResolver interface {
	Resolve(c *gin.Context) (userID uint64, role string, ok bool)
}

type PublicHandler struct {
	imageSvc            *service.ImageService
	storageCfg          *config.StorageConfig
	rdb                 *redis.Client
	signer              *imgproxy.Signer
	imgproxyURL         string
	client              *http.Client
	publicConfigService *service.PublicConfigService
	publicStatsService  *service.PublicStatsService
	resolver            sessionResolver
}

func NewPublicHandler(imageSvc *service.ImageService, storageCfg *config.StorageConfig, rdb *redis.Client, signer *imgproxy.Signer, imgproxyURL string, publicConfigService *service.PublicConfigService, publicStatsService *service.PublicStatsService, resolver sessionResolver) *PublicHandler {
	return &PublicHandler{
		imageSvc:            imageSvc,
		storageCfg:          storageCfg,
		rdb:                 rdb,
		signer:              signer,
		imgproxyURL:         imgproxyURL,
		client:              &http.Client{Timeout: 10 * time.Second},
		publicConfigService: publicConfigService,
		publicStatsService:  publicStatsService,
		resolver:            resolver,
	}
}

func (h *PublicHandler) GetConfig(c *gin.Context) {
	config, appErr := h.publicConfigService.Get()
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, config)
}

func (h *PublicHandler) GetStats(c *gin.Context) {
	stats, appErr := h.publicStatsService.Get()
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.Success(c, stats)
}

func (h *PublicHandler) ServeImage(c *gin.Context) {
	link := c.Param("link")

	uniqueLink, format := parseLink(link)

	image, appErr := h.imageSvc.GetByUniqueLink(uniqueLink)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}

	isPrivate := image.Visibility == "private"
	applyImageCacheHeaders(c, isPrivate)

	if isPrivate {
		if !h.isOwnerOrAdmin(c, image.UserID) {
			tok := extractToken(c)
			switch h.imageSvc.ValidateAccessToken(image.ID, tok) {
			case service.TokenValid:
				// ok
			case service.TokenRevoked:
				response.Error(c, errcode.ErrPrivateTokenRevoked)
				return
			default:
				// TokenExpired / TokenNotFound / missing token
				response.Error(c, errcode.ErrPrivateTokenInvalid)
				return
			}
		}
	}

	imageFile, fileErr := h.imageSvc.GetImageFile(image.ImageFileID)
	if fileErr != nil {
		response.Error(c, fileErr)
		return
	}

	go h.rdb.Incr(c.Request.Context(), fmt.Sprintf("views:%d", image.ID))

	// R2: redirect public originals to CDN
	if h.imageSvc.IsR2Enabled() && !isPrivate && format == "" {
		r2URL := h.imageSvc.R2PublicURL(imageFile.OriginalPath)
		if r2URL != "" {
			c.Redirect(302, r2URL)
			return
		}
	}

	if format == "" {
		if h.imageSvc.IsR2Enabled() {
			reader, err := h.imageSvc.R2Download(imageFile.OriginalPath)
			if err != nil {
				response.Error(c, errcode.New(4041, "文件不存在", 404))
				return
			}
			defer reader.Close()
			contentType := imageFile.MimeType
			if contentType == "image/svg+xml" {
				contentType = "application/octet-stream"
			}
			c.Header("Content-Type", contentType)
			c.Header("X-Content-Type-Options", "nosniff")
			c.Status(200)
			io.Copy(c.Writer, reader)
			return
		}
		fullPath := filepath.Join(h.storageCfg.BasePath, imageFile.OriginalPath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			response.Error(c, errcode.New(4041, "文件不存在", 404))
			return
		}
		contentType := imageFile.MimeType
		if contentType == "image/svg+xml" {
			contentType = "application/octet-stream"
		}
		c.Header("Content-Type", contentType)
		c.Header("X-Content-Type-Options", "nosniff")
		c.File(fullPath)
		return
	}

	quality := 80
	if q := c.Query("q"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 && v <= 100 {
			quality = v
		}
	}
	width, _ := strconv.Atoi(c.Query("w"))
	height, _ := strconv.Atoi(c.Query("h"))
	const maxDim = 4096
	if width < 0 {
		width = 0
	}
	if width > maxDim {
		width = maxDim
	}
	if height < 0 {
		height = 0
	}
	if height > maxDim {
		height = maxDim
	}

	var source string
	if h.imageSvc.IsR2Enabled() {
		source = h.imageSvc.R2PublicURL(imageFile.OriginalPath)
	} else {
		source = "local:///images/" + imageFile.OriginalPath
	}
	var path string
	if width > 0 && height > 0 {
		if format == "png" {
			path = fmt.Sprintf("/rs:fill:%d:%d/f:%s/plain/%s", width, height, format, source)
		} else {
			path = fmt.Sprintf("/rs:fill:%d:%d/q:%d/f:%s/plain/%s", width, height, quality, format, source)
		}
	} else {
		if format == "png" {
			path = fmt.Sprintf("/f:%s/plain/%s", format, source)
		} else {
			path = fmt.Sprintf("/q:%d/f:%s/plain/%s", quality, format, source)
		}
	}

	if wm := h.publicConfigService.GetWatermark(); wm != nil && wm.Enabled {
		path = strings.Replace(path, "/plain/", fmt.Sprintf("/wm:%s:%s/plain/", wm.Opacity, wm.Position), 1)
	}

	signedPath := h.signer.SignPath(path)
	resp, err := h.client.Get(h.imgproxyURL + signedPath)
	if err != nil {
		response.Error(c, errcode.ErrImgproxy)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		response.Error(c, errcode.ErrImgproxy)
		return
	}

	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(200)
	io.Copy(c.Writer, resp.Body)
}

func applyImageCacheHeaders(c *gin.Context, private bool) {
	if private {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
	} else {
		c.Header("Cache-Control", "no-cache, must-revalidate")
	}
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("X-Accel-Expires", "0")
	c.Header("Surrogate-Control", "no-store")
	c.Header("Vary", "Accept, X-Image-Token, Authorization")
}

func (h *PublicHandler) isOwnerOrAdmin(c *gin.Context, ownerID uint64) bool {
	if h.resolver == nil {
		return false
	}
	uid, role, ok := h.resolver.Resolve(c)
	if !ok {
		return false
	}
	return uid == ownerID || role == "admin"
}

func parseLink(link string) (uniqueLink string, format string) {
	dotIdx := strings.LastIndex(link, ".")
	if dotIdx == -1 {
		return link, ""
	}
	ext := link[dotIdx+1:]
	valid := map[string]bool{"webp": true, "avif": true, "jpg": true, "jpeg": true, "png": true, "gif": true}
	if valid[ext] {
		if ext == "jpeg" {
			ext = "jpg"
		}
		return link[:dotIdx], ext
	}
	return link, ""
}

func extractToken(c *gin.Context) string {
	if t := c.Query("token"); t != "" {
		return t
	}
	if t := c.GetHeader("X-Image-Token"); t != "" {
		return t
	}
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
