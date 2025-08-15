// Package utils 高性能JSON处理工具
package utils

import (
	"sync"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
)

// 预编译的常用响应缓存
var (
	responseCache = sync.Map{}

	// 预序列化的API元数据（在启动时设置）
	cachedAPIMetadata []byte

	// 预序列化的常用错误响应
	cachedErrorResponses = make(map[string][]byte)
	initOnce             sync.Once
)

// FastMarshal 高性能JSON序列化
func FastMarshal(v interface{}) ([]byte, error) {
	return sonic.Marshal(v)
}

// FastUnmarshal 高性能JSON反序列化
func FastUnmarshal(data []byte, v interface{}) error {
	return sonic.Unmarshal(data, v)
}

// FastMarshalString 高性能JSON序列化为字符串
func FastMarshalString(v interface{}) (string, error) {
	return sonic.MarshalString(v)
}

// FastUnmarshalString 高性能JSON字符串反序列化
func FastUnmarshalString(data string, v interface{}) error {
	return sonic.UnmarshalString(data, v)
}

// RespondJSONFast 高性能JSON响应
func RespondJSONFast(c *gin.Context, data interface{}) {
	c.Header("Content-Type", "application/json; charset=utf-8")

	// 尝试使用sonic进行快速序列化
	if jsonData, err := FastMarshal(data); err == nil {
		c.Data(200, "application/json", jsonData)
	} else {
		// 降级到标准JSON
		c.JSON(200, data)
	}
}

// GetCachedResponse 获取缓存的响应
func GetCachedResponse(key string) ([]byte, bool) {
	if cached, ok := responseCache.Load(key); ok {
		return cached.([]byte), true
	}
	return nil, false
}

// SetCachedResponse 设置缓存的响应
func SetCachedResponse(key string, data []byte) {
	responseCache.Store(key, data)
}

// GetCachedAPIMetadata 获取缓存的API元数据
func GetCachedAPIMetadata() []byte {
	return cachedAPIMetadata
}

// SetCachedAPIMetadata 设置缓存的API元数据
func SetCachedAPIMetadata(data []byte) {
	cachedAPIMetadata = data
}

// GetCachedErrorResponse 获取缓存的错误响应
func GetCachedErrorResponse(errorType string) []byte {
	initErrorResponses()
	return cachedErrorResponses[errorType]
}

// initErrorResponses 初始化常用错误响应缓存
func initErrorResponses() {
	initOnce.Do(func() {
		// 预序列化常用错误响应
		errorResponses := map[string]interface{}{
			"invalid_token": map[string]string{
				"error":        "ForbiddenOperationException",
				"errorMessage": "Invalid token.",
			},
			"invalid_credentials": map[string]string{
				"error":        "ForbiddenOperationException",
				"errorMessage": "Invalid credentials. Invalid username or password.",
			},
			"player_not_found": map[string]string{
				"error":        "NotFoundException",
				"errorMessage": "Player not found.",
			},
			"user_not_found": map[string]string{
				"error":        "NotFoundException",
				"errorMessage": "User not found.",
			},
			"rate_limit_exceeded": map[string]string{
				"error":        "ForbiddenOperationException",
				"errorMessage": "Rate limit exceeded. Please try again later.",
			},
		}

		for key, response := range errorResponses {
			if data, err := FastMarshal(response); err == nil {
				cachedErrorResponses[key] = data
			}
		}
	})
}

// RespondCachedError 返回缓存的错误响应
func RespondCachedError(c *gin.Context, statusCode int, errorType string) {
	if cachedResponse := GetCachedErrorResponse(errorType); cachedResponse != nil {
		c.Data(statusCode, "application/json", cachedResponse)
	} else {
		// 降级到标准错误响应
		RespondError(c, statusCode, errorType, "Unknown error")
	}
}

// InitErrorResponseCache 初始化错误响应缓存（公开函数，用于缓存预热）
func InitErrorResponseCache() {
	initErrorResponses()
}

// RespondCachedAPIMetadata 返回缓存的API元数据
func RespondCachedAPIMetadata(c *gin.Context) {
	if cachedAPIMetadata != nil {
		c.Data(200, "application/json", cachedAPIMetadata)
	} else {
		// 降级到动态生成
		errorResp := gin.H{"error": "API metadata not cached"}
		if jsonData, err := FastMarshal(errorResp); err == nil {
			c.Data(200, "application/json", jsonData)
		} else {
			c.JSON(200, errorResp)
		}
	}
}
