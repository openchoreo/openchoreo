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
	"github.com/openchoreo/openchoreo/internal/auditconfig"
	"github.com/openchoreo/openchoreo/internal/authz"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/logging"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	openapihandlers "github.com/openchoreo/openchoreo/internal/openchoreo-api/api/handlers"
	apiaudit "github.com/openchoreo/openchoreo/internal/openchoreo-api/audit"
	k8s "github.com/openchoreo/openchoreo/internal/openchoreo-api/clients"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/mcphandlers"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	autobuildsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/autobuild"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	workflowrunsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
	"github.com/openchoreo/openchoreo/internal/remoteconnect"
	"github.com/openchoreo/openchoreo/internal/server"
	"github.com/openchoreo/openchoreo/internal/server/middleware"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	apilogger "github.com/openchoreo/openchoreo/internal/server/middleware/logger"
	mcpmiddleware "github.com/openchoreo/openchoreo/internal/server/middleware/mcp"
	"github.com/openchoreo/openchoreo/internal/version"
	"github.com/openchoreo/openchoreo/pkg/mcp"
	"github.com/openchoreo/openchoreo/pkg/mcp/mcpaudit"
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
	planeK8sClientMgr := kubernetesClient.NewManagerWithProxyTLS(&kubernetesClient.ProxyTLSConfig{
		CACertPath:     cfg.ClusterGateway.TLS.CACertPath,
		ClientCertPath: cfg.ClusterGateway.TLS.ClientCertPath,
		ClientKeyPath:  cfg.ClusterGateway.TLS.ClientKeyPath,
		Insecure:       cfg.ClusterGateway.TLS.Insecure,
	})
	logger.Info("Workflow plane client manager created with proxy TLS configuration",
		"caCert", cfg.ClusterGateway.TLS.CACertPath != "",
		"clientCert", cfg.ClusterGateway.TLS.ClientCertPath != "",
		"clientKey", cfg.ClusterGateway.TLS.ClientKeyPath != "",
		"insecure", cfg.ClusterGateway.TLS.Insecure)
	if cfg.ClusterGateway.URL != "" && cfg.ClusterGateway.TLS.Insecure {
		logger.Warn("Cluster gateway TLS verification is disabled (cluster_gateway.tls.insecure). " +
			"Do not use this setting in production.")
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
		var err error
		gwClient, err = gatewayClient.NewClientWithConfig(&gatewayClient.Config{
			BaseURL: gatewayURL,
			TLS: gatewayClient.TLSConfig{
				CAFile:             cfg.ClusterGateway.TLS.CACertPath,
				ClientCertFile:     cfg.ClusterGateway.TLS.ClientCertPath,
				ClientKeyFile:      cfg.ClusterGateway.TLS.ClientKeyPath,
				InsecureSkipVerify: cfg.ClusterGateway.TLS.Insecure,
			},
		})
		if err != nil {
			logger.Error("Failed to create cluster gateway client", slog.Any("error", err))
			os.Exit(1)
		}
		logger.Info("gateway client initialized",
			"url", gatewayURL,
			"caCert", cfg.ClusterGateway.TLS.CACertPath != "",
			"clientCert", cfg.ClusterGateway.TLS.ClientCertPath != "",
			"insecure", cfg.ClusterGateway.TLS.Insecure)
	}

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
		k8sClient, runtime.pap, runtime.pdp, planeClientProvider, cfg.SecretManagement, logger, gwClient, webhookProcessor,
	)

	// Initialize OpenAPI handlers
	openapiHandler := openapihandlers.New(services, logger.With("component", "openapi-handlers"), &cfg)
	strictHandler := gen.NewStrictHandler(openapiHandler, nil)

	// Initialize JWT middleware
	jwtMiddleware := openapihandlers.InitJWTMiddleware(&cfg, logger)

	// Initialize middlewares for OpenAPI handler
	authMiddleware := auth.OpenAPIAuth(jwtMiddleware, gen.BearerAuthScopes)

	// Build the one audit Emitter shared by both the REST and MCP surfaces, so
	// a policy applies identically regardless of which one produced the event.
	// cfg.Validate() (above) already ran the same conversion and would have
	// failed startup on an invalid policy; a non-nil error here is defensive.
	auditVocab := auditconfig.NewVocabulary(apiaudit.GetOperations())
	auditPolicies, err := cfg.Audit.BuildPolicySet(auditVocab, cfg.Security.KnownActorTypes())
	if err != nil {
		logger.Error("Failed to build audit policy set", slog.Any("error", err))
		os.Exit(1)
	}
	auditEmitter, err := audit.NewEmitter("openchoreo-api", auditPolicies, audit.NewLogger(logger))
	if err != nil {
		logger.Error("Failed to build audit emitter", slog.Any("error", err))
		os.Exit(1)
	}

	// Create base mux for the OpenAPI router.
	// Non-OpenAPI routes (e.g. /mcp) are registered here before the generated
	// routes, so they share the same mux without an extra wrapping layer.
	baseMux := http.NewServeMux()

	// MCP endpoint (only if enabled)
	if cfg.MCP.Enabled {
		mcpLogger := logger.With("component", "mcp")

		// Build MCP toolsets from config
		toolsets := buildMCPToolsets(&cfg, services, mcpLogger)

		// MCP middleware chain:
		//   logger → unauthenticated audit → auth401 interceptor → JWT auth → handler
		mcpLoggerMw := apilogger.LoggerMiddleware(mcpLogger)
		resourceMetadataURL := cfg.Server.PublicURL + "/.well-known/oauth-protected-resource"
		mcpAuth401Mw := mcpmiddleware.Auth401Interceptor(resourceMetadataURL, cfg.Identity.MCPOAuthScopes)
		mcpBindings, err := apiaudit.MCPBindings()
		if err != nil {
			logger.Error("Failed to build MCP audit bindings", slog.Any("error", err))
			os.Exit(1)
		}
		mcpServer, err := mcp.NewHTTPServer(mcpLogger, toolsets, runtime.pdp, mcpaudit.MiddlewareOptions{
			Emitter:  auditEmitter,
			Bindings: mcpBindings,
			Enabled:  cfg.Audit.Enabled,
		})
		if err != nil {
			logger.Error("Failed to build MCP HTTP server", slog.Any("error", err))
			os.Exit(1)
		}
		// The audit middleware goes outside jwtMiddleware for the same reason
		// it does in OpenAPIMiddlewares: auth answers a rejected request itself
		// and never calls next, so mcpaudit's own middleware — which lives
		// inside the MCP server, below all of this — never sees a 401.
		// Auth401Interceptor only adds a WWW-Authenticate header; it emits
		// nothing. OriginMCP so an MCP token rejection isn't recorded as if
		// it had arrived over REST.
		unauthedMCPMw := audit.NewUnauthenticatedMiddleware(auditEmitter, audit.OriginMCP, cfg.Audit.Enabled)
		mcpHandler := middleware.Chain(mcpLoggerMw, unauthedMCPMw, mcpAuth401Mw, jwtMiddleware)(mcpServer)

		baseMux.Handle("/mcp", mcpHandler)
	}

	// Remote-connect resolve endpoint (only if enabled). Plain JSON handler registered on
	// the baseMux like /mcp — authenticated by the JWT middleware, with authorization
	// (component:connect) enforced inside the handler. Not part of the strict OpenAPI
	// chain. The stream endpoint that consumes its capability is registered further
	// down, alongside exec/wirelogs, once the cluster gateway is known to be available.
	var remoteConnectHandler *openapihandlers.RemoteConnectHandler
	if cfg.RemoteConnect.Enabled {
		remoteConnectAuthzChecker := svcpkg.NewAuthzChecker(runtime.pdp, logger.With("component", "remote-connect-authz"))
		remoteConnectHandler, err = openapihandlers.NewRemoteConnectHandler(
			k8sClient, planeClientProvider, remoteConnectAuthzChecker, cfg.RemoteConnect, logger)
		if err != nil {
			logger.Error("Failed to initialize remote-connect handler", slog.Any("error", err))
			os.Exit(1)
		}
		// Resolve: authenticated by the JWT middleware; authorization (component:connect)
		// enforced inside the handler.
		baseMux.Handle("POST /api/v1/remote-connect:resolve", jwtMiddleware(remoteConnectHandler))
		// Authorize: the remote-agent's per-stream callback. Registered WITHOUT the JWT
		// middleware — the remote-agent has no user JWT; the CP-signed capability in the
		// request body is the credential, verified inside the handler.
		authorizeHandler := openapihandlers.NewRemoteConnectAuthorizeHandler(
			remoteConnectHandler.VerifyKey(), remoteConnectHandler.TouchAgent, logger)
		baseMux.Handle("POST "+remoteconnect.AuthorizePath, authorizeHandler)
		// Heartbeat: the remote-agent's periodic liveness callback while it has live
		// sessions. Also unauthenticated at the middleware layer — the presented
		// capability is the credential (verified, expiry tolerated, inside the handler).
		heartbeatHandler := openapihandlers.NewRemoteConnectHeartbeatHandler(
			remoteConnectHandler.VerifyKey(), remoteConnectHandler.TouchAgent, logger)
		baseMux.Handle("POST "+remoteconnect.HeartbeatPath, heartbeatHandler)
		logger.Info("Remote-connect resolve + authorize + heartbeat endpoints registered",
			"resolve", "/api/v1/remote-connect:resolve",
			"authorize", remoteconnect.AuthorizePath, "heartbeat", remoteconnect.HeartbeatPath)

		// Reaper: GC remote-agents idle past the configured TTL, across every data plane.
		reaper := openapihandlers.NewRemoteAgentReaper(k8sClient, planeClientProvider, cfg.RemoteConnect, logger)
		go reaper.Start(ctx)
		logger.Info("Remote-connect remote-agent reaper started",
			"interval", cfg.RemoteConnect.ReaperInterval(), "ttl", cfg.RemoteConnect.ReaperTTL())
	}

	// Create OpenAPI handler with middleware chain. The chain's ordering rationale
	// lives in openapihandlers.OpenAPIMiddlewares, the single place route middleware
	// is composed. The generated routes are registered on the baseMux alongside /mcp.
	openapiMiddlewares, err := openapihandlers.OpenAPIMiddlewares(openapihandlers.OpenAPIMiddlewareOptions{
		Logger:         logger,
		AuthMiddleware: authMiddleware,
		AuditEmitter:   auditEmitter,
		AuditEnabled:   cfg.Audit.Enabled,
	})
	if err != nil {
		logger.Error("Failed to build OpenAPI middlewares", slog.Any("error", err))
		os.Exit(1)
	}
	handler := gen.HandlerWithOptions(strictHandler, gen.StdHTTPServerOptions{
		BaseRouter:  baseMux,
		Middlewares: openapiMiddlewares,
	})

	// Exec WebSocket and wirelogs endpoints are registered on a top-level mux
	// that wraps the OpenAPI handler: neither is in openapi.yaml, so neither
	// has an operationId to cross-reference against a spec — they get their
	// own hand-declared audit middleware instead (NewExecWirelogsAuditMiddleware).
	// The JWT middleware is applied directly to the exec handler for authentication.
	// Authorization is enforced inside the handler via AuthzChecker (component:exec).
	var topHandler http.Handler = handler
	if cfg.ClusterGateway.Enabled && gatewayURL != "" {
		execWirelogsAuditMw, err := openapihandlers.NewExecWirelogsAuditMiddleware(logger, auditEmitter, cfg.Audit.Enabled)
		if err != nil {
			logger.Error("Failed to build exec/wirelogs audit middleware", slog.Any("error", err))
			os.Exit(1)
		}
		// Outside jwtMiddleware, mirroring OpenAPIMiddlewares' ordering: auth
		// short-circuits a rejected request and never calls next, so the
		// pattern-map-driven middleware inside it never runs on a 401. These
		// two routes reach the data plane — a live shell and a live traffic
		// stream — so a rejected attempt on them is exactly the event worth
		// recording.
		unauthedExecWirelogsMw := audit.NewUnauthenticatedMiddleware(auditEmitter, audit.OriginAPI, cfg.Audit.Enabled)

		execAuthzChecker := svcpkg.NewAuthzChecker(runtime.pdp, logger.With("component", "exec-authz"))
		gwTLSConf, err := gatewayClient.BuildTLSConfig(&gatewayClient.TLSConfig{
			CAFile:             cfg.ClusterGateway.TLS.CACertPath,
			ClientCertFile:     cfg.ClusterGateway.TLS.ClientCertPath,
			ClientKeyFile:      cfg.ClusterGateway.TLS.ClientKeyPath,
			InsecureSkipVerify: cfg.ClusterGateway.TLS.Insecure,
		})
		if err != nil {
			logger.Error("Failed to build gateway TLS config for exec", slog.Any("error", err))
			os.Exit(1)
		}
		execHandler := openapihandlers.NewExecHandler(k8sClient, gwClient, gatewayURL, gwTLSConf, execAuthzChecker, logger)
		authedExecHandler := unauthedExecWirelogsMw(jwtMiddleware(execWirelogsAuditMw.Handler(execHandler)))

		// Wirelogs handler shares the same gateway TLS config and authz checker
		// (authz reuses logs:view at the component scope).
		wirelogsAuthzChecker := svcpkg.NewAuthzChecker(runtime.pdp, logger.With("component", "wirelogs-authz"))
		wirelogsHandler := openapihandlers.NewWirelogsHandler(
			k8sClient, gwClient, gatewayURL, gwTLSConf, wirelogsAuthzChecker, logger,
		)
		authedWirelogsHandler := unauthedExecWirelogsMw(jwtMiddleware(execWirelogsAuditMw.Handler(wirelogsHandler)))

		topMux := http.NewServeMux()
		topMux.Handle(openapihandlers.ExecRoutePattern, authedExecHandler)
		topMux.Handle(openapihandlers.WirelogsRoutePattern, authedWirelogsHandler)

		// remote-connect is not served through this gateway mux: occ dials the
		// per-project+env remote-agent's dedicated L4 Service directly. The control plane
		// only resolves + provisions and authorizes streams via the remote-agent callback
		// (both on baseMux).

		topMux.Handle("/", handler)
		topHandler = topMux
		logger.Info("Exec endpoint registered", "path", "/exec/namespaces/{ns}/components/{name}")
		logger.Info("Wirelogs endpoint registered",
			"path", "/api/v1/namespaces/{namespace}/environments/{environment}/wirelogs")
	}

	// Create server from configuration
	srv := server.New(cfg.Server.ToServerConfig(), topHandler, logger)

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
// Each enabled toolset is backed by the handler services layer. An unknown
// toolset name in cfg.MCP.Toolsets never reaches here — config.Validate
// (config.MCPConfig.ValidateMCPConfig) already rejects it against the same
// validToolsets set at startup.
func buildMCPToolsets(cfg *config.Config, svc *handlerservices.Services, logger *slog.Logger) *tools.Toolsets {
	toolsetsMap := cfg.MCP.ParseToolsets()

	logger.Info("Initializing MCP server", slog.Any("enabled_toolsets", cfg.MCP.Toolsets))

	handler := mcphandlers.NewMCPHandler(svc)
	return tools.NewToolsets(handler, toolsetsMap)
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
