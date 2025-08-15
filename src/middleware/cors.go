package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS 跨域资源共享中间件（性能优化版）
func CORS() gin.HandlerFunc {
	// 预定义常用的CORS头，避免重复字符串分配
	const (
		allowOrigin      = "Access-Control-Allow-Origin"
		allowMethods     = "Access-Control-Allow-Methods"
		allowHeaders     = "Access-Control-Allow-Headers"
		exposeHeaders    = "Access-Control-Expose-Headers"
		allowCredentials = "Access-Control-Allow-Credentials"

		methodsValue     = "GET, POST, PUT, DELETE, OPTIONS"
		headersValue     = "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization"
		exposeValue      = "Content-Length"
		credentialsValue = "true"
	)

	return func(c *gin.Context) {
		// 快速处理预检请求
		if c.Request.Method == http.MethodOptions {
			c.Header(allowOrigin, "*")
			c.Header(allowMethods, methodsValue)
			c.Header(allowHeaders, headersValue)
			c.Header(exposeHeaders, exposeValue)
			c.Header(allowCredentials, credentialsValue)
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		// 设置CORS头（优化：减少Header调用次数）
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			c.Header(allowOrigin, origin)
		} else {
			c.Header(allowOrigin, "*")
		}

		c.Header(allowMethods, methodsValue)
		c.Header(allowHeaders, headersValue)
		c.Header(exposeHeaders, exposeValue)
		c.Header(allowCredentials, credentialsValue)

		c.Next()
	}
}
