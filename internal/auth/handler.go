package auth

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler exposes admin endpoints for API key management.
// These routes should be protected by the ADMIN_SECRET env var (see main.go).
type Handler struct {
	store       *Store
	adminSecret string // required to call any admin endpoint
}

// NewHandler creates a key-management handler.
// adminSecret is compared against the X-Admin-Secret header on each request.
func NewHandler(store *Store, adminSecret string) *Handler {
	return &Handler{store: store, adminSecret: adminSecret}
}

// ─── Admin guard ──────────────────────────────────────────────────────────────

func (h *Handler) guardAdmin(c *gin.Context) bool {
	if h.adminSecret == "" {
		return true // no secret configured → open (dev mode)
	}
	provided := c.GetHeader("X-Admin-Secret")
	if provided != h.adminSecret {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "invalid_admin_secret",
			"hint":  "Provide the correct value in the X-Admin-Secret header",
		})
		return false
	}
	return true
}

// ─── Endpoints ────────────────────────────────────────────────────────────────

// CreateKey generates a new API key.
// POST /admin/keys
// Headers: X-Admin-Secret: <secret>
// Body:
//
//	{
//	  "name":        "My System",
//	  "description": "optional note",
//	  "arthur_url":  "http://localhost:8420",   // optional, defaults to platform default
//	  "jaeger_url":  "http://localhost:16686"   // optional
//	}
func (h *Handler) CreateKey(c *gin.Context) {
	if !h.guardAdmin(c) {
		return
	}

	var body struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		ArthurURL   string `json:"arthur_url"`
		JaegerURL   string `json:"jaeger_url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	k, err := h.store.Create(body.Name, body.Description, body.ArthurURL, body.JaegerURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "key_generation_failed", "detail": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"api_key":     k.Key,
		"name":        k.Name,
		"description": k.Description,
		"arthur_url":  k.ArthurURL,
		"jaeger_url":  k.JaegerURL,
		"created_at":  k.CreatedAt.Format(time.RFC3339),
		"message":     "Save this key — it will not be shown again in plaintext through the list endpoint.",
	})
}

// ListKeys returns all keys with masked key strings.
// GET /admin/keys
// Headers: X-Admin-Secret: <secret>
func (h *Handler) ListKeys(c *gin.Context) {
	if !h.guardAdmin(c) {
		return
	}

	all := h.store.List()
	type row struct {
		KeyMasked   string     `json:"key_masked"`   // obs_xxxx...xxxx
		Name        string     `json:"name"`
		Description string     `json:"description,omitempty"`
		ArthurURL   string     `json:"arthur_url,omitempty"`
		JaegerURL   string     `json:"jaeger_url,omitempty"`
		CreatedAt   string     `json:"created_at"`
		LastUsedAt  *string    `json:"last_used_at,omitempty"`
		Revoked     bool       `json:"revoked"`
	}

	rows := make([]row, 0, len(all))
	for _, k := range all {
		masked := maskKey(k.Key)
		var lastUsed *string
		if k.LastUsedAt != nil {
			s := k.LastUsedAt.Format(time.RFC3339)
			lastUsed = &s
		}
		rows = append(rows, row{
			KeyMasked:   masked,
			Name:        k.Name,
			Description: k.Description,
			ArthurURL:   k.ArthurURL,
			JaegerURL:   k.JaegerURL,
			CreatedAt:   k.CreatedAt.Format(time.RFC3339),
			LastUsedAt:  lastUsed,
			Revoked:     k.Revoked,
		})
	}

	c.JSON(http.StatusOK, gin.H{"keys": rows, "count": len(rows)})
}

// RevokeKey marks a key as revoked without deleting it.
// POST /admin/keys/revoke
// Headers: X-Admin-Secret: <secret>
// Body: {"key": "obs_..."}
func (h *Handler) RevokeKey(c *gin.Context) {
	if !h.guardAdmin(c) {
		return
	}

	var body struct {
		Key string `json:"key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	if !h.store.Revoke(body.Key) {
		c.JSON(http.StatusNotFound, gin.H{"error": "key_not_found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Key revoked successfully",
		"key_masked": maskKey(body.Key),
		"revoked_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// DeleteKey removes a key permanently.
// DELETE /admin/keys
// Headers: X-Admin-Secret: <secret>
// Body: {"key": "obs_..."}
func (h *Handler) DeleteKey(c *gin.Context) {
	if !h.guardAdmin(c) {
		return
	}

	var body struct {
		Key string `json:"key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	if !h.store.Delete(body.Key) {
		c.JSON(http.StatusNotFound, gin.H{"error": "key_not_found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Key deleted permanently",
		"key_masked": maskKey(body.Key),
	})
}

// VerifyKey lets the Flutter app verify its key without making a real API call.
// GET /auth/verify
// Headers: Authorization: Bearer obs_... (or X-API-Key)
// No admin secret needed — the API key itself is the credential.
func (h *Handler) VerifyKey(c *gin.Context) {
	// This route sits BEHIND the API key middleware (applied in main.go),
	// so if we reach here the key is already valid.
	k := KeyFromContext(c)
	if k == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":       true,
		"name":        k.Name,
		"description": k.Description,
		"arthur_url":  k.ArthurURL,
		"jaeger_url":  k.JaegerURL,
		"created_at":  k.CreatedAt.Format(time.RFC3339),
	})
}

// ─── Helper ───────────────────────────────────────────────────────────────────

// maskKey shows only the prefix and last 4 chars: obs_xxxxxx...ef12
func maskKey(key string) string {
	if len(key) <= 12 {
		return key // too short to mask meaningfully
	}
	return key[:8] + "..." + key[len(key)-4:]
}
