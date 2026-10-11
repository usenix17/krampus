package middleware

import (
	"crypto/subtle"
	"krampus/server/config"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

var machineIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// ValidateMachineID rejects path-traversal and junk machine identifiers on
// the sync endpoints (Santa sends a hardware UUID).
func ValidateMachineID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !machineIDPattern.MatchString(c.Param("machine_id")) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid machine id"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// SantaAuthMiddleware protects the Santa sync endpoints with a shared secret.
// When SANTA_SYNC_KEY is unset the endpoints stay open (a startup warning is
// logged) so existing deployments that authenticate at the ingress keep working.
// When set, requests must present the key via either:
//
//	Authorization: Bearer <key>
//	X-Santa-Key: <key>
//
// When SANTA_SYNC_ENFORCE is false (staged rollout), a missing or invalid key
// is logged as a warning and the request is allowed through instead of
// rejected, so clients can be migrated onto the key without breaking sync.
func SantaAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := config.AppConfig.SantaSyncKey
		if key == "" {
			c.Next()
			return
		}

		provided := c.GetHeader("X-Santa-Key")
		if provided == "" {
			if parts := strings.SplitN(c.GetHeader("Authorization"), " ", 2); len(parts) == 2 && parts[0] == "Bearer" {
				provided = parts[1]
			}
		}

		// Constant-time compare to avoid leaking the key via timing.
		if subtle.ConstantTimeCompare([]byte(provided), []byte(key)) != 1 {
			if config.AppConfig.SantaSyncEnforce {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid sync credentials"})
				c.Abort()
				return
			}
			log.Printf("WARNING: sync request without valid key from %s (machine %s) - allowed, SANTA_SYNC_ENFORCE=false", c.ClientIP(), c.Param("machine_id"))
		}

		c.Next()
	}
}

// BodyLimitMiddleware caps the request body size to guard against memory
// exhaustion from unbounded (and potentially unauthenticated) uploads.
func BodyLimitMiddleware(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
