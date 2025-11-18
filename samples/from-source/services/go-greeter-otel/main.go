package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var tracer trace.Tracer

// initTracer 初始化 OpenTelemetry tracer
func initTracer(ctx context.Context) (*sdktrace.TracerProvider, error) {
	// 從環境變數獲取配置
	endpoint := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT",
		"clickstack-collector.openchoreo-observability-plane.svc.cluster.local:4317")
	serviceName := getEnv("OTEL_SERVICE_NAME", "greeter-service")
	serviceVersion := getEnv("SERVICE_VERSION", "1.0.0")
	deploymentEnv := getEnv("DEPLOYMENT_ENV", "development")

	log.Printf("Initializing OpenTelemetry: endpoint=%s, service=%s", endpoint, serviceName)

	// 為 exporter 建立超時 context，避免 collector 不可用時一直阻塞
	exportCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 創建 OTLP gRPC exporter
	var exporterOpts []otlptracegrpc.Option
	exporterOpts = append(exporterOpts,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithTLSCredentials(insecure.NewCredentials()),
		otlptracegrpc.WithDialOption(grpc.WithBlock()),
	)

	if apiKey := os.Getenv("HYPERDX_API_KEY"); apiKey != "" {
		headers := map[string]string{
			"x-api-key":     apiKey,
			"Authorization": apiKey,
		}
		exporterOpts = append(exporterOpts, otlptracegrpc.WithHeaders(headers))
	}

	exporter, err := otlptracegrpc.New(exportCtx, exporterOpts...)
	if err != nil {
		return nil, err
	}

	// 創建 resource（服務元數據）
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
			semconv.DeploymentEnvironmentKey.String(deploymentEnv),
		),
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, err
	}

	// 創建 TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithMaxExportBatchSize(512),
			sdktrace.WithBatchTimeout(5*time.Second),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// 設置全局 TracerProvider 和 Propagator
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Printf("OpenTelemetry initialized successfully")
	return tp, nil
}

type GreetResponse struct {
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	TraceID   string `json:"trace_id,omitempty"`
}

func main() {
	ctx := context.Background()

	// 初始化 OpenTelemetry
	tp, err := initTracer(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.Printf("Error shutting down tracer: %v", err)
		}
	}()

	// 獲取 tracer 實例
	tracer = otel.Tracer("greeter-service")

	// 創建 HTTP 路由
	mux := http.NewServeMux()

	// 包裝所有 handlers 以自動創建 spans
	mux.Handle("/greeter/greet", otelhttp.NewHandler(
		http.HandlerFunc(greetHandler),
		"greet",
		otelhttp.WithSpanOptions(trace.WithSpanKind(trace.SpanKindServer)),
	))

	mux.Handle("/greeter/health", otelhttp.NewHandler(
		http.HandlerFunc(healthHandler),
		"health",
	))

	// 啟動 HTTP Server
	port := getEnv("PORT", "9090")
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 優雅關閉
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()

	log.Printf("🚀 Greeter service starting on port %s", port)
	log.Printf("📊 Traces will be sent to OTEL Collector")
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}

// greetHandler 處理問候請求
func greetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)

	// 獲取查詢參數
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "World"
	}

	// 添加 span attributes
	span.SetAttributes(
		attribute.String("user.name", name),
		attribute.String("http.method", r.Method),
		attribute.String("http.path", r.URL.Path),
	)

	// 創建子 span 來追蹤業務邏輯
	greeting := generateGreetingWithSpan(ctx, name)

	// 獲取 trace ID 用於響應
	traceID := span.SpanContext().TraceID().String()

	// 構建響應
	response := GreetResponse{
		Message:   greeting,
		Timestamp: time.Now().Format(time.RFC3339),
		TraceID:   traceID,
	}

	// 記錄事件
	span.AddEvent("greeting_generated", trace.WithAttributes(
		attribute.String("greeting", greeting),
	))

	// 返回 JSON 響應
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Trace-ID", traceID)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// generateGreetingWithSpan 生成問候語並創建 span
func generateGreetingWithSpan(ctx context.Context, name string) string {
	ctx, span := tracer.Start(ctx, "generate-greeting",
		trace.WithAttributes(attribute.String("input.name", name)),
	)
	defer span.End()

	// 模擬一些處理時間
	time.Sleep(10 * time.Millisecond)

	greeting := formatGreeting(ctx, name)

	span.SetAttributes(attribute.String("output.greeting", greeting))
	return greeting
}

// formatGreeting 格式化問候語
func formatGreeting(ctx context.Context, name string) string {
	_, span := tracer.Start(ctx, "format-greeting")
	defer span.End()

	// 簡單的格式化邏輯
	currentHour := time.Now().Hour()
	var timeOfDay string

	switch {
	case currentHour < 12:
		timeOfDay = "Good morning"
	case currentHour < 18:
		timeOfDay = "Good afternoon"
	default:
		timeOfDay = "Good evening"
	}

	span.SetAttributes(attribute.String("time_of_day", timeOfDay))
	return timeOfDay + ", " + name + "!"
}

// healthHandler 健康檢查端點
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// getEnv 從環境變數獲取值，如果不存在則返回預設值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
