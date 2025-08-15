// Package cache 缓存工厂实现
package cache

import (
	"fmt"

	"yggdrasil-api-go/src/cache/database"
	"yggdrasil-api-go/src/cache/file"
	"yggdrasil-api-go/src/cache/memory"
	"yggdrasil-api-go/src/cache/redis"
)

// DefaultCacheFactory 默认缓存工厂
type DefaultCacheFactory struct{}

// NewCacheFactory 创建缓存工厂
func NewCacheFactory() CacheFactory {
	return &DefaultCacheFactory{}
}

// CreateTokenCache 创建Token缓存实例
func (f *DefaultCacheFactory) CreateTokenCache(cacheType string, options map[string]any) (TokenCache, error) {
	switch cacheType {
	case "memory":
		return memory.NewTokenCache(options)
	case "redis":
		return redis.NewTokenCache(options)
	case "file":
		return file.NewTokenCache(options)
	case "database":
		return database.NewTokenCache(options)
	default:
		return nil, fmt.Errorf("unsupported token cache type: %s", cacheType)
	}
}

// CreateSessionCache 创建Session缓存实例
func (f *DefaultCacheFactory) CreateSessionCache(cacheType string, options map[string]any) (SessionCache, error) {
	switch cacheType {
	case "memory":
		return memory.NewSessionCache(options)
	case "redis":
		return redis.NewSessionCache(options)
	case "file":
		return file.NewSessionCache(options)
	case "database":
		return database.NewSessionCache(options)
	default:
		return nil, fmt.Errorf("unsupported session cache type: %s", cacheType)
	}
}

// GetSupportedTypes 获取支持的缓存类型
func (f *DefaultCacheFactory) GetSupportedTypes() []string {
	return []string{"memory", "redis", "file", "database"}
}
