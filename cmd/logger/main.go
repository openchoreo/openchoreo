// Copyright (c) 2025 openchoreo
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	"github.com/openchoreo/openchoreo/internal/logger/config"
	"github.com/openchoreo/openchoreo/internal/logger/handlers"
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
	logger, err := initLogger(cfg.LogLevel)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
	defer func() {
		_ = logger.Sync()
	}()

	// Initialize OpenSearch client
	osClient, err := opensearch.NewClient(&cfg.OpenSearch, logger)
	if err != nil {
		logger.Fatal("Failed to initialize OpenSearch client", zap.Error(err))
	}

	// Initialize logging service
	loggingService := service.NewLoggingService(osClient, cfg, logger)

	// Initialize HTTP server
	e := echo.New()
	e.HideBanner = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Health check endpoint
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "healthy"})
	})

	// Initialize handlers
	handler := handlers.NewHandler(loggingService, logger)

	// V2 API routes
	v2 := e.Group("/api/v2")
	{
		// Component logs
		v2.POST("/logs/component/:componentId", handler.GetComponentLogs)

		// Project logs
		v2.POST("/logs/project/:projectId", handler.GetProjectLogs)

		// Gateway logs
		v2.POST("/logs/gateway", handler.GetGatewayLogs)

		// Organization logs
		v2.POST("/logs/org/:orgId", handler.GetOrganizationLogs)
	}

	// Start server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		logger.Info("Starting server", zap.String("address", addr))
		if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server shutdown complete")
}

func initLogger(level string) (*zap.Logger, error) {
	var cfg zap.Config
	if level == "debug" {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}

	switch level {
	case "debug":
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	return cfg.Build()
}
