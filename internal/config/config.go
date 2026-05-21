// Package config centralises all environment-driven configuration
// for observe-platform so every handler receives a typed struct
// instead of raw strings.
package config

import (
	"os"
	"time"
)

// ── Gemini ───────────────────────────────────────────────────────────────────

type GeminiConfig struct {
	APIKey          string
	APIURL          string
	Temperature     float64
	MaxOutputTokens int
	Timeout         time.Duration
}

func LoadGemini() GeminiConfig {
	return GeminiConfig{
		APIKey:          envOr("GEMINI_API_KEY", ""),
		APIURL:          envOr("GEMINI_API_URL", "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"),
		Temperature:     0.1,
		MaxOutputTokens: 1500,
		Timeout:         30 * time.Second,
	}
}

// ── Arthur ───────────────────────────────────────────────────────────────────

type ArthurConfig struct {
	BaseURL      string
	HealthPath   string
	SnapshotPath string
	NewsPath     string
	Timeout      time.Duration
}

func LoadArthur() ArthurConfig {
	return ArthurConfig{
		BaseURL:      envOr("ARTHUR_API_URL", "http://localhost:8420"),
		HealthPath:   "/health",
		SnapshotPath: "/market/snapshot",
		NewsPath:     "/market/news",
		Timeout:      15 * time.Second,
	}
}

// ── Jaeger ───────────────────────────────────────────────────────────────────

type JaegerConfig struct {
	BaseURL string
	Timeout time.Duration
}

func LoadJaeger() JaegerConfig {
	return JaegerConfig{
		BaseURL: envOr("JAEGER_API_URL", "http://localhost:16686"),
		Timeout: 10 * time.Second,
	}
}

// ── helper ───────────────────────────────────────────────────────────────────

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
