package middleware

import (
	"krampus/server/config"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware handles CORS for frontend requests.
//
// Credentials (cookies) are only permitted for origins that exactly match an
// entry in CORS_ALLOWED_ORIGINS. A "*" entry in that list still enables a
// permissive fallback for non-matching origins, but it is never combined
// with Access-Control-Allow-Credentials -- a wildcard origin is never
// reflected with credentials, matching the credential-less fallback used
// when no allow-list is configured at all.
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := config.AppConfig.CORSAllowedOrigins

		if len(allowed) > 0 {
			// The response differs by Origin on every branch (reflect,
			// wildcard fallback, deny), so caches must always vary on it.
			c.Writer.Header().Add("Vary", "Origin")
			switch {
			case originMatchesExactly(origin, allowed):
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			case containsWildcard(allowed):
				// "*" in the allow-list: permissive but credential-less, same
				// contract as the unconfigured fallback.
				c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			}
		} else {
			// No allow-list configured: permissive but credential-less.
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}

		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func originMatchesExactly(origin string, allowed []string) bool {
	for _, a := range allowed {
		if a != "*" && a == origin {
			return true
		}
	}
	return false
}

func containsWildcard(allowed []string) bool {
	for _, a := range allowed {
		if a == "*" {
			return true
		}
	}
	return false
}
