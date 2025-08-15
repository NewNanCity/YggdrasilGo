// Package handlers 材质处理器
package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	storage "yggdrasil-api-go/src/storage/interface"
	"yggdrasil-api-go/src/utils"

	"github.com/gin-gonic/gin"
)

// TextureHandler 材质处理器
type TextureHandler struct {
	storage storage.Storage
}

// NewTextureHandler 创建新的材质处理器
func NewTextureHandler(storage storage.Storage) *TextureHandler {
	return &TextureHandler{
		storage: storage,
	}
}

// UploadTexture 通用材质上传 (符合Yggdrasil规范)
func (h *TextureHandler) UploadTexture(c *gin.Context) {
	uuid := c.Param("uuid")
	textureType := c.Param("textureType")

	// 验证材质类型
	var storageTextureType storage.TextureType
	switch strings.ToLower(textureType) {
	case "skin":
		storageTextureType = storage.TextureTypeSkin
	case "cape":
		storageTextureType = storage.TextureTypeCape
	default:
		utils.RespondError(c, 400, "BadRequest", "Invalid texture type")
		return
	}

	// 检查是否支持上传
	if !h.storage.IsUploadSupported() {
		utils.RespondError(c, 501, "NotImplemented", "Texture upload not supported")
		return
	}

	// 获取上传的文件
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		utils.RespondError(c, 400, "BadRequest", "No file uploaded")
		return
	}
	defer file.Close()

	// 读取文件内容
	data, err := io.ReadAll(file)
	if err != nil {
		utils.RespondError(c, 500, "InternalServerError", "Failed to read file")
		return
	}

	// 验证文件大小
	if len(data) > 1024*1024 { // 1MB限制
		utils.RespondError(c, 413, "PayloadTooLarge", "File too large")
		return
	}

	// 创建材质元数据
	metadata := &storage.TextureMetadata{
		FileSize:   int64(len(data)),
		UploadedAt: time.Now(),
		// Hash 将在存储层计算
	}

	// 上传材质
	textureInfo, err := h.storage.UploadTexture(storageTextureType, uuid, data, metadata)
	if err != nil {
		utils.RespondError(c, 500, "InternalServerError", fmt.Sprintf("Failed to upload texture: %v", err))
		return
	}

	// 返回成功响应
	response := gin.H{
		"success": true,
		"texture": gin.H{
			"type": textureInfo.Type,
			"url":  textureInfo.URL,
			"hash": textureInfo.Metadata.Hash,
		},
	}
	utils.RespondJSONFast(c, response)
}

// UploadSkin 上传皮肤
func (h *TextureHandler) UploadSkin(c *gin.Context) {
	h.uploadTexture(c, storage.TextureTypeSkin)
}

// UploadCape 上传披风
func (h *TextureHandler) UploadCape(c *gin.Context) {
	h.uploadTexture(c, storage.TextureTypeCape)
}

// uploadTexture 通用材质上传处理
func (h *TextureHandler) uploadTexture(c *gin.Context, textureType storage.TextureType) {
	// 检查是否支持上传
	if !h.storage.IsUploadSupported() {
		utils.RespondError(c, 501, "NotImplemented", "Texture upload is not supported")
		return
	}

	// 获取玩家UUID
	playerUUID := c.Param("uuid")
	if playerUUID == "" {
		utils.RespondError(c, 400, "BadRequest", "Player UUID is required")
		return
	}

	// 验证UUID格式
	if !utils.IsValidUUID(playerUUID) {
		utils.RespondError(c, 400, "BadRequest", "Invalid UUID format")
		return
	}

	// 获取上传的文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		utils.RespondError(c, 400, "BadRequest", "No file uploaded")
		return
	}
	defer file.Close()

	// 检查文件大小
	if header.Size > 1024*1024 { // 1MB限制
		utils.RespondError(c, 413, "PayloadTooLarge", "File too large")
		return
	}

	// 检查文件类型
	contentType := header.Header.Get("Content-Type")
	if !isAllowedContentType(contentType) {
		utils.RespondError(c, 415, "UnsupportedMediaType", "Unsupported file type")
		return
	}

	// 读取文件数据
	data, err := io.ReadAll(file)
	if err != nil {
		utils.RespondError(c, 500, "InternalServerError", "Failed to read file")
		return
	}

	// 创建材质元数据
	metadata := &storage.TextureMetadata{
		UploadedAt: time.Now(),
		FileSize:   header.Size,
		Hash:       utils.CalculateHash(data),
	}

	// 如果是皮肤，检查模型类型
	if textureType == storage.TextureTypeSkin {
		model := c.PostForm("model")
		if model == "alex" || model == "slim" {
			metadata.Model = model
			metadata.Slim = true
		} else {
			metadata.Model = "steve"
			metadata.Slim = false
		}
	}

	// 上传材质
	textureInfo, err := h.storage.UploadTexture(textureType, playerUUID, data, metadata)
	if err != nil {
		utils.RespondError(c, 500, "InternalServerError", fmt.Sprintf("Failed to upload texture: %v", err))
		return
	}

	// 返回成功响应
	utils.RespondJSON(c, gin.H{
		"success": true,
		"texture": textureInfo,
	})
}

// GetTexture 获取材质
func (h *TextureHandler) GetTexture(c *gin.Context) {
	textureType := storage.TextureType(strings.ToUpper(c.Param("type")))
	playerUUID := c.Param("uuid")

	// 验证参数
	if textureType != storage.TextureTypeSkin && textureType != storage.TextureTypeCape {
		utils.RespondError(c, 400, "BadRequest", "Invalid texture type")
		return
	}

	if !utils.IsValidUUID(playerUUID) {
		utils.RespondError(c, 400, "BadRequest", "Invalid UUID format")
		return
	}

	// 获取材质信息
	textureInfo, err := h.storage.GetTexture(textureType, playerUUID)
	if err != nil {
		utils.RespondError(c, 404, "NotFound", "Texture not found")
		return
	}

	// 重定向到材质URL
	c.Redirect(http.StatusFound, textureInfo.URL)
}

// DeleteTexture 删除材质
func (h *TextureHandler) DeleteTexture(c *gin.Context) {
	// 检查是否支持上传（删除也需要上传功能）
	if !h.storage.IsUploadSupported() {
		utils.RespondError(c, 501, "NotImplemented", "Texture management is not supported")
		return
	}

	textureType := storage.TextureType(strings.ToUpper(c.Param("type")))
	playerUUID := c.Param("uuid")

	// 验证参数
	if textureType != storage.TextureTypeSkin && textureType != storage.TextureTypeCape {
		utils.RespondError(c, 400, "BadRequest", "Invalid texture type")
		return
	}

	if !utils.IsValidUUID(playerUUID) {
		utils.RespondError(c, 400, "BadRequest", "Invalid UUID format")
		return
	}

	// 删除材质
	err := h.storage.DeleteTexture(textureType, playerUUID)
	if err != nil {
		utils.RespondError(c, 500, "InternalServerError", fmt.Sprintf("Failed to delete texture: %v", err))
		return
	}

	// 返回成功响应
	utils.RespondJSON(c, gin.H{
		"success": true,
		"message": "Texture deleted successfully",
	})
}

// isAllowedContentType 检查是否为允许的文件类型
func isAllowedContentType(contentType string) bool {
	allowedTypes := []string{
		"image/png",
		"image/jpeg",
		"image/jpg",
	}

	for _, allowed := range allowedTypes {
		if contentType == allowed {
			return true
		}
	}
	return false
}
