// Package metrics aggregates service-level metrics from Jaeger
// and Arthur API for the Flutter dashboard charts.
//
// Flow:
//   Flutter → GET /api/metrics/services
//           → GET /api/metrics/service/:name?range=1h
//
// Both endpoints try to pull real data from Jaeger first.
// If Jaeger is unreachable they fall back to deterministic mock
// data so the Flutter charts always have something to render.
package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"observe-platform/internal/config"
)

// Handler serves metrics endpoints.
type Handler struct {
	jaeger config.JaegerConfig
	arthur config.ArthurConfig
	client *http.Client
}

func NewHandler(jaeger config.JaegerConfig, arthur config.ArthurConfig) *Handler {
	return &Handler{
		jaeger: jaeger,
		arthur: arthur,
		client: &http.Client{Timeout: jaeger.Timeout},
	}
}

// ─── Models ──────────────────────────────────────────────────────────────────

// ServiceSummary is one row in the service list.
type ServiceSummary struct {
	Name            string    `json:"name"`
	RequestCount    int       `json:"request_count"`
	ErrorCount      int       `json:"error_count"`
	ErrorRate       float64   `json:"error_rate"`
	P95LatencyMs    float64   `json:"p95_latency_ms"`
	AvgLatencyMs    float64   `json:"avg_latency_ms"`
	LatencySparkline []float64 `json:"latency_sparkline"`
	LastSeen        string    `json:"last_seen"`
}

// LatencyPoint is one data point in the latency time-series.
type LatencyPoint struct {
	T   string  `json:"t"`   // ISO timestamp
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
}

// RatePoint is one data point for error-rate / throughput charts.
type RatePoint struct {
	T    string  `json:"t"`
	Rate float64 `json:"rate"` // error rate %  OR  req/s
}

// ServiceMetricsResponse is the full detail payload for one service.
type ServiceMetricsResponse struct {
	Service               string         `json:"service"`
	Range                 string         `json:"range"`
	LatencyTimeseries     []LatencyPoint `json:"latency_timeseries"`
	ErrorRateTimeseries   []RatePoint    `json:"error_rate_timeseries"`
	ThroughputTimeseries  []RatePoint    `json:"throughput_timeseries"`
	Percentiles           map[string]float64 `json:"percentiles"`
}

// ─── Handlers ────────────────────────────────────────────────────────────────

// ListServices returns a summary row for every service known to Jaeger.
// GET /api/metrics/services
func (h *Handler) ListServices(c *gin.Context) {
	services, err := h.fetchJaegerServices()
	if err != nil {
		// Jaeger offline → return realistic mock
		c.JSON(http.StatusOK, gin.H{
			"services": mockServiceList(),
			"source":   "mock",
		})
		return
	}

	summaries := make([]ServiceSummary, 0, len(services))
	for _, svc := range services {
		summary := h.buildServiceSummary(svc)
		summaries = append(summaries, summary)
	}

	// Sort: services with errors first
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].ErrorRate > summaries[j].ErrorRate
	})

	c.JSON(http.StatusOK, gin.H{
		"services": summaries,
		"count":    len(summaries),
		"source":   "jaeger",
	})
}

// ServiceMetrics returns timeseries charts for one service.
// GET /api/metrics/service/:name?range=1h
func (h *Handler) ServiceMetrics(c *gin.Context) {
	serviceName := c.Param("name")
	rangeStr := c.DefaultQuery("range", "1h")

	lookback := rangeToDuration(rangeStr)
	buckets := rangeToPoints(rangeStr) // how many chart points

	// Try real Jaeger data
	traces, err := h.fetchTracesForService(serviceName, lookback)
	if err != nil || len(traces) == 0 {
		// Fallback to mock timeseries
		c.JSON(http.StatusOK, mockServiceMetrics(serviceName, rangeStr, buckets))
		return
	}

	resp := buildMetricsFromTraces(serviceName, rangeStr, traces, lookback, buckets)
	c.JSON(http.StatusOK, resp)
}

// ─── Jaeger API calls ─────────────────────────────────────────────────────────

func (h *Handler) fetchJaegerServices() ([]string, error) {
	url := fmt.Sprintf("%s/api/services", h.jaeger.BaseURL)
	resp, err := h.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

type jaegerTrace struct {
	TraceID string         `json:"traceID"`
	Spans   []jaegerSpan   `json:"spans"`
}

type jaegerSpan struct {
	SpanID        string         `json:"spanID"`
	OperationName string         `json:"operationName"`
	Duration      int64          `json:"duration"` // microseconds
	StartTime     int64          `json:"startTime"` // unix microseconds
	Tags          []jaegerTag    `json:"tags"`
}

type jaegerTag struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

func (h *Handler) fetchTracesForService(service string, lookback time.Duration) ([]jaegerTrace, error) {
	endUs := time.Now().UnixMicro()
	startUs := time.Now().Add(-lookback).UnixMicro()
	url := fmt.Sprintf(
		"%s/api/traces?service=%s&start=%d&end=%d&limit=500",
		h.jaeger.BaseURL, service, startUs, endUs,
	)

	resp, err := h.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Data []jaegerTrace `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (h *Handler) buildServiceSummary(svc string) ServiceSummary {
	traces, err := h.fetchTracesForService(svc, time.Hour)
	if err != nil || len(traces) == 0 {
		return ServiceSummary{
			Name:            svc,
			RequestCount:    0,
			ErrorRate:       0,
			P95LatencyMs:    0,
			LatencySparkline: []float64{},
			LastSeen:        time.Now().UTC().Format(time.RFC3339),
		}
	}

	durations := make([]float64, 0, len(traces))
	errors := 0
	sparkRaw := make([]float64, 0, 10)

	for i, tr := range traces {
		if len(tr.Spans) == 0 {
			continue
		}
		root := tr.Spans[0]
		durMs := float64(root.Duration) / 1000.0
		durations = append(durations, durMs)
		if i < 10 {
			sparkRaw = append(sparkRaw, durMs)
		}
		for _, tag := range root.Tags {
			if tag.Key == "error" {
				if v, ok := tag.Value.(bool); ok && v {
					errors++
				}
			}
			if tag.Key == "http.status_code" {
				if code := toFloat(tag.Value); code >= 500 {
					errors++
				}
			}
		}
	}

	sort.Float64s(durations)
	p95 := percentile(durations, 95)
	avg := average(durations)
	errRate := 0.0
	if len(traces) > 0 {
		errRate = float64(errors) / float64(len(traces)) * 100
	}

	return ServiceSummary{
		Name:            svc,
		RequestCount:    len(traces),
		ErrorCount:      errors,
		ErrorRate:       math.Round(errRate*100) / 100,
		P95LatencyMs:    math.Round(p95*100) / 100,
		AvgLatencyMs:    math.Round(avg*100) / 100,
		LatencySparkline: sparkRaw,
		LastSeen:        time.Now().UTC().Format(time.RFC3339),
	}
}

// ─── Build timeseries from real traces ───────────────────────────────────────

func buildMetricsFromTraces(
	service, rangeStr string,
	traces []jaegerTrace,
	lookback time.Duration,
	buckets int,
) ServiceMetricsResponse {

	now := time.Now()
	bucketDur := lookback / time.Duration(buckets)

	// Bucket traces by time
	type bucket struct {
		durations []float64
		errors    int
		total     int
	}
	bkts := make([]bucket, buckets)

	for _, tr := range traces {
		if len(tr.Spans) == 0 {
			continue
		}
		root := tr.Spans[0]
		startT := time.UnixMicro(root.StartTime)
		idx := int(now.Sub(startT) / bucketDur)
		if idx < 0 {
			idx = 0
		}
		if idx >= buckets {
			idx = buckets - 1
		}
		// reverse: idx 0 = most recent
		revIdx := buckets - 1 - idx
		if revIdx < 0 {
			revIdx = 0
		}

		durMs := float64(root.Duration) / 1000.0
		bkts[revIdx].durations = append(bkts[revIdx].durations, durMs)
		bkts[revIdx].total++

		for _, tag := range root.Tags {
			if tag.Key == "error" {
				if v, ok := tag.Value.(bool); ok && v {
					bkts[revIdx].errors++
				}
			}
			if tag.Key == "http.status_code" {
				if code := toFloat(tag.Value); code >= 500 {
					bkts[revIdx].errors++
				}
			}
		}
	}

	latencyTs := make([]LatencyPoint, buckets)
	errorTs := make([]RatePoint, buckets)
	throughputTs := make([]RatePoint, buckets)
	allDurations := make([]float64, 0)

	for i := 0; i < buckets; i++ {
		t := now.Add(-lookback).Add(bucketDur * time.Duration(i)).Format(time.RFC3339)
		b := bkts[i]
		sort.Float64s(b.durations)
		allDurations = append(allDurations, b.durations...)

		p50 := percentile(b.durations, 50)
		p95 := percentile(b.durations, 95)
		p99 := percentile(b.durations, 99)

		latencyTs[i] = LatencyPoint{T: t, P50: p50, P95: p95, P99: p99}

		errRate := 0.0
		if b.total > 0 {
			errRate = float64(b.errors) / float64(b.total) * 100
		}
		errorTs[i] = RatePoint{T: t, Rate: math.Round(errRate*100) / 100}

		rps := float64(b.total) / bucketDur.Seconds()
		throughputTs[i] = RatePoint{T: t, Rate: math.Round(rps*100) / 100}
	}

	sort.Float64s(allDurations)
	percs := map[string]float64{
		"p50": math.Round(percentile(allDurations, 50)*100) / 100,
		"p90": math.Round(percentile(allDurations, 90)*100) / 100,
		"p95": math.Round(percentile(allDurations, 95)*100) / 100,
		"p99": math.Round(percentile(allDurations, 99)*100) / 100,
	}

	return ServiceMetricsResponse{
		Service:              service,
		Range:                rangeStr,
		LatencyTimeseries:    latencyTs,
		ErrorRateTimeseries:  errorTs,
		ThroughputTimeseries: throughputTs,
		Percentiles:          percs,
	}
}

// ─── Mock fallbacks ───────────────────────────────────────────────────────────

func mockServiceList() []ServiceSummary {
	now := time.Now().UTC().Format(time.RFC3339)
	return []ServiceSummary{
		{
			Name: "auth-service", RequestCount: 1420, ErrorCount: 12,
			ErrorRate: 0.85, P95LatencyMs: 145, AvgLatencyMs: 78,
			LatencySparkline: []float64{120, 135, 128, 190, 145, 132, 155, 140, 138, 145},
			LastSeen: now,
		},
		{
			Name: "order-service", RequestCount: 890, ErrorCount: 67,
			ErrorRate: 7.53, P95LatencyMs: 2300, AvgLatencyMs: 820,
			LatencySparkline: []float64{200, 450, 890, 2100, 2300, 1800, 2200, 2400, 2100, 2300},
			LastSeen: now,
		},
		{
			Name: "payment-service", RequestCount: 560, ErrorCount: 3,
			ErrorRate: 0.54, P95LatencyMs: 89, AvgLatencyMs: 55,
			LatencySparkline: []float64{78, 82, 85, 79, 91, 88, 86, 89, 84, 89},
			LastSeen: now,
		},
		{
			Name: "product-service", RequestCount: 2100, ErrorCount: 5,
			ErrorRate: 0.24, P95LatencyMs: 52, AvgLatencyMs: 34,
			LatencySparkline: []float64{45, 48, 52, 49, 51, 53, 50, 48, 52, 51},
			LastSeen: now,
		},
		{
			Name: "user-service", RequestCount: 780, ErrorCount: 2,
			ErrorRate: 0.26, P95LatencyMs: 67, AvgLatencyMs: 43,
			LatencySparkline: []float64{60, 63, 65, 62, 67, 64, 66, 63, 67, 65},
			LastSeen: now,
		},
	}
}

func mockServiceMetrics(service, rangeStr string, buckets int) ServiceMetricsResponse {
	now := time.Now()
	lookback := rangeToDuration(rangeStr)
	bucketDur := lookback / time.Duration(buckets)

	latencyTs := make([]LatencyPoint, buckets)
	errorTs := make([]RatePoint, buckets)
	throughputTs := make([]RatePoint, buckets)

	// Deterministic-looking curves based on service name hash
	seed := float64(len(service))
	for i := 0; i < buckets; i++ {
		t := now.Add(-lookback).Add(bucketDur * time.Duration(i)).Format(time.RFC3339)
		phase := float64(i) / float64(buckets) * math.Pi * 2
		noise := math.Sin(phase+seed)*0.3 + 1.0

		p50 := math.Round((50+20*noise)*10) / 10
		p95 := math.Round((p50*2.2+noise*30)*10) / 10
		p99 := math.Round((p95*1.4+noise*10)*10) / 10

		// Simulate an error spike around 1/3 of the range
		errRate := 0.5 + math.Sin(phase*1.5)*0.3
		if i == buckets/3 {
			errRate = 8.5
			p95 *= 4
			p99 *= 5
		}

		rps := math.Round((80+40*math.Sin(phase+seed+1))*100) / 100

		latencyTs[i] = LatencyPoint{T: t, P50: p50, P95: p95, P99: p99}
		errorTs[i] = RatePoint{T: t, Rate: math.Round(math.Abs(errRate)*100) / 100}
		throughputTs[i] = RatePoint{T: t, Rate: math.Abs(rps)}
	}

	return ServiceMetricsResponse{
		Service:              service,
		Range:                rangeStr,
		LatencyTimeseries:    latencyTs,
		ErrorRateTimeseries:  errorTs,
		ThroughputTimeseries: throughputTs,
		Percentiles: map[string]float64{
			"p50": 52, "p90": 145, "p95": 230, "p99": 480,
		},
	}
}

// ─── Utility ──────────────────────────────────────────────────────────────────

func rangeToDuration(r string) time.Duration {
	switch r {
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "6h":
		return 6 * time.Hour
	case "24h":
		return 24 * time.Hour
	default: // "1h"
		return time.Hour
	}
}

func rangeToPoints(r string) int {
	switch r {
	case "5m":
		return 10
	case "15m":
		return 15
	case "6h":
		return 24
	case "24h":
		return 24
	default:
		return 20
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func average(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func toFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	}
	return 0
}
