// Package middleware 提供HTTP中间件
package middleware

import (
	"strings"

	"yggdrasil-api-go/src/config"

	"github.com/gin-gonic/gin"
)

const apiLocationHeader = "X-Authlib-Injector-API-Location"

// ResolveRequestScheme 解析请求协议，优先读取 X-Forwarded-Proto。
func ResolveRequestScheme(forwardedProto string, isTLS bool) string {
	if forwardedProto != "" {
		value := strings.ToLower(strings.TrimSpace(strings.Split(forwardedProto, ",")[0]))
		if value == "http" || value == "https" {
			return value
		}
	}

	if isTLS {
		return "https"
	}

	return "http"
}

// APILocation 为响应添加 Authlib-Injector API Location 头。
func APILocation(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		forwardedProto := ""
		if cfg.IsTrustedProxy(c.Request.RemoteAddr) {
			forwardedProto = c.GetHeader("X-Forwarded-Proto")
		}
		scheme := ResolveRequestScheme(forwardedProto, c.Request.TLS != nil)
		apiLocation := cfg.GetAPILocation(scheme, c.Request.Host)
		c.Header(apiLocationHeader, apiLocation)
		c.Next()
	}
}
