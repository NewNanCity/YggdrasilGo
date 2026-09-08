// Package blessing_skin BlessingSkin兼容存储实现
package blessing_skin

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"yggdrasil-api-go/src/sharedauth"
	storage "yggdrasil-api-go/src/storage/interface"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Storage BlessingSkin兼容存储
type Storage struct {
	db            *gorm.DB
	config        *Config
	textureConfig *TextureConfig // 全局材质配置
	uuidGen       *UUIDGenerator
	optionsMgr    *OptionsManager
	textureSigner *TextureSigner
}

// TextureConfig 材质配置（从全局配置传入）
type TextureConfig struct {
	BaseURL string // 材质基础URL
}

// Config BlessingSkin存储配置
type Config struct {
	DatabaseDSN            string // MySQL连接字符串
	Debug                  bool   // 调试模式
	TextureBaseURLOverride bool   // 为true时使用配置文件的texture.base_url而不是options中的site_url
	Salt                   string // 密码加密盐值 (对应BlessingSkin的SALT)
	PwdMethod              string // 密码加密方法 (对应BlessingSkin的PWD_METHOD)
	AppKey                 string // 应用密钥 (对应BlessingSkin的APP_KEY)
}

// NewStorage 创建BlessingSkin存储实例
func NewStorage(options map[string]any, textureConfig *TextureConfig) (storage.Storage, error) {
	// 解析配置
	cfg := &Config{}
	if dsn, ok := options["database_dsn"].(string); ok {
		cfg.DatabaseDSN = dsn
	} else {
		return nil, fmt.Errorf("database_dsn is required for blessing_skin storage")
	}

	if debug, ok := options["debug"].(bool); ok {
		cfg.Debug = debug
	}

	if textureBaseURLOverride, ok := options["texture_base_url_override"].(bool); ok {
		cfg.TextureBaseURLOverride = textureBaseURLOverride
	}

	// 解析安全配置
	if salt, ok := options["salt"].(string); ok {
		cfg.Salt = salt
	} else {
		cfg.Salt = "blessing_skin_salt" // 默认盐值
	}

	if pwdMethod, ok := options["pwd_method"].(string); ok {
		cfg.PwdMethod = pwdMethod
	} else {
		cfg.PwdMethod = "BCRYPT" // 默认加密方法
	}

	if appKey, ok := options["app_key"].(string); ok {
		cfg.AppKey = appKey
	} else {
		cfg.AppKey = "base64:your_app_key_here" // 默认应用密钥
	}

	// 连接数据库
	gormConfig := &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Silent), // 禁用GORM日志避免干扰
	}

	if cfg.Debug {
		gormConfig.Logger = logger.Default.LogMode(logger.Info)
	}

	db, err := gorm.Open(mysql.Open(cfg.DatabaseDSN), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	// 优化数据库连接池配置
	if err := optimizeDBConnection(db); err != nil {
		return nil, fmt.Errorf("failed to optimize database connection: %w", err)
	}

	// 使用传入的缓存实例

	// 创建存储实例
	storage := &Storage{
		db:            db,
		config:        cfg,
		textureConfig: textureConfig,
	}

	// 初始化组件
	storage.uuidGen = NewUUIDGenerator(storage)
	storage.optionsMgr = NewOptionsManager(storage)
	storage.textureSigner = NewTextureSigner(storage)

	// 配置管理器已在NewOptionsManager中初始化，无需重复调用

	preloadLegacyUUIDs := true
	if configured, exists := options["preload_legacy_uuids"]; exists {
		if value, ok := configured.(bool); ok {
			preloadLegacyUUIDs = value
		}
	}
	if preloadLegacyUUIDs {
		// Legacy mode still relies on name-based UUID mappings.
		if err := storage.preloadUUIDs(); err != nil {
			// 预热失败不影响启动，只记录警告
			fmt.Printf("⚠️  UUID cache preload failed: %v\n", err)
		}
	}

	return storage, nil
}

// Close 关闭存储连接
func (s *Storage) Close() error {
	if s.db != nil {
		sqlDB, err := s.db.DB()
		if err == nil {
			return sqlDB.Close()
		}
	}
	return nil
}

// Ping 检查存储连接
func (s *Storage) Ping() error {
	if s.db == nil {
		return fmt.Errorf("database not connected")
	}

	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// GetStorageType 获取存储类型
func (s *Storage) GetStorageType() string {
	return "blessing_skin"
}

// optimizeDBConnection 优化数据库连接池配置
func optimizeDBConnection(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// 根据生产环境需求配置连接池
	sqlDB.SetMaxOpenConns(100)                 // 最大连接数
	sqlDB.SetMaxIdleConns(10)                  // 最大空闲连接数
	sqlDB.SetConnMaxLifetime(time.Hour)        // 连接最大生存时间
	sqlDB.SetConnMaxIdleTime(10 * time.Minute) // 空闲连接最大时间

	fmt.Printf("🔧 Database connection pool optimized: MaxOpen=%d, MaxIdle=%d\n", 100, 10)
	return nil
}

// preloadUUIDs UUID缓存预热
func (s *Storage) preloadUUIDs() error {
	// 计算预热数量：min(500, maxCacheSize/2)
	maxCacheSize := s.uuidGen.cache.maxSize
	if maxCacheSize <= 0 {
		maxCacheSize = 1000 // 默认缓存大小
	}

	preloadSize := min(500, max(10, maxCacheSize/2)) // max(10, min(500, maxCacheSize/2))

	// 批量查询最常用的UUID映射（按ID排序，假设ID越小越常用）
	var mappings []UUIDMapping
	err := s.db.Table("uuid").
		Select("name, uuid").
		Order("id ASC").
		Limit(preloadSize).
		Find(&mappings).Error
	if err != nil {
		return fmt.Errorf("failed to preload UUIDs: %w", err)
	}

	// 批量添加到缓存
	preloadCount := 0
	for _, mapping := range mappings {
		s.uuidGen.cache.PutMapping(mapping.Name, mapping.UUID)
		preloadCount++
	}

	if preloadCount > 0 {
		fmt.Printf("🚀 UUID cache preloaded: %d mappings (max cache: %d)\n", preloadCount, maxCacheSize)
	}

	return nil
}

// GetDB 获取数据库实例（内部使用）
func (s *Storage) GetDB() *gorm.DB {
	return s.db
}

// SharedAuthDB exposes the same primary connection used for BlessingSkin data.
func (s *Storage) SharedAuthDB() (*sql.DB, error) {
	if s.db == nil {
		return nil, fmt.Errorf("blessing skin database is not connected")
	}
	return s.db.DB()
}

type sharedAccountPolicy struct {
	storage             *Storage
	requireVerification bool
}

func (p sharedAccountPolicy) Allowed(permission int, verified bool) bool {
	return permission != -1 && (!p.requireVerification || verified)
}

func (p sharedAccountPolicy) VerifyPassword(password, storedHash string) bool {
	return p.storage.verifyPassword(password, storedHash)
}

// SharedAuthPolicy adapts only explicitly supported BlessingSkin password rules.
func (s *Storage) SharedAuthPolicy(requireVerification bool) (sharedauth.AccountPolicy, error) {
	if s.config == nil {
		return nil, fmt.Errorf("blessing skin configuration is unavailable")
	}
	method := strings.ToUpper(s.config.PwdMethod)
	switch method {
	case "BCRYPT", "ARGON2I", "PHP_PASSWORD_HASH", "MD5", "SALTED2MD5", "SHA256", "SALTED2SHA256", "SHA512", "SALTED2SHA512":
		return sharedAccountPolicy{storage: s, requireVerification: requireVerification}, nil
	default:
		return nil, fmt.Errorf("unsupported BlessingSkin password method %q", s.config.PwdMethod)
	}
}

// GetUUIDGenerator 获取UUID生成器（内部使用）
func (s *Storage) GetUUIDGenerator() *UUIDGenerator {
	return s.uuidGen
}

// GetOptionsManager 获取配置管理器（内部使用）
func (s *Storage) GetOptionsManager() *OptionsManager {
	return s.optionsMgr
}

// GetTextureSigner 获取材质签名器（内部使用）
func (s *Storage) GetTextureSigner() *TextureSigner {
	return s.textureSigner
}

// GetSignatureKeyPair 获取签名用的密钥对（私钥和公钥）
func (s *Storage) GetSignatureKeyPair() (privateKey string, publicKey string, err error) {
	return s.textureSigner.GetSignatureKeyPair()
}
