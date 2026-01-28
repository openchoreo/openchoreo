// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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

	// Create shutdown context
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	k8sClient, err := k8s.NewK8sClient()
	if err != nil {
		logger.Error("Failed to initialize Kubernetes client", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize authorization
	pap, pdp, err := authz.Initialize(cfg.Security.Authorization.ToAuthzConfig(), logger)
	if err != nil {
		logger.Error("Failed to initialize authorization", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize build plane client manager
	var k8sClientMgr *kubernetesClient.KubeMultiClientManager
	if cfg.ClusterGateway.TLS.CACertPath != "" || cfg.ClusterGateway.TLS.ClientCertPath != "" || cfg.ClusterGateway.TLS.ClientKeyPath != "" {
		k8sClientMgr = kubernetesClient.NewManagerWithProxyTLS(&kubernetesClient.ProxyTLSConfig{
			CACertPath:     cfg.ClusterGateway.TLS.CACertPath,
			ClientCertPath: cfg.ClusterGateway.TLS.ClientCertPath,
			ClientKeyPath:  cfg.ClusterGateway.TLS.ClientKeyPath,
		})
		logger.Info("Build plane client manager created with proxy TLS configuration",
			"caCert", cfg.ClusterGateway.TLS.CACertPath != "",
			"clientCert", cfg.ClusterGateway.TLS.ClientCertPath != "",
			"clientKey", cfg.ClusterGateway.TLS.ClientKeyPath != "")
	} else {
		k8sClientMgr = kubernetesClient.NewManager()
		if cfg.ClusterGateway.URL != "" {
			logger.Warn("Using insecure mode for cluster gateway connection. " +
				"Consider configuring TLS certificates for production deployments.")
		}
	}

	// Determine cluster gateway URL
	gatewayURL := ""
	if cfg.ClusterGateway.Enabled {
		gatewayURL = cfg.ClusterGateway.URL
	}

	// Initialize services with PAP and PDP
	services := services.NewServices(k8sClient, k8sClientMgr, pap, pdp, logger, gatewayURL)

	// Initialize legacy HTTP handlers with unified config
	legacyHandler := handlers.New(services, &cfg, logger.With("component", "legacy-handlers"))
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

	// Create server from configuration
	srv := server.New(cfg.Server.ToServerConfig(), migrationRouter, logger)

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
	flags.String("server-bind-address", config.ServerDefaults().BindAddress, "Server bind address")
	flags.Int("server-port", config.ServerDefaults().Port, "Server port")
	flags.String("log-level", config.LoggingDefaults().Level, "Log level (debug, info, warn, error)")

	// Direct flags - bound to variables for immediate use
	flags.StringVar(&cli.configPath, "config", "", "Path to config file")
	flags.BoolVar(&cli.dumpConfig, "dump-config", false, "Print loaded configuration and exit")

	return flags, cli
}
