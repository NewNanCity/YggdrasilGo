// Package storage 存储类型定义
package storage

import (
	"time"

	"yggdrasil-api-go/src/config"
	"yggdrasil-api-go/src/yggdrasil"
)

// UserStorage 用户存储接口
type UserStorage interface {
	// GetUserByEmail 根据邮箱获取用户
	GetUserByEmail(email string) (*yggdrasil.User, error)

	// GetUserByID 根据用户ID获取用户
	GetUserByID(userID string) (*yggdrasil.User, error)

	// GetUserByPlayerName 根据角色名获取用户
	GetUserByPlayerName(playerName string) (*yggdrasil.User, error)

	// GetUserByUUID 根据UUID获取用户
	GetUserByUUID(uuid string) (*yggdrasil.User, error)

	// AuthenticateUser 用户认证
	AuthenticateUser(username, password string) (*yggdrasil.User, error)
}

// ProfileStorage 角色存储接口
type ProfileStorage interface {
	// GetProfileByUUID 根据UUID获取角色
	GetProfileByUUID(uuid string) (*yggdrasil.Profile, error)

	// GetProfileByName 根据名称获取角色
	GetProfileByName(name string) (*yggdrasil.Profile, error)

	// GetProfilesByNames 根据名称列表批量获取角色
	GetProfilesByNames(names []string) ([]*yggdrasil.Profile, error)

	// GetProfilesByUserEmail 获取用户的所有角色
	GetProfilesByUserEmail(userEmail string) ([]*yggdrasil.Profile, error)

	// GetUserProfiles 根据用户UUID获取角色
	GetUserProfiles(userUUID string) ([]*yggdrasil.Profile, error)
}

// TextureStorage 材质存储接口
type TextureStorage interface {
	// UploadTexture 上传材质文件
	UploadTexture(textureType TextureType, playerUUID string, data []byte, metadata *TextureMetadata) (*TextureInfo, error)

	// GetTexture 获取材质文件
	GetTexture(textureType TextureType, playerUUID string) (*TextureInfo, error)

	// GetPlayerTextures 获取角色的所有材质
	GetPlayerTextures(playerUUID string) (map[TextureType]*TextureInfo, error)

	// DeleteTexture 删除材质文件
	DeleteTexture(textureType TextureType, playerUUID string) error

	// GetTextureURL 计算材质URL
	GetTextureURL(textureType TextureType, playerUUID string) string

	// IsUploadSupported 检查是否支持材质上传
	IsUploadSupported() bool
}

// TextureType 材质类型
type TextureType string

const (
	TextureTypeSkin TextureType = "SKIN"
	TextureTypeCape TextureType = "CAPE"
)

// TextureMetadata 材质元数据
type TextureMetadata struct {
	Model      string         `json:"model,omitempty"` // 皮肤模型（steve/alex）
	Slim       bool           `json:"slim,omitempty"`  // 是否为纤细模型
	UploadedAt time.Time      `json:"uploaded_at"`     // 上传时间
	FileSize   int64          `json:"file_size"`       // 文件大小
	Hash       string         `json:"hash"`            // 文件哈希
	Extra      map[string]any `json:"extra,omitempty"` // 额外信息
}

// TextureInfo 材质信息
type TextureInfo struct {
	Type     TextureType      `json:"type"`     // 材质类型
	URL      string           `json:"url"`      // 材质URL
	Metadata *TextureMetadata `json:"metadata"` // 材质元数据
}

// Storage 统一存储接口（只负责业务数据）
type Storage interface {
	UserStorage
	ProfileStorage
	TextureStorage

	// Close 关闭存储连接
	Close() error

	// Ping 检查存储连接
	Ping() error

	// GetStorageType 获取存储类型
	GetStorageType() string

	// GetSignatureKeyPair 获取签名用的密钥对（私钥和公钥）
	// 只有部分存储类型支持此方法，其他存储类型返回错误
	GetSignatureKeyPair() (privateKey string, publicKey string, err error)
}

// StorageFactory 存储工厂接口
type StorageFactory interface {
	// CreateStorage 创建存储实例
	CreateStorage(config *config.StorageConfig, textureConfig *config.TextureConfig) (Storage, error)

	// GetSupportedTypes 获取支持的存储类型
	GetSupportedTypes() []string
}
