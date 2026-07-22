// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/handler"
	"github.com/kserksi/summerain/internal/middleware"
	"github.com/kserksi/summerain/internal/pkg/errcode"
	"github.com/kserksi/summerain/internal/pkg/imgproxy"
	"github.com/kserksi/summerain/internal/pkg/response"
	"github.com/kserksi/summerain/internal/repository"
	"github.com/kserksi/summerain/internal/service"
	"github.com/kserksi/summerain/internal/worker"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to configure database pool: %v", err)
	}
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)

	if err := repository.BootstrapDatabase(context.Background(), db); err != nil {
		log.Fatalf("failed to bootstrap database: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
		PoolSize: cfg.Redis.PoolSize,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("failed to connect redis: %v", err)
	}

	userRepo := repository.NewUserRepo(db)
	sessionRepo := repository.NewSessionRepo(db, rdb)
	imageRepo := repository.NewImageRepo(db)
	imageFileRepo := repository.NewImageFileRepo(db)
	tokenRepo := repository.NewImageAccessTokenRepo(db)
	uploadQueueRepo := repository.NewUploadQueueRepo(db)
	notificationRepo := repository.NewNotificationRepo(db)
	auditLogRepo := repository.NewAuditLogRepo(db)
	configRepo := repository.NewSystemConfigRepo(db)
	if storedCaptcha, findErr := configRepo.FindByKey("captcha_provider"); findErr == nil {
		if validateErr := config.ValidateCaptchaCrossOriginIsolation(storedCaptcha.ConfigValue, cfg.Server.CrossOriginIsolation); validateErr != nil {
			log.Fatalf("invalid stored CAPTCHA configuration: %v", validateErr)
		}
	} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
		log.Fatalf("failed to validate stored CAPTCHA configuration: %v", findErr)
	}

	imgproxySvc := service.NewImgproxyService(&cfg.Imgproxy)
	v2UploadSvc, err := service.NewV2UploadService(db, cfg, configRepo, imgproxySvc)
	if err != nil {
		log.Fatalf("failed to initialize V2 upload pipeline: %v", err)
	}
	notificationSvc := service.NewNotificationService(notificationRepo)
	captchaVerifier := service.NewCaptchaVerifier(cfg.Captcha)
	r2Svc := service.NewR2Service(configRepo)
	authSvc := service.NewAuthService(userRepo, sessionRepo, rdb, captchaVerifier, &service.ConfigRepoWrapper{Repo: configRepo})
	imageSvc := service.NewImageService(db, rdb, imageRepo, imageFileRepo, tokenRepo, uploadQueueRepo, configRepo, imgproxySvc, notificationSvc, &cfg.Storage, r2Svc)
	userSvc := service.NewUserService(db, auditLogRepo, notificationSvc)
	adminSvc := service.NewAdminService(db, configRepo, notificationSvc, &cfg.Storage, rdb, imageRepo, imageFileRepo, imageSvc, cfg.Server.CrossOriginIsolation)
	publicConfigSvc := service.NewPublicConfigService(configRepo, cfg.Captcha, rdb)
	publicStatsSvc := service.NewPublicStatsService(db)

	signer := imgproxy.NewSigner(cfg.Imgproxy.Key, cfg.Imgproxy.Salt, cfg.Imgproxy.PublicURL)

	authMw := middleware.NewAuthMiddleware(sessionRepo, userRepo)
	csrfMw := middleware.NewCSRFMiddleware(sessionRepo)
	rateLimitMw := middleware.NewRateLimitMiddleware(rdb)

	authHandler := handler.NewAuthHandler(authSvc)
	imageHandler := handler.NewImageHandler(imageSvc)
	v2UploadHandler := handler.NewV2UploadHandler(v2UploadSvc)
	publicHandler := handler.NewPublicHandler(imageSvc, &cfg.Storage, rdb, signer, cfg.Imgproxy.BaseURL, publicConfigSvc, publicStatsSvc, authMw)
	userHandler := handler.NewUserHandler(userSvc)
	notificationHandler := handler.NewNotificationHandler(notificationSvc)
	adminHandler := handler.NewAdminHandler(adminSvc)

	gin.SetMode(cfg.Server.Mode)
	r := gin.New()

	if err := r.SetTrustedProxies([]string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"}); err != nil {
		log.Fatalf("failed to set trusted proxies: %v", err)
	}

	r.MaxMultipartMemory = 8 << 20 // 8MB,跟前端单文件限制对齐,改的时候两边一起改

	r.Use(middleware.RequestID())
	r.Use(gin.Recovery())
	r.Use(middleware.SecurityHeaders(cfg.Server.CrossOriginIsolation))
	r.Use(middleware.LimitJSONBody(middleware.DefaultMaximumJSONBodyBytes))
	readiness := &readinessGate{}

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.GET("/ready", func(c *gin.Context) {
		if readiness.IsDraining() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "draining"})
			return
		}
		sqlDB, err := db.DB()
		if err != nil {
			c.JSON(503, gin.H{"status": "error", "detail": "db connection lost"})
			return
		}
		if err := sqlDB.Ping(); err != nil {
			c.JSON(503, gin.H{"status": "error", "detail": "db unreachable"})
			return
		}
		if err := rdb.Ping(c.Request.Context()).Err(); err != nil {
			c.JSON(503, gin.H{"status": "error", "detail": "redis unreachable"})
			return
		}
		c.JSON(200, gin.H{"status": "ready"})
	})

	r.GET("/i/:link", publicHandler.ServeImage)
	r.GET("/i/:link/:variant", publicHandler.ServeVariant)

	api := r.Group("/api/v1")
	api.GET("/public/config", publicHandler.GetConfig)
	api.GET("/public/stats", publicHandler.GetStats)

	auth := api.Group("/auth")
	{
		auth.POST("/register", rateLimitMw.LoginLimit(), authHandler.Register)
		auth.POST("/login", rateLimitMw.LoginLimit(), authHandler.Login)
		auth.POST("/logout", authMw.Required(), csrfMw.Validate(), authHandler.Logout)
		auth.POST("/csrf/refresh", authMw.Required(), csrfMw.RefreshGuard(), authHandler.RefreshCSRF)
		auth.GET("/me", authMw.Required(), authHandler.Me)
		auth.POST("/device-login", rateLimitMw.LoginLimit(), authHandler.DeviceLogin)
		auth.POST("/device-bootstrap", rateLimitMw.BootstrapLimit(), authMw.BootstrapAuth(), authHandler.DeviceBootstrap)
		auth.POST("/device-heartbeat", authMw.Required(), authHandler.DeviceHeartbeat)
		auth.DELETE("/device-shutdown", authMw.Required(), authHandler.DeviceShutdown)
		auth.GET("/device-identities", authMw.Required(), authHandler.ListDeviceIdentities)
		auth.DELETE("/device-identities/:id", authMw.Required(), csrfMw.Validate(), authHandler.RevokeIdentity)
		auth.GET("/sessions", authMw.Required(), authHandler.ListSessions)
		auth.DELETE("/sessions/:id", authMw.Required(), csrfMw.Validate(), authHandler.RevokeSession)
	}

	images := api.Group("/images", authMw.Required())
	{
		images.GET("/", imageHandler.List)
		if cfg.ImageV2.Enabled {
			images.POST("/", csrfMw.Validate(), func(c *gin.Context) {
				response.Error(c, errcode.ErrV2UploadRequired)
			})
		} else {
			images.POST("/", csrfMw.Validate(), imageHandler.Upload)
		}
		images.GET("/:id", imageHandler.Get)
		images.DELETE("/:id", csrfMw.Validate(), imageHandler.Delete)
		images.PATCH("/:id/visibility", csrfMw.Validate(), imageHandler.ToggleVisibility)
		images.POST("/:id/tokens", csrfMw.Validate(), imageHandler.IssueToken)
		images.DELETE("/:id/tokens", csrfMw.Validate(), imageHandler.RevokeToken)
	}

	uploads := api.Group("/uploads", authMw.Required())
	{
		uploads.GET("/recipe", v2UploadHandler.Recipe)
		uploads.POST("/", csrfMw.Validate(), v2UploadHandler.Init)
		uploads.POST("/status", csrfMw.Validate(), v2UploadHandler.BatchStatus)
		uploads.GET("/:uploadID", v2UploadHandler.Status)
		uploads.PUT("/:uploadID/parts/:kind", csrfMw.Validate(), v2UploadHandler.PutPart)
		uploads.POST("/:uploadID/complete", csrfMw.Validate(), v2UploadHandler.Complete)
		uploads.DELETE("/:uploadID", csrfMw.Validate(), v2UploadHandler.Cancel)
	}

	upload := api.Group("/upload", authMw.Required())
	{
		upload.GET("/queue/:id", imageHandler.GetUploadQueue)

		imagesAuth := api.Group("/images", authMw.Required())
		imagesAuth.GET("/batch-download", imageHandler.BatchDownload)
	}

	user := api.Group("/user", authMw.Required())
	{
		user.GET("/profile", userHandler.GetProfile)
		user.PATCH("/password", csrfMw.Validate(), userHandler.ChangePassword)
		user.PATCH("/avatar", csrfMw.Validate(), userHandler.UpdateAvatar)
	}

	notifications := api.Group("/notifications", authMw.Required())
	{
		notifications.GET("/", notificationHandler.List)
		notifications.PATCH("/:id/read", csrfMw.Validate(), notificationHandler.MarkRead)
		notifications.PATCH("/batch-read", csrfMw.Validate(), notificationHandler.MarkAllRead)
		notifications.DELETE("/:id", csrfMw.Validate(), notificationHandler.Delete)
		notifications.DELETE("/clear", csrfMw.Validate(), notificationHandler.ClearAll)
	}

	admin := api.Group("/admin", authMw.Required(), csrfMw.Validate(), adminHandler.RequireAdmin)
	{
		admin.GET("/users", adminHandler.ListUsers)
		admin.PATCH("/users/:id/status", adminHandler.SetUserStatus)
		admin.POST("/users/:id/request-deletion", adminHandler.RequestUserDeletion)
		admin.POST("/users/:id/cancel-deletion", adminHandler.CancelUserDeletion)
		admin.PATCH("/users/:id/quota", adminHandler.UpdateUserQuota)
		admin.GET("/configs", adminHandler.GetConfigs)
		admin.PATCH("/configs", adminHandler.UpdateConfigs)
		admin.GET("/stats", adminHandler.GetStats)
		admin.GET("/images", adminHandler.ListImages)
		admin.DELETE("/images/:id", adminHandler.DeleteImage)
		admin.POST("/r2/reload", func(c *gin.Context) {
			imageSvc.ReloadR2()
			response.Success(c, gin.H{"enabled": imageSvc.IsR2Enabled()})
		})
		admin.POST("/r2/test", func(c *gin.Context) {
			var req struct {
				Endpoint  string `json:"endpoint"`
				AccessKey string `json:"access_key"`
				SecretKey string `json:"secret_key"`
				Bucket    string `json:"bucket"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.Error(c, errcode.New(1001, "参数错误", 400))
				return
			}
			if req.Endpoint == "" || req.AccessKey == "" || req.SecretKey == "" || req.Bucket == "" {
				response.Error(c, errcode.New(1001, "请填写完整 R2 配置", 400))
				return
			}
			if err := service.TestR2Connection(req.Endpoint, req.AccessKey, req.SecretKey, req.Bucket); err != nil {
				response.Error(c, errcode.New(5001, err.Error(), 400))
				return
			}
			response.Success(c, gin.H{"ok": true})
		})
	}

	// TODO: 后续上了 cdn 这块静态服务可以挪到 nginx,Go 这边只管 API
	webRoot, err := filepath.Abs("./web")
	if err != nil {
		log.Fatalf("failed to resolve web root: %v", err)
	}
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") {
			c.JSON(404, gin.H{"code": 404, "message": "not found"})
			return
		}
		target := filepath.Join(webRoot, filepath.Clean("/"+path))
		if !strings.HasPrefix(target+string(filepath.Separator), webRoot+string(filepath.Separator)) {
			c.Data(http.StatusForbidden, "text/plain; charset=utf-8", []byte("forbidden"))
			return
		}
		if info, statErr := os.Stat(target); statErr == nil && !info.IsDir() {
			c.File(target)
			return
		}
		c.File(filepath.Join(webRoot, "index.html"))
	})

	workerCtx, workerCancel := context.WithCancel(context.Background())
	wm := worker.NewManager(db, rdb, cfg, v2UploadSvc, r2Svc)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		wm.Start(workerCtx)
	}()

	if configs, err := configRepo.FindAll(); err == nil {
		// 启动时重新生成水印 SVG,不然改了配置要重启 imgproxy 才生效,体验很差
		cfgMap := make(map[string]string)
		for _, c := range configs {
			cfgMap[c.ConfigKey] = c.ConfigValue
		}
		if changed, err := service.RegenerateWatermark(cfgMap, cfg.Storage.BasePath); err != nil {
			log.Printf("[WATERMARK] failed to generate SVG: %v", err)
		} else if changed {
			log.Println("[WATERMARK] SVG generated.")
		}
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Server.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Minute,
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	listenerErr := make(chan error, 1)
	go func() {
		log.Printf("server starting on :%s", cfg.Server.Port)
		if err := unexpectedHTTPServerError(srv.ListenAndServe()); err != nil {
			listenerErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	var triggerErr error
	select {
	case sig := <-quit:
		log.Printf("received %s, shutting down server...", sig)
	case triggerErr = <-listenerErr:
		log.Printf("HTTP listener stopped, shutting down server: %v", triggerErr)
	}
	signal.Stop(quit)

	coordinator := lifecycleCoordinator{
		gate:       readiness,
		server:     srv,
		workers:    wm,
		workerDone: workerDone,
		hardCancel: workerCancel,
		closeRedis: rdb.Close,
		closeSQL:   sqlDB.Close,
		timeouts:   productionLifecycleTimeouts(),
	}
	shutdownErr := coordinator.Shutdown(context.Background())
	shutdownErr = errors.Join(triggerErr, shutdownErr)
	if shutdownErr != nil {
		log.Printf("server stopped with error: %v", shutdownErr)
		os.Exit(1)
	}

	log.Println("server exited")
}
