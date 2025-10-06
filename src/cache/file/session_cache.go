// Package file 文件Session缓存实现（BlessingSkin兼容）
package file

import (
	"fmt"
	"sync"
	"time"

	"yggdrasil-api-go/src/yggdrasil"
)

// SessionCache 文件Session缓存（Laravel兼容）
type SessionCache struct {
	cache *LaravelFileCache
	mu    sync.RWMutex
}

// NewSessionCache 创建文件Session缓存
func NewSessionCache(options map[string]any) (*SessionCache, error) {
	cacheDir := "storage/framework/cache"
	if dir, ok := options["cache_dir"].(string); ok && dir != "" {
		cacheDir = dir
	}

	return &SessionCache{
		cache: NewLaravelFileCache(cacheDir),
	}, nil
}

// Store 存储Session（优化版：验证JWT但只存储必要信息）
func (c *SessionCache) Store(serverID string, session *yggdrasil.Session) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Session固定过期时间为30秒（与Yggdrasil标准一致）
	ttl := 30 * time.Second

	// 创建简化的Session对象（不存储AccessToken和ProfileID）
	cacheSession := &yggdrasil.Session{
		ServerID:    serverID,
		AccessToken: session.AccessToken, // 仍然存储AccessToken以供验证
		ProfileID:   session.ProfileID,  // 仍然存储ProfileID以供验证
		ClientIP:    session.ClientIP,
		CreatedAt:   session.CreatedAt,
	}

	sessionKey := generateYggdrasilSessionKey(serverID)
	if err := c.cache.Store(sessionKey, cacheSession, ttl); err != nil {
		return fmt.Errorf("failed to store session: %w", err)
	}

	return nil
}

// Get 获取Session（优化版：直接从缓存字段构建Session对象）
func (c *SessionCache) Get(serverID string) (*yggdrasil.Session, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	sessionKey := generateYggdrasilSessionKey(serverID)

	var session yggdrasil.Session
	if err := c.cache.Get(sessionKey, &session); err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// 直接返回缓存的Session对象（已经是简化版）
	return &session, nil
}

// Delete 删除Session
func (c *SessionCache) Delete(serverID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sessionKey := generateYggdrasilSessionKey(serverID)
	return c.cache.Delete(sessionKey)
}

// CleanupExpired 清理过期Session
func (c *SessionCache) CleanupExpired() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.cache.CleanupExpired()
}

// Close 关闭缓存连接
func (c *SessionCache) Close() error {
	// 文件缓存无需关闭操作
	return nil
}

// GetCacheType 获取缓存类型
func (c *SessionCache) GetCacheType() string {
	return "file"
}
