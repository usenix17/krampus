package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"krampus/server/config"
	"krampus/server/database"
	"krampus/server/models"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents JWT claims
type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Email    string `json:"email,omitempty"`
	jwt.RegisteredClaims
}

// GenerateToken creates a new JWT token for the user
func GenerateToken(user *models.User) (string, error) {
	expiresAt := time.Now().Add(config.AppConfig.JWTExpiry)

	claims := &Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "krampus",
			Subject:   user.Username,
		},
	}

	if user.Email != nil {
		claims.Email = *user.Email
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(config.AppConfig.JWTSecret))
	if err != nil {
		return "", err
	}

	// Store session in database for revocation capability
	tokenHash := hashToken(tokenString)
	_, err = database.DB.Exec(
		`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
		user.ID, tokenHash, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("failed to store session: %w", err)
	}

	return tokenString, nil
}

// ValidateToken parses and validates a JWT token
func ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(config.AppConfig.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	// Check if session exists and hasn't been revoked
	tokenHash := hashToken(tokenString)
	var count int
	err = database.DB.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE token_hash = ? AND expires_at > datetime('now')`,
		tokenHash,
	).Scan(&count)
	if err != nil || count == 0 {
		return nil, fmt.Errorf("session not found or expired")
	}

	return claims, nil
}

// RevokeToken revokes a JWT token by removing it from the sessions table
func RevokeToken(tokenString string) error {
	tokenHash := hashToken(tokenString)
	_, err := database.DB.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// CleanupExpiredSessions removes expired sessions from the database
func CleanupExpiredSessions() error {
	_, err := database.DB.Exec(`DELETE FROM sessions WHERE expires_at < datetime('now')`)
	return err
}

// hashToken creates a SHA256 hash of the token for storage
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
