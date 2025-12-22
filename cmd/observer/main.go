// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/handlers"
	"github.com/openchoreo/openchoreo/internal/observer/mcp"
	"github.com/openchoreo/openchoreo/internal/observer/middleware"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/internal/observer/prometheus"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	apiconfig "github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
)

func main() {
	// Create bootstrap logger for early initialization
	bootstrapLogger := createBootstrapLogger()

	// Initialize configuration
	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("Failed to load configuration",
			"error", err,
			"component", "observer-service",
			"phase", "initialization",
		)
		os.Exit(1)
	}

	// Initialize logger with proper configuration
	logger := initLogger(cfg.LogLevel)
	logger.Info("Configuration loaded successfully", "log_level", cfg.LogLevel)

	// Initialize OpenSearch client
	osClient, err := opensearch.NewClient(&cfg.OpenSearch, logger)
	if err != nil {
		log.Fatalf("Failed to initialize OpenSearch client: %v", err)
	}

	// Initialize Prometheus client
	promClient, err := prometheus.NewClient(&cfg.Prometheus, logger)
	if err != nil {
		log.Fatalf("Failed to initialize Prometheus client: %v", err)
	}

	// Initialize metrics service
	metricsService := prometheus.NewMetricsService(promClient, logger)

	// Initialize logging service
	loggingService := service.NewLoggingService(osClient, metricsService, cfg, logger)

	// Initialize HTTP server
	mux := http.NewServeMux()

	// Initialize handlers
	handler := handlers.NewHandler(loggingService, logger, cfg.Alerting.WebhookSecret)

	// Health check endpoint (no JWT authentication)
	mux.HandleFunc("GET /health", handler.Health)

	// API routes - Build Logs
	mux.HandleFunc("POST /api/logs/build/{buildId}", handler.GetBuildLogs)

	// API routes - Logs
	mux.HandleFunc("POST /api/logs/component/{componentId}", handler.GetComponentLogs)
	mux.HandleFunc("POST /api/logs/project/{projectId}", handler.GetProjectLogs)
	mux.HandleFunc("POST /api/logs/gateway", handler.GetGatewayLogs)
	mux.HandleFunc("POST /api/logs/org/{orgId}", handler.GetOrganizationLogs)

	// API routes - Traces
	mux.HandleFunc("POST /api/traces", handler.GetTraces)

	// API routes - Metrics
	mux.HandleFunc("POST /api/metrics/component/http", handler.GetComponentHTTPMetrics)
	mux.HandleFunc("POST /api/metrics/component/usage", handler.GetComponentResourceMetrics)

	// API routes - Alerting
	mux.HandleFunc("PUT /api/alerting/rule/{sourceType}/{ruleName}", handler.UpsertAlertingRule)
	mux.HandleFunc("DELETE /api/alerting/rule/{sourceType}/{ruleName}", handler.DeleteAlertingRule)
	mux.HandleFunc("POST /api/alerting/webhook/{secret}", handler.AlertingWebhook) // Internal webhook for alerting

	// MCP endpoint
	mux.Handle("/mcp", mcp.NewHTTPServer(&mcp.MCPHandler{Service: loggingService}))

	// Initialize JWT middleware
	jwtAuth := initJWTMiddleware(logger)

	// Create a custom middleware that applies JWT only to non-health endpoints
	jwtForAPIOnly := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip JWT for health endpoint
			if r.Method == "GET" && r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}
			// Apply JWT for all other endpoints
			jwtAuth(next).ServeHTTP(w, r)
		})
	}

	// Apply middleware with selective JWT
	handlerWithMiddleware := middleware.Chain(
		middleware.Logger(logger),
		middleware.Recovery(logger),
		jwtForAPIOnly,
	)(mux)

	// Create HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      handlerWithMiddleware,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server
	go func() {
		logger.Info("Starting server", "address", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown using signal context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Wait for interrupt signal
	<-ctx.Done()

	logger.Info("Shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	logger.Info("Server shutdown complete")
}

func initLogger(level string) *slog.Logger {
	var logLevel slog.Level

	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	// Use JSON handler for production, text handler for debug
	var handler slog.Handler
	if level == "debug" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// createBootstrapLogger creates a minimal logger for early initialization
func createBootstrapLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	// Use JSON handler for structured logging
	handler := slog.NewJSONHandler(os.Stderr, opts)
	return slog.New(handler)
}

// initJWTMiddleware initializes the JWT authentication middleware with configuration from environment
func initJWTMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	jwtDisabled := os.Getenv(apiconfig.EnvJWTDisabled) == "true"
	jwksURL := os.Getenv(apiconfig.EnvJWKSURL)
	jwtIssuer := os.Getenv(apiconfig.EnvJWTIssuer)
	jwtAudience := os.Getenv(apiconfig.EnvJWTAudience)
	jwksURLTLSInsecureSkipVerify := os.Getenv(apiconfig.EnvJWKSURLTLSInsecureSkipVerify) == "true"

	// Configure JWT middleware
	config := jwt.Config{
		Disabled:                     jwtDisabled,
		JWKSURL:                      jwksURL,
		ValidateIssuer:               jwtIssuer,
		ValidateAudience:             jwtAudience,
		JWKSURLTLSInsecureSkipVerify: jwksURLTLSInsecureSkipVerify,
		TokenLookup:                  "header:x-openchoreo-token",
		Logger:                       logger,
	}

	return jwt.Middleware(config)
}
