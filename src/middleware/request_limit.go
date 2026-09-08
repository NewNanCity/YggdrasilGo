package middleware

import (
	"net/http"

	"yggdrasil-api-go/src/utils"

	"github.com/gin-gonic/gin"
)

// LimitRequestBody bounds all request bodies before endpoint-specific decoding.
func LimitRequestBody(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.Next()
			return
		}
		if c.Request.ContentLength > maxBytes {
			utils.RespondError(c, http.StatusRequestEntityTooLarge,
				"RequestEntityTooLarge", "Request body exceeds the configured limit")
			c.Abort()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
