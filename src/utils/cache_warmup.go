// Package utils 缓存预热工具
package utils

import (
	"fmt"
	"log"
	"os"
	"time"

	"yggdrasil-api-go/src/config"
	storage "yggdrasil-api-go/src/storage/interface"
	"yggdrasil-api-go/src/yggdrasil"
)

// CacheWarmupConfig 缓存预热配置
type CacheWarmupConfig struct {
	EnableAPIMetadata bool          // 是否预热API元数据
	EnableErrorCache  bool          // 是否预热错误响应
	UserCacheDuration time.Duration // 用户缓存持续时间
}

// WarmupCaches 预热所有缓存
func WarmupCaches(cfg *config.Config, store storage.Storage) error {
	log.Printf("🔥 开始缓存预热...")
	start := time.Now()

	// 检查响应缓存配置
	if !cfg.Cache.Response.Enabled {
		log.Printf("ℹ️  响应缓存已禁用，跳过预热")
		return nil
	}

	// 1. 预热错误响应缓存
	if cfg.Cache.Response.ErrorResponses {
		if err := warmupErrorResponses(); err != nil {
			log.Printf("⚠️  错误响应缓存预热失败: %v", err)
		} else {
			log.Printf("✅ 错误响应缓存预热完成")
		}
	} else {
		log.Printf("ℹ️  错误响应缓存已禁用")
	}

	// 2. 预热API元数据缓存
	if cfg.Cache.Response.APIMetadata {
		if err := warmupAPIMetadata(cfg, store); err != nil {
			log.Printf("⚠️  API元数据缓存预热失败: %v", err)
		} else {
			log.Printf("✅ API元数据缓存预热完成")
		}
	} else {
		log.Printf("ℹ️  API元数据缓存已禁用")
	}

	// 3. 预热UUID缓存（如果存储支持）
	if err := warmupUUIDCache(store); err != nil {
		log.Printf("⚠️  UUID缓存预热失败: %v", err)
	} else {
		log.Printf("✅ UUID缓存预热完成")
	}

	duration := time.Since(start)
	log.Printf("🎉 缓存预热完成，耗时: %v", duration)
	return nil
}

// warmupErrorResponses 预热错误响应缓存
func warmupErrorResponses() error {
	// 初始化错误响应缓存
	InitErrorResponseCache()
	return nil
}

// warmupAPIMetadata 预热API元数据缓存
func warmupAPIMetadata(cfg *config.Config, store storage.Storage) error {
	// 为常用的host预生成API元数据
	commonHosts := []string{
		"localhost:8080",
		"127.0.0.1:8080",
		cfg.Server.Host + ":" + fmt.Sprintf("%d", cfg.Server.Port),
	}

	for _, host := range commonHosts {
		// 构建链接
		links := make(map[string]string)
		for key := range cfg.Yggdrasil.Meta.Links {
			links[key] = cfg.GetLinkURL(key, host)
		}

		// 添加默认链接
		if _, exists := links["homepage"]; !exists {
			links["homepage"] = cfg.GetLinkURL("homepage", host)
		}
		if _, exists := links["register"]; !exists {
			links["register"] = cfg.GetLinkURL("register", host)
		}

		// 加载公钥
		var publicKey string
		var err error

		// 对于blessingskin存储，从options表读取密钥对
		if store.GetStorageType() == "blessing_skin" {
			_, publicKey, err = store.GetSignatureKeyPair()
		} else {
			// 对于其他存储类型，从配置文件读取公钥
			publicKey, err = loadPublicKey(cfg.Yggdrasil.Keys.PublicKeyPath)
		}

		if err != nil {
			log.Printf("⚠️  Failed to load public key for cache warmup: %v", err)
			publicKey = "" // 使用空字符串作为降级
		}

		// 构建元数据
		metadata := yggdrasil.APIMetadata{
			Meta: yggdrasil.MetaInfo{
				ServerName:            cfg.Yggdrasil.Meta.ServerName,
				ImplementationName:    cfg.Yggdrasil.Meta.ImplementationName,
				ImplementationVersion: cfg.Yggdrasil.Meta.ImplementationVersion,
				Links:                 links,
				FeatureNonEmailLogin:  cfg.Yggdrasil.Features.NonEmailLogin,
			},
			SkinDomains:        cfg.Yggdrasil.SkinDomains,
			SignaturePublicKey: publicKey,
		}

		// 序列化并缓存
		if jsonData, err := FastMarshal(metadata); err == nil {
			cacheKey := "api_metadata_" + host
			SetCachedResponse(cacheKey, jsonData)
		}
	}

	return nil
}

// warmupUUIDCache 预热UUID缓存
func warmupUUIDCache(_ storage.Storage) error {
	// 这个功能已经在storage层实现了
	// 这里只是确认预热完成
	return nil
}

// loadPublicKey 加载公钥文件
func loadPublicKey(publicKeyPath string) (string, error) {
	data, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read public key file: %w", err)
	}
	return string(data), nil
}

// GetCacheStats 获取所有缓存统计信息
func GetCacheStats() map[string]any {
	stats := make(map[string]any)

	// 全局性能指标
	stats["performance"] = GlobalMetrics.GetStats()

	// 响应缓存统计
	responseCount := 0
	responseCache.Range(func(key, value any) bool {
		responseCount++
		return true
	})
	stats["response_cache"] = map[string]any{
		"cached_responses": responseCount,
	}

	// 错误响应缓存统计
	stats["error_cache"] = map[string]any{
		"cached_errors": len(cachedErrorResponses),
	}

	return stats
}

// PrintCacheStats 打印缓存统计信息
func PrintCacheStats() {
	stats := GetCacheStats()

	fmt.Printf("\n📊 Cache Statistics:\n")

	if perfStats, ok := stats["performance"].(map[string]any); ok {
		fmt.Printf("  Performance: QPS=%.2f, Cache Hit Rate=%.2f%%\n",
			GlobalMetrics.GetQPS(), perfStats["cache_hit_rate"])
	}

	if respStats, ok := stats["response_cache"].(map[string]any); ok {
		fmt.Printf("  Response Cache: %d cached responses\n", respStats["cached_responses"])
	}

	if errStats, ok := stats["error_cache"].(map[string]any); ok {
		fmt.Printf("  Error Cache: %d cached errors\n", errStats["cached_errors"])
	}
}
