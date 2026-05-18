// Package traces queries the Jaeger HTTP API and returns simplified
// trace/span data for the Flutter dashboard.
package traces

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	jaegerBase string
	client     *http.Client
}

func NewHandler(jaegerBase string) *Handler {
	return &Handler{
		jaegerBase: jaegerBase,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// ListTraces returns recent traces from Jaeger for arthur-api and otel-demo-jonathan.
// GET /api/traces?service=arthur-api&limit=20
func (h *Handler) ListTraces(c *gin.Context) {
	service := c.DefaultQuery("service", "arthur-api")
	limit := c.DefaultQuery("limit", "20")

	url := fmt.Sprintf("%s/api/traces?service=%s&limit=%s&lookback=1h", h.jaegerBase, service, limit)
	resp, err := h.client.Get(url)
	if err != nil {
		// Return mock data when Jaeger is not reachable (useful for local dev without docker)
		c.JSON(http.StatusOK, mockTraces(service))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var raw any
	if json.Unmarshal(body, &raw) == nil {
		// Flatten Jaeger response into our simpler shape
		c.JSON(http.StatusOK, simplifyJaeger(raw, service))
	} else {
		c.JSON(http.StatusOK, mockTraces(service))
	}
}

// GetTrace returns detail spans for a single traceID.
// GET /api/traces/:traceID
func (h *Handler) GetTrace(c *gin.Context) {
	traceID := c.Param("traceID")
	url := fmt.Sprintf("%s/api/traces/%s", h.jaegerBase, traceID)

	resp, err := h.client.Get(url)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "jaeger_unreachable"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var raw any
	json.Unmarshal(body, &raw)
	c.JSON(resp.StatusCode, raw)
}

// ---- Simplified Jaeger response structures ----

type SimpleTrace struct {
	TraceID    string       `json:"trace_id"`
	Service    string       `json:"service"`
	Operation  string       `json:"operation"`
	Status     string       `json:"status"`
	DurationMs float64      `json:"duration_ms"`
	Timestamp  string       `json:"timestamp"`
	Spans      []SimpleSpan `json:"spans"`
}

type SimpleSpan struct {
	SpanID     string         `json:"span_id"`
	Operation  string         `json:"operation"`
	DurationMs float64        `json:"duration_ms"`
	Status     string         `json:"status"`
	Tags       map[string]any `json:"tags,omitempty"`
}

func simplifyJaeger(raw any, service string) gin.H {
	// Jaeger response: {"data": [...], "total": N, ...}
	m, ok := raw.(map[string]any)
	if !ok {
		return mockTraces(service)
	}
	data, _ := m["data"].([]any)

	var traces []SimpleTrace
	for _, item := range data {
		tr, ok := item.(map[string]any)
		if !ok {
			continue
		}
		traceID, _ := tr["traceID"].(string)
		spans, _ := tr["spans"].([]any)

		var simpleSpans []SimpleSpan
		rootOp := "unknown"
		status := "ok"
		var totalDur float64

		for i, s := range spans {
			sp, ok := s.(map[string]any)
			if !ok {
				continue
			}
			op, _ := sp["operationName"].(string)
			dur, _ := sp["duration"].(float64)
			durMs := dur / 1000

			spStatus := "ok"
			if tags, ok := sp["tags"].([]any); ok {
				for _, t := range tags {
					tag, _ := t.(map[string]any)
					if tag["key"] == "error" && tag["value"] == true {
						spStatus = "error"
						status = "error"
					}
					if tag["key"] == "http.status_code" {
						if code, ok := tag["value"].(float64); ok && code >= 500 {
							spStatus = "error"
							status = "error"
						}
					}
				}
			}

			spid, _ := sp["spanID"].(string)
			simpleSpans = append(simpleSpans, SimpleSpan{
				SpanID:     spid,
				Operation:  op,
				DurationMs: durMs,
				Status:     spStatus,
			})

			if i == 0 {
				rootOp = op
				totalDur = durMs
			}
			_ = totalDur
		}

		// timestamp from first span
		ts := ""
		if len(spans) > 0 {
			if sp, ok := spans[0].(map[string]any); ok {
				if st, ok := sp["startTime"].(float64); ok {
					ts = time.UnixMicro(int64(st)).UTC().Format(time.RFC3339)
				}
			}
		}

		// service name from processes
		svcName := service
		if procs, ok := tr["processes"].(map[string]any); ok {
			for _, pv := range procs {
				if proc, ok := pv.(map[string]any); ok {
					if n, ok := proc["serviceName"].(string); ok {
						svcName = n
						break
					}
				}
			}
		}

		traces = append(traces, SimpleTrace{
			TraceID:    traceID,
			Service:    svcName,
			Operation:  rootOp,
			Status:     status,
			DurationMs: totalDur,
			Timestamp:  ts,
			Spans:      simpleSpans,
		})
	}

	return gin.H{"traces": traces, "service": service, "count": len(traces)}
}

func mockTraces(service string) gin.H {
	now := time.Now().UTC()
	return gin.H{
		"traces": []SimpleTrace{
			{TraceID: "abc123def456", Service: service, Operation: "GET /market/snapshot",
				Status: "ok", DurationMs: 142, Timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339),
				Spans: []SimpleSpan{
					{SpanID: "s001", Operation: "GET /market/snapshot", DurationMs: 142, Status: "ok"},
					{SpanID: "s002", Operation: "binance.FetchPrice", DurationMs: 87, Status: "ok"},
					{SpanID: "s003", Operation: "indicator.ComputeRSI", DurationMs: 12, Status: "ok"},
				},
			},
			{TraceID: "xyz789abc012", Service: service, Operation: "GET /market/alerts",
				Status: "error", DurationMs: 5300, Timestamp: now.Add(-5 * time.Minute).Format(time.RFC3339),
				Spans: []SimpleSpan{
					{SpanID: "s004", Operation: "GET /market/alerts", DurationMs: 5300, Status: "error"},
					{SpanID: "s005", Operation: "hyperliquid.FetchFunding", DurationMs: 5290, Status: "error"},
				},
			},
			{TraceID: "def321ghi654", Service: service, Operation: "GET /playbooks",
				Status: "ok", DurationMs: 23, Timestamp: now.Add(-10 * time.Minute).Format(time.RFC3339),
				Spans: []SimpleSpan{
					{SpanID: "s006", Operation: "GET /playbooks", DurationMs: 23, Status: "ok"},
				},
			},
		},
		"service": service,
		"count":   3,
		"note":    "mock_data_jaeger_unreachable",
	}
}
