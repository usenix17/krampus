package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"krampus/server/config"

	"github.com/gin-gonic/gin"
)

func corsHeaders(t *testing.T, allowed []string, origin string) http.Header {
	t.Helper()
	gin.SetMode(gin.TestMode)
	config.AppConfig = &config.Config{CORSAllowedOrigins: allowed}
	r := gin.New()
	r.Use(CORSMiddleware())
	r.GET("/x", func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", origin)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Header()
}

func TestWildcardNeverReflectsWithCredentials(t *testing.T) {
	h := corsHeaders(t, []string{"*"}, "https://evil.example")
	if h.Get("Access-Control-Allow-Origin") == "https://evil.example" &&
		h.Get("Access-Control-Allow-Credentials") == "true" {
		t.Fatal("reflected arbitrary origin with credentials")
	}
	if h.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("ACAO=%q, want *", h.Get("Access-Control-Allow-Origin"))
	}
	if h.Get("Access-Control-Allow-Credentials") == "true" {
		t.Error("credentials must not accompany wildcard")
	}
	if h.Get("Vary") != "Origin" {
		t.Errorf("Vary=%q, want Origin on every configured-list branch", h.Get("Vary"))
	}
}

func TestExactOriginGetsCredentials(t *testing.T) {
	h := corsHeaders(t, []string{"https://app.example"}, "https://app.example")
	if h.Get("Access-Control-Allow-Origin") != "https://app.example" ||
		h.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("exact origin not allowed with credentials: %v", h)
	}
}

func TestMixedListExactMatchWins(t *testing.T) {
	h := corsHeaders(t, []string{"*", "https://app.example"}, "https://app.example")
	if h.Get("Access-Control-Allow-Origin") != "https://app.example" ||
		h.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("exact match in mixed list not reflected with credentials: %v", h)
	}

	h = corsHeaders(t, []string{"*", "https://app.example"}, "https://other.example")
	if h.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("ACAO=%q, want *", h.Get("Access-Control-Allow-Origin"))
	}
	if h.Get("Access-Control-Allow-Credentials") == "true" {
		t.Error("credentials must not accompany wildcard fallback")
	}
}
