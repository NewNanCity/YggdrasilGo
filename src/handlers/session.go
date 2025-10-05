package handlers

import (
	"fmt"
	"time"

	"yggdrasil-api-go/src/cache"
	"yggdrasil-api-go/src/config"
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
	config       *config.Config
}

// NewSessionHandler 创建新的会话处理器
func NewSessionHandler(storage storage.Storage, tokenCache cache.TokenCache, sessionCache cache.SessionCache, cfg *config.Config) *SessionHandler {
	return &SessionHandler{
		storage:      storage,
		tokenCache:   tokenCache,
		sessionCache: sessionCache,
		config:       cfg,
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

	// 通过用户名获取角色信息
	profile, err := h.storage.GetProfileByName(username)
	if err != nil {
		utils.RespondNoContent(c)
		return
	}

	// 验证角色UUID是否与会话中的ProfileID匹配
	if profile.ID != session.ProfileID {
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

	// 为角色属性生成数字签名（根据Yggdrasil规范要求）
	for i := range profile.Properties {
		if profile.Properties[i].Signature == "" {
			signature, err := h.generateSignature(profile.Properties[i].Value)
			if err != nil {
				// 签名生成失败，记录错误但不影响响应
				// 继续返回无签名的数据
				continue
			}
			profile.Properties[i].Signature = signature
		}
	}

	// 返回完整的角色信息（包含属性和签名）
	utils.RespondJSON(c, profile)
}

// generateSignature 生成属性值的数字签名（高性能版本）
func (h *SessionHandler) generateSignature(value string) (string, error) {
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
func (h *SessionHandler) loadSignatureKeyPair() (privateKey string, publicKey string, err error) {
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
