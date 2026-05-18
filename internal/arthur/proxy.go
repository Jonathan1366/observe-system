// Package arthur proxies calls to the Arthur API (port 8420).
// All responses are passed through as-is so the Flutter client
// always sees the real data shape from Arthur.
package arthur

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler holds the base URL of the Arthur API.
type Handler struct {
	baseURL string
	client  *http.Client
}

// NewHandler creates a new Arthur proxy handler.
func NewHandler(baseURL string) *Handler {
	return &Handler{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (h *Handler) Health(c *gin.Context)        { h.proxy(c, "/health") }
func (h *Handler) Snapshot(c *gin.Context)      { h.proxy(c, "/market/snapshot") }
func (h *Handler) Alerts(c *gin.Context)        { h.proxy(c, "/market/alerts") }
func (h *Handler) Performance(c *gin.Context)   { h.proxy(c, "/trade/performance") }
func (h *Handler) Positions(c *gin.Context)     { h.proxy(c, "/trade/positions") }
func (h *Handler) Playbooks(c *gin.Context)     { h.proxy(c, "/playbooks") }
func (h *Handler) MarketContext(c *gin.Context) { h.proxy(c, "/market/context") }

// proxy forwards a GET request to Arthur and passes the response body through.
func (h *Handler) proxy(c *gin.Context, path string) {
	url := fmt.Sprintf("%s%s", h.baseURL, path)

	resp, err := h.client.Get(url)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":  "arthur_api_unreachable",
			"detail": err.Error(),
			"url":    url,
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "read_error", "detail": err.Error()})
		return
	}

	// Try to decode as JSON first; fall through to raw bytes if not
	var parsed any
	if json.Unmarshal(body, &parsed) == nil {
		c.JSON(resp.StatusCode, parsed)
	} else {
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
}
