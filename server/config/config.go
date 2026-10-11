package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// OIDC Configuration
	OIDCProviderURL  string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURL  string
	OIDCScopes       []string
	OIDCAllowHS256   bool // Permit HMAC-signed ID tokens verified with the client secret

	// JWT Configuration
	JWTSecret    string
	JWTExpiry    time.Duration
	CookieSecure bool // Forces the Secure flag on auth cookies

	// Application Configuration
	VoteThreshold     int
	AdminEmails       []string
	SyncBaseURL       string
	ServerPort        string
	StaleMachineHours int // Age after which a machine counts as stale on the dashboard

	// CORS Configuration
	CORSAllowedOrigins []string

	// Santa Sync Configuration
	SantaSyncKey    string // Shared secret required on sync endpoints (empty = unauthenticated)
	SantaClientMode string // Desired client mode pushed to agents: MONITOR or LOCKDOWN
	// SantaSyncEnforce: false = log-only mode for staged sync-key rollout;
	// invalid/missing keys are logged but allowed.
	SantaSyncEnforce bool

	// Discord Configuration
	DiscordWebhookURL string // Webhook for new-proposal alerts (empty = disabled)

	// Database Configuration
	DatabasePath string
}

var AppConfig *Config

// Load loads configuration from environment variables
func Load() *Config {
	// Load .env file if it exists (silently ignore if not found)
	_ = godotenv.Load()

	config := &Config{
		// OIDC
		OIDCProviderURL:  getEnv("OIDC_PROVIDER_URL", ""),
		OIDCClientID:     getEnv("OIDC_CLIENT_ID", ""),
		OIDCClientSecret: getEnv("OIDC_CLIENT_SECRET", ""),
		OIDCRedirectURL:  getEnv("OIDC_REDIRECT_URL", "http://localhost:8080/auth/callback"),
		OIDCScopes:       strings.Split(getEnv("OIDC_SCOPES", "openid,profile,email"), ","),
		OIDCAllowHS256:   strings.EqualFold(getEnv("OIDC_ALLOW_HS256", "false"), "true"),

		// JWT
		JWTSecret:    getEnv("JWT_SECRET", defaultJWTSecret),
		JWTExpiry:    parseDuration(getEnv("JWT_EXPIRY", "24h")),
		CookieSecure: strings.EqualFold(getEnv("COOKIE_SECURE", "false"), "true"),

		// Application
		VoteThreshold:     parseInt(getEnv("VOTE_THRESHOLD", "3"), 3),
		AdminEmails:       parseList(getEnv("ADMIN_EMAILS", "")),
		SyncBaseURL:       getEnv("SYNC_BASE_URL", "http://localhost:8080"),
		ServerPort:        getEnv("SERVER_PORT", "8080"),
		StaleMachineHours: parseInt(getEnv("STALE_MACHINE_HOURS", "24"), 24),

		// CORS
		CORSAllowedOrigins: parseList(getEnv("CORS_ALLOWED_ORIGINS", "")),

		// Santa Sync
		SantaSyncKey:     getEnv("SANTA_SYNC_KEY", ""),
		SantaClientMode:  strings.ToUpper(getEnv("SANTA_CLIENT_MODE", "LOCKDOWN")),
		SantaSyncEnforce: !strings.EqualFold(getEnv("SANTA_SYNC_ENFORCE", "true"), "false"),

		// Discord
		DiscordWebhookURL: getEnv("DISCORD_WEBHOOK_URL", ""),

		// Database
		DatabasePath: getEnv("DATABASE_PATH", "./database/krampus.db"),
	}

	// Validation
	if config.OIDCProviderURL == "" {
		log.Println("WARNING: OIDC_PROVIDER_URL not set - OIDC authentication will not work")
	}
	if config.OIDCClientID == "" {
		log.Println("WARNING: OIDC_CLIENT_ID not set - OIDC authentication will not work")
	}
	if config.OIDCClientSecret == "" {
		log.Println("WARNING: OIDC_CLIENT_SECRET not set - OIDC authentication will not work")
	}
	if config.JWTSecret == defaultJWTSecret {
		// Refuse to run with the default secret when Gin is in release mode.
		if strings.EqualFold(os.Getenv("GIN_MODE"), "release") {
			log.Fatal("FATAL: JWT_SECRET is set to the default value in release mode - refusing to start")
		}
		log.Println("WARNING: Using default JWT secret - change JWT_SECRET in production!")
	}
	if config.SantaSyncKey == "" {
		log.Println("WARNING: SANTA_SYNC_KEY not set - Santa sync endpoints are UNAUTHENTICATED")
	}
	if config.SantaClientMode != "MONITOR" && config.SantaClientMode != "LOCKDOWN" {
		log.Printf("WARNING: invalid SANTA_CLIENT_MODE %q, falling back to LOCKDOWN", config.SantaClientMode)
		config.SantaClientMode = "LOCKDOWN"
	}
	if config.VoteThreshold < 1 {
		log.Printf("WARNING: VOTE_THRESHOLD %d is below 1, which would auto-finalize proposals on any vote - clamping to 1", config.VoteThreshold)
		config.VoteThreshold = 1
	}
	if config.StaleMachineHours < 1 {
		log.Printf("WARNING: STALE_MACHINE_HOURS %d is below 1, which would mark every machine stale instantly - resetting to 24", config.StaleMachineHours)
		config.StaleMachineHours = 24
	}

	AppConfig = config
	return config
}

const defaultJWTSecret = "change-me-in-production"

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseInt(value string, fallback int) int {
	i, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("Failed to parse int from %q, using %d", value, fallback)
		return fallback
	}
	return i
}

func parseDuration(value string) time.Duration {
	d, err := time.ParseDuration(value)
	if err != nil {
		log.Printf("Failed to parse duration from '%s', using 24h\n", value)
		return 24 * time.Hour
	}
	return d
}

func parseList(value string) []string {
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// IsAdminEmail checks if an email is in the admin list
func (c *Config) IsAdminEmail(email string) bool {
	for _, adminEmail := range c.AdminEmails {
		if strings.EqualFold(adminEmail, email) {
			return true
		}
	}
	return false
}
