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
	"observe-platform/internal/traces"
)

func main() {
	// Load .env file (best-effort)
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── OTel: instrument this platform itself ────────────────────────────────
	tp, err := setupTracer(ctx)
	if err != nil {
		log.Printf("[WARN] OTel tracer setup failed: %v — continuing without tracing", err)
	} else {
		defer func() {
			shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tp.Shutdown(shutCtx)
		}()
	}

	// ── Config from env ──────────────────────────────────────────────────────
	arthurBase  := envOr("ARTHUR_API_URL",  "http://localhost:8420")
	jaegerBase  := envOr("JAEGER_API_URL",  "http://localhost:16686")
	geminiKey   := envOr("GEMINI_API_KEY",  "")
	port        := envOr("PORT",            "8090")
	adminSecret := envOr("ADMIN_SECRET",    "") // empty = open (dev mode)
	keysFile    := envOr("API_KEYS_FILE",   "api_keys.json")

	// ── API-key store (persisted to api_keys.json) ───────────────────────────
	keyStore := auth.NewStore(keysFile)

	// Seed a default key from env so the Flutter app works out of the box
	if defaultKey := envOr("DEFAULT_API_KEY", ""); defaultKey != "" {
		keyStore.Seed(&auth.APIKey{
			Key:         defaultKey,
			Name:        envOr("DEFAULT_KEY_NAME", "Default"),
			Description: "Auto-seeded from DEFAULT_API_KEY env var",
			ArthurURL:   arthurBase,
			JaegerURL:   jaegerBase,
			CreatedAt:   time.Now().UTC(),
		})
		log.Printf("[auth] Default API key seeded: %s...", defaultKey[:min(12, len(defaultKey))])
	}

	// ── Handlers ─────────────────────────────────────────────────────────────
	arthurH := arthur.NewHandler(arthurBase)
	tracesH := traces.NewHandler(jaegerBase)
	aiH     := ai.NewHandler(geminiKey, arthurBase, jaegerBase)
	authH   := auth.NewHandler(keyStore, adminSecret)

	// ── Router ───────────────────────────────────────────────────────────────
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:  []string{"*"},
		AllowMethods:  []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowHeaders:  []string{"Content-Type", "Authorization", "X-API-Key", "X-Admin-Secret"},
		AllowCredentials: false,
	}))
	r.Use(otelgin.Middleware("observe-platform"))

	// ── Public routes (no API key required) ──────────────────────────────────
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "observe-platform",
			"ts":      time.Now().UTC(),
		})
	})

	// ── Admin routes (API key NOT required; protected by X-Admin-Secret) ─────
	admin := r.Group("/admin")
	{
		admin.POST("/keys",        authH.CreateKey) // generate new key
		admin.GET("/keys",         authH.ListKeys)  // list all keys (masked)
		admin.POST("/keys/revoke", authH.RevokeKey) // revoke a key
		admin.DELETE("/keys",      authH.DeleteKey) // permanently delete a key
	}

	// ── Authenticated API routes (API key required) ───────────────────────────
	api := r.Group("/api")
	api.Use(auth.Middleware(keyStore))
	{
		// Flutter verify endpoint — call this first to confirm key is valid
		api.GET("/auth/verify", authH.VerifyKey)

		// Arthur trading API proxy
		api.GET("/arthur/health",       arthurH.Health)
		api.GET("/arthur/snapshot",     arthurH.Snapshot)
		api.GET("/arthur/alerts",       arthurH.Alerts)
		api.GET("/arthur/performance",  arthurH.Performance)
		api.GET("/arthur/positions",    arthurH.Positions)
		api.GET("/arthur/playbooks",    arthurH.Playbooks)
		api.GET("/arthur/context",      arthurH.MarketContext)

		// OTel / Jaeger proxy
		api.GET("/traces",             tracesH.ListTraces)
		api.GET("/traces/:traceID",    tracesH.GetTrace)

		// AI diagnosis
		api.POST("/ai/diagnose",       aiH.Diagnose)
		api.GET("/ai/summary",         aiH.SystemSummary)
	}

	// ── HTTP server ───────────────────────────────────────────────────────────
	srv := &http.Server{Addr: ":" + port, Handler: r}

	go func() {
		log.Printf("[observe-platform] Listening      → :%s", port)
		log.Printf("[observe-platform] Arthur API     → %s", arthurBase)
		log.Printf("[observe-platform] Jaeger         → %s", jaegerBase)
		log.Printf("[observe-platform] Gemini AI      → key_set=%v", geminiKey != "")
		log.Printf("[observe-platform] Admin secret   → set=%v", adminSecret != "")
		log.Printf("[observe-platform] Keys file      → %s", keysFile)
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

// min is needed because Go 1.21+ builtin min isn't guaranteed in all versions.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

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
