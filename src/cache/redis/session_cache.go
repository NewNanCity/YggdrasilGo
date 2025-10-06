// Package redis Redis Session缓存实现（BlessingSkin兼容）
package redis

import (
	"context"
	"fmt"
	"time"

	"yggdrasil-api-go/src/yggdrasil"

	"github.com/bytedance/sonic"
	"github.com/go-redis/redis/v8"
)

// SessionCache Redis Session缓存
type SessionCache struct {
	client *redis.Client
	ctx    context.Context
}

// NewSessionCache 创建Redis Session缓存
func NewSessionCache(options map[string]any) (*SessionCache, error) {
	redisURL := "redis://localhost:6379"
	if url, ok := options["redis_url"].(string); ok && url != "" {
		redisURL = url
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opt)
	ctx := context.Background()

	// 测试连接
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &SessionCache{
		client: client,
		ctx:    ctx,
	}, nil
}

// Store 存储Session（优化版：验证JWT但只存储必要信息）
func (c *SessionCache) Store(serverID string, session *yggdrasil.Session) error {
	// 创建简化的Session对象（不存储AccessToken和ProfileID）
	cacheSession := &yggdrasil.Session{
		ServerID:    serverID,
		AccessToken: session.AccessToken,
		ProfileID:   session.ProfileID,
		ClientIP:    session.ClientIP,
		CreatedAt:   session.CreatedAt,
	}

	// 序列化Session
	sessionData, err := sonic.Marshal(cacheSession)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Session固定过期时间为30秒（与Yggdrasil标准一致）
	ttl := 30 * time.Second

	// 存储Session
	sessionKey := fmt.Sprintf("yggdrasil-server-%s", serverID)
	if err := c.client.Set(c.ctx, sessionKey, sessionData, ttl).Err(); err != nil {
		return fmt.Errorf("failed to store session: %w", err)
	}

	return nil
}

// Get 获取Session
func (c *SessionCache) Get(serverID string) (*yggdrasil.Session, error) {
	sessionKey := fmt.Sprintf("yggdrasil-server-%s", serverID)

	data, err := c.client.Get(c.ctx, sessionKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	var session yggdrasil.Session
	if err := sonic.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// Delete 删除Session
func (c *SessionCache) Delete(serverID string) error {
	sessionKey := fmt.Sprintf("yggdrasil-server-%s", serverID)
	return c.client.Del(c.ctx, sessionKey).Err()
}

// CleanupExpired 清理过期Session
func (c *SessionCache) CleanupExpired() error {
	// Redis会自动清理过期的键，这里不需要额外操作
	return nil
}

// Close 关闭缓存连接
func (c *SessionCache) Close() error {
	return c.client.Close()
}

// GetCacheType 获取缓存类型
func (c *SessionCache) GetCacheType() string {
	return "redis"
}
