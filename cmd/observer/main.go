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

	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	k8s "github.com/openchoreo/openchoreo/internal/observer/clients"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/handlers"
	"github.com/openchoreo/openchoreo/internal/observer/mcp"
	observermiddleware "github.com/openchoreo/openchoreo/internal/observer/middleware"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/internal/observer/prometheus"
	"github.com/openchoreo/openchoreo/internal/observer/service"
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

	// Initialize metrics service
	metricsService := prometheus.NewMetricsService(promClient, logger)

	// Initialize logs backend (optional - based on experimental flag)
	var logsBackend observability.LogsBackend
	if cfg.Experimental.UseLogsBackend {
		logger.Info("Experimental feature active: Using logs backend",
			"backend_url", cfg.Experimental.LogsBackendURL)

		// Initialize HTTP-based backend (e.g., OpenObserve)
		backendConfig := service.LogsBackendConfig{
			BaseURL: cfg.Experimental.LogsBackendURL,
			Timeout: cfg.Experimental.LogsBackendTimeout,
		}
		logsBackend = service.NewLogsBackend(backendConfig)
		logger.Info("Logs backend initialized")
	} else {
		logger.Info("Using OpenSearch for component logs")
	}

	// Initialize logging service
	loggingService := service.NewLoggingService(osClient, metricsService, k8sClient, cfg, logger, logsBackend)

	// Initialize authz client
	authzClient, err := observerAuthz.NewClient(&cfg.Authz, logger.With("component", "authz-client"))
	if err != nil {
		logger.Error("Failed to create authz client", "error", err)
		os.Exit(1)
	}

	// Initialize HTTP server
	mux := http.NewServeMux()

	// Initialize handlers
	handler := handlers.NewHandler(
		loggingService, logger, authzClient, cfg.Alerting.RCAServiceURL,
	)

	// ===== Initialize Middlewares =====

	// Global middlewares - applies to all routes
	loggerMiddleware := observermiddleware.Logger(logger)
	recoveryMiddleware := observermiddleware.Recovery(logger)

	// Create route builder with global middleware
	routes := middleware.NewRouteBuilder(mux).With(loggerMiddleware, recoveryMiddleware)

	// ===== Public Routes (No Authentication Required) =====

	// Health check endpoint
	routes.HandleFunc("GET /health", handler.Health)

	// OAuth Protected Resource Metadata endpoint
	routes.HandleFunc("GET /.well-known/oauth-protected-resource", oauthProtectedResourceMetadata(logger))

	// ===== Internal Routes (No Authentication Required) =====
	// TODO: Expose through a separate route group
	routes.HandleFunc("PUT /api/alerting/rule/{sourceType}/{ruleName}", handler.UpsertAlertingRule)
	routes.HandleFunc("DELETE /api/alerting/rule/{sourceType}/{ruleName}", handler.DeleteAlertingRule)

	// ===== Vendor-specific Alerting Webhook Endpoint (No JWT Authentication) =====
	// TODO: Expose through a separate route group
	routes.HandleFunc("POST /api/alerting/webhook/{alertSource}", handler.AlertingWebhook)

	// ===== Protected API Routes (JWT Authentication Required) =====

	// Initialize JWT middleware
	jwtAuth := initJWTMiddleware(cfg, logger)

	// Create protected route group with JWT auth
	api := routes.With(jwtAuth)

	// API routes - Build Logs
	api.HandleFunc("GET /api/v1/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/"+
		"workflow-runs/{runName}/logs", handler.GetComponentWorkflowRunLogs)
	api.HandleFunc("POST /api/logs/build/{buildId}", handler.GetBuildLogs) // TODO: Deprecate this endpoint
	api.HandleFunc("GET /api/v1/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/"+
		"workflow-runs/{runName}/events", handler.GetComponentWorkflowRunEvents)

	// API routes - Workflow Run Logs
	api.HandleFunc("POST /api/v1/workflow-runs/{runId}/logs", handler.GetWorkflowRunLogs)

	// API routes - Logs
	api.HandleFunc("POST /api/logs/component/{componentId}", handler.GetComponentLogs)
	api.HandleFunc("POST /api/logs/project/{projectId}", handler.GetProjectLogs)
	api.HandleFunc("POST /api/logs/gateway", handler.GetGatewayLogs)
	api.HandleFunc("POST /api/logs/namespace/{namespaceName}", handler.GetNamespaceLogs)

	// API routes - Traces
	api.HandleFunc("POST /api/traces", handler.GetTraces)

	// API routes - Metrics
	api.HandleFunc("POST /api/metrics/component/http", handler.GetComponentHTTPMetrics)
	api.HandleFunc("POST /api/metrics/component/usage", handler.GetComponentResourceMetrics)

	// MCP endpoint with chained middleware (logger -> recovery -> auth401 -> jwt -> handler)
	mcpMiddleware := initMCPMiddleware(logger)
	mcpRoutes := routes.Group(mcpMiddleware, jwtAuth)
	mcpRoutes.Handle("/mcp", mcp.NewHTTPServer(&mcp.MCPHandler{Service: loggingService}))

	// Create HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
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
