package handlers

import (
	"strconv"

	storage "yggdrasil-api-go/src/storage/interface"
	"yggdrasil-api-go/src/utils"

	"github.com/gin-gonic/gin"
)

// ProfileHandler 角色处理器
type ProfileHandler struct {
	storage storage.Storage
}

// NewProfileHandler 创建新的角色处理器
func NewProfileHandler(storage storage.Storage) *ProfileHandler {
	return &ProfileHandler{
		storage: storage,
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

	// 如果unsigned为true，移除签名信息
	if unsigned {
		for i := range profile.Properties {
			profile.Properties[i].Signature = ""
		}
	}

	utils.RespondJSONFast(c, profile)
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
