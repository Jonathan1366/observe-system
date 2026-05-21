package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"observe-platform/internal/ai"
	"observe-platform/internal/arthur"
	"observe-platform/internal/auth"
	"observe-platform/internal/config"
	"observe-platform/internal/metrics"
	"observe-platform/internal/traces"
)

func main() {
	// Load .env file (best-effort — ok if missing)
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── OTel: self-instrument this platform ──────────────────────────────────
	tp, err := setupTracer(ctx)
	if err != nil {
		log.Printf("[WARN] OTel tracer setup failed: %v — continuing without self-tracing", err)
	} else {
		defer func() {
			shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tp.Shutdown(shutCtx)
		}()
	}

	// ── Typed config from env ────────────────────────────────────────────────
	geminiCfg := config.LoadGemini()
	arthurCfg := config.LoadArthur()
	jaegerCfg := config.LoadJaeger()

	port        := envOr("PORT",          "8090")
	adminSecret := envOr("ADMIN_SECRET",  "")
	keysFile    := envOr("API_KEYS_FILE", "api_keys.json")

	// ── API-key store ────────────────────────────────────────────────────────
	keyStore := auth.NewStore(keysFile)
	if defaultKey := envOr("DEFAULT_API_KEY", ""); defaultKey != "" {
		keyStore.Seed(&auth.APIKey{
			Key:         defaultKey,
			Name:        envOr("DEFAULT_KEY_NAME", "Default"),
			Description: "Auto-seeded from DEFAULT_API_KEY env var",
			ArthurURL:   arthurCfg.BaseURL,
			JaegerURL:   jaegerCfg.BaseURL,
			CreatedAt:   time.Now().UTC(),
		})
		pfx := defaultKey
		if len(pfx) > 12 {
			pfx = pfx[:12]
		}
		log.Printf("[auth] Default API key seeded: %s...", pfx)
	}

	// ── Handlers ─────────────────────────────────────────────────────────────
	arthurH  := arthur.NewHandler(arthurCfg.BaseURL)
	tracesH  := traces.NewHandler(jaegerCfg.BaseURL)
	aiH      := ai.NewHandler(geminiCfg, arthurCfg)
	metricsH := metrics.NewHandler(jaegerCfg, arthurCfg)
	authH    := auth.NewHandler(keyStore, adminSecret)

	// ── Router ───────────────────────────────────────────────────────────────
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization", "X-API-Key", "X-Admin-Secret"},
		AllowCredentials: false,
	}))
	r.Use(otelgin.Middleware("observe-platform"))

	// ── Public ───────────────────────────────────────────────────────────────
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "observe-platform",
			"ts":      time.Now().UTC(),
		})
	})

	// ── Admin (protected by X-Admin-Secret, no API key needed) ───────────────
	adm := r.Group("/admin")
	{
		adm.POST("/keys",        authH.CreateKey)
		adm.GET("/keys",         authH.ListKeys)
		adm.POST("/keys/revoke", authH.RevokeKey)
		adm.DELETE("/keys",      authH.DeleteKey)
	}

	// ── Authenticated API (API key required for all routes below) ────────────
	api := r.Group("/api")
	api.Use(auth.Middleware(keyStore))
	{
		// ── Auth ──────────────────────────────────────────────────────────────
		api.GET("/auth/verify", authH.VerifyKey)

		// ── Arthur proxy (trading backend) ───────────────────────────────────
		api.GET("/arthur/health",      arthurH.Health)
		api.GET("/arthur/snapshot",    arthurH.Snapshot)
		api.GET("/arthur/alerts",      arthurH.Alerts)
		api.GET("/arthur/performance", arthurH.Performance)
		api.GET("/arthur/positions",   arthurH.Positions)
		api.GET("/arthur/playbooks",   arthurH.Playbooks)
		api.GET("/arthur/context",     arthurH.MarketContext)

		// ── OTel / Jaeger traces ──────────────────────────────────────────────
		api.GET("/traces",          tracesH.ListTraces)
		api.GET("/traces/:traceID", tracesH.GetTrace)

		// ── Service metrics (for dashboard charts) ────────────────────────────
		// GET /api/metrics/services           → list all services + RED stats
		// GET /api/metrics/service/:name      → timeseries for one service
		api.GET("/metrics/services",        metricsH.ListServices)
		api.GET("/metrics/service/:name",   metricsH.ServiceMetrics)

		// ── AI endpoints ─────────────────────────────────────────────────────
		// POST /api/ai/diagnose  → RCA from trace/span data
		// GET  /api/ai/summary   → overall system health summary
		// POST /api/ai/insight   → contextual insight for a dashboard section
		api.POST("/ai/diagnose", aiH.Diagnose)
		api.GET("/ai/summary",   aiH.SystemSummary)
		api.POST("/ai/insight",  aiH.SectionInsight)
	}

	// ── HTTP server ───────────────────────────────────────────────────────────
	srv := &http.Server{Addr: ":" + port, Handler: r}

	go func() {
		log.Printf("[observe-platform] Listening   → :%s", port)
		log.Printf("[observe-platform] Arthur API  → %s", arthurCfg.BaseURL)
		log.Printf("[observe-platform] Jaeger      → %s", jaegerCfg.BaseURL)
		log.Printf("[observe-platform] Gemini AI   → key_set=%v", geminiCfg.APIKey != "")
		log.Printf("[observe-platform] Admin       → secret_set=%v", adminSecret != "")
		log.Printf("[observe-platform] Keys file   → %s", keysFile)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[observe-platform] shutting down…")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}

// ─── OTel self-instrumentation ────────────────────────────────────────────────

func setupTracer(ctx context.Context) (*sdktrace.TracerProvider, error) {
	endpoint := envOr("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	res, _ := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("observe-platform"),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
