// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/pkg/imgproxy"
	"github.com/kserksi/summerain/internal/pkg/response"
	"github.com/kserksi/summerain/internal/service"
)

type sessionResolver interface {
	Resolve(c *gin.Context) (userID uint64, role string, ok bool)
}

const (
	v1DynamicGenerationConcurrency   = 2
	v1DynamicGenerationQueueDepth    = 4
	v1BackgroundFormatConcurrency    = 1
	v1DynamicMaximumResponseBytes    = 96 << 20
	v1BackgroundMaximumResponseBytes = 32 << 20
	v1GenerationTimeout              = 35 * time.Second
)

var (
	errDynamicImageQueueFull  = errors.New("dynamic image generation queue is full")
	errGeneratedImageTooLarge = errors.New("generated image exceeds the response limit")
	errUnsafeStoragePath      = errors.New("storage path escapes the configured root")
)

type generatedImageFile struct {
	path        string
	contentType string
}

type dynamicImageCall struct {
	done            chan struct{}
	result          generatedImageFile
	err             error
	cancel          context.CancelFunc
	capacityRelease func()
	capacityOnce    sync.Once
	references      int
	complete        bool
}

func (c *dynamicImageCall) releaseCapacity() {
	if c.capacityRelease != nil {
		c.capacityOnce.Do(c.capacityRelease)
	}
}

// dynamicImageGroup lets concurrent requests for the same transformation
// share one imgproxy response without retaining that response in Go memory.
type dynamicImageGroup struct {
	mu    sync.Mutex
	calls map[string]*dynamicImageCall
}

func (g *dynamicImageGroup) acquire(key string) (*dynamicImageCall, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.calls == nil {
		g.calls = make(map[string]*dynamicImageCall)
	}
	if call, ok := g.calls[key]; ok {
		call.references++
		return call, false
	}
	call := &dynamicImageCall{done: make(chan struct{}), references: 1}
	g.calls[key] = call
	return call, true
}

func (g *dynamicImageGroup) finish(key string, call *dynamicImageCall, result generatedImageFile, err error) {
	g.mu.Lock()
	call.result = result
	call.err = err
	call.complete = true
	close(call.done)
	remove := call.references == 0
	if current, ok := g.calls[key]; remove && ok && current == call {
		delete(g.calls, key)
	}
	g.mu.Unlock()
	if remove {
		if result.path != "" {
			_ = os.Remove(result.path)
		}
		call.releaseCapacity()
	}
}

func (g *dynamicImageGroup) setLifecycle(call *dynamicImageCall, cancel context.CancelFunc, capacityRelease func()) {
	g.mu.Lock()
	call.cancel = cancel
	call.capacityRelease = capacityRelease
	g.mu.Unlock()
}

func (g *dynamicImageGroup) release(key string, call *dynamicImageCall) {
	g.mu.Lock()
	call.references--
	remove := call.references == 0
	var cancel context.CancelFunc
	if remove && !call.complete {
		cancel = call.cancel
	}
	if current, ok := g.calls[key]; remove && ok && current == call {
		delete(g.calls, key)
	}
	path := call.result.path
	complete := call.complete
	g.mu.Unlock()
	if remove && path != "" {
		_ = os.Remove(path)
	}
	if remove && complete {
		call.releaseCapacity()
	}
	if cancel != nil {
		cancel()
	}
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
	dynamicSemaphore    chan struct{}
	dynamicCapacity     chan struct{}
	backgroundSemaphore chan struct{}
	dynamicImages       dynamicImageGroup
	backgroundInFlight  sync.Map
}

func NewPublicHandler(imageSvc *service.ImageService, storageCfg *config.StorageConfig, rdb *redis.Client, signer *imgproxy.Signer, imgproxyURL string, publicConfigService *service.PublicConfigService, publicStatsService *service.PublicStatsService, resolver sessionResolver) *PublicHandler {
	return &PublicHandler{
		imageSvc:            imageSvc,
		storageCfg:          storageCfg,
		rdb:                 rdb,
		signer:              signer,
		imgproxyURL:         imgproxyURL,
		client:              &http.Client{Timeout: 30 * time.Second},
		publicConfigService: publicConfigService,
		publicStatsService:  publicStatsService,
		resolver:            resolver,
		dynamicSemaphore:    make(chan struct{}, v1DynamicGenerationConcurrency),
		dynamicCapacity:     make(chan struct{}, v1DynamicGenerationConcurrency+v1DynamicGenerationQueueDepth),
		backgroundSemaphore: make(chan struct{}, v1BackgroundFormatConcurrency),
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
	image, isPrivate, ok := h.authorizedImage(c, uniqueLink)
	if !ok {
		return
	}

	if image.PipelineVersion >= model.ImagePipelineVersionV2 {
		kind := model.ImageVariantKindPublish
		if c.Query("type") == "thumbnail" {
			kind = model.ImageVariantKindGallery
		}
		if h.rdb != nil {
			_ = h.rdb.Incr(c.Request.Context(), fmt.Sprintf("views:%d", image.ID)).Err()
		}
		h.serveV2Variant(c, image, kind)
		return
	}

	imageFile, fileErr := h.imageSvc.GetImageFile(image.ImageFileID)
	if fileErr != nil {
		response.Error(c, fileErr)
		return
	}
	storageTarget, storageErr := h.imageSvc.ResolveV1StorageTarget(image.PipelineVersion, imageFile)
	if storageErr != nil {
		h.failV1StorageTarget(c, imageFile, storageErr)
		return
	}
	remote := storageTarget.Backend == service.V1StorageBackendR2
	remoteOriginalURL := ""
	if remote {
		remoteOriginalURL, storageErr = h.imageSvc.R2PublicURLForTarget(
			imageFile.OriginalPath,
			storageTarget.Endpoint,
			storageTarget.Bucket,
		)
		if storageErr != nil || remoteOriginalURL == "" {
			if storageErr == nil {
				storageErr = errors.New("remote storage returned an empty public URL")
			}
			h.failV1StorageTarget(c, imageFile, storageErr)
			return
		}
	}

	if h.rdb != nil {
		_ = h.rdb.Incr(c.Request.Context(), fmt.Sprintf("views:%d", image.ID)).Err()
	}

	if remote && !isPrivate && format == "" {
		c.Redirect(http.StatusFound, remoteOriginalURL)
		return
	}

	if format == "" {
		if remote {
			reader, err := h.imageSvc.R2DownloadForTarget(
				c.Request.Context(),
				imageFile.OriginalPath,
				storageTarget.Endpoint,
				storageTarget.Bucket,
			)
			if err != nil {
				h.failV1StorageTarget(c, imageFile, err)
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
			_, _ = io.Copy(c.Writer, reader)
			return
		}
		storedFile, err := openStorageFile(h.storageCfg.BasePath, imageFile.OriginalPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				response.Error(c, errcode.New(4041, "文件不存在", 404))
			} else {
				response.Error(c, errcode.ErrInternal)
			}
			return
		}
		defer storedFile.file.Close()
		contentType := imageFile.MimeType
		if contentType == "image/svg+xml" {
			contentType = "application/octet-stream"
		}
		c.Header("Content-Type", contentType)
		c.Header("X-Content-Type-Options", "nosniff")
		http.ServeContent(c.Writer, c.Request, storedFile.info.Name(), storedFile.info.ModTime(), storedFile.file)
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

	// Fast path: serve pre-processed file from disk when no resize is requested.
	// Pre-processed files were generated at upload time at original dimensions.
	if width == 0 && height == 0 {
		if processedPath, ok := service.V1PersistentVariantPath(imageFile.ProcessedPath, format); ok {
			// Background AVIF files are intentionally local. The uploaded WebP
			// follows the ImageFile storage lineage and must never fall back to a
			// coincidentally matching local path for an R2-backed record.
			if remote && format == "webp" {
				if h.serveV1RemoteObject(c, imageFile, storageTarget, processedPath, "image/webp", !isPrivate) {
					return
				}
				return
			}
			processedFile, openErr := openStorageFile(h.storageCfg.BasePath, processedPath)
			if openErr == nil {
				defer processedFile.file.Close()
				applyImageCacheHeaders(c, isPrivate)
				c.Header("Content-Type", "image/"+format)
				c.Header("X-Content-Type-Options", "nosniff")
				http.ServeContent(c.Writer, c.Request, processedFile.info.Name(), processedFile.info.ModTime(), processedFile.file)
				return
			}
			if !errors.Is(openErr, os.ErrNotExist) {
				response.Error(c, errcode.ErrInternal)
				return
			}
		}

		// AVIF progressive enhancement: degrade to WebP, trigger background AVIF generation.
		if format == "avif" {
			webpPath, ok := service.V1PersistentVariantPath(imageFile.ProcessedPath, "webp")
			if !ok {
				response.Error(c, errcode.ErrInternal)
				return
			}
			if remote {
				h.triggerBackgroundFormat(imageFile, "avif", quality, remoteOriginalURL)
				h.serveV1RemoteObject(c, imageFile, storageTarget, webpPath, "image/webp", !isPrivate)
				return
			}
			webpFile, webpErr := openStorageFile(h.storageCfg.BasePath, webpPath)
			if webpErr == nil {
				defer webpFile.file.Close()
				bgSource := ""
				if originalFile, originalErr := openStorageFile(h.storageCfg.BasePath, imageFile.OriginalPath); originalErr == nil {
					bgSource = "local:///images/" + filepath.ToSlash(originalFile.relativePath)
					_ = originalFile.file.Close()
				}
				if bgSource != "" {
					h.triggerBackgroundFormat(imageFile, "avif", quality, bgSource)
				}

				c.Header("Content-Type", "image/webp")
				c.Header("X-Content-Type-Options", "nosniff")
				if isPrivate {
					c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
				} else {
					c.Header("Cache-Control", "public, max-age=10, must-revalidate")
					c.Header("X-Accel-Expires", "10")
				}
				http.ServeContent(c.Writer, c.Request, webpFile.info.Name(), webpFile.info.ModTime(), webpFile.file)
				return
			}
			if !errors.Is(webpErr, os.ErrNotExist) {
				response.Error(c, errcode.ErrInternal)
				return
			}
		}
	}

	var source string
	if remote {
		source = remoteOriginalURL
	} else {
		sourceFile, openErr := openStorageFile(h.storageCfg.BasePath, imageFile.OriginalPath)
		if openErr != nil {
			if errors.Is(openErr, os.ErrNotExist) {
				response.Error(c, errcode.New(4041, "文件不存在", 404))
			} else {
				response.Error(c, errcode.ErrInternal)
			}
			return
		}
		_ = sourceFile.file.Close()
		source = "local:///images/" + filepath.ToSlash(sourceFile.relativePath)
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

	generated, release, err := h.loadDynamicImage(c.Request.Context(), path)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		if errors.Is(err, errDynamicImageQueueFull) {
			c.Header("Retry-After", "2")
			response.Error(c, errcode.New(1003, "图片处理服务繁忙，请稍后重试", http.StatusServiceUnavailable))
			return
		}
		response.Error(c, errcode.ErrImgproxy)
		return
	}
	defer release()
	c.Header("Content-Type", generated.contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(generated.path)
}

func (h *PublicHandler) failV1StorageTarget(c *gin.Context, imageFile *model.ImageFile, cause error) {
	imageFileID := uint64(0)
	if imageFile != nil {
		imageFileID = imageFile.ID
	}
	log.Printf("[v1-storage] image_file=%d target unavailable: %v", imageFileID, cause)
	response.Error(c, errcode.New(1003, "对象存储暂不可用", http.StatusServiceUnavailable))
}

func (h *PublicHandler) serveV1RemoteObject(c *gin.Context, imageFile *model.ImageFile, target service.V1StorageTarget, key, contentType string, redirectPublic bool) bool {
	if imageFile == nil {
		h.failV1StorageTarget(c, imageFile, errors.New("image file is missing"))
		return false
	}
	if redirectPublic {
		remoteURL, err := h.imageSvc.R2PublicURLForTarget(key, target.Endpoint, target.Bucket)
		if err != nil || remoteURL == "" {
			if err == nil {
				err = errors.New("remote storage returned an empty public URL")
			}
			h.failV1StorageTarget(c, imageFile, err)
			return false
		}
		c.Redirect(http.StatusFound, remoteURL)
		return true
	}
	reader, err := h.imageSvc.R2DownloadForTarget(c.Request.Context(), key, target.Endpoint, target.Bucket)
	if err != nil {
		h.failV1StorageTarget(c, imageFile, err)
		return false
	}
	defer reader.Close()
	applyImageCacheHeaders(c, true)
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, reader)
	return true
}

func (h *PublicHandler) ServeVariant(c *gin.Context) {
	uniqueLink, _ := parseLink(c.Param("link"))
	image, _, ok := h.authorizedImage(c, uniqueLink)
	if !ok {
		return
	}
	if image.PipelineVersion < model.ImagePipelineVersionV2 {
		response.Error(c, errcode.New(4041, "固定图片变体不存在", 404))
		return
	}
	variantName := strings.TrimSuffix(strings.ToLower(c.Param("variant")), ".webp")
	switch variantName {
	case model.ImageVariantKindMaster, model.ImageVariantKindGallery, model.ImageVariantKindAdmin, model.ImageVariantKindPublish:
	default:
		response.Error(c, errcode.New(4041, "固定图片变体不存在", 404))
		return
	}
	restricted := isRestrictedV2Variant(variantName)
	if restricted {
		applyImageCacheHeaders(c, true)
	}
	if restricted && !h.isOwnerOrAdmin(c, image.UserID) {
		response.Error(c, errcode.New(4031, "无权访问此图片变体", http.StatusForbidden))
		return
	}
	if h.rdb != nil {
		_ = h.rdb.Incr(c.Request.Context(), fmt.Sprintf("views:%d", image.ID)).Err()
	}
	h.serveV2Variant(c, image, variantName)
}

func (h *PublicHandler) authorizedImage(c *gin.Context, link string) (*model.Image, bool, bool) {
	image, appErr := h.imageSvc.GetByUniqueLink(link)
	if appErr != nil {
		response.Error(c, appErr)
		return nil, false, false
	}
	isPrivate := image.Visibility == "private"
	if isPrivate || image.PipelineVersion < model.ImagePipelineVersionV2 {
		applyImageCacheHeaders(c, isPrivate)
	}
	if isPrivate && !h.isOwnerOrAdmin(c, image.UserID) {
		switch h.imageSvc.ValidateAccessToken(image.ID, extractToken(c)) {
		case service.TokenValid:
		case service.TokenRevoked:
			response.Error(c, errcode.ErrPrivateTokenRevoked)
			return nil, true, false
		default:
			response.Error(c, errcode.ErrPrivateTokenInvalid)
			return nil, true, false
		}
	}
	return image, isPrivate, true
}

func (h *PublicHandler) serveV2Variant(c *gin.Context, image *model.Image, kind string) {
	variant, appErr := h.imageSvc.GetActiveVariant(image.ID, kind)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	storedFile, err := openStorageFile(h.storageCfg.BasePath, variant.StoragePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			response.Error(c, errcode.New(4041, "图片文件不存在", 404))
		} else {
			response.Error(c, errcode.ErrInternal)
		}
		return
	}
	defer storedFile.file.Close()
	applyV2ImageCacheHeaders(c, image.Visibility == "private" || isRestrictedV2Variant(kind))
	c.Header("Content-Type", "image/webp")
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, storedFile.info.Name(), storedFile.info.ModTime(), storedFile.file)
}

func isRestrictedV2Variant(kind string) bool {
	return kind == model.ImageVariantKindMaster || kind == model.ImageVariantKindAdmin
}

func applyV2ImageCacheHeaders(c *gin.Context, private bool) {
	if private {
		applyImageCacheHeaders(c, true)
		return
	}
	// A V2 public alias can change when visibility changes. Keep every browser,
	// reverse-proxy, and CDN cache inside the accepted ten-minute privacy window
	// even when durable purge delivery is unavailable or temporarily failing.
	c.Header("Cache-Control", "public, max-age=600, s-maxage=600, must-revalidate")
	c.Header("X-Accel-Expires", "600")
	c.Header("Surrogate-Control", "max-age=600")
}

func applyImageCacheHeaders(c *gin.Context, private bool) {
	if private {
		// Private images: never cache — token validation must run on every request.
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Header("X-Accel-Expires", "0")
		c.Header("Surrogate-Control", "no-store")
		c.Header("Vary", "Accept, X-Image-Token, Authorization")
	} else {
		// V1 aliases may become private. Bound every cache layer to the accepted
		// privacy-change propagation window even if purge delivery is delayed.
		c.Header("Cache-Control", "public, max-age=600, s-maxage=600, must-revalidate")
		c.Header("X-Accel-Expires", "600")
		c.Header("Surrogate-Control", "max-age=600")
	}
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

func (h *PublicHandler) withWatermark(ctx context.Context, wm *service.WatermarkConfig, process func(*service.WatermarkConfig) error) error {
	if wm == nil || !wm.Enabled {
		return process(wm)
	}
	return service.WithCurrentWatermark(ctx, h.storageCfg.BasePath, func() error {
		return process(wm)
	})
}

func applyWatermarkToV1Path(path string, wm *service.WatermarkConfig) string {
	if wm == nil || !wm.Enabled {
		return path
	}
	return strings.Replace(path, "/plain/", fmt.Sprintf("/wm:%s:%s/plain/", wm.Opacity, wm.Position), 1)
}

func (h *PublicHandler) loadDynamicImage(waitCtx context.Context, path string) (generatedImageFile, func(), error) {
	wm := h.publicConfigService.GetWatermark()
	key := dynamicImageKey(path, wm)
	call, leader := h.dynamicImages.acquire(key)
	if leader {
		select {
		case h.dynamicCapacity <- struct{}{}:
			generationCtx, cancel := context.WithTimeout(context.Background(), v1GenerationTimeout)
			h.dynamicImages.setLifecycle(call, cancel, func() { <-h.dynamicCapacity })
			go h.runDynamicImageCall(generationCtx, cancel, key, call, path, wm)
		default:
			h.dynamicImages.finish(key, call, generatedImageFile{}, errDynamicImageQueueFull)
		}
	}

	select {
	case <-call.done:
		if call.err != nil {
			h.dynamicImages.release(key, call)
			return generatedImageFile{}, nil, call.err
		}
		return call.result, func() { h.dynamicImages.release(key, call) }, nil
	case <-waitCtx.Done():
		h.dynamicImages.release(key, call)
		return generatedImageFile{}, nil, waitCtx.Err()
	}
}

func dynamicImageKey(path string, wm *service.WatermarkConfig) string {
	if wm == nil || !wm.Enabled {
		return path + "\x00wm:off"
	}
	return path + "\x00wm:on:" + wm.Opacity + ":" + wm.Position
}

func (h *PublicHandler) runDynamicImageCall(ctx context.Context, cancel context.CancelFunc, key string, call *dynamicImageCall, path string, wm *service.WatermarkConfig) {
	var result generatedImageFile
	var generationErr error
	defer func() {
		if recovered := recover(); recovered != nil {
			generationErr = fmt.Errorf("dynamic image generation panic: %v", recovered)
		}
		cancel()
		h.dynamicImages.finish(key, call, result, generationErr)
	}()
	result, generationErr = h.generateDynamicImage(ctx, path, wm)
}

func (h *PublicHandler) generateDynamicImage(ctx context.Context, path string, wm *service.WatermarkConfig) (generatedImageFile, error) {
	select {
	case h.dynamicSemaphore <- struct{}{}:
		defer func() { <-h.dynamicSemaphore }()
	case <-ctx.Done():
		return generatedImageFile{}, ctx.Err()
	}

	if err := os.MkdirAll(h.storageCfg.TempPath, 0750); err != nil {
		return generatedImageFile{}, err
	}
	file, err := os.CreateTemp(h.storageCfg.TempPath, "v1-dynamic-*.image")
	if err != nil {
		return generatedImageFile{}, err
	}
	tempPath := file.Name()
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(tempPath)
		}
	}()
	if err := file.Chmod(0640); err != nil {
		return generatedImageFile{}, err
	}

	contentType := "application/octet-stream"
	err = h.withWatermark(ctx, wm, func(wm *service.WatermarkConfig) error {
		requestPath := applyWatermarkToV1Path(path, wm)
		signedPath := h.signer.SignPath(requestPath)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.imgproxyURL+signedPath, nil)
		if err != nil {
			return err
		}
		resp, err := h.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("imgproxy returned status %d", resp.StatusCode)
		}
		if value := resp.Header.Get("Content-Type"); strings.HasPrefix(value, "image/") {
			contentType = value
		}
		return copyGeneratedImage(file, resp.Body, v1DynamicMaximumResponseBytes)
	})
	if err != nil {
		return generatedImageFile{}, err
	}
	if err := file.Sync(); err != nil {
		return generatedImageFile{}, err
	}
	if err := file.Close(); err != nil {
		return generatedImageFile{}, err
	}
	keep = true
	return generatedImageFile{path: tempPath, contentType: contentType}, nil
}

func copyGeneratedImage(dst io.Writer, src io.Reader, maximum int64) error {
	written, err := io.Copy(dst, io.LimitReader(src, maximum+1))
	if err != nil {
		return err
	}
	if written > maximum {
		return errGeneratedImageTooLarge
	}
	return nil
}

type openedStorageFile struct {
	file         *os.File
	info         os.FileInfo
	relativePath string
}

func openStorageFile(basePath, storedPath string) (*openedStorageFile, error) {
	if basePath == "" || storedPath == "" || filepath.IsAbs(storedPath) {
		return nil, errUnsafeStoragePath
	}
	relativePath := filepath.Clean(filepath.FromSlash(storedPath))
	if relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return nil, errUnsafeStoragePath
	}

	absoluteBase, err := filepath.Abs(basePath)
	if err != nil {
		return nil, err
	}
	resolvedBase, err := filepath.EvalSymlinks(absoluteBase)
	if err != nil {
		return nil, err
	}
	resolvedPath, err := filepath.EvalSymlinks(filepath.Join(absoluteBase, relativePath))
	if err != nil {
		return nil, err
	}
	resolvedRelative, err := filepath.Rel(resolvedBase, resolvedPath)
	if err != nil || resolvedRelative == "." || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) {
		return nil, errUnsafeStoragePath
	}

	file, err := os.Open(resolvedPath)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errUnsafeStoragePath
	}
	return &openedStorageFile{file: file, info: info, relativePath: resolvedRelative}, nil
}

func (h *PublicHandler) triggerBackgroundFormat(imageFile *model.ImageFile, format string, quality int, source string) {
	if imageFile == nil || format != "avif" || source == "" {
		return
	}
	variantPath, ok := service.V1PersistentVariantPath(imageFile.ProcessedPath, format)
	if !ok {
		return
	}
	variantKey := strings.TrimSuffix(filepath.Base(variantPath), filepath.Ext(variantPath))
	key := variantPath
	if _, loaded := h.backgroundInFlight.LoadOrStore(key, true); loaded {
		return
	}
	select {
	case h.backgroundSemaphore <- struct{}{}:
	default:
		h.backgroundInFlight.Delete(key)
		return
	}
	go func() {
		defer h.backgroundInFlight.Delete(key)
		defer func() { <-h.backgroundSemaphore }()

		diskPath := filepath.Join(h.storageCfg.BasePath, variantPath)
		if _, err := os.Stat(diskPath); err == nil {
			return
		}

		processedDir := filepath.Dir(diskPath)
		if err := os.MkdirAll(processedDir, 0755); err != nil {
			log.Printf("[bg-%s] create output directory failed for %s: %v", format, variantKey, err)
			return
		}
		file, err := os.CreateTemp(processedDir, "."+format+"-*.part")
		if err != nil {
			log.Printf("[bg-%s] create temporary file failed for %s: %v", format, variantKey, err)
			return
		}
		tempPath := file.Name()
		keepTemp := true
		defer func() {
			_ = file.Close()
			if keepTemp {
				_ = os.Remove(tempPath)
			}
		}()
		if err := file.Chmod(0644); err != nil {
			log.Printf("[bg-%s] chmod temporary file failed for %s: %v", format, variantKey, err)
			return
		}

		path := fmt.Sprintf("/q:%d/f:%s/plain/%s", quality, format, source)
		wm := h.publicConfigService.GetWatermark()
		ctx, cancel := context.WithTimeout(context.Background(), v1GenerationTimeout)
		defer cancel()
		err = h.withWatermark(ctx, wm, func(wm *service.WatermarkConfig) error {
			requestPath := applyWatermarkToV1Path(path, wm)
			signedPath := h.signer.SignPath(requestPath)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.imgproxyURL+signedPath, nil)
			if err != nil {
				return err
			}
			resp, err := h.client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("imgproxy returned status %d", resp.StatusCode)
			}
			return copyGeneratedImage(file, resp.Body, v1BackgroundMaximumResponseBytes)
		})
		if err != nil {
			log.Printf("[bg-%s] imgproxy generation failed for %s: %v", format, variantKey, err)
			return
		}
		if err := file.Sync(); err != nil {
			log.Printf("[bg-%s] sync to disk failed for %s: %v", format, variantKey, err)
			return
		}
		if err := file.Close(); err != nil {
			log.Printf("[bg-%s] close temporary file failed for %s: %v", format, variantKey, err)
			return
		}
		if err := os.Rename(tempPath, diskPath); err != nil {
			log.Printf("[bg-%s] write to disk failed for %s: %v", format, variantKey, err)
			return
		}
		keepTemp = false
		if info, err := os.Stat(diskPath); err == nil {
			log.Printf("[bg-%s] generated %s (%d bytes)", format, diskPath, info.Size())
		}
	}()
}
