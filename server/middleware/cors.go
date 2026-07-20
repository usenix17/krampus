package middleware

import (
	"krampus/server/config"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware handles CORS for frontend requests.
//
// Credentials (cookies) are only permitted for explicitly allow-listed origins
// via CORS_ALLOWED_ORIGINS. When no origins are configured we fall back to a
// wildcard WITHOUT credentials, which is safe because the browser will refuse
// to attach cookies. This avoids the invalid/unsafe "*" + credentials pairing.
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := config.AppConfig.CORSAllowedOrigins

		if len(allowed) > 0 {
			if originAllowed(origin, allowed) {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
				c.Writer.Header().Add("Vary", "Origin")
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

func originAllowed(origin string, allowed []string) bool {
	for _, a := range allowed {
		if a == "*" || a == origin {
			return true
		}
	}
	return false
}
