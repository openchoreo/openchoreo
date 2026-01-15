// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/pflag"

	"github.com/openchoreo/openchoreo/internal/authz"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/logging"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	openapihandlers "github.com/openchoreo/openchoreo/internal/openchoreo-api/api/handlers"
	k8s "github.com/openchoreo/openchoreo/internal/openchoreo-api/clients"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/handlers"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	apilogger "github.com/openchoreo/openchoreo/internal/server/middleware/logger"
	"github.com/openchoreo/openchoreo/internal/server/middleware/router"
	"github.com/openchoreo/openchoreo/internal/version"
)

func main() {
	flags, cli := setupFlags()
	_ = flags.Parse(os.Args[1:]) // ExitOnError mode handles parse errors

	// Bootstrap logger for pre-configuration errors
	bootLogger := logging.Bootstrap(version.Get().Name)

	// Load unified configuration
	loader, err := config.NewLoader(cli.configPath, flags)
	if err != nil {
		bootLogger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Print merged config and exit
	if cli.dumpConfig {
		if err := loader.DumpYAML(os.Stdout); err != nil {
			bootLogger.Error("Failed to dump configuration", "error", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Unmarshal and validate configuration
	var cfg config.Config
	if err := loader.Unmarshal("", &cfg); err != nil {
		bootLogger.Error("Failed to unmarshal configuration", "error", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		var validationErrs coreconfig.ValidationErrors
		if errors.As(err, &validationErrs) {
			for _, e := range validationErrs {
				bootLogger.Error("Invalid configuration", "field", e.Field, "message", e.Message)
			}
		} else {
			bootLogger.Error("Invalid configuration", "error", err)
		}
		os.Exit(1)
	}

	// Set up runtime logger from configuration
	logger := logging.NewWithComponent(cfg.Logging.ToLoggingConfig(), version.Get().Name)

	// Log startup with version info
	logger.Info("Starting", version.GetLogKeyValues()...)

	port, _ := flags.GetInt("server-port")

	// Create shutdown context
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	k8sClient, err := k8s.NewK8sClient()
	if err != nil {
		logger.Error("Failed to initialize Kubernetes client", slog.Any("error", err))
		os.Exit(1)
	}

	// Load legacy configuration
	// Deprecated: Will be removed once handlers migrate to unified config
	legacyConfigPath := os.Getenv("OPENCHOREO_API_CONFIG_PATH")
	if legacyConfigPath == "" {
		legacyConfigPath = "config.yaml"
	}

	cfgLegacy, err := config.LoadLegacy(legacyConfigPath)
	if err != nil {
		logger.Error("Failed to load legacy configuration file",
			slog.String("config_path", legacyConfigPath),
			slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("Loaded legacy configuration from file",
		slog.String("config_path", legacyConfigPath))

	// Initialize authorization
	authzConfig := authz.AuthZConfig{
		Enabled:                  os.Getenv("AUTHZ_ENABLED") == "true",
		DatabasePath:             os.Getenv("AUTHZ_DATABASE_PATH"),
		DefaultAuthzDataFilePath: os.Getenv("AUTHZ_DEFAULT_AUTHZ_DATA_FILE_PATH"),
		EnableCache:              false,
	}
	pap, pdp, err := authz.Initialize(authzConfig, logger.With("component", "authz"))
	if err != nil {
		logger.Error("Failed to initialize authorization", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize services with PAP and PDP
	services := services.NewServices(k8sClient, kubernetesClient.NewManager(), pap, pdp, logger)

	// Initialize legacy HTTP handlers with config for user type management
	legacyHandler := handlers.New(services, cfgLegacy, logger.With("component", "legacy-handlers"))
	legacyRoutes := legacyHandler.Routes()

	// Initialize OpenAPI handlers
	openapiHandler := openapihandlers.New(services, logger.With("component", "openapi-handlers"), &cfg)
	strictHandler := gen.NewStrictHandler(openapiHandler, nil)

	// Initialize middlewares for OpenAPI handler
	loggerMiddleware := apilogger.LoggerMiddleware(logger.With("component", "openapi"))
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
		Addr:            ":" + strconv.Itoa(port),
		ReadTimeout:     15 * time.Second,
		WriteTimeout:    15 * time.Second,
		IdleTimeout:     60 * time.Second,
		ShutdownTimeout: 30 * time.Second,
	}
	srv := server.New(serverCfg, migrationRouter, logger.With("component", "server"))

	// Start server
	if err := srv.Run(ctx); err != nil {
		logger.Error("Server error", slog.Any("error", err))
	}

	// Close authorization database connection
	if casbinEnforcer, ok := pap.(interface{ Close() error }); ok {
		if err := casbinEnforcer.Close(); err != nil {
			logger.Error("Failed to close authorization database", slog.Any("error", err))
		}
	}

	logger.Info("Server stopped gracefully")
}

// cliFlags holds direct command-line flags that control program behavior.
type cliFlags struct {
	configPath string // Path to config file
	dumpConfig bool   // Print loaded configuration and exit
}

// setupFlags creates and configures the CLI flags for openchoreo-api.
// Returns the flag set and a struct for direct flags.
func setupFlags() (*pflag.FlagSet, *cliFlags) {
	flags := pflag.NewFlagSet("openchoreo-api", pflag.ExitOnError)
	cli := &cliFlags{}

	// Config flags - values loaded for configurations
	flags.Int("server-port", config.ServerDefaults().Port, "HTTP server port")
	flags.String("log-level", config.LoggingDefaults().Level, "Log level (debug, info, warn, error)")

	// Direct flags - bound to variables for immediate use
	flags.StringVar(&cli.configPath, "config", "", "Path to config file")
	flags.BoolVar(&cli.dumpConfig, "dump-config", false, "Print loaded configuration and exit")

	return flags, cli
}
