package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLimitRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("rejects_known_oversized_body_before_handler", func(t *testing.T) {
		called := false
		router := gin.New()
		router.Use(LimitRequestBody(4))
		router.POST("/", func(c *gin.Context) { called = true })
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("12345"))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusRequestEntityTooLarge || called {
			t.Fatalf("status=%d called=%v", response.Code, called)
		}
	})

	t.Run("bounds_chunked_body_reads", func(t *testing.T) {
		router := gin.New()
		router.Use(LimitRequestBody(4))
		router.POST("/", func(c *gin.Context) {
			_, err := io.ReadAll(c.Request.Body)
			var tooLarge *http.MaxBytesError
			if !errors.As(err, &tooLarge) {
				t.Fatalf("body read error=%v", err)
			}
			c.Status(http.StatusRequestEntityTooLarge)
		})
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("12345"))
		request.ContentLength = -1
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status=%d", response.Code)
		}
	})
}
