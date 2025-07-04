// Copyright 2025 The OpenChoreo Authors
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
	"syscall"
	"time"

	"github.com/openchoreo/openchoreo/internal/logger/config"
	"github.com/openchoreo/openchoreo/internal/logger/handlers"
	"github.com/openchoreo/openchoreo/internal/logger/middleware"
	"github.com/openchoreo/openchoreo/internal/logger/opensearch"
	"github.com/openchoreo/openchoreo/internal/logger/service"
)

func main() {
	// Initialize configuration
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("Failed to load configuration: %v", err))
	}

	// Initialize logger
	logger := initLogger(cfg.LogLevel)

	// Initialize OpenSearch client
	osClient, err := opensearch.NewClient(&cfg.OpenSearch, logger)
	if err != nil {
		logger.Error("Failed to initialize OpenSearch client", "error", err)
		os.Exit(1)
	}

	// Initialize logging service
	loggingService := service.NewLoggingService(osClient, cfg, logger)

	// Initialize HTTP server
	mux := http.NewServeMux()

	// Initialize handlers
	handler := handlers.NewHandler(loggingService, logger)

	// Health check endpoint
	mux.HandleFunc("GET /health", handler.Health)

	// API routes
	mux.HandleFunc("POST /api/logs/component/{componentId}", handler.GetComponentLogs)
	mux.HandleFunc("POST /api/logs/project/{projectId}", handler.GetProjectLogs)
	mux.HandleFunc("POST /api/logs/gateway", handler.GetGatewayLogs)
	mux.HandleFunc("POST /api/logs/org/{orgId}", handler.GetOrganizationLogs)

	// Apply middleware
	handlerWithMiddleware := middleware.Chain(
		middleware.Logger(logger),
		middleware.Recovery(logger),
		middleware.CORS(),
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
			logger.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
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
