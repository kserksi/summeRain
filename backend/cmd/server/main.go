package main

import (
	"context"
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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/summerain/image-gallery/internal/config"
	"github.com/summerain/image-gallery/internal/handler"
	"github.com/summerain/image-gallery/internal/middleware"
	"github.com/summerain/image-gallery/internal/model"
	"github.com/summerain/image-gallery/internal/pkg/imgproxy"
	"github.com/summerain/image-gallery/internal/repository"
	"github.com/summerain/image-gallery/internal/service"
	"github.com/summerain/image-gallery/internal/worker"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	cfg := config.Load()

	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	repository.MigrateLegacyTokens(db)

	if err := db.AutoMigrate(
		&model.User{},
		&model.Session{},
		&model.CSRFToken{},
		&model.ImageFile{},
		&model.Image{},
		&model.ImageAccessToken{},
		&model.Notification{},
		&model.SystemConfig{},
		&model.UploadQueue{},
		&model.AuditLog{},
	); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	repository.SeedDefaultConfigs(db)

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
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

	imgproxySvc := service.NewImgproxyService(&cfg.Imgproxy)
	notificationSvc := service.NewNotificationService(notificationRepo)
	captchaVerifier := service.NewCaptchaVerifier(cfg.Captcha)
	authSvc := service.NewAuthService(userRepo, sessionRepo, rdb, captchaVerifier)
	imageSvc := service.NewImageService(db, rdb, imageRepo, imageFileRepo, tokenRepo, uploadQueueRepo, configRepo, imgproxySvc, notificationSvc, &cfg.Storage)
	userSvc := service.NewUserService(db, auditLogRepo, notificationSvc)
	adminSvc := service.NewAdminService(db, configRepo, notificationSvc, &cfg.Storage, rdb, imageRepo, imageFileRepo, imageSvc)
	publicConfigSvc := service.NewPublicConfigService(configRepo, cfg.Captcha, rdb)
	publicStatsSvc := service.NewPublicStatsService(db)

	signer := imgproxy.NewSigner(cfg.Imgproxy.Key, cfg.Imgproxy.Salt, cfg.Imgproxy.PublicURL)

	authMw := middleware.NewAuthMiddleware(sessionRepo, userRepo)
	csrfMw := middleware.NewCSRFMiddleware(sessionRepo)
	rateLimitMw := middleware.NewRateLimitMiddleware(rdb)

	authHandler := handler.NewAuthHandler(authSvc)
	imageHandler := handler.NewImageHandler(imageSvc)
	publicHandler := handler.NewPublicHandler(imageSvc, &cfg.Storage, rdb, signer, cfg.Imgproxy.BaseURL, publicConfigSvc, publicStatsSvc, authMw)
	userHandler := handler.NewUserHandler(userSvc)
	notificationHandler := handler.NewNotificationHandler(notificationSvc)
	adminHandler := handler.NewAdminHandler(adminSvc)

	gin.SetMode(cfg.Server.Mode)
	r := gin.New()

	if err := r.SetTrustedProxies([]string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"}); err != nil {
		log.Fatalf("failed to set trusted proxies: %v", err)
	}

	r.MaxMultipartMemory = 8 << 20

	r.Use(middleware.RequestID())
	r.Use(gin.Recovery())
	r.Use(middleware.SecurityHeaders())

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.GET("/ready", func(c *gin.Context) {
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

	api := r.Group("/api/v1")
	api.GET("/public/config", publicHandler.GetConfig)
	api.GET("/public/stats", publicHandler.GetStats)

	auth := api.Group("/auth")
	{
		auth.POST("/register", rateLimitMw.LoginLimit(), authHandler.Register)
		auth.POST("/login", rateLimitMw.LoginLimit(), authHandler.Login)
		auth.POST("/logout", authMw.Required(), csrfMw.Validate(), authHandler.Logout)
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
		images.POST("/", csrfMw.Validate(), imageHandler.Upload)
		images.GET("/:id", imageHandler.Get)
		images.DELETE("/:id", csrfMw.Validate(), imageHandler.Delete)
		images.PATCH("/:id/visibility", csrfMw.Validate(), imageHandler.ToggleVisibility)
		images.POST("/:id/tokens", csrfMw.Validate(), imageHandler.IssueToken)
		images.DELETE("/:id/tokens", csrfMw.Validate(), imageHandler.RevokeToken)
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
	}

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
	wm := worker.NewManager(db, rdb, cfg)
	go wm.Start(workerCtx)

	if configs, err := configRepo.FindAll(); err == nil {
		cfgMap := make(map[string]string)
		for _, c := range configs {
			cfgMap[c.ConfigKey] = c.ConfigValue
		}
		if changed, err := service.RegenerateWatermark(cfgMap, cfg.Storage.BasePath); err != nil {
			log.Printf("[WATERMARK] failed to generate SVG: %v", err)
		} else if changed {
			log.Println("[WATERMARK] SVG generated. Restart imgproxy to apply.")
		}
	}

	srv := &http.Server{
		Addr:    ":" + cfg.Server.Port,
		Handler: r,
	}

	go func() {
		log.Printf("server starting on :%s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")

	workerCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	log.Println("server exited")
}
