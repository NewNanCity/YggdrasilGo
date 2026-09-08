package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"yggdrasil-api-go/src/config"

	"github.com/gin-gonic/gin"
)

func TestResolveRequestScheme(t *testing.T) {
	tests := []struct {
		name           string
		forwardedProto string
		isTLS          bool
		want           string
	}{
		{name: "forwarded proto single", forwardedProto: "https", isTLS: false, want: "https"},
		{name: "forwarded proto chain", forwardedProto: "https,http", isTLS: false, want: "https"},
		{name: "forwarded proto chain with spaces", forwardedProto: " https , http ", isTLS: false, want: "https"},
		{name: "fallback tls", forwardedProto: "", isTLS: true, want: "https"},
		{name: "fallback http", forwardedProto: "", isTLS: false, want: "http"},
		{name: "invalid forwarded proto", forwardedProto: "javascript", isTLS: false, want: "http"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveRequestScheme(tt.forwardedProto, tt.isTLS)
			if got != tt.want {
				t.Fatalf("ResolveRequestScheme() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPILocationUsesForwardedHeadersOnlyFromTrustedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name       string
		remoteAddr string
		wantAPI    string
		wantIP     string
	}{
		{name: "trusted proxy", remoteAddr: "10.1.2.3:1234", wantAPI: "https://api.example.invalid/", wantIP: "203.0.113.8"},
		{name: "untrusted proxy", remoteAddr: "192.0.2.9:1234", wantAPI: "http://api.example.invalid/", wantIP: "192.0.2.9"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Server.TrustedProxies = []string{"10.0.0.0/8"}
			if err := cfg.Validate(); err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			if err := router.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
				t.Fatal(err)
			}
			router.Use(APILocation(cfg))
			router.GET("/", func(c *gin.Context) {
				c.Header("X-Test-Client-IP", c.ClientIP())
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, "http://api.example.invalid/", nil)
			request.RemoteAddr = tt.remoteAddr
			request.Header.Set("X-Forwarded-For", "203.0.113.8")
			request.Header.Set("X-Forwarded-Proto", "https")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if got := response.Header().Get(apiLocationHeader); got != tt.wantAPI {
				t.Fatalf("API location=%q want=%q", got, tt.wantAPI)
			}
			if got := response.Header().Get("X-Test-Client-IP"); got != tt.wantIP {
				t.Fatalf("client IP=%q want=%q", got, tt.wantIP)
			}
		})
	}
}

func TestAPILocationIgnoresForwardedProtoWithoutTrustedProxyConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.DefaultConfig()
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	router.Use(APILocation(cfg))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "http://api.example.invalid/", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if got := response.Header().Get(apiLocationHeader); got != "http://api.example.invalid/" {
		t.Fatalf("untrusted forwarded proto changed API location: %q", got)
	}
}
