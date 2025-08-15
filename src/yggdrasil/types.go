// Package yggdrasil 定义了Yggdrasil API的公共类型
package yggdrasil

import "time"

// User 用户模型
type User struct {
	ID       string    `json:"id"`       // 用户UUID
	Email    string    `json:"email"`    // 邮箱
	Password string    `json:"-"`        // 密码（不序列化）
	Profiles []Profile `json:"profiles"` // 用户拥有的角色列表
}

// Profile 角色模型
type Profile struct {
	ID         string            `json:"id"`         // 角色UUID（无符号）
	Name       string            `json:"name"`       // 角色名称
	Properties []ProfileProperty `json:"properties"` // 角色属性
}

// ProfileProperty 角色属性
type ProfileProperty struct {
	Name      string `json:"name"`                // 属性名称
	Value     string `json:"value"`               // 属性值
	Signature string `json:"signature,omitempty"` // 数字签名（可选）
}

// Token 令牌模型
type Token struct {
	AccessToken string    `json:"accessToken"` // 访问令牌
	ClientToken string    `json:"clientToken"` // 客户端令牌
	ProfileID   string    `json:"profileId"`   // 绑定的角色ID
	Owner       string    `json:"owner"`       // 令牌所有者（用户ID）
	CreatedAt   time.Time `json:"createdAt"`   // 创建时间
	ExpiresAt   time.Time `json:"expiresAt"`   // 过期时间
}

// IsValid 检查令牌是否有效
func (t *Token) IsValid() bool {
	return time.Now().Before(t.ExpiresAt)
}

// AuthenticateRequest 登录请求
type AuthenticateRequest struct {
	Username    string `json:"username" binding:"required"` // 用户名/邮箱
	Password    string `json:"password" binding:"required"` // 密码
	ClientToken string `json:"clientToken"`                 // 客户端令牌（可选）
	RequestUser bool   `json:"requestUser"`                 // 是否返回用户信息
	Agent       Agent  `json:"agent"`                       // 客户端信息
}

// Agent 客户端信息
type Agent struct {
	Name    string `json:"name"`    // 客户端名称
	Version int    `json:"version"` // 版本
}

// AuthenticateResponse 登录响应
type AuthenticateResponse struct {
	AccessToken       string    `json:"accessToken"`               // 访问令牌
	ClientToken       string    `json:"clientToken"`               // 客户端令牌
	AvailableProfiles []Profile `json:"availableProfiles"`         // 可用角色列表
	SelectedProfile   *Profile  `json:"selectedProfile,omitempty"` // 选中的角色
	User              *UserInfo `json:"user,omitempty"`            // 用户信息（可选）
}

// UserInfo 用户信息
type UserInfo struct {
	ID         string            `json:"id"`         // 用户ID
	Properties []ProfileProperty `json:"properties"` // 用户属性
}

// RefreshRequest 刷新令牌请求
type RefreshRequest struct {
	AccessToken     string   `json:"accessToken" binding:"required"` // 访问令牌
	ClientToken     string   `json:"clientToken"`                    // 客户端令牌（可选）
	RequestUser     bool     `json:"requestUser"`                    // 是否返回用户信息
	SelectedProfile *Profile `json:"selectedProfile"`                // 要选择的角色（可选）
}

// RefreshResponse 刷新令牌响应
type RefreshResponse struct {
	AccessToken     string    `json:"accessToken"`               // 新的访问令牌
	ClientToken     string    `json:"clientToken"`               // 客户端令牌
	SelectedProfile *Profile  `json:"selectedProfile,omitempty"` // 选中的角色
	User            *UserInfo `json:"user,omitempty"`            // 用户信息（可选）
}

// ValidateRequest 验证令牌请求
type ValidateRequest struct {
	AccessToken string `json:"accessToken" binding:"required"` // 访问令牌
	ClientToken string `json:"clientToken"`                    // 客户端令牌（可选）
}

// InvalidateRequest 撤销令牌请求
type InvalidateRequest struct {
	AccessToken string `json:"accessToken" binding:"required"` // 访问令牌
	ClientToken string `json:"clientToken"`                    // 客户端令牌（可选）
}

// SignoutRequest 登出请求
type SignoutRequest struct {
	Username string `json:"username" binding:"required"` // 用户名/邮箱
	Password string `json:"password" binding:"required"` // 密码
}

// JoinRequest 客户端进入服务器请求
type JoinRequest struct {
	AccessToken     string `json:"accessToken" binding:"required"`     // 访问令牌
	SelectedProfile string `json:"selectedProfile" binding:"required"` // 选中的角色UUID
	ServerID        string `json:"serverId" binding:"required"`        // 服务器ID
}

// Session 会话信息
type Session struct {
	ServerID    string    `json:"serverId"`    // 服务器ID
	AccessToken string    `json:"accessToken"` // 访问令牌
	ProfileID   string    `json:"profileId"`   // 角色ID
	ClientIP    string    `json:"clientIp"`    // 客户端IP
	CreatedAt   time.Time `json:"createdAt"`   // 创建时间
}

// IsValid 检查会话是否有效（30秒内）
func (s *Session) IsValid() bool {
	return time.Since(s.CreatedAt) < 30*time.Second
}

// APIMetadata API元数据
type APIMetadata struct {
	Meta               MetaInfo `json:"meta"`               // 元数据
	SkinDomains        []string `json:"skinDomains"`        // 皮肤域名白名单
	SignaturePublicKey string   `json:"signaturePublickey"` // 签名公钥
}

// MetaInfo 服务器元数据
type MetaInfo struct {
	ServerName            string            `json:"serverName"`              // 服务器名称
	ImplementationName    string            `json:"implementationName"`      // 实现名称
	ImplementationVersion string            `json:"implementationVersion"`   // 实现版本
	Links                 map[string]string `json:"links"`                   // 相关链接
	FeatureNonEmailLogin  bool              `json:"feature.non_email_login"` // 支持非邮箱登录
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Error        string `json:"error"`           // 错误类型
	ErrorMessage string `json:"errorMessage"`    // 错误消息
	Cause        string `json:"cause,omitempty"` // 错误原因（可选）
}
