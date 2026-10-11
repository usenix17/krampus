package handlers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"krampus/server/config"
	"krampus/server/database"
	"krampus/server/models"
	"krampus/server/services"

	"github.com/gin-gonic/gin"
)

func TestSetCookieFlags(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config.AppConfig = &config.Config{CookieSecure: true}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	setCookie(c, "token", "v", 60)

	setCookieHeader := w.Header().Get("Set-Cookie")
	if !strings.Contains(setCookieHeader, "SameSite=Lax") {
		t.Errorf("Set-Cookie=%q, want SameSite=Lax", setCookieHeader)
	}
	if !strings.Contains(setCookieHeader, "Secure") {
		t.Errorf("Set-Cookie=%q, want Secure when CookieSecure=true", setCookieHeader)
	}
	if !strings.Contains(setCookieHeader, "HttpOnly") {
		t.Errorf("Set-Cookie=%q, want HttpOnly", setCookieHeader)
	}

	config.AppConfig = &config.Config{CookieSecure: false}
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	setCookie(c2, "token", "v", 60)

	setCookieHeader2 := w2.Header().Get("Set-Cookie")
	if strings.Contains(setCookieHeader2, "Secure") {
		t.Errorf("Set-Cookie=%q, want no Secure when CookieSecure=false and no TLS", setCookieHeader2)
	}
	if !strings.Contains(setCookieHeader2, "SameSite=Lax") {
		t.Errorf("Set-Cookie=%q, want SameSite=Lax", setCookieHeader2)
	}
	if !strings.Contains(setCookieHeader2, "HttpOnly") {
		t.Errorf("Set-Cookie=%q, want HttpOnly", setCookieHeader2)
	}
}

func TestLogoutRevokesCookieSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config.AppConfig = &config.Config{JWTSecret: "test-secret", JWTExpiry: time.Hour}
	if err := database.Initialize(filepath.Join(t.TempDir(), "t.db")); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.DB.Exec(
		`INSERT INTO users (id, username, role) VALUES (1, 'alice', 'USER')`,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	user := &models.User{ID: 1, Username: "alice", Role: "USER"}
	token, err := services.GenerateToken(user)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	var sessionsBefore int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessionsBefore); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessionsBefore != 1 {
		t.Fatalf("sessionsBefore=%d, want 1", sessionsBefore)
	}

	router := gin.New()
	router.POST("/auth/logout", Logout)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer httponly")
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200, body=%s", w.Code, w.Body.String())
	}

	var sessionsAfter int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessionsAfter); err != nil {
		t.Fatalf("count sessions after: %v", err)
	}
	if sessionsAfter != 0 {
		t.Errorf("sessionsAfter=%d, want 0 (session should be revoked)", sessionsAfter)
	}

	setCookieHeader := w.Header().Get("Set-Cookie")
	if !strings.Contains(setCookieHeader, "token=") {
		t.Fatalf("Set-Cookie=%q, want it to clear the token cookie", setCookieHeader)
	}
	if !strings.Contains(setCookieHeader, "Max-Age=0") && !strings.Contains(setCookieHeader, "Max-Age=-1") {
		t.Errorf("Set-Cookie=%q, want Max-Age=0 (or negative) to clear the cookie", setCookieHeader)
	}
}
