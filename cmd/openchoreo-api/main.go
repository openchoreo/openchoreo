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

	"github.com/spf13/pflag"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/authz"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/logging"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	openapihandlers "github.com/openchoreo/openchoreo/internal/openchoreo-api/api/handlers"
	k8s "github.com/openchoreo/openchoreo/internal/openchoreo-api/clients"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/mcphandlers"
	autobuildsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/autobuild"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	workflowrunsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
	"github.com/openchoreo/openchoreo/internal/server"
	"github.com/openchoreo/openchoreo/internal/server/middleware"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	apilogger "github.com/openchoreo/openchoreo/internal/server/middleware/logger"
	mcpmiddleware "github.com/openchoreo/openchoreo/internal/server/middleware/mcp"
	"github.com/openchoreo/openchoreo/internal/version"
	"github.com/openchoreo/openchoreo/pkg/mcp"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
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

	// Create a Kubernetes client for the service layer and PAP.
	k8sClient, err := k8s.NewK8sClient()
	if err != nil {
		logger.Error("Failed to create Kubernetes client", slog.Any("error", err))
		os.Exit(1)
	}

	// Set up runtime
	runtime, err := setupRuntime(ctx, &cfg, k8sClient, logger)
	if err != nil {
		logger.Error("Failed to initialize authorization", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize workflow plane client manager
	var planeK8sClientMgr *kubernetesClient.KubeMultiClientManager
	if cfg.ClusterGateway.TLS.CACertPath != "" ||
		cfg.ClusterGateway.TLS.ClientCertPath != "" ||
		cfg.ClusterGateway.TLS.ClientKeyPath != "" {
		planeK8sClientMgr = kubernetesClient.NewManagerWithProxyTLS(&kubernetesClient.ProxyTLSConfig{
			CACertPath:     cfg.ClusterGateway.TLS.CACertPath,
			ClientCertPath: cfg.ClusterGateway.TLS.ClientCertPath,
			ClientKeyPath:  cfg.ClusterGateway.TLS.ClientKeyPath,
		})
		logger.Info("Workflow plane client manager created with proxy TLS configuration",
			"caCert", cfg.ClusterGateway.TLS.CACertPath != "",
			"clientCert", cfg.ClusterGateway.TLS.ClientCertPath != "",
			"clientKey", cfg.ClusterGateway.TLS.ClientKeyPath != "")
	} else {
		planeK8sClientMgr = kubernetesClient.NewManager()
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

	// Create gateway client to fetch workflowplane pod logs/events
	var gwClient *gatewayClient.Client
	if cfg.ClusterGateway.Enabled {
		if gatewayURL == "" {
			logger.Error("No cluster gateway URL provided", "clusterGateway", cfg.ClusterGateway)
			os.Exit(1)
		}
		gwClient = gatewayClient.NewClient(gatewayURL)
	}
	logger.Info("gateway client initialized", "url", gatewayURL)

	// Start background processes (manager + cache sync when authz enabled)
	if err := runtime.start(ctx); err != nil {
		logger.Error("Failed to start authorization runtime", slog.Any("error", err))
		os.Exit(1)
	}

	// Create plane client provider for services that need to talk to remote planes.
	planeClientProvider := kubernetesClient.NewPlaneClientProvider(planeK8sClientMgr, gatewayURL)

	// Create the internal (unauthz) workflow run service used by the webhook processor.
	// Webhook requests are authenticated via HMAC signature validation instead of user-level auth.
	baseWfRunSvc := workflowrunsvc.NewService(
		k8sClient, planeClientProvider, gwClient, logger.With("service", "workflowrun"),
	)

	// Create the webhook processor that finds affected components and triggers workflow runs.
	webhookProcessor := autobuildsvc.NewWebhookProcessor(k8sClient, baseWfRunSvc, logger.With("service", "webhook"))

	// Initialize all handler services
	services := handlerservices.NewServices(
		k8sClient, runtime.pap, runtime.pdp, planeClientProvider, logger, gwClient, webhookProcessor,
	)

	// Initialize OpenAPI handlers
	openapiHandler := openapihandlers.New(services, logger.With("component", "openapi-handlers"), &cfg)
	strictHandler := gen.NewStrictHandler(openapiHandler, nil)

	// Initialize JWT middleware
	jwtMiddleware := openapihandlers.InitJWTMiddleware(&cfg, logger)

	// Initialize middlewares for OpenAPI handler
	loggerMiddleware := apilogger.LoggerMiddleware(logger.With("component", "openapi"))
	authMiddleware := auth.OpenAPIAuth(jwtMiddleware, gen.BearerAuthScopes)

	// Create base mux for the OpenAPI router.
	// Non-OpenAPI routes (e.g. /mcp) are registered here before the generated
	// routes, so they share the same mux without an extra wrapping layer.
	baseMux := http.NewServeMux()

	// MCP endpoint (only if enabled)
	if cfg.MCP.Enabled {
		mcpLogger := logger.With("component", "mcp")

		// Build MCP toolsets from config
		toolsets := buildMCPToolsets(&cfg, services, mcpLogger)

		// MCP middleware chain: logger → auth401 interceptor → JWT auth → handler
		mcpLoggerMw := apilogger.LoggerMiddleware(mcpLogger)
		resourceMetadataURL := cfg.Server.PublicURL + "/.well-known/oauth-protected-resource"
		mcpAuth401Mw := mcpmiddleware.Auth401Interceptor(resourceMetadataURL)
		mcpHandler := middleware.Chain(mcpLoggerMw, mcpAuth401Mw, jwtMiddleware)(mcp.NewHTTPServer(toolsets))

		baseMux.Handle("/mcp", mcpHandler)
	}

	// Create OpenAPI handler with middleware chain (order: logger → auth → webhookBody → handler)
	// Middlewares are applied last-to-first (last entry becomes the outermost wrapper).
	// Execution order: loggerMiddleware → authMiddleware → webhookRawBodyMiddleware → handler.
	// loggerMiddleware must be outermost so it captures all responses, including 401s from auth.
	// webhookRawBodyMiddleware must be innermost (before the strict handler decodes the body)
	// so that HMAC signature validation can access the original raw bytes.
	// The generated routes are registered on the baseMux alongside /mcp.
	handler := gen.HandlerWithOptions(strictHandler, gen.StdHTTPServerOptions{
		BaseRouter:  baseMux,
		Middlewares: []gen.MiddlewareFunc{openapihandlers.WebhookRawBodyMiddleware, authMiddleware, loggerMiddleware},
	})

	// Create server from configuration
	srv := server.New(cfg.Server.ToServerConfig(), handler, logger)

	// Start server
	if err := srv.Run(ctx); err != nil {
		logger.Error("Server error", slog.Any("error", err))
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

// runtime holds the components initialized at startup.
type runtime struct {
	pap authzcore.PAP
	pdp authzcore.PDP
	// start runs any background processes (manager, cache sync). No-op when authz disabled.
	start func(context.Context) error
}

// buildMCPToolsets creates the MCP toolsets from the configuration.
// Each enabled toolset is backed by the handler services layer.
func buildMCPToolsets(cfg *config.Config, svc *handlerservices.Services, logger *slog.Logger) *tools.Toolsets {
	toolsetsMap := cfg.MCP.ParseToolsets()

	logger.Info("Initializing MCP server", slog.Any("enabled_toolsets", cfg.MCP.Toolsets))

	handler := mcphandlers.NewMCPHandler(svc)

	toolsets := &tools.Toolsets{}
	for toolsetType := range toolsetsMap {
		switch toolsetType {
		case tools.ToolsetNamespace:
			toolsets.NamespaceToolset = handler
			logger.Debug("Enabled MCP toolset", slog.String("toolset", "namespace"))
		case tools.ToolsetProject:
			toolsets.ProjectToolset = handler
			logger.Debug("Enabled MCP toolset", slog.String("toolset", "project"))
		case tools.ToolsetComponent:
			toolsets.ComponentToolset = handler
			logger.Debug("Enabled MCP toolset", slog.String("toolset", "component"))
		case tools.ToolsetDeployment:
			toolsets.DeploymentToolset = handler
			logger.Debug("Enabled MCP toolset", slog.String("toolset", "deployment"))
		case tools.ToolsetBuild:
			toolsets.BuildToolset = handler
			logger.Debug("Enabled MCP toolset", slog.String("toolset", "build"))
		case tools.ToolsetPE:
			toolsets.PEToolset = handler
			logger.Debug("Enabled MCP toolset", slog.String("toolset", "pe"))
		default:
			logger.Warn("Unknown toolset type", slog.String("toolset", string(toolsetType)))
		}
	}
	return toolsets
}

// setupRuntime bootstraps the authorization runtime. When authorization is
// enabled it creates a controller-runtime manager with an informer-based cache
// for the authz CRDs; when disabled the manager is left nil and
// authz.Initialize returns a passthrough implementation.
func setupRuntime(
	ctx context.Context, cfg *config.Config, k8sClient client.Client, logger *slog.Logger,
) (*runtime, error) {
	authzCfg := cfg.Security.Authorization
	var mgr ctrl.Manager

	// When enabled, create a controller-runtime manager with informers for authz CRDs
	if cfg.Security.Enabled && authzCfg.Enabled {
		logger.Info("Setting up controller manager for authorization CRD informers")
		cacheOpts := cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&openchoreov1alpha1.AuthzRole{}:               {},
				&openchoreov1alpha1.ClusterAuthzRole{}:        {},
				&openchoreov1alpha1.AuthzRoleBinding{}:        {},
				&openchoreov1alpha1.ClusterAuthzRoleBinding{}: {},
			},
		}
		if authzCfg.ResyncInterval > 0 {
			cacheOpts.SyncPeriod = &authzCfg.ResyncInterval
			logger.Info("Informer resync enabled", "interval", authzCfg.ResyncInterval)
		}

		var err error
		mgr, err = ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			LeaderElection: false,
			Metrics:        metricsserver.Options{BindAddress: "0"},
			Cache:          cacheOpts,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create controller manager: %w", err)
		}
	}

	pap, pdp, err := authz.Initialize(ctx, mgr, authzCfg.ToAuthzConfig(cfg.Security.Enabled), k8sClient, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize authorization: %w", err)
	}

	rt := &runtime{pap: pap, pdp: pdp, start: func(context.Context) error { return nil }}
	if mgr != nil {
		rt.start = func(ctx context.Context) error {
			go func() {
				if err := mgr.Start(ctx); err != nil {
					logger.Error("Controller manager error", slog.Any("error", err))
				}
			}()

			// timeout to avoid blocking startup indefinitely
			syncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			// Wait for cache sync
			if !mgr.GetCache().WaitForCacheSync(syncCtx) {
				return fmt.Errorf("failed to sync authz cache")
			}
			logger.Info("Authz cache synced - policies loaded")
			return nil
		}
	}

	return rt, nil
}
