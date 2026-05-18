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
	"observe-platform/internal/traces"
)

func main() {

	//load .env
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- OTel setup: instrument the platform itself ---
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

	// --- Config from env (with sensible defaults) ---
	arthurBase := envOr("ARTHUR_API_URL", "http://localhost:8420")
	jaegerBase := envOr("JAEGER_API_URL", "http://localhost:16686")
	geminiKey := envOr("GEMINI_API_KEY", "")
	port := envOr("PORT", "8090")

	// --- Build router ---
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: false,
	}))
	r.Use(otelgin.Middleware("observe-platform"))

	// Handlers
	arthurH := arthur.NewHandler(arthurBase)
	tracesH := traces.NewHandler(jaegerBase)
	aiH := ai.NewHandler(geminiKey, arthurBase, jaegerBase) // ← pass geminiKey
	// Routes: Arthur proxy
	api := r.Group("/api")
	{
		api.GET("/arthur/health", arthurH.Health)
		api.GET("/arthur/snapshot", arthurH.Snapshot)
		api.GET("/arthur/alerts", arthurH.Alerts)
		api.GET("/arthur/performance", arthurH.Performance)
		api.GET("/arthur/positions", arthurH.Positions)
		api.GET("/arthur/playbooks", arthurH.Playbooks)
		api.GET("/arthur/context", arthurH.MarketContext)

		// OTel / Jaeger proxy
		api.GET("/traces", tracesH.ListTraces)
		api.GET("/traces/:traceID", tracesH.GetTrace)

		// AI diagnosis
		api.POST("/ai/diagnose", aiH.Diagnose)
		api.GET("/ai/summary", aiH.SystemSummary)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "observe-platform", "ts": time.Now().UTC()})
	})

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		log.Printf("[observe-platform] Listening on :%s", port)
		log.Printf("[observe-platform] Arthur API → %s", arthurBase)
		log.Printf("[observe-platform] Jaeger     → %s", jaegerBase)
		log.Printf("[observe-platform] AI         → Gemini (key_set=%v)", geminiKey != "") // ← tambah ini
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[observe-platform] shutting down...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
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
