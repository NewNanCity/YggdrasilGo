// Package database 数据库Token缓存实现
package database

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"yggdrasil-api-go/src/utils"
	"yggdrasil-api-go/src/yggdrasil"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// CacheToken 数据库缓存Token表结构（优化设计）
type CacheToken struct {
	// 复合主键：用户ID + TokenID（从JWT中提取）
	UserID  string `gorm:"primaryKey;column:user_id;size:50" json:"user_id"`  // 用户ID（JWT.sub）
	TokenID string `gorm:"primaryKey;column:token_id;size:50" json:"token_id"` // TokenID（JWT.yggt）

	// Token信息
	ClientToken string `gorm:"column:client_token;size:255" json:"client_token"` // ClientToken（验证用）
    ProfileID   string `gorm:"column:profile_id;size:50" json:"profile_id"`   // ProfileID（从JWT中提取）

	// 时间信息
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	ExpiresAt time.Time `gorm:"index;column:expires_at;not null" json:"expires_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`

	// 用于动态表名
	tablePrefix string `gorm:"-"`
}

// TableName 指定表名（支持前缀）
func (ct CacheToken) TableName() string {
	if ct.tablePrefix != "" {
		return ct.tablePrefix + "tokens"
	}
	return "cache_tokens"
}

// TokenCache 数据库Token缓存
type TokenCache struct {
	db          *gorm.DB
	tablePrefix string
	mu          sync.RWMutex
}

// NewTokenCache 创建数据库Token缓存
func NewTokenCache(options map[string]any) (*TokenCache, error) {
	dsn, ok := options["dsn"].(string)
	if !ok || dsn == "" {
		return nil, fmt.Errorf("database DSN is required")
	}

	// 获取表前缀
	tablePrefix, _ := options["table_prefix"].(string)

	// 获取debug配置，默认为false
	debug, _ := options["debug"].(bool)

	// 确定日志级别
	logLevel := logger.Silent
	if debug {
		logLevel = logger.Info
	}

	// 根据DSN自动选择数据库驱动
	var db *gorm.DB
	var err error

	if strings.HasPrefix(dsn, "file:") || strings.HasSuffix(dsn, ".db") {
		// SQLite数据库
		db, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logLevel),
		})
	} else {
		// MySQL数据库
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logLevel),
		})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// 创建TokenCache实例
	cache := &TokenCache{
		db:          db,
		tablePrefix: tablePrefix,
	}

	// 创建带前缀的表结构实例用于迁移
	tokenModel := cache.newCacheToken()

	// 使用Table()方法指定表名进行迁移
	tableName := tokenModel.TableName()
	if err := db.Table(tableName).AutoMigrate(&CacheToken{}); err != nil {
		return nil, fmt.Errorf("failed to migrate %s table: %w", tableName, err)
	}

	// 注释：不启动内部清理，使用全局清理例程
	// cache.startCleanup()

	return cache, nil
}

// newCacheToken 创建带表前缀的CacheToken实例
func (c *TokenCache) newCacheToken() *CacheToken {
	return &CacheToken{tablePrefix: c.tablePrefix}
}

// Store 存储Token（优化版：先验证JWT，提取信息）
func (c *TokenCache) Store(token *yggdrasil.Token) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 第一步：验证JWT并提取信息
	claims, err := utils.ValidateJWT(token.AccessToken)
	if err != nil {
		return fmt.Errorf("invalid JWT token: %w", err)
	}

	// 存储到数据库（只存储JWT中没有的信息）
	cacheToken := c.newCacheToken()
	cacheToken.UserID = claims.UserID   // 从JWT中获取用户ID
	cacheToken.TokenID = claims.TokenID // 从JWT中获取TokenID
	cacheToken.ClientToken = token.ClientToken
	cacheToken.ProfileID = token.ProfileID
	cacheToken.CreatedAt = token.CreatedAt
	cacheToken.ExpiresAt = token.ExpiresAt
	cacheToken.UpdatedAt = time.Now()

	// 使用Table()方法明确指定表名进行Save操作
	tableName := cacheToken.TableName()
	result := c.db.Table(tableName).Save(cacheToken)
	if result.Error != nil {
		return fmt.Errorf("failed to store token: %w", result.Error)
	}

	return nil
}

// Get 获取Token（优化版：先验证JWT，按需查询数据库）
func (c *TokenCache) Get(accessToken string) (*yggdrasil.Token, error) {
	// 第一步：验证JWT（本地计算，极快）
	claims, err := utils.ValidateJWT(accessToken)
	if err != nil {
		return nil, fmt.Errorf("invalid JWT token: %w", err)
	}

	// 第二步：从数据库获取ClientToken等额外信息
	c.mu.RLock()
	defer c.mu.RUnlock()

	cacheToken := c.newCacheToken()
	result := c.db.Table(cacheToken.TableName()).Where("user_id = ? AND token_id = ? AND expires_at > ?",
		claims.UserID, claims.TokenID, time.Now()).First(cacheToken)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("token not found in cache")
		}
		return nil, fmt.Errorf("failed to get token: %w", result.Error)
	}

	// 构建Token对象（结合JWT信息和数据库信息）
	token := &yggdrasil.Token{
		AccessToken: accessToken,
		ClientToken: cacheToken.ClientToken,
		ProfileID:   claims.ProfileID,
		Owner:       claims.UserID, // 注意：这里应该是用户ID，不是邮箱
		CreatedAt:   cacheToken.CreatedAt,
		ExpiresAt:   cacheToken.ExpiresAt,
	}

	return token, nil
}

// Delete 删除Token（优化版：先验证JWT，提取用户ID和TokenID）
func (c *TokenCache) Delete(accessToken string) error {
	// 先验证JWT并提取信息
	claims, err := utils.ValidateJWT(accessToken)
	if err != nil {
		// JWT无效，但仍然尝试删除（兼容性）
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cacheToken := c.newCacheToken()
	result := c.db.Table(cacheToken.TableName()).Where("user_id = ? AND token_id = ?",
		claims.UserID, claims.TokenID).Delete(cacheToken)
	if result.Error != nil {
		return fmt.Errorf("failed to delete token: %w", result.Error)
	}

	return nil
}

// GetUserTokens 获取用户的所有Token（按用户ID查询）
func (c *TokenCache) GetUserTokens(userID string) ([]*yggdrasil.Token, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cacheToken := c.newCacheToken()
	var cacheTokens []CacheToken
	result := c.db.Table(cacheToken.TableName()).Where("user_id = ? AND expires_at > ?", userID, time.Now()).Find(&cacheTokens)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get user tokens: %w", result.Error)
	}

	var tokens []*yggdrasil.Token
	for _, ct := range cacheTokens {
		// 由于我们没有存储完整的AccessToken，这里需要重新构建
		// 但实际上，GetUserTokens主要用于删除用户的所有token，不需要完整的Token对象
		token := &yggdrasil.Token{
			AccessToken: "", // 不需要完整的AccessToken
			ClientToken: ct.ClientToken,
			ProfileID:   ct.ProfileID,
			Owner:       ct.UserID,
			CreatedAt:   ct.CreatedAt,
			ExpiresAt:   ct.ExpiresAt,
		}
		tokens = append(tokens, token)
	}

	return tokens, nil
}

// DeleteUserTokens 删除用户的所有Token（按用户ID）
func (c *TokenCache) DeleteUserTokens(userID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cacheToken := c.newCacheToken()
	result := c.db.Table(cacheToken.TableName()).Where("user_id = ?", userID).Delete(&CacheToken{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete user tokens: %w", result.Error)
	}

	return nil
}

// GetUserTokenCount 获取用户Token数量
func (c *TokenCache) GetUserTokenCount(userID string) (int, error) {
	tokens, err := c.GetUserTokens(userID)
	if err != nil {
		return 0, err
	}
	return len(tokens), nil
}

// CleanupExpired 清理过期Token
func (c *TokenCache) CleanupExpired() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cacheToken := c.newCacheToken()
	result := c.db.Table(cacheToken.TableName()).Where("expires_at <= ?", time.Now()).Delete(cacheToken)
	if result.Error != nil {
		return fmt.Errorf("failed to cleanup expired tokens: %w", result.Error)
	}

	return nil
}

// Close 关闭缓存连接
func (c *TokenCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.db != nil {
		sqlDB, err := c.db.DB()
		if err == nil {
			sqlDB.Close()
		}
	}

	return nil
}

// GetCacheType 获取缓存类型
func (c *TokenCache) GetCacheType() string {
	return "database"
}
