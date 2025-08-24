package handlers

import (
	"fmt"
	"strconv"

	"yggdrasil-api-go/src/config"
	storage "yggdrasil-api-go/src/storage/interface"
	"yggdrasil-api-go/src/utils"

	"github.com/gin-gonic/gin"
)

// ProfileHandler 角色处理器
type ProfileHandler struct {
	storage storage.Storage
	config  *config.Config
}

// NewProfileHandler 创建新的角色处理器
func NewProfileHandler(storage storage.Storage, cfg *config.Config) *ProfileHandler {
	return &ProfileHandler{
		storage: storage,
		config:  cfg,
	}
}

// GetProfileByUUID 根据UUID获取角色档案
func (h *ProfileHandler) GetProfileByUUID(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		utils.RespondIllegalArgument(c, "Missing UUID parameter")
		return
	}

	// 获取unsigned参数，默认为true（不包含签名）
	unsigned := true
	if unsignedParam := c.Query("unsigned"); unsignedParam != "" {
		if parsed, err := strconv.ParseBool(unsignedParam); err == nil {
			unsigned = parsed
		}
	}

	// 获取角色信息
	profile, err := h.storage.GetProfileByUUID(uuid)
	if err != nil {
		// 角色不存在，返回204
		utils.RespondNoContent(c)
		return
	}

	// 处理签名逻辑
	if unsigned {
		// 如果unsigned为true，移除签名信息
		for i := range profile.Properties {
			profile.Properties[i].Signature = ""
		}
	} else {
		// 如果unsigned为false，检查是否需要生成签名
		for i := range profile.Properties {
			if profile.Properties[i].Signature == "" {
				// 生成签名
				signature, err := h.generateSignature(profile.Properties[i].Value)
				if err != nil {
					// 签名生成失败，记录错误但不影响响应
					// 可以选择返回错误或继续返回无签名的数据
					continue
				}
				profile.Properties[i].Signature = signature
			}
		}
	}

	utils.RespondJSONFast(c, profile)
}

// generateSignature 生成属性值的数字签名（高性能版本）
func (h *ProfileHandler) generateSignature(value string) (string, error) {
	// 尝试获取缓存的RSA密钥对
	rsaPrivateKey, _, err := GetCachedRSAKeyPair()
	if err != nil {
		// 如果缓存未命中，回退到传统方式
		privateKey, _, err := h.loadSignatureKeyPair()
		if err != nil {
			return "", fmt.Errorf("failed to load signature key pair: %w", err)
		}
		return utils.SignData(value, privateKey)
	}

	// 使用高性能签名函数（直接使用解析好的RSA密钥）
	return utils.SignDataWithRSAKey(value, rsaPrivateKey)
}

// loadSignatureKeyPair 加载签名密钥对
func (h *ProfileHandler) loadSignatureKeyPair() (privateKey string, publicKey string, err error) {
	// 对于blessingskin存储，从options表读取密钥对
	if h.storage.GetStorageType() == "blessing_skin" {
		return h.storage.GetSignatureKeyPair()
	}

	// 对于其他存储类型，从配置文件读取密钥对
	return utils.LoadOrGenerateKeyPair(
		h.config.Yggdrasil.Keys.PrivateKeyPath,
		h.config.Yggdrasil.Keys.PublicKeyPath,
	)
}

// SearchMultipleProfiles 按名称批量查询角色
func (h *ProfileHandler) SearchMultipleProfiles(c *gin.Context) {
	var names []string
	if err := c.ShouldBindJSON(&names); err != nil {
		utils.RespondIllegalArgument(c, "Invalid request format")
		return
	}

	// 限制查询数量（防止CC攻击）
	maxProfiles := 10
	if len(names) > maxProfiles {
		utils.RespondForbiddenOperation(c, "Too many profiles requested")
		return
	}

	// 批量查询角色
	profiles, err := h.storage.GetProfilesByNames(names)
	if err != nil {
		utils.RespondError(c, 500, "InternalServerError", "Failed to query profiles")
		return
	}

	// 构建简化的响应（不包含属性）
	// 初始化为空数组，确保即使没有结果也返回[]而不是null
	result := make([]map[string]string, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, map[string]string{
			"id":   profile.ID,
			"name": profile.Name,
		})
	}

	utils.RespondJSONFast(c, result)
}

// SearchSingleProfile 根据用户名查询单个角色
func (h *ProfileHandler) SearchSingleProfile(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		utils.RespondIllegalArgument(c, "Missing username parameter")
		return
	}

	// 获取角色信息
	profile, err := h.storage.GetProfileByName(username)
	if err != nil {
		// 角色不存在，返回204
		utils.RespondNoContent(c)
		return
	}

	// 返回简化的角色信息
	result := map[string]string{
		"id":   profile.ID,
		"name": profile.Name,
	}

	utils.RespondJSONFast(c, result)
}
