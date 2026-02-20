// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/handlers"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	authzsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/authz"
	buildplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/buildplane"
	clusterbuildplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterbuildplane"
	clustercomponenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype"
	clusterdataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterdataplane"
	clusterobservabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterobservabilityplane"
	componentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
	componentreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentrelease"
	componenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype"
	dataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/dataplane"
	environmentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/environment"
	observabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityplane"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	releasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/release"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	traitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/trait"
	workloadsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workload"
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

	// Set up runtime
	runtime, err := setupRuntime(ctx, &cfg, logger)
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

	// Create gateway client to fetch buildplane pod logs/events
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

	// Create a direct Kubernetes client for the service layer.
	k8sClient, err := k8s.NewK8sClient()
	if err != nil {
		logger.Error("Failed to create Kubernetes client", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize services with PAP and PDP
	services := services.NewServices(k8sClient, k8sClientMgr, runtime.pap, runtime.pdp, logger, gatewayURL, gwClient)

	// Initialize legacy HTTP handlers with unified config
	legacyHandler := handlers.New(services, &cfg, logger.With("component", "legacy-handlers"))
	legacyRoutes := legacyHandler.Routes()

	// Initialize project service for the new K8s-native API design
	projectService := projectsvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "project-service"))

	// Initialize build plane service for the new K8s-native API design
	buildPlaneService := buildplanesvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "buildplane-service"))

	// Initialize cluster build plane service for the new K8s-native API design
	clusterBuildPlaneService := clusterbuildplanesvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "clusterbuildplane-service"))

	// Initialize cluster component type service for the new K8s-native API design
	clusterComponentTypeService := clustercomponenttypesvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "clustercomponenttype-service"))

	// Initialize cluster data plane service for the new K8s-native API design
	clusterDataPlaneService := clusterdataplanesvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "clusterdataplane-service"))

	// Initialize cluster observability plane service for the new K8s-native API design
	clusterObservabilityPlaneService := clusterobservabilityplanesvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "clusterobservabilityplane-service"))

	// Initialize data plane service for the new K8s-native API design
	dataPlaneService := dataplanesvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "dataplane-service"))

	// Initialize component service for the new K8s-native API design
	componentService := componentsvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "component-service"))

	// Initialize component release service for the new K8s-native API design
	componentReleaseService := componentreleasesvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "componentrelease-service"))

	// Initialize component type service for the new K8s-native API design
	componentTypeService := componenttypesvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "componenttype-service"))

	// Initialize environment service for the new K8s-native API design
	environmentService := environmentsvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "environment-service"))

	// Initialize observability plane service for the new K8s-native API design
	observabilityPlaneService := observabilityplanesvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "observabilityplane-service"))

	// Initialize release service for the new K8s-native API design
	releaseService := releasesvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "release-service"))

	// Initialize release binding service for the new K8s-native API design
	releaseBindingService := releasebindingsvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "releasebinding-service"))

	// Initialize trait service for the new K8s-native API design
	traitService := traitsvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "trait-service"))

	// Initialize workload service for the new K8s-native API design
	workloadService := workloadsvc.NewServiceWithAuthz(k8sClient, runtime.pdp, logger.With("component", "workload-service"))

	// Initialize authz service with authz
	authzService := authzsvc.NewServiceWithAuthz(runtime.pap, runtime.pdp, k8sClient, logger.With("component", "authz-service"))

	// Initialize OpenAPI handlers
	openapiHandler := openapihandlers.New(services, authzService, projectService, buildPlaneService, clusterBuildPlaneService, clusterComponentTypeService, clusterDataPlaneService, clusterObservabilityPlaneService, dataPlaneService, componentService, componentReleaseService, componentTypeService, environmentService, observabilityPlaneService, releaseService, releaseBindingService, traitService, workloadService, logger.With("component", "openapi-handlers"), &cfg)
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

// setupRuntime bootstraps the authorization runtime. When authorization is
// enabled it creates a controller-runtime manager with an informer-based cache
// for the authz CRDs; when disabled the manager is left nil and
// authz.Initialize returns a passthrough implementation.
func setupRuntime(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*runtime, error) {
	authzCfg := cfg.Security.Authorization
	var mgr ctrl.Manager

	// When enabled, create a controller-runtime manager with informers for authz CRDs
	if authzCfg.Enabled {
		logger.Info("Setting up controller manager for authorization CRD informers")
		cacheOpts := cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&openchoreov1alpha1.AuthzRole{}:               {},
				&openchoreov1alpha1.AuthzClusterRole{}:        {},
				&openchoreov1alpha1.AuthzRoleBinding{}:        {},
				&openchoreov1alpha1.AuthzClusterRoleBinding{}: {},
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

	pap, pdp, err := authz.Initialize(ctx, mgr, authzCfg.ToAuthzConfig(), logger)
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
