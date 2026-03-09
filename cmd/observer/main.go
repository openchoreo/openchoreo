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
	"sync"
	"syscall"

	apihandler "github.com/openchoreo/openchoreo/internal/observer/api/handlers"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	k8s "github.com/openchoreo/openchoreo/internal/observer/clients"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	observermcp "github.com/openchoreo/openchoreo/internal/observer/mcp"
	observermiddleware "github.com/openchoreo/openchoreo/internal/observer/middleware"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/internal/observer/prometheus"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/store/alertentry"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
	apiconfig "github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
	mcpmiddleware "github.com/openchoreo/openchoreo/internal/server/middleware/mcp"
	"github.com/openchoreo/openchoreo/internal/server/oauth"
	"github.com/openchoreo/openchoreo/pkg/observability"
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

	// Initialize Kubernetes client for fetching notification channel configs
	k8sClient, err := k8s.NewK8sClient()
	if err != nil {
		logger.Warn("Failed to initialize Kubernetes client, alert notifications will be disabled",
			"error", err)
		// Continue without k8s client - notifications will be skipped
	}

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

	// Initialize Prometheus metrics service
	// TODO: Remove this once the metrics adapter is implemented
	promService := prometheus.NewMetricsService(promClient, logger)

	// Initialize logs adapter (optional)
	var logsAdapter observability.LogsAdapter
	if cfg.Adapters.LogsAdapterEnabled {
		logger.Info("Using logs adapter",
			"adapter_url", cfg.Adapters.LogsAdapterURL)

		// Initialize HTTP-based adapter (e.g., OpenObserve)
		adapterConfig := service.LogsAdapterConfig{
			BaseURL: cfg.Adapters.LogsAdapterURL,
			Timeout: cfg.Adapters.LogsAdapterTimeout,
		}
		logsAdapter = service.NewLogsAdapter(adapterConfig)
		logger.Info("Logs adapter initialized")
	} else {
		logger.Info("Using OpenSearch for component logs")
	}

	// Initialize tracing adapter (optional)
	var tracingAdapter observability.TracingAdapter
	if cfg.Adapters.TracingAdapterEnabled {
		logger.Info("Using tracing adapter",
			"adapter_url", cfg.Adapters.TracingAdapterURL)

		adapterConfig := service.TracingAdapterConfig{
			BaseURL: cfg.Adapters.TracingAdapterURL,
			Timeout: cfg.Adapters.TracingAdapterTimeout,
		}
		var adapterErr error
		tracingAdapter, adapterErr = service.NewTracingAdapter(adapterConfig)
		if adapterErr != nil {
			logger.Error("Failed to create tracing adapter", "error", adapterErr)
			os.Exit(1)
		}
		logger.Info("Tracing adapter initialized")
	} else {
		logger.Info("Using OpenSearch for traces")
	}

	// Initialize authz client
	authzClient, err := observerAuthz.NewClient(&cfg.Authz, logger.With("component", "authz-client"))
	if err != nil {
		logger.Error("Failed to create authz client", "error", err)
		os.Exit(1)
	}

	// Initialize HTTP server
	mux := http.NewServeMux()

	// Initialize resource UID resolver for name-to-UID resolution
	uidResolver := service.NewResourceUIDResolver(&cfg.UIDResolver, logger.With("component", "resource-resolver"))

	// Initialize logs service
	logsService, logsServiceErr := service.NewLogsService(
		logsAdapter, uidResolver, cfg, logger.With("component", "logs-service"),
	)
	if logsServiceErr != nil {
		logger.Error("Failed to initialize logs service", "error", logsServiceErr)
		os.Exit(1)
	}

	// Initialize metrics service
	metricsService, metricsServiceErr := service.NewMetricsService(
		promService, uidResolver, logger.With("component", "metrics-service"),
	)
	if metricsServiceErr != nil {
		logger.Error("Failed to initialize metrics service", "error", metricsServiceErr)
		os.Exit(1)
	}

	// Initialize traces service
	tracesService, tracesServiceErr := service.NewTracesService(
		tracingAdapter, uidResolver, cfg, logger.With("component", "traces-service"),
	)
	if tracesServiceErr != nil {
		logger.Error("Failed to initialize traces service", "error", tracesServiceErr)
		os.Exit(1)
	}
	logger.Info("Traces service initialized")

	// Initialize health service
	healthService, healthServiceErr := service.NewHealthService(logger.With("component", "health-service"))
	if healthServiceErr != nil {
		logger.Error("Failed to initialize health service", "error", healthServiceErr)
		os.Exit(1)
	}

	alertEntryStore, err := alertentry.New(
		cfg.Alerting.AlertStoreBackend,
		cfg.Alerting.AlertStoreDSN,
		logger.With("component", "alert-entry-store"),
	)
	if err != nil {
		log.Fatalf("Failed to initialize alert entry store: %v", err)
	}
	if err := alertEntryStore.Initialize(context.Background()); err != nil {
		log.Fatalf("Failed to initialize alert entry store schema: %v", err)
	}
	defer func() {
		if closeErr := alertEntryStore.Close(); closeErr != nil {
			logger.Error("Failed to close alert entry store", "error", closeErr)
		}
	}()

	incidentEntryStore, err := incidententry.New(
		cfg.Alerting.AlertStoreBackend,
		cfg.Alerting.AlertStoreDSN,
		logger.With("component", "incident-entry-store"),
	)
	if err != nil {
		log.Fatalf("Failed to initialize incident entry store: %v", err)
	}
	if err := incidentEntryStore.Initialize(context.Background()); err != nil {
		log.Fatalf("Failed to initialize incident entry store schema: %v", err)
	}
	defer func() {
		if closeErr := incidentEntryStore.Close(); closeErr != nil {
			logger.Error("Failed to close incident entry store", "error", closeErr)
		}
	}()

	// Initialize alert service for the internal v1alpha1 API
	alertService := service.NewAlertService(
		osClient,
		opensearch.NewQueryBuilder(cfg.OpenSearch.IndexPrefix),
		alertEntryStore,
		incidentEntryStore,
		k8sClient,
		cfg,
		logger.With("component", "alert-service"),
		cfg.Alerting.RCAServiceURL,
		cfg.Alerting.AIRCAEnabled,
	)

	// Wrap services with authorization checks.
	// Both the API handler and MCP handler share the same authz-wrapped instances
	// so authorization logic is enforced once, in the service layer.
	authzLogsService := service.NewLogsServiceWithAuthz(logsService, authzClient, logger.With("component", "authz-logs"))
	authzMetricsService := service.NewMetricsServiceWithAuthz(
		metricsService, authzClient, logger.With("component", "authz-metrics"))
	authzTracesService := service.NewTracesServiceWithAuthz(
		tracesService, authzClient, logger.With("component", "authz-traces"))

	// Initialize new API handler
	newAPIHandler := apihandler.NewHandler(
		healthService,
		authzLogsService,
		authzMetricsService,
		alertService,
		authzTracesService,
		logger.With("component", "api-handler"),
	)

	// ===== Initialize Middlewares =====

	// Global middlewares - applies to all routes
	loggerMiddleware := observermiddleware.Logger(logger)
	recoveryMiddleware := observermiddleware.Recovery(logger)

	// Create route builder with global middleware
	routes := middleware.NewRouteBuilder(mux).With(loggerMiddleware, recoveryMiddleware)

	// ===== Public Routes (No Authentication Required) =====

	// Health check endpoint (new API)
	routes.HandleFunc("GET /health", newAPIHandler.Health)

	// OAuth Protected Resource Metadata endpoint
	routes.HandleFunc("GET /.well-known/oauth-protected-resource", oauthProtectedResourceMetadata(logger))

	// ===== Protected API Routes (JWT Authentication Required) =====

	// Initialize JWT middleware
	jwtAuth := initJWTMiddleware(cfg, logger)

	// Create protected route group with JWT auth
	api := routes.With(jwtAuth)

	// ===== New API Routes (v1) =====
	api.HandleFunc("POST /api/v1/logs/query", newAPIHandler.QueryLogs)
	api.HandleFunc("POST /api/v1/metrics/query", newAPIHandler.QueryMetrics)

	// ===== New API Routes (v1alpha1) - Traces =====
	api.HandleFunc("POST /api/v1alpha1/traces/query", newAPIHandler.QueryTraces)
	api.HandleFunc("POST /api/v1alpha1/traces/{traceId}/spans/query", newAPIHandler.QuerySpansForTrace)
	api.HandleFunc("GET /api/v1alpha1/traces/{traceId}/spans/{spanId}", newAPIHandler.GetSpanDetailsForTrace)

	// Initialize new MCP handler backed by the authz-wrapped service layer
	newMCPHandler, err := observermcp.NewMCPHandler(
		healthService,
		authzLogsService,
		authzMetricsService,
		alertService,
		authzTracesService,
		logger.With("component", "mcp-handler"),
	)
	if err != nil {
		log.Fatalf("Failed to create MCP handler: %v", err)
	}
	newMCPServer := observermcp.NewHTTPServer(newMCPHandler)

	// MCP endpoint with chained middleware (logger -> recovery -> auth401 -> jwt -> handler)
	mcpMiddleware := initMCPMiddleware(logger)
	mcpRoutes := routes.Group(mcpMiddleware, jwtAuth)
	mcpRoutes.Handle("/mcp", newMCPServer)

	// Create HTTP server
	// CORS wraps the entire mux so it intercepts OPTIONS preflight requests
	// before the mux's method-based routing returns 405.
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      observermiddleware.CORS(cfg.CORS.AllowedOrigins)(mux),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// ===== Internal Server (port 8081) — v1alpha1 alert CRUD =====
	internalMux := http.NewServeMux()
	internalRoutes := middleware.NewRouteBuilder(internalMux).With(loggerMiddleware, recoveryMiddleware)
	internalRoutes.HandleFunc(
		"POST /api/v1alpha1/alerts/sources/{sourceType}/rules", newAPIHandler.CreateAlertRule)
	internalRoutes.HandleFunc(
		"GET /api/v1alpha1/alerts/sources/{sourceType}/rules/{ruleName}", newAPIHandler.GetAlertRule)
	internalRoutes.HandleFunc(
		"PUT /api/v1alpha1/alerts/sources/{sourceType}/rules/{ruleName}", newAPIHandler.UpdateAlertRule)
	internalRoutes.HandleFunc(
		"DELETE /api/v1alpha1/alerts/sources/{sourceType}/rules/{ruleName}", newAPIHandler.DeleteAlertRule)

	// ===== v1alpha1 Alert Webhook Endpoint  =====
	internalRoutes.HandleFunc("POST /api/v1alpha1/alerts/webhook", newAPIHandler.HandleAlertWebhook)

	internalAddr := fmt.Sprintf(":%d", cfg.Server.InternalPort)
	internalServer := &http.Server{
		Addr:         internalAddr,
		Handler:      internalMux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start main server
	go func() {
		logger.Info("Starting server", "address", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Start internal server
	go func() {
		logger.Info("Starting internal server", "address", internalAddr)
		if err := internalServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start internal server: %v", err)
		}
	}()

	// Graceful shutdown using signal context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Wait for interrupt signal
	<-ctx.Done()

	logger.Info("Shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Main server forced to shutdown: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := internalServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("Internal server forced to shutdown: %v", err)
		}
	}()

	wg.Wait()
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
func initJWTMiddleware(cfg *config.Config, logger *slog.Logger) func(http.Handler) http.Handler {
	jwtDisabled := os.Getenv(apiconfig.EnvJWTDisabled) == "true"
	jwksURL := os.Getenv(apiconfig.EnvJWKSURL)
	jwtIssuer := os.Getenv(apiconfig.EnvJWTIssuer)
	jwtAudience := os.Getenv(apiconfig.EnvJWTAudience)
	jwksURLTLSInsecureSkipVerify := os.Getenv(apiconfig.EnvJWKSURLTLSInsecureSkipVerify) == "true"

	// Convert single audience string to slice (for backward compatibility)
	var jwtAudiences []string
	if jwtAudience != "" {
		jwtAudiences = []string{jwtAudience}
	}

	// Create OAuth2 user type detector from configuration
	var detector *jwt.Resolver
	if len(cfg.Auth.UserTypes) > 0 {
		var err error
		detector, err = jwt.NewResolver(cfg.Auth.UserTypes)
		if err != nil {
			logger.Error("Failed to create JWT subject resolver", "error", err)
		} else {
			logger.Info("JWT subject resolver initialized", "user_types_count", len(cfg.Auth.UserTypes))
		}
	}

	// Configure JWT middleware
	jwtConfig := jwt.Config{
		Disabled:                     jwtDisabled,
		JWKSURL:                      jwksURL,
		ValidateIssuer:               jwtIssuer,
		ValidateAudiences:            jwtAudiences,
		JWKSURLTLSInsecureSkipVerify: jwksURLTLSInsecureSkipVerify,
		Detector:                     detector,
		Logger:                       logger,
	}

	return jwt.Middleware(jwtConfig)
}

// initMCPMiddleware initializes the MCP middleware that adds WWW-Authenticate header to 401 responses
func initMCPMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	// Get observer base URL from environment variables
	observerBaseURL := os.Getenv("OBSERVER_BASE_URL")
	if observerBaseURL == "" {
		// Default to localhost for development
		observerBaseURL = "http://localhost:9097"
		logger.Warn("OBSERVER_BASE_URL not set, using default", "url", observerBaseURL)
	}
	resourceMetadataURL := observerBaseURL + "/.well-known/oauth-protected-resource"

	return mcpmiddleware.Auth401Interceptor(resourceMetadataURL)
}

// oauthProtectedResourceMetadata returns a handler for OAuth 2.0 protected resource metadata
// as defined in RFC 9728 and related OAuth standards
func oauthProtectedResourceMetadata(logger *slog.Logger) http.HandlerFunc {
	// Get configuration from environment variables
	observerBaseURL := os.Getenv("OBSERVER_BASE_URL")
	if observerBaseURL == "" {
		// Default to localhost for development
		observerBaseURL = "http://localhost:9097"
		logger.Warn("OBSERVER_BASE_URL not set, using default", "url", observerBaseURL)
	}

	authServerBaseURL := os.Getenv(apiconfig.EnvAuthServerBaseURL)
	if authServerBaseURL == "" {
		authServerBaseURL = apiconfig.DefaultThunderBaseURL
	}

	// Create and return metadata handler
	return oauth.NewMetadataHandler(oauth.MetadataHandlerConfig{
		ResourceName: "OpenChoreo Observer MCP Server",
		ResourceURL:  observerBaseURL + "/mcp",
		AuthorizationServers: []string{
			authServerBaseURL,
		},
		Logger: logger,
	})
}
