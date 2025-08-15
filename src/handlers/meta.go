// Package handlers 提供HTTP请求处理器
package handlers

import (
	"fmt"
	"os"
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

	// 加载公钥
	publicKey, err := h.loadPublicKey()
	if err != nil {
		utils.RespondError(c, 500, "InternalServerError", "Failed to load public key")
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

// loadPublicKey 加载公钥
func (h *MetaHandler) loadPublicKey() (string, error) {
	// 对于blessingskin存储，从options表读取私钥并提取公钥
	if h.storage.GetStorageType() == "blessing_skin" {
		return h.storage.GetPublicKey()
	}

	// 对于其他存储类型，从配置文件读取公钥
	data, err := os.ReadFile(h.config.Yggdrasil.Keys.PublicKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read public key file: %w", err)
	}
	return string(data), nil
}
