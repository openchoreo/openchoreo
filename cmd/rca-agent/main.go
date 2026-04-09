// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers/openai"

	observermiddleware "github.com/openchoreo/openchoreo/internal/observer/middleware"
	"github.com/openchoreo/openchoreo/internal/rca-agent/api"
	rcaAuthz "github.com/openchoreo/openchoreo/internal/rca-agent/authz"
	"github.com/openchoreo/openchoreo/internal/rca-agent/config"
	"github.com/openchoreo/openchoreo/internal/rca-agent/service"
	"github.com/openchoreo/openchoreo/internal/rca-agent/store"
	"github.com/openchoreo/openchoreo/internal/server/middleware"
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
			"component", "rca-agent",
			"phase", "initialization",
		)
		os.Exit(1)
	}

	// Initialize logger with proper configuration
	logger := initLogger(cfg.LogLevel)
	logger.Info("Configuration loaded successfully", "log_level", cfg.LogLevel)

	// Initialize report store
	reportStore, err := store.New(
		cfg.Report.Backend,
		cfg.Report.DatabaseURI,
		logger.With("component", "report-store"),
	)
	if err != nil {
		logger.Error("Failed to initialize report store", "error", err)
		os.Exit(1)
	}
	if err := reportStore.Initialize(context.Background()); err != nil {
		logger.Error("Failed to initialize report store schema", "error", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := reportStore.Close(); closeErr != nil {
			logger.Error("Failed to close report store", "error", closeErr)
		}
	}()

	// Initialize authz client
	authzClient, err := rcaAuthz.NewClient(&cfg.Authz, logger.With("component", "authz-client"))
	if err != nil {
		logger.Error("Failed to create authz client", "error", err)
		os.Exit(1)
	}

	// Initialize LLM provider
	llmProvider, err := openai.New(anyllm.WithAPIKey(cfg.LLM.APIKey))
	if err != nil {
		logger.Error("Failed to initialize LLM provider", "error", err)
		os.Exit(1)
	}
	logger.Info("LLM provider initialized", "model", cfg.LLM.ModelName)

	// Graceful shutdown using signal context — created early so the service
	// can derive background-analysis contexts from it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initialize agent service
	agentService := service.New(ctx, llmProvider, cfg, reportStore, logger.With("component", "agent-service"))

	// Validate external dependencies (LLM, OAuth2, MCP) — fail fast like Python.
	if err := agentService.ValidateConnectivity(context.Background()); err != nil {
		logger.Error("Startup validation failed", "error", err)
		os.Exit(1)
	}

	// Set up HTTP handler
	mux := http.NewServeMux()
	handler := api.NewHandler(logger.With("component", "api"), reportStore, authzClient, agentService, cfg.Server.StreamWriteTimeout)

	// Initialize JWT middleware
	jwtAuth := initJWTMiddleware(cfg, logger)

	// Build routes with middleware
	recoveryMiddleware := observermiddleware.Recovery(logger)
	loggerMiddleware := observermiddleware.Logger(logger)

	routes := middleware.NewRouteBuilder(mux)

	// Public routes (no authentication)
	routes.HandleFunc("GET /health", handler.Health)

	// Protected routes (JWT authentication required)
	protected := routes.With(jwtAuth)
	protected.HandleFunc("POST /api/v1alpha1/rca-agent/chat", handler.Chat)
	protected.HandleFunc("GET /api/v1/rca-agent/reports", handler.ListReports)
	protected.HandleFunc("GET /api/v1/rca-agent/reports/{report_id}", handler.GetReport)
	protected.HandleFunc("PUT /api/v1/rca-agent/reports/{report_id}", handler.UpdateReport)

	// Create external HTTP server with middleware chain: recovery → logging → CORS → routes
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpHandler := observermiddleware.Chain(
		recoveryMiddleware,
		loggerMiddleware,
		observermiddleware.CORS(cfg.CORS.AllowedOrigins),
	)(mux)

	server := &http.Server{
		Addr:         addr,
		Handler:      httpHandler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	internalMux := http.NewServeMux()
	internalRoutes := middleware.NewRouteBuilder(internalMux).With(loggerMiddleware)
	internalRoutes.HandleFunc("POST /api/v1alpha1/rca-agent/analyze", handler.Analyze)

	internalAddr := fmt.Sprintf(":%d", cfg.Server.InternalPort)
	internalServer := &http.Server{
		Addr:         internalAddr,
		Handler:      internalMux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start external server
	serverErr := make(chan error, 2)
	go func() {
		logger.Info("Starting server", "address", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("external server: %w", err)
		}
	}()

	// Start internal server
	go func() {
		logger.Info("Starting internal server", "address", internalAddr)
		if err := internalServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("internal server: %w", err)
		}
	}()

	// Wait for interrupt signal or server error
	select {
	case <-ctx.Done():
	case err := <-serverErr:
		logger.Error("Server failed", "error", err)
	}

	logger.Info("Shutting down servers...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("Main server forced to shutdown", "error", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := internalServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("Internal server forced to shutdown", "error", err)
		}
	}()

	wg.Wait()
	logger.Info("Server shutdown complete")
}

func initLogger(level string) *slog.Logger {
	var logLevel slog.Level

	level = strings.ToLower(level)
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

	var handler slog.Handler
	if level == "debug" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

func createBootstrapLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	handler := slog.NewJSONHandler(os.Stderr, opts)
	return slog.New(handler)
}

// initJWTMiddleware initializes the JWT authentication middleware from configuration.
func initJWTMiddleware(cfg *config.Config, logger *slog.Logger) func(http.Handler) http.Handler {
	var jwtAudiences []string
	if cfg.Auth.JWTAudience != "" {
		jwtAudiences = []string{cfg.Auth.JWTAudience}
	}

	// Create subject type resolver from configuration
	var detector *jwt.Resolver
	if len(cfg.Auth.SubjectTypes) > 0 {
		var err error
		detector, err = jwt.NewResolver(cfg.Auth.SubjectTypes)
		if err != nil {
			logger.Error("Failed to create JWT subject resolver", "error", err)
		} else {
			logger.Info("JWT subject resolver initialized", "subject_types_count", len(cfg.Auth.SubjectTypes))
		}
	}

	jwtConfig := jwt.Config{
		Disabled:                     cfg.Auth.JWTDisabled,
		JWKSURL:                      cfg.Auth.JWTJWKSURL,
		ValidateIssuer:               cfg.Auth.JWTIssuer,
		ValidateAudiences:            jwtAudiences,
		JWKSURLTLSInsecureSkipVerify: cfg.Auth.TLSInsecureSkipVerify,
		Detector:                     detector,
		Logger:                       logger,
	}

	return jwt.Middleware(jwtConfig)
}
