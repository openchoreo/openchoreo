// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/openchoreo/openchoreo/internal/authz"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/cmdutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	openapihandlers "github.com/openchoreo/openchoreo/internal/openchoreo-api/api/handlers"
	k8s "github.com/openchoreo/openchoreo/internal/openchoreo-api/clients"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/handlers"
	apilogger "github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	"github.com/openchoreo/openchoreo/internal/server/middleware/router"
)

var (
	port = flag.Int("port", 8080, "port http server runs on")
)

func main() {
	flag.Parse()

	// Get log level from environment variable, default to "info"
	baseLogger := cmdutil.SetupLogger(os.Getenv(config.EnvLogLevel))
	slog.SetDefault(baseLogger)

	// Create shutdown context
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	k8sClient, err := k8s.NewK8sClient()
	if err != nil {
		baseLogger.Error("Failed to initialize Kubernetes client", slog.Any("error", err))
		os.Exit(1)
	}

	// Load configuration
	configPath := os.Getenv("OPENCHOREO_API_CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		baseLogger.Error("Failed to load configuration file",
			slog.String("config_path", configPath),
			slog.Any("error", err))
		os.Exit(1)
	}

	baseLogger.Info("Loaded configuration from file",
		slog.String("config_path", configPath))

	// Initialize authorization
	authzConfig := authz.AuthZConfig{
		Enabled:                  os.Getenv("AUTHZ_ENABLED") == "true",
		DatabasePath:             os.Getenv("AUTHZ_DATABASE_PATH"),
		DefaultAuthzDataFilePath: os.Getenv("AUTHZ_DEFAULT_AUTHZ_DATA_FILE_PATH"),
		EnableCache:              false,
	}
	pap, pdp, err := authz.Initialize(authzConfig, baseLogger.With("component", "authz"))
	if err != nil {
		baseLogger.Error("Failed to initialize authorization", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize services with PAP and PDP
	services := services.NewServices(k8sClient, kubernetesClient.NewManager(), pap, pdp, baseLogger)

	// Initialize legacy HTTP handlers with config for user type management
	legacyHandler := handlers.New(services, cfg, baseLogger.With("component", "legacy-handlers"))
	legacyRoutes := legacyHandler.Routes()

	// Initialize OpenAPI handlers
	openapiHandler := openapihandlers.New(services, baseLogger.With("component", "openapi-handlers"))
	strictHandler := gen.NewStrictHandler(openapiHandler, nil)

	// Initialize middlewares for OpenAPI handler
	loggerMiddleware := apilogger.LoggerMiddleware(baseLogger.With("component", "openapi"))
	jwtMiddleware := legacyHandler.InitJWTMiddleware()
	authMiddleware := auth.OpenAPIAuth(jwtMiddleware, gen.BearerAuthScopes)

	// Create OpenAPI handler with middleware chain (order: logger → auth → handler)
	openapiRoutes := gen.HandlerWithOptions(strictHandler, gen.StdHTTPServerOptions{
		Middlewares: []gen.MiddlewareFunc{loggerMiddleware, authMiddleware},
	})

	// Create migration router that routes based on X-Use-OpenAPI header
	// - X-Use-OpenAPI: true → OpenAPI handlers (new spec-first implementation)
	// - Header absent → Legacy handlers (existing implementation)
	migrationRouter := router.OpenAPIMigrationRouter(openapiRoutes, legacyRoutes)

	// Server configuration
	serverCfg := server.Config{
		Addr:            ":" + strconv.Itoa(*port),
		ReadTimeout:     15 * time.Second,
		WriteTimeout:    15 * time.Second,
		IdleTimeout:     60 * time.Second,
		ShutdownTimeout: 30 * time.Second,
	}
	srv := server.New(serverCfg, migrationRouter, baseLogger.With("component", "server"))

	// Start server
	if err := srv.Run(ctx); err != nil {
		baseLogger.Error("Server error", slog.Any("error", err))
	}

	// Close authorization database connection
	if casbinEnforcer, ok := pap.(interface{ Close() error }); ok {
		if err := casbinEnforcer.Close(); err != nil {
			baseLogger.Error("Failed to close authorization database", slog.Any("error", err))
		}
	}

	baseLogger.Info("Server stopped gracefully")
}
