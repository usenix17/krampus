package handlers

import (
	"krampus/server/config"
	"krampus/server/database"
	"krampus/server/middleware"
	"krampus/server/models"
	"krampus/server/services"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// setCookie applies SameSite=Lax and the Secure flag (TLS or COOKIE_SECURE)
// to every auth cookie.
func setCookie(c *gin.Context, name, value string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	secure := config.AppConfig.CookieSecure || c.Request.TLS != nil
	c.SetCookie(name, value, maxAge, "/", "", secure, true)
}

// Login initiates the OIDC authentication flow
func Login(c *gin.Context) {
	// Check if OIDC is configured
	if services.OIDCProvider == nil || services.OAuth2Config == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "OIDC authentication is not configured",
			"help":  "Please configure OIDC_PROVIDER_URL, OIDC_CLIENT_ID, and OIDC_CLIENT_SECRET in your .env file",
		})
		return
	}

	// Generate state token for CSRF protection
	state, err := services.GenerateStateToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate state token"})
		return
	}

	// Store state in session/cookie for validation in callback
	// For simplicity, we'll use a secure cookie
	setCookie(c, "oidc_state", state, 600) // 10 minutes

	// Redirect to OIDC provider
	authURL := services.GetAuthCodeURL(state)
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// Callback handles the OIDC provider callback
func Callback(c *gin.Context) {
	// Validate state parameter
	state := c.Query("state")
	cookieState, err := c.Cookie("oidc_state")
	if err != nil || state != cookieState {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid state parameter"})
		return
	}

	// Clear the state cookie
	setCookie(c, "oidc_state", "", -1)

	// Get authorization code
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Authorization code not provided"})
		return
	}

	// Exchange code for token
	ctx := c.Request.Context()
	oauth2Token, err := services.ExchangeCodeForToken(ctx, code)
	if err != nil {
		log.Printf("Token exchange failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to exchange code for token"})
		return
	}

	// Extract ID token
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No ID token in response"})
		return
	}

	// Verify ID token
	idToken, err := services.VerifyIDToken(ctx, rawIDToken)
	if err != nil {
		log.Printf("ID token verification failed: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Failed to verify ID token"})
		return
	}

	// Get or create user
	user, err := services.GetOrCreateUser(ctx, idToken)
	if err != nil {
		log.Printf("Failed to get or create user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process user"})
		return
	}

	// Generate JWT token
	jwtToken, err := services.GenerateToken(user)
	if err != nil {
		log.Printf("Failed to generate JWT: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate session token"})
		return
	}

	// Set token in cookie for security
	setCookie(c, "token", jwtToken, 86400) // 24 hours

	// Redirect to frontend login page which will handle the token from cookie
	c.Redirect(http.StatusTemporaryRedirect, "/login?auth=success")
}

// Logout revokes the current session and clears the auth cookie.
func Logout(c *gin.Context) {
	tokenString := ""
	if h := c.GetHeader("Authorization"); len(h) > 7 && h[:7] == "Bearer " {
		tokenString = h[7:]
	}
	if tokenString == "" || tokenString == "httponly" {
		if cookieToken, err := c.Cookie("token"); err == nil {
			tokenString = cookieToken
		}
	}
	if tokenString != "" && tokenString != "httponly" {
		if err := services.RevokeToken(tokenString); err != nil {
			log.Printf("Failed to revoke token: %v", err)
		}
	}
	setCookie(c, "token", "", -1)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

// Me returns the current user's information
func Me(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Fetch user from database
	var user models.User
	err := database.DB.QueryRow(
		`SELECT id, username, role, oidc_subject, email, created_at, last_login
		 FROM users WHERE id = ?`,
		userID,
	).Scan(&user.ID, &user.Username, &user.Role, &user.OIDCSubject, &user.Email, &user.CreatedAt, &user.LastLogin)

	if err != nil {
		log.Printf("Failed to fetch user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user information"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// Health check endpoint
func Health(c *gin.Context) {
	// Check database connection
	if err := database.DB.Ping(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":   "unhealthy",
			"database": "disconnected",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"database":  "connected",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
