// Package ai provides endpoints that call Gemini API to analyze
// Arthur API telemetry data and produce Root Cause Analysis.
package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const geminiModel = "gemini-2.0-flash"
const geminiAPIURL = "https://generativelanguage.googleapis.com/v1beta/models/" + geminiModel + ":generateContent"

type Handler struct {
	geminiKey  string
	arthurBase string
	jaegerBase string
	client     *http.Client
}

func NewHandler(geminiKey, arthurBase, jaegerBase string) *Handler {
	return &Handler{
		geminiKey:  geminiKey,
		arthurBase: arthurBase,
		jaegerBase: jaegerBase,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

// DiagnoseRequest is what Flutter POSTs to trigger AI diagnosis.
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

// DiagnoseResponse is what Flutter receives.
type DiagnoseResponse struct {
	ErrorType        string   `json:"error_type"`
	RootCause        string   `json:"root_cause"`
	AffectedServices []string `json:"affected_services"`
	Recommendation   []string `json:"recommendation"`
	ConfidenceScore  float64  `json:"confidence_score"`
	AnalyzedAt       string   `json:"analyzed_at"`
}

// Diagnose takes trace/log context from Flutter and returns AI RCA.
// POST /api/ai/diagnose
func (h *Handler) Diagnose(c *gin.Context) {
	var req DiagnoseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
		return
	}

	if h.geminiKey == "" {
		c.JSON(http.StatusOK, DiagnoseResponse{
			ErrorType:        "Configuration Warning",
			RootCause:        "GEMINI_API_KEY is not set on the platform backend. Set the env var to enable real AI diagnosis.",
			AffectedServices: []string{req.ServiceName},
			Recommendation: []string{
				"Set GEMINI_API_KEY=your_key when running observe-platform",
				"Restart the platform backend service",
			},
			ConfidenceScore: 1.0,
			AnalyzedAt:      time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	prompt := buildPrompt(req)
	result, err := h.callGemini(prompt)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "gemini_api_error", "detail": err.Error()})
		return
	}

	result.AnalyzedAt = time.Now().UTC().Format(time.RFC3339)
	c.JSON(http.StatusOK, result)
}

// SystemSummary fetches Arthur snapshot + recent traces and asks Gemini for a summary.
// GET /api/ai/summary
func (h *Handler) SystemSummary(c *gin.Context) {
	arthurHealth := h.fetchJSON(h.arthurBase + "/health")
	arthurSnapshot := h.fetchJSON(h.arthurBase + "/market/snapshot")

	if h.geminiKey == "" {
		c.JSON(http.StatusOK, gin.H{
			"summary":          "AI summary unavailable — set GEMINI_API_KEY on the platform backend.",
			"arthur_reachable": arthurHealth != nil,
		})
		return
	}

	ctx := map[string]any{
		"arthur_health":   arthurHealth,
		"market_snapshot": arthurSnapshot,
	}
	ctxJSON, _ := json.MarshalIndent(ctx, "", "  ")

	fullPrompt := fmt.Sprintf(`You are an SRE AI assistant monitoring Arthur, an automated crypto trading backend system.
Analyze the system context provided and return a JSON object with this exact structure:
{
  "status": "healthy|degraded|critical",
  "one_liner": "brief system status sentence",
  "key_signals": ["signal1", "signal2", "signal3"],
  "recommendation": "what engineer should do now"
}
Return ONLY valid JSON. No markdown, no preamble, no backticks.

Arthur system context:
%s`, string(ctxJSON))

	result, err := h.callGeminiRaw(fullPrompt)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "gemini_api_error", "detail": err.Error()})
		return
	}

	var parsed any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		c.JSON(http.StatusOK, gin.H{"summary": result})
		return
	}
	c.JSON(http.StatusOK, parsed)
}

// ---- Prompt builder ----

func buildPrompt(req DiagnoseRequest) string {
	spansJSON, _ := json.MarshalIndent(req.Spans, "", "  ")
	extraJSON, _ := json.MarshalIndent(req.ExtraCtx, "", "  ")
	return fmt.Sprintf(`You are a senior backend engineer and SRE expert specializing in distributed systems observability and root cause analysis for automated trading systems.

Analyze this backend error from Arthur API (automated crypto trading system):

SERVICE: %s
OPERATION: %s
STATUS: %s
DURATION: %.2fms
TRACE ID: %s

LOG MESSAGE:
%s

TRACE SPANS:
%s

EXTRA CONTEXT:
%s

Return ONLY a valid JSON object with this exact structure (no markdown, no backticks, no preamble):
{
  "error_type": "string — short error classification",
  "root_cause": "string — detailed explanation of what went wrong",
  "affected_services": ["list", "of", "services"],
  "recommendation": ["step 1", "step 2", "step 3"],
  "confidence_score": 0.0 to 1.0
}`,
		req.ServiceName, req.Operation, req.Status, req.DurationMs, req.TraceID,
		req.LogMessage, string(spansJSON), string(extraJSON))
}

// ---- Gemini API structs ----

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

// ---- Gemini API call ----

func (h *Handler) callGemini(userPrompt string) (*DiagnoseResponse, error) {
	raw, err := h.callGeminiRaw(userPrompt)
	if err != nil {
		return nil, err
	}

	// Strip markdown fences if Gemini wraps with ```json
	cleaned := raw
	if len(cleaned) > 7 && cleaned[:7] == "```json" {
		cleaned = cleaned[7:]
	}
	if len(cleaned) > 3 && cleaned[len(cleaned)-3:] == "```" {
		cleaned = cleaned[:len(cleaned)-3]
	}

	var result DiagnoseResponse
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		result = DiagnoseResponse{
			ErrorType:        "Parse Error",
			RootCause:        raw,
			AffectedServices: []string{"unknown"},
			Recommendation:   []string{"Check Gemini API response format"},
			ConfidenceScore:  0.5,
		}
	}
	return &result, nil
}

func (h *Handler) callGeminiRaw(prompt string) (string, error) {
	reqBody := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
		GenerationConfig: geminiGenerationConfig{
			Temperature:     0.1,
			MaxOutputTokens: 1000,
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("%s?key=%s", geminiAPIURL, h.geminiKey)
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini http error: %w", err)
	}
	defer resp.Body.Close()

	// respBytes, _ := io.ReadAll(resp.Body)
	// if resp.StatusCode != http.StatusOK {
	// 	return "", fmt.Errorf("gemini API %d: %s", resp.StatusCode, string(respBytes))
	// }

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Printf("[Gemini ERROR] status=%d body=%s", resp.StatusCode, string(respBytes))
		return "", fmt.Errorf("gemini API %d: %s", resp.StatusCode, string(respBytes))
	}
	log.Printf("[Gemini OK] status=%d response_len=%d", resp.StatusCode, len(respBytes))

	var gr geminiResponse
	if err := json.Unmarshal(respBytes, &gr); err != nil {
		return "", fmt.Errorf("gemini response parse: %w", err)
	}
	if gr.Error != nil {
		return "", fmt.Errorf("gemini error %d: %s", gr.Error.Code, gr.Error.Message)
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty gemini response")
	}
	return gr.Candidates[0].Content.Parts[0].Text, nil
}

func (h *Handler) fetchJSON(url string) any {
	resp, err := h.client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var result any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}
