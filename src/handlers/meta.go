// Package handlers 提供HTTP请求处理器
package handlers

import (
	"crypto/rsa"
	"fmt"
	"sync"
	"yggdrasil-api-go/src/config"
	storage "yggdrasil-api-go/src/storage/interface"
	"yggdrasil-api-go/src/utils"
	"yggdrasil-api-go/src/yggdrasil"

	"github.com/gin-gonic/gin"
)

// MetaHandler 元数据处理器
type MetaHandler struct {
	storage storage.Storage
	config  *config.Config
}

// NewMetaHandler 创建新的元数据处理器
func NewMetaHandler(storage storage.Storage, cfg *config.Config) *MetaHandler {
	return &MetaHandler{
		storage: storage,
		config:  cfg,
	}
}

// GetAPIMetadata 获取API元数据（启用响应缓存）
func (h *MetaHandler) GetAPIMetadata(c *gin.Context) {
	// 尝试从缓存获取响应
	cacheKey := "api_metadata_" + c.Request.Host
	if cached, exists := utils.GetCachedResponse(cacheKey); exists {
		c.Data(200, "application/json", cached)
		return
	}

	// 获取请求的Host头
	host := c.GetHeader("Host")
	if host == "" {
		host = c.Request.Host
	}

	// 动态生成链接
	links := make(map[string]string)
	for key := range h.config.Yggdrasil.Meta.Links {
		links[key] = h.config.GetLinkURL(key, host)
	}

	// 如果配置中没有基本链接，添加默认链接
	if _, exists := links["homepage"]; !exists {
		links["homepage"] = h.config.GetLinkURL("homepage", host)
	}
	if _, exists := links["register"]; !exists {
		links["register"] = h.config.GetLinkURL("register", host)
	}

	// 加载密钥对（只需要公钥用于API元数据）
	_, publicKey, err := h.loadSignatureKeyPair()
	if err != nil {
		utils.RespondError(c, 500, "InternalServerError", "Failed to load signature key pair")
		return
	}

	metadata := yggdrasil.APIMetadata{
		Meta: yggdrasil.MetaInfo{
			ServerName:            h.config.Yggdrasil.Meta.ServerName,
			ImplementationName:    h.config.Yggdrasil.Meta.ImplementationName,
			ImplementationVersion: h.config.Yggdrasil.Meta.ImplementationVersion,
			Links:                 links,
			FeatureNonEmailLogin:  h.config.Yggdrasil.Features.NonEmailLogin,
		},
		SkinDomains:        h.config.Yggdrasil.SkinDomains,
		SignaturePublicKey: publicKey,
	}

	// 使用高性能JSON响应并缓存结果
	if jsonData, err := utils.FastMarshal(metadata); err == nil {
		// 缓存响应（5分钟）
		utils.SetCachedResponse(cacheKey, jsonData)
		c.Data(200, "application/json", jsonData)
	} else {
		// 降级到标准JSON
		utils.RespondJSON(c, metadata)
	}
}

// 缓存的密钥对
var (
	cachedPrivateKey    string
	cachedPublicKey     string
	cachedRSAPrivateKey *rsa.PrivateKey
	cachedRSAPublicKey  *rsa.PublicKey
	keyPairCached       bool
	keyPairMutex        sync.RWMutex
)

// loadSignatureKeyPair 加载签名密钥对并缓存
func (h *MetaHandler) loadSignatureKeyPair() (privateKey string, publicKey string, err error) {
	// 先检查缓存
	keyPairMutex.RLock()
	if keyPairCached {
		defer keyPairMutex.RUnlock()
		return cachedPrivateKey, cachedPublicKey, nil
	}
	keyPairMutex.RUnlock()

	// 获取写锁进行加载
	keyPairMutex.Lock()
	defer keyPairMutex.Unlock()

	// 双重检查，防止并发加载
	if keyPairCached {
		return cachedPrivateKey, cachedPublicKey, nil
	}

	// 对于blessingskin存储，从options表读取密钥对
	if h.storage.GetStorageType() == "blessing_skin" {
		privateKey, publicKey, err = h.storage.GetSignatureKeyPair()
		if err != nil {
			return "", "", fmt.Errorf("failed to get signature key pair from storage: %w", err)
		}
	} else {
		// 对于其他存储类型，从配置文件读取密钥对
		privateKey, publicKey, err = utils.LoadOrGenerateKeyPair(
			h.config.Yggdrasil.Keys.PrivateKeyPath,
			h.config.Yggdrasil.Keys.PublicKeyPath,
		)
		if err != nil {
			return "", "", fmt.Errorf("failed to load key pair from files: %w", err)
		}
	}

	// 解析并缓存RSA密钥对
	rsaPrivateKey, err := utils.ParsePrivateKey(privateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse private key: %w", err)
	}

	rsaPublicKey := &rsaPrivateKey.PublicKey

	// 缓存所有密钥信息
	cachedPrivateKey = privateKey
	cachedPublicKey = publicKey
	cachedRSAPrivateKey = rsaPrivateKey
	cachedRSAPublicKey = rsaPublicKey
	keyPairCached = true

	return privateKey, publicKey, nil
}

// GetCachedRSAKeyPair 获取缓存的RSA密钥对（高性能版本）
func GetCachedRSAKeyPair() (privateKey *rsa.PrivateKey, publicKey *rsa.PublicKey, err error) {
	keyPairMutex.RLock()
	defer keyPairMutex.RUnlock()

	if !keyPairCached {
		return nil, nil, fmt.Errorf("RSA key pair not cached, call loadSignatureKeyPair first")
	}

	return cachedRSAPrivateKey, cachedRSAPublicKey, nil
}
