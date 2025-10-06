// Package database 数据库Session缓存实现
package database

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"yggdrasil-api-go/src/yggdrasil"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// CacheSession 数据库缓存Session表结构（优化设计）
type CacheSession struct {
	// 主键：服务器ID
	ServerID string `gorm:"primaryKey;column:server_id;size:255" json:"server_id"` // 服务器ID

	// Session信息（只存储必要信息）
	ClientIP    string `gorm:"size:45;column:client_ip;not null" json:"client_ip"`                          // 客户端IP
	AccessToken string `gorm:"size:512;column:access_token;not null;default:''" json:"access_token"` // AccessToken（验证用）
	ProfileID   string `gorm:"size:50;column:profile_id;not null;default:''" json:"profile_id"`                         // 角色ID（冗余字段，暂不使用）

	// 时间信息
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	ExpiresAt time.Time `gorm:"index;column:expires_at;not null" json:"expires_at"`

	// 用于动态表名
	tablePrefix string `gorm:"-"`
}

// TableName 指定表名（支持前缀）
func (cs CacheSession) TableName() string {
	if cs.tablePrefix != "" {
		return cs.tablePrefix + "sessions"
	}
	return "cache_sessions"
}

// SessionCache 数据库Session缓存
type SessionCache struct {
	db          *gorm.DB
	tablePrefix string
	mu          sync.RWMutex
}

// NewSessionCache 创建数据库Session缓存
func NewSessionCache(options map[string]any) (*SessionCache, error) {
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

	// 创建SessionCache实例
	cache := &SessionCache{
		db:          db,
		tablePrefix: tablePrefix,
	}

	// 创建带前缀的表结构实例用于迁移
	sessionModel := cache.newCacheSession()

	// 使用Table()方法指定表名进行迁移
	tableName := sessionModel.TableName()
	if err := db.Table(tableName).AutoMigrate(&CacheSession{}); err != nil {
		return nil, fmt.Errorf("failed to migrate %s table: %w", tableName, err)
	}

	// 注释：不启动内部清理，使用全局清理例程
	// cache.startCleanup()

	return cache, nil
}

// newCacheSession 创建带表前缀的CacheSession实例
func (c *SessionCache) newCacheSession() *CacheSession {
	return &CacheSession{tablePrefix: c.tablePrefix}
}

// Store 存储Session
func (c *SessionCache) Store(serverID string, session *yggdrasil.Session) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Session默认30秒过期（与Yggdrasil标准一致）
	expiresAt := time.Now().Add(30 * time.Second)

	// 存储到数据库（只存储必要信息，不存储AccessToken和ProfileID）
	cacheSession := c.newCacheSession()
	cacheSession.ServerID = serverID
	cacheSession.AccessToken = session.AccessToken
	cacheSession.ProfileID = session.ProfileID
	cacheSession.ClientIP = session.ClientIP
	cacheSession.CreatedAt = session.CreatedAt
	cacheSession.ExpiresAt = expiresAt

	// 使用Table()方法明确指定表名进行Save操作
	result := c.db.Table(cacheSession.TableName()).Save(cacheSession)
	if result.Error != nil {
		return fmt.Errorf("failed to store session: %w", result.Error)
	}

	return nil
}

// Get 获取Session（优化版：直接从数据库字段构建Session对象）
func (c *SessionCache) Get(serverID string) (*yggdrasil.Session, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cacheSession := c.newCacheSession()
	result := c.db.Table(cacheSession.TableName()).Where("server_id = ? AND expires_at > ?", serverID, time.Now()).First(cacheSession)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("failed to get session: %w", result.Error)
	}

	// 直接从数据库字段构建Session对象（不需要反序列化）
	session := &yggdrasil.Session{
		ServerID:    cacheSession.ServerID,
		AccessToken: cacheSession.AccessToken,
		ProfileID:   cacheSession.ProfileID,
		ClientIP:    cacheSession.ClientIP,
		CreatedAt:   cacheSession.CreatedAt,
	}

	return session, nil
}

// Delete 删除Session（优化版：直接按ServerID删除）
func (c *SessionCache) Delete(serverID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cacheSession := c.newCacheSession()
	result := c.db.Table(cacheSession.TableName()).Where("server_id = ?", serverID).Delete(&CacheSession{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete session: %w", result.Error)
	}

	return nil
}

// CleanupExpired 清理过期Session
func (c *SessionCache) CleanupExpired() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cacheSession := c.newCacheSession()
	result := c.db.Table(cacheSession.TableName()).Where("expires_at <= ?", time.Now()).Delete(cacheSession)
	if result.Error != nil {
		return fmt.Errorf("failed to cleanup expired sessions: %w", result.Error)
	}

	return nil
}

// Close 关闭缓存连接
func (c *SessionCache) Close() error {
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
func (c *SessionCache) GetCacheType() string {
	return "database"
}
