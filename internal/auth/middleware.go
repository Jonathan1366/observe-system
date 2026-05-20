package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const ctxKeyAPIKey = "auth_api_key"

// Middleware returns a Gin handler that validates the API key on every request.
// The key is looked up in the provided Store. On success the *APIKey is stored
// in the Gin context under the key "auth_api_key".
//
// Clients supply the key via ONE of:
//   Authorization: Bearer obs_<hex>
//   X-API-Key: obs_<hex>
func Middleware(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := extractKey(c)
		if raw == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing_api_key",
				"hint":  "Supply key via 'Authorization: Bearer <key>' or 'X-API-Key: <key>' header",
			})
			return
		}

		k, ok := store.Get(raw)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid_or_revoked_api_key",
			})
			return
		}

		// Record last-used timestamp asynchronously (fire-and-forget)
		go store.Touch(raw)

		c.Set(ctxKeyAPIKey, k)
		c.Next()
	}
}

// KeyFromContext retrieves the validated *APIKey injected by Middleware.
// Returns nil if the key is not present (e.g. on public routes).
func KeyFromContext(c *gin.Context) *APIKey {
	if v, exists := c.Get(ctxKeyAPIKey); exists {
		if k, ok := v.(*APIKey); ok {
			return k
		}
	}
	return nil
}

// extractKey reads the raw API key string from request headers.
func extractKey(c *gin.Context) string {
	// 1. Authorization: Bearer obs_...
	if auth := c.GetHeader("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}
	// 2. X-API-Key: obs_...
	if k := c.GetHeader("X-API-Key"); k != "" {
		return k
	}
	return ""
}
