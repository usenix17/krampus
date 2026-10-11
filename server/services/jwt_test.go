package services

import (
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"krampus/server/config"
	"krampus/server/database"
	"krampus/server/models"
)

func setupJWTTest(t *testing.T) *models.User {
	t.Helper()
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
	return &models.User{ID: 1, Username: "alice", Role: "USER"}
}

// bareUTCDatetime matches SQLite's bare "YYYY-MM-DD HH:MM:SS" format with no
// 'T' separator, no timezone offset suffix, and no fractional seconds -- the
// canonical form datetime('now') produces, which ValidateToken and
// CleanupExpiredSessions compare against lexically.
var bareUTCDatetime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)

func TestGenerateTokenStoresBareUTCExpiry(t *testing.T) {
	user := setupJWTTest(t)

	if _, err := GenerateToken(user); err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	var stored string
	if err := database.DB.QueryRow(
		`SELECT CAST(expires_at AS TEXT) FROM sessions WHERE user_id = 1`,
	).Scan(&stored); err != nil {
		t.Fatalf("query expires_at: %v", err)
	}
	if !bareUTCDatetime.MatchString(stored) {
		t.Errorf("expires_at = %q, want bare UTC datetime matching %s", stored, bareUTCDatetime.String())
	}
}

func TestValidateTokenRoundTrip(t *testing.T) {
	user := setupJWTTest(t)

	token, err := GenerateToken(user)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.UserID != user.ID {
		t.Errorf("claims.UserID = %d, want %d", claims.UserID, user.ID)
	}
	if claims.Role != user.Role {
		t.Errorf("claims.Role = %q, want %q", claims.Role, user.Role)
	}
}
