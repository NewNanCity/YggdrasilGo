// Package middleware 提供HTTP中间件
package middleware

import (
	"strings"

	"yggdrasil-api-go/src/config"

	"github.com/gin-gonic/gin"
)

const apiLocationHeader = "X-Authlib-Injector-API-Location"

// APILocation 为响应添加 Authlib-Injector API Location 头。
func APILocation(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		scheme := c.GetHeader("X-Forwarded-Proto")
		if scheme == "" {
			if c.Request.TLS != nil {
				scheme = "https"
			} else {
				scheme = "http"
			}
		} else {
			scheme = strings.TrimSpace(strings.Split(scheme, ",")[0])
		}

		apiLocation := cfg.GetAPILocation(scheme, c.Request.Host)
		c.Header(apiLocationHeader, apiLocation)
		c.Next()
	}
}
