// Package ai provides endpoints that call Gemini API to analyze
// OTel telemetry data and produce Root Cause Analysis + section insights.
package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"observe-platform/internal/config"
)

// ─── Handler ──────────────────────────────────────────────────────────────────

type Handler struct {
	gemini       config.GeminiConfig
	arthur       config.ArthurConfig
	client       *http.Client
	arthurClient *http.Client
}

func NewHandler(gemini config.GeminiConfig, arthur config.ArthurConfig) *Handler {
	return &Handler{
		gemini:       gemini,
		arthur:       arthur,
		client:       &http.Client{Timeout: gemini.Timeout},
		arthurClient: &http.Client{Timeout: arthur.Timeout},
	}
}

// ─── /api/ai/diagnose ─────────────────────────────────────────────────────────

// DiagnoseRequest is what Flutter POSTs to trigger AI RCA.
type DiagnoseRequest struct {
	TraceID     string         `json:"trace_id"`
	ServiceName string         `json:"service_name"`
	Operation   string         `json:"operation"`
	Status      string         `json:"status"`
	DurationMs  float64        `json:"duration_ms"`
	Spans       []SpanInfo     `json:"spans"`
	LogMessage  string         `json:"log_message"`
	ExtraCtx    map[string]any `json:"extra_ctx,omitempty"`
}

type SpanInfo struct {
	SpanID     string  `json:"span_id"`
	Operation  string  `json:"operation"`
	DurationMs float64 `json:"duration_ms"`
	Status     string  `json:"status"`
}

// DiagnoseResponse is what Flutter renders in the trace detail panel.
type DiagnoseResponse struct {
	ErrorType        string   `json:"error_type"`
	RootCause        string   `json:"root_cause"`
	AffectedServices []string `json:"affected_services"`
	Recommendation   []string `json:"recommendation"`
	ConfidenceScore  float64  `json:"confidence_score"`
	AnalyzedAt       string   `json:"analyzed_at"`
}

// Diagnose — POST /api/ai/diagnose
func (h *Handler) Diagnose(c *gin.Context) {
	var req DiagnoseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
		return
	}

	if h.gemini.APIKey == "" {
		c.JSON(http.StatusOK, DiagnoseResponse{
			ErrorType: "Configuration Warning",
			RootCause: "GEMINI_API_KEY is not set. Set it in the .env file to enable real AI diagnosis.",
			AffectedServices: []string{req.ServiceName},
			Recommendation: []string{
				"Set GEMINI_API_KEY=<your_key> in observe-platform/.env",
				"Restart the platform backend",
			},
			ConfidenceScore: 1.0,
			AnalyzedAt:      time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	result, err := h.callGemini(buildDiagnosePrompt(req))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "gemini_error", "detail": err.Error()})
		return
	}
	result.AnalyzedAt = time.Now().UTC().Format(time.RFC3339)
	c.JSON(http.StatusOK, result)
}

// ─── /api/ai/summary ──────────────────────────────────────────────────────────

// SystemSummary — GET /api/ai/summary
// Fetches Arthur health + snapshot then asks Gemini for a one-liner status.
func (h *Handler) SystemSummary(c *gin.Context) {
	arthurHealth := h.fetchJSON(h.arthurURL(h.arthur.HealthPath))
	arthurSnapshot := h.fetchJSON(h.arthurURL(h.arthur.SnapshotPath))

	if h.gemini.APIKey == "" {
		c.JSON(http.StatusOK, gin.H{
			"status":           "unknown",
			"one_liner":        "AI summary unavailable — GEMINI_API_KEY not set.",
			"key_signals":      []string{},
			"recommendation":   "Set GEMINI_API_KEY in the platform .env file.",
			"arthur_reachable": arthurHealth != nil,
		})
		return
	}

	ctxJSON, _ := json.MarshalIndent(map[string]any{
		"arthur_health":   arthurHealth,
		"market_snapshot": arthurSnapshot,
	}, "", "  ")

	prompt := fmt.Sprintf(`You are an SRE AI assistant monitoring Arthur, an automated crypto trading backend.
Analyze the system context and return ONLY a valid JSON object (no markdown, no backticks):
{
  "status": "healthy|degraded|critical",
  "one_liner": "one-sentence system status",
  "key_signals": ["signal1", "signal2", "signal3"],
  "recommendation": "single actionable sentence"
}

System context:
%s`, string(ctxJSON))

	raw, err := h.callGeminiRaw(prompt)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "gemini_error", "detail": err.Error()})
		return
	}

	var parsed any
	if json.Unmarshal([]byte(stripFences(raw)), &parsed) == nil {
		c.JSON(http.StatusOK, parsed)
		return
	}
	c.JSON(http.StatusOK, gin.H{"one_liner": raw, "status": "unknown"})
}

// ─── /api/ai/insight ──────────────────────────────────────────────────────────

// InsightRequest is what Flutter POSTs for per-section AI insight.
type InsightRequest struct {
	// Section identifies which dashboard panel is asking.
	// Known values: "service_detail" | "services_list" | "traces" | "dashboard"
	Section string         `json:"section"`
	// Context carries whatever the screen has: service name, time range,
	// aggregated stats, recent error counts, etc.
	Context map[string]any `json:"context"`
}

// InsightResponse is rendered as the inline AI card on each screen.
type InsightResponse struct {
	Summary        string   `json:"summary"`
	Severity       string   `json:"severity"` // good | info | warning | critical
	Findings       []string `json:"findings"`
	Recommendation string   `json:"recommendation"`
}

// SectionInsight — POST /api/ai/insight
// Called by the Flutter dashboard to get contextual AI insight
// for the currently visible screen section.
func (h *Handler) SectionInsight(c *gin.Context) {
	var req InsightRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
		return
	}

	if h.gemini.APIKey == "" {
		c.JSON(http.StatusOK, InsightResponse{
			Summary:        "AI insights disabled — set GEMINI_API_KEY in the platform backend.",
			Severity:       "info",
			Findings:       []string{},
			Recommendation: "Configure GEMINI_API_KEY in observe-platform/.env and restart.",
		})
		return
	}

	// Optionally enrich context with live Arthur data for the dashboard section
	enriched := h.enrichContext(req.Section, req.Context)
	ctxJSON, _ := json.MarshalIndent(enriched, "", "  ")

	prompt := buildInsightPrompt(req.Section, string(ctxJSON))

	raw, err := h.callGeminiRaw(prompt)
	if err != nil {
		// Return a graceful fallback instead of an error so the chart UI stays clean
		c.JSON(http.StatusOK, InsightResponse{
			Summary:        "AI analysis temporarily unavailable.",
			Severity:       "info",
			Findings:       []string{},
			Recommendation: "Check GEMINI_API_KEY and platform logs.",
		})
		return
	}

	cleaned := stripFences(raw)
	var result InsightResponse
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		// Gemini returned plain text instead of JSON — wrap it gracefully
		c.JSON(http.StatusOK, InsightResponse{
			Summary:        cleaned,
			Severity:       "info",
			Findings:       []string{},
			Recommendation: "",
		})
		return
	}
	c.JSON(http.StatusOK, result)
}

// enrichContext optionally fetches extra live data from Arthur to
// make the insight prompt richer without Flutter having to send it.
func (h *Handler) enrichContext(section string, ctx map[string]any) map[string]any {
	enriched := make(map[string]any)
	for k, v := range ctx {
		enriched[k] = v
	}
	switch section {
	case "dashboard":
		enriched["arthur_health"] = h.fetchJSON(h.arthurURL(h.arthur.HealthPath))
		enriched["market_snapshot"] = h.fetchJSON(h.arthurURL(h.arthur.SnapshotPath))
	case "service_detail":
		// Forward market snapshot for correlation (trading load → service load)
		enriched["market_snapshot"] = h.fetchJSON(h.arthurURL(h.arthur.SnapshotPath))
	}
	return enriched
}

// buildInsightPrompt returns a section-specific Gemini prompt.
func buildInsightPrompt(section, ctxJSON string) string {
	descriptions := map[string]string{
		"service_detail":  "a service detail page showing latency (p50/p95/p99), error rate, and throughput timeseries charts",
		"services_list":   "a services overview page with RED metrics (Rate, Errors, Duration) for each microservice",
		"traces":          "a distributed tracing list page showing recent OTel spans and error traces",
		"dashboard":       "the main observability dashboard with overall system health, active services, and recent errors",
	}
	desc := descriptions[section]
	if desc == "" {
		desc = "an observability dashboard section"
	}

	return fmt.Sprintf(`You are an SRE AI assistant. The engineer is viewing %s.

Analyze the data below and return ONLY a valid JSON object (no markdown, no backticks):
{
  "summary": "1-2 sentence main observation referencing specific numbers from the data",
  "severity": "good|info|warning|critical",
  "findings": ["specific finding with number", "second finding", "third finding"],
  "recommendation": "single most important actionable step right now"
}

Dashboard section data:
%s`, desc, ctxJSON)
}

// ─── Prompt builders ──────────────────────────────────────────────────────────

func buildDiagnosePrompt(req DiagnoseRequest) string {
	spansJSON, _ := json.MarshalIndent(req.Spans, "", "  ")
	extraJSON, _ := json.MarshalIndent(req.ExtraCtx, "", "  ")
	return fmt.Sprintf(`You are a senior backend engineer and SRE expert in distributed systems observability.

Analyze this backend error:

SERVICE:   %s
OPERATION: %s
STATUS:    %s
DURATION:  %.2f ms
TRACE ID:  %s

LOG MESSAGE:
%s

TRACE SPANS:
%s

EXTRA CONTEXT:
%s

Return ONLY a valid JSON object (no markdown, no backticks, no preamble):
{
  "error_type": "short error classification",
  "root_cause": "detailed explanation of what went wrong and why",
  "affected_services": ["service1", "service2"],
  "recommendation": ["step 1", "step 2", "step 3"],
  "confidence_score": 0.0
}`,
		req.ServiceName, req.Operation, req.Status, req.DurationMs, req.TraceID,
		req.LogMessage, string(spansJSON), string(extraJSON))
}

// ─── Gemini API ───────────────────────────────────────────────────────────────

type geminiRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

func (h *Handler) callGemini(prompt string) (*DiagnoseResponse, error) {
	raw, err := h.callGeminiRaw(prompt)
	if err != nil {
		return nil, err
	}
	cleaned := stripFences(raw)
	var result DiagnoseResponse
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return &DiagnoseResponse{
			ErrorType:        "Parse Error",
			RootCause:        raw,
			AffectedServices: []string{"unknown"},
			Recommendation:   []string{"Check Gemini API response format in platform logs"},
			ConfidenceScore:  0.5,
		}, nil
	}
	return &result, nil
}

func (h *Handler) callGeminiRaw(prompt string) (string, error) {
	body, _ := json.Marshal(geminiRequest{
		Contents: []geminiContent{{Parts: []geminiPart{{Text: prompt}}}},
		GenerationConfig: geminiGenerationConfig{
			Temperature:     h.gemini.Temperature,
			MaxOutputTokens: h.gemini.MaxOutputTokens,
		},
	})

	reqURL, err := h.geminiURL()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini http: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Printf("[Gemini ERROR] status=%d body=%s", resp.StatusCode, string(respBytes))
		return "", fmt.Errorf("gemini API %d: %s", resp.StatusCode, string(respBytes))
	}
	log.Printf("[Gemini OK] status=%d len=%d", resp.StatusCode, len(respBytes))

	var gr geminiResponse
	if err := json.Unmarshal(respBytes, &gr); err != nil {
		return "", fmt.Errorf("gemini parse: %w", err)
	}
	if gr.Error != nil {
		return "", fmt.Errorf("gemini error %d: %s", gr.Error.Code, gr.Error.Message)
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty gemini response")
	}
	return gr.Candidates[0].Content.Parts[0].Text, nil
}

func (h *Handler) geminiURL() (string, error) {
	ep, err := url.Parse(h.gemini.APIURL)
	if err != nil {
		return "", fmt.Errorf("invalid GEMINI_API_URL: %w", err)
	}
	q := ep.Query()
	q.Set("key", h.gemini.APIKey)
	ep.RawQuery = q.Encode()
	return ep.String(), nil
}

func (h *Handler) arthurURL(path string) string {
	return fmt.Sprintf("%s%s", h.arthur.BaseURL, path)
}

func (h *Handler) fetchJSON(rawURL string) any {
	resp, err := h.arthurClient.Get(rawURL)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var result any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

// stripFences removes ```json / ``` markdown wrapping that Gemini sometimes adds.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
