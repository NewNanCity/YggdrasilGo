// Package main 是Yggdrasil API服务器的主程序入口
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"yggdrasil-api-go/src/cache"
	"yggdrasil-api-go/src/config"
	"yggdrasil-api-go/src/handlers"
	"yggdrasil-api-go/src/middleware"
	"yggdrasil-api-go/src/sharedauth"
	storage_factory "yggdrasil-api-go/src/storage"
	"yggdrasil-api-go/src/storage/blessing_skin"
	"yggdrasil-api-go/src/utils"

	"github.com/gin-gonic/gin"
)

func main() {
	// 解析命令行参数
	configPath := flag.String("config", path.Join("conf", "config.yml"), "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("✅ Loaded config from: %s", *configPath)

	// 确保密钥对存在（对于非BlessingSkin存储）
	if cfg.Storage.Type != "blessing_skin" {
		_, _, err = utils.LoadOrGenerateKeyPair(cfg.Yggdrasil.Keys.PrivateKeyPath, cfg.Yggdrasil.Keys.PublicKeyPath)
		if err != nil {
			log.Fatalf("Failed to load or generate key pair: %v", err)
		}
		log.Printf("✅ Loaded RSA key pair from %s and %s", cfg.Yggdrasil.Keys.PrivateKeyPath, cfg.Yggdrasil.Keys.PublicKeyPath)
	} else {
		log.Printf("✅ RSA key pair will be loaded from BlessingSkin database options table")
	}

	if cfg.Auth.Mode == "legacy" {
		utils.SetJWTSecret(cfg.Auth.JWTSecret)
	}

	// 创建存储实例
	storageFactory := storage_factory.NewStorageFactory()
	store, err := storageFactory.CreateStorage(&cfg.Storage, &cfg.Texture)
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	log.Printf("✅ Using %s storage", store.GetStorageType())

	mode := cfg.Auth.Mode
	var tokenCache cache.TokenCache
	var sessionCache cache.SessionCache
	if mode == "legacy" {
		cacheFactory := cache.NewCacheFactory()
		tokenCache, err = cacheFactory.CreateTokenCache(cfg.Cache.Token.Type, cfg.Cache.Token.Options)
		if err != nil {
			log.Fatalf("Failed to create token cache: %v", err)
		}
		sessionCache, err = cacheFactory.CreateSessionCache(cfg.Cache.Session.Type, cfg.Cache.Session.Options)
		if err != nil {
			log.Fatalf("Failed to create session cache: %v", err)
		}
		log.Printf("✅ Token cache initialized: %s", cfg.Cache.Token.Type)
		log.Printf("✅ Session cache initialized: %s", cfg.Cache.Session.Type)
	}

	// shared_mysql authorization must not preheat legacy name-to-UUID state.
	if mode == "legacy" && cfg.Cache.User.Enabled {
		cache.InitUserCache(cfg.Cache.User.Duration)
		log.Printf("✅ User cache initialized: %v duration", cfg.Cache.User.Duration)
	} else if mode == "legacy" {
		log.Printf("ℹ️  User cache disabled")
	}

	if mode == "legacy" {
		if err := utils.WarmupCaches(cfg, store); err != nil {
			log.Printf("⚠️  Cache warmup failed: %v", err)
		}
	}

	var sharedService *sharedauth.Service
	if mode == "shared_mysql" {
		blessingStore, ok := store.(*blessing_skin.Storage)
		if !ok {
			log.Fatal("auth.mode shared_mysql requires storage.type blessing_skin")
		}
		db, dbErr := blessingStore.SharedAuthDB()
		if dbErr != nil {
			log.Fatalf("Failed to access shared BlessingSkin database: %v", dbErr)
		}
		accountPolicy, policyErr := blessingStore.SharedAuthPolicy(cfg.Auth.RequireVerification)
		if policyErr != nil {
			log.Fatalf("Failed to configure BlessingSkin account policy: %v", policyErr)
		}
		sharedService, err = sharedauth.NewAuth(db, cfg.Security.ReadTimeout, cfg.Auth.TokenExpiration, accountPolicy)
		if err != nil {
			log.Fatalf("Failed to configure shared authentication: %v", err)
		}
		if err := sharedService.Ready(context.Background()); err != nil {
			log.Fatalf("Shared authentication is not ready: %v", err)
		}
		if _, err := sharedService.CleanupExpired(context.Background(), 1000); err != nil {
			log.Fatalf("Shared authentication cleanup check failed: %v", err)
		}
		log.Printf("✅ Shared MySQL authentication initialized")
	}

	// 创建处理器（直接传入存储和缓存）
	metaHandler := handlers.NewMetaHandler(store, cfg)
	authHandler := handlers.NewAuthHandler(store, tokenCache, sessionCache)
	sessionHandler := handlers.NewSessionHandler(store, tokenCache, sessionCache, cfg)
	if sharedService != nil {
		authHandler = handlers.NewSharedAuthHandler(store, tokenCache, sessionCache, sharedService, cfg.Auth)
		sessionHandler = handlers.NewSharedSessionHandler(store, tokenCache, sessionCache, cfg, sharedService)
	}
	profileHandler := handlers.NewProfileHandler(store, cfg)
	if sharedService != nil {
		profileHandler = handlers.NewSharedProfileHandler(store, cfg, sharedService)
	}
	textureHandler := handlers.NewTextureHandler(store)

	// 设置Gin模式
	gin.SetMode(gin.ReleaseMode)

	// 创建路由器
	router := gin.New()
	router.RemoveExtraSlash = true
	router.RedirectTrailingSlash = true
	if err := router.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		log.Fatalf("Failed to configure trusted proxies: %v", err)
	}

	// 添加中间件
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(middleware.LimitRequestBody(cfg.Security.MaxRequestBytes))
	router.Use(middleware.CORS())
	router.Use(middleware.PerformanceMonitor()) // 性能监控中间件

	// 根据配置决定是否使用基础路径
	var baseGroup *gin.RouterGroup
	if cfg.Server.BaseURL != "" {
		baseGroup = router.Group(cfg.Server.BaseURL)
	} else {
		baseGroup = router.Group("")
	}
	baseGroup.Use(middleware.APILocation(cfg))

	// API元数据端点
	baseGroup.GET("/", metaHandler.GetAPIMetadata)

	// 性能监控端点
	baseGroup.GET("/metrics", func(c *gin.Context) {
		stats := utils.GlobalMetrics.GetStats()
		utils.RespondJSONFast(c, stats)
	})

	// 认证服务器端点
	authGroup := baseGroup.Group("/authserver")
	authGroup.Use(middleware.CheckContentType())
	{
		// 需要速率限制的端点（如果启用）
		if cfg.Rate.Enabled {
			rateLimitedGroup := authGroup.Group("")
			rateLimitedGroup.Use(middleware.RateLimit(cfg.Rate.AuthInterval))
			{
				rateLimitedGroup.POST("/authenticate", authHandler.Authenticate)
				rateLimitedGroup.POST("/signout", authHandler.Signout)
			}
		} else {
			authGroup.POST("/authenticate", authHandler.Authenticate)
			authGroup.POST("/signout", authHandler.Signout)
		}

		// 其他认证端点
		authGroup.POST("/refresh", authHandler.Refresh)
		authGroup.POST("/validate", authHandler.Validate)
		authGroup.POST("/invalidate", authHandler.Invalidate)
	}

	// 会话服务器端点
	sessionGroup := baseGroup.Group("/sessionserver/session/minecraft")
	{
		sessionGroup.POST("/join", middleware.CheckContentType(), sessionHandler.Join)
		sessionGroup.GET("/hasJoined", sessionHandler.HasJoined)
		sessionGroup.GET("/profile/:uuid", profileHandler.GetProfileByUUID)
	}

	// API端点
	apiGroup := baseGroup.Group("/api")
	{
		apiGroup.POST("/profiles/minecraft", middleware.CheckContentType(), profileHandler.SearchMultipleProfiles)
		apiGroup.GET("/users/profiles/minecraft/:username", profileHandler.SearchSingleProfile)

		// 材质管理端点 (符合Yggdrasil规范)
		apiGroup.PUT("/user/profile/:uuid/:textureType", middleware.CheckContentType(), textureHandler.UploadTexture)
		apiGroup.DELETE("/user/profile/:uuid/:textureType", textureHandler.DeleteTexture)
	}

	// 启动清理协程（支持分布式部署）
	if mode == "legacy" {
		go startCleanupRoutines(tokenCache, sessionCache)
	} else {
		go startSharedCleanup(sharedService)
	}

	// 启动服务器
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	apiRoot := fmt.Sprintf("http://localhost:%d%s", cfg.Server.Port, cfg.Server.BaseURL)
	if cfg.Server.BaseURL == "" {
		apiRoot = fmt.Sprintf("http://localhost:%d/", cfg.Server.Port)
	} else if !strings.HasSuffix(apiRoot, "/") {
		apiRoot += "/"
	}

	log.Printf("🚀 Yggdrasil API Server starting on %s", addr)
	log.Printf("📖 API Documentation: http://localhost:%d", cfg.Server.Port)
	log.Printf("🔗 API Root: %s", apiRoot)
	if cfg.Server.BaseURL != "" {
		log.Printf("📍 Base URL: %s", cfg.Server.BaseURL)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: cfg.Security.ReadTimeout,
		ReadTimeout:       cfg.Security.ReadTimeout,
		WriteTimeout:      cfg.Security.WriteTimeout,
		IdleTimeout:       cfg.Security.IdleTimeout,
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func startSharedCleanup(service *sharedauth.Service) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		result, err := service.CleanupExpired(context.Background(), 1000)
		if err != nil {
			log.Printf("Shared authentication cleanup failed: %v", err)
			continue
		}
		if result.Tokens != 0 || result.Sessions != 0 {
			log.Printf("Shared authentication cleanup removed tokens=%d sessions=%d", result.Tokens, result.Sessions)
		}
	}
}

// startCleanupRoutines 启动 legacy 清理协程。
func startCleanupRoutines(tokenCache cache.TokenCache, sessionCache cache.SessionCache) {
	ticker := time.NewTicker(5 * time.Minute) // 每5分钟清理一次
	defer ticker.Stop()

	for range ticker.C {
		log.Println("🧹 Running cleanup routine...")

		// 清理过期Token
		if err := tokenCache.CleanupExpired(); err != nil {
			log.Printf("❌ Failed to cleanup expired tokens: %v", err)
		}

		// 清理过期Session
		if err := sessionCache.CleanupExpired(); err != nil {
			log.Printf("❌ Failed to cleanup expired sessions: %v", err)
		}

		log.Println("✅ Cleanup routine completed")
	}
}
