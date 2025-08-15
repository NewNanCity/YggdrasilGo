package handlers

import (
	"time"

	"yggdrasil-api-go/src/cache"
	storage "yggdrasil-api-go/src/storage/interface"
	"yggdrasil-api-go/src/utils"
	"yggdrasil-api-go/src/yggdrasil"

	"github.com/gin-gonic/gin"
)

// SessionHandler 会话处理器
type SessionHandler struct {
	storage      storage.Storage
	tokenCache   cache.TokenCache
	sessionCache cache.SessionCache
}

// NewSessionHandler 创建新的会话处理器
func NewSessionHandler(storage storage.Storage, tokenCache cache.TokenCache, sessionCache cache.SessionCache) *SessionHandler {
	return &SessionHandler{
		storage:      storage,
		tokenCache:   tokenCache,
		sessionCache: sessionCache,
	}
}

// Join 客户端进入服务器（优化版：JWT优先验证）
func (h *SessionHandler) Join(c *gin.Context) {
	var req yggdrasil.JoinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondIllegalArgument(c, "Invalid request format")
		return
	}

	// 第一步：验证JWT（本地计算，极快）
	claims, err := utils.ValidateJWT(req.AccessToken)
	if err != nil {
		utils.RespondInvalidToken(c)
		return
	}

	// 第二步：验证选中的角色是否与JWT中的角色一致
	if claims.ProfileID == "" || claims.ProfileID != req.SelectedProfile {
		utils.RespondForbiddenOperation(c, "Selected profile does not match token")
		return
	}

	// 创建会话记录（使用JWT中的信息，无需查询数据库）
	session := &yggdrasil.Session{
		ServerID:    req.ServerID,
		AccessToken: req.AccessToken, // session缓存会从中提取用户信息
		ProfileID:   claims.ProfileID,
		ClientIP:    c.ClientIP(),
		CreatedAt:   time.Now(),
	}

	// 存储会话
	if err := h.sessionCache.Store(req.ServerID, session); err != nil {
		utils.RespondError(c, 500, "InternalServerError", "Failed to store session")
		return
	}

	utils.RespondNoContent(c)
}

// HasJoined 服务端验证客户端
func (h *SessionHandler) HasJoined(c *gin.Context) {
	username := c.Query("username")
	serverID := c.Query("serverId")
	clientIP := c.Query("ip") // 可选参数

	if username == "" || serverID == "" {
		utils.RespondIllegalArgument(c, "Missing required parameters")
		return
	}

	// 获取会话信息
	session, err := h.sessionCache.Get(serverID)
	if err != nil || !session.IsValid() {
		// 会话不存在或已过期，返回204
		utils.RespondNoContent(c)
		return
	}

	// 直接通过用户名获取角色信息（更符合Yggdrasil协议逻辑）
	profile, err := h.storage.GetProfileByName(username)
	if err != nil {
		utils.RespondNoContent(c)
		return
	}

	// 如果提供了IP参数，验证IP是否匹配
	if clientIP != "" && session.ClientIP != clientIP {
		utils.RespondNoContent(c)
		return
	}

	// 验证成功，删除会话（一次性使用）
	h.sessionCache.Delete(serverID)

	// 返回完整的角色信息（包含属性和签名）
	utils.RespondJSON(c, profile)
}
