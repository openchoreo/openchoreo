// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"

	apiaudit "github.com/openchoreo/openchoreo/internal/openchoreo-api/audit"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/mcphandlers"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server/middleware"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
	mcpmiddleware "github.com/openchoreo/openchoreo/internal/server/middleware/mcp"
	"github.com/openchoreo/openchoreo/internal/version"
	"github.com/openchoreo/openchoreo/pkg/mcp"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// Handler holds the services and provides HTTP handlers
type Handler struct {
	services *services.Services
	config   *config.Config
	logger   *slog.Logger
}

// New creates a new Handler instance
func New(services *services.Services, cfg *config.Config, logger *slog.Logger) *Handler {
	return &Handler{
		services: services,
		config:   cfg,
		logger:   logger,
	}
}

// Routes sets up all HTTP routes and returns the configured handler
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	v1 := "/api/v1"

	// ===== Initialize Middlewares =====

	// Global middlewares - applies to all routes
	loggerMiddleware := logger.LoggerMiddleware(h.logger)

	// Create route builder with global middleware
	routes := middleware.NewRouteBuilder(mux).With(loggerMiddleware)

	// ===== Public Routes (No Authentication Required) =====

	// Health & Readiness checks
	routes.HandleFunc("GET /health", h.Health)
	routes.HandleFunc("GET /ready", h.Ready)

	// Version endpoint
	routes.HandleFunc("GET /version", h.Version)

	// OAuth Protected Resource Metadata endpoint
	routes.HandleFunc("GET /.well-known/oauth-protected-resource", h.OAuthProtectedResourceMetadata)

	// Webhook endpoints (public - called by Git providers)
	routes.HandleFunc("POST "+v1+"/webhooks/github", h.HandleGitHubWebhook)
	routes.HandleFunc("POST "+v1+"/webhooks/gitlab", h.HandleGitLabWebhook)
	routes.HandleFunc("POST "+v1+"/webhooks/bitbucket", h.HandleBitbucketWebhook)

	// ===== Protected API Routes (JWT Authentication Required) =====

	// JWT authentication middleware - applies to protected routes only
	jwtAuth := h.InitJWTMiddleware()

	// Audit logging middleware - applies to protected routes only
	auditMiddleware := h.initAuditMiddleware()

	// MCP endpoint
	toolsets := getMCPServerToolsets(h)

	// MCP middleware
	mcpMiddleware := h.initMCPMiddleware()

	// MCP endpoint with chained middleware (logger -> auth401 -> jwt -> handler)
	mcpRoutes := routes.Group(mcpMiddleware, jwtAuth)
	mcpRoutes.Handle("/mcp", mcp.NewHTTPServer(toolsets))

	// Create protected route group with JWT auth and audit logging
	// Middleware order: logger -> jwt -> audit -> handler
	api := routes.With(jwtAuth, auditMiddleware)

	// Controlplane namespace operations
	api.HandleFunc("GET "+v1+"/namespaces", h.ListNamespaces)
	api.HandleFunc("POST "+v1+"/namespaces", h.CreateNamespace)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}", h.GetNamespace)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/secret-references", h.ListSecretReferences)

	// Apply/Delete operations (kubectl-like)
	api.HandleFunc("POST "+v1+"/apply", h.ApplyResource)
	api.HandleFunc("DELETE "+v1+"/delete", h.DeleteResource)

	// DataPlane management
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/dataplanes", h.ListDataPlanes)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/dataplanes", h.CreateDataPlane)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/dataplanes/{dpName}", h.GetDataPlane)

	// Environment management
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/environments", h.ListEnvironments)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/environments", h.CreateEnvironment)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/environments/{envName}", h.GetEnvironment)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/environments/{envName}/observer-url", h.GetEnvironmentObserverURL)

	// BuildPlane management
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/buildplanes", h.ListBuildPlanes)

	// ComponentType endpoints
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/component-types", h.ListComponentTypes)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/component-types/{ctName}/schema", h.GetComponentTypeSchema)

	// Workflow endpoints (generic workflows)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/workflows", h.ListWorkflows)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/workflows/{workflowName}/schema", h.GetWorkflowSchema)

	// ComponentWorkflow endpoints (component-specific workflows)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/component-workflows", h.ListComponentWorkflows)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/component-workflows/{cwName}/schema", h.GetComponentWorkflowSchema)
	api.HandleFunc("PATCH "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workflow-parameters", h.UpdateComponentWorkflowParameters)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workflow-runs", h.CreateComponentWorkflowRun)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workflow-runs", h.ListComponentWorkflowRuns)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workflow-runs/{runName}", h.GetComponentWorkflowRun)

	// Trait endpoints
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/traits", h.ListTraits)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/traits/{traitName}/schema", h.GetTraitSchema)

	// Project management
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects", h.ListProjects)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/projects", h.CreateProject)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}", h.GetProject)
	api.HandleFunc("DELETE "+v1+"/namespaces/{namespaceName}/projects/{projectName}", h.DeleteProject)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/deployment-pipeline", h.GetProjectDeploymentPipeline)

	// Component management
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components", h.ListComponents)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components", h.CreateComponent)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}", h.GetComponent)
	api.HandleFunc("DELETE "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}", h.DeleteComponent)
	api.HandleFunc("PATCH "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}", h.PatchComponent)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/schema", h.GetComponentSchema)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/environments/{environmentName}/release", h.GetEnvironmentRelease)

	// Component trait management
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/traits", h.ListComponentTraits)
	api.HandleFunc("PUT "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/traits", h.UpdateComponentTraits)

	// Component bindings
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/bindings", h.GetComponentBinding)
	api.HandleFunc("PATCH "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/bindings/{bindingName}", h.UpdateComponentBinding)

	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/component-releases", h.ListComponentReleases)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/component-releases", h.CreateComponentRelease)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/component-releases/{releaseName}", h.GetComponentRelease)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/component-releases/{releaseName}/schema", h.GetComponentReleaseSchema)

	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/release-bindings", h.ListReleaseBindings)
	api.HandleFunc("PATCH "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/release-bindings/{bindingName}", h.PatchReleaseBinding)

	// Deployment endpoint
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/deploy", h.DeployRelease)

	// Promotion endpoint
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/promote", h.PromoteComponent)

	// Observer URL endpoints
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/environments/{environmentName}/observer-url", h.GetComponentObserverURL)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/observer-url", h.GetBuildObserverURL)

	// Workload management
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workloads", h.CreateWorkload)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workloads", h.GetWorkloads)

	// Authorization admin endpoints
	api.HandleFunc("GET "+v1+"/authz/roles", h.ListRoles)
	api.HandleFunc("GET "+v1+"/authz/roles/{roleName}", h.GetRole)
	api.HandleFunc("POST "+v1+"/authz/roles", h.AddRole)
	api.HandleFunc("PUT "+v1+"/authz/roles/{roleName}", h.UpdateRole)
	api.HandleFunc("DELETE "+v1+"/authz/roles/{roleName}", h.RemoveRole)
	api.HandleFunc("GET "+v1+"/authz/role-mappings", h.ListRoleMappings)
	api.HandleFunc("POST "+v1+"/authz/role-mappings", h.AddRoleMapping)
	api.HandleFunc("PUT "+v1+"/authz/role-mappings/{mappingId}", h.UpdateRoleMapping)
	api.HandleFunc("DELETE "+v1+"/authz/role-mappings/{mappingId}", h.RemoveRoleMapping)
	api.HandleFunc("GET "+v1+"/authz/actions", h.ListActions)

	// User types endpoint
	api.HandleFunc("GET "+v1+"/user-types", h.listUserTypes)

	// Authorization evaluation endpoints
	api.HandleFunc("POST "+v1+"/authz/evaluate", h.Evaluate)
	api.HandleFunc("POST "+v1+"/authz/batch-evaluate", h.BatchEvaluate)
	api.HandleFunc("GET "+v1+"/authz/profile", h.GetSubjectProfile)

	// ObservabilityPlane management
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/observabilityplanes", h.ListObservabilityPlanes)

	return mux
}

func (h *Handler) listUserTypes(w http.ResponseWriter, r *http.Request) {
	userTypes := h.config.Security.UserTypes
	writeSuccessResponse(w, http.StatusOK, userTypes)
}

// InitJWTMiddleware initializes the JWT authentication middleware with configuration from environment.
// Exported for reuse by the new OpenAPI-generated server.
//
// TODO: Refactor to move JWT configuration to the config package. Reading environment variables
// should not be the responsibility of the handlers package. Consider creating a config.JWTConfig
// struct and a config.NewJWTMiddleware() function instead.
func (h *Handler) InitJWTMiddleware() func(http.Handler) http.Handler {
	// Get JWT configuration from environment variables
	jwtDisabled := os.Getenv(config.EnvJWTDisabled) == "true"
	jwksURL := os.Getenv(config.EnvJWKSURL)
	jwtIssuer := os.Getenv(config.EnvJWTIssuer)
	jwtAudience := os.Getenv(config.EnvJWTAudience) // Optional
	jwksURLTLSInsecureSkipVerify := os.Getenv(config.EnvJWKSURLTLSInsecureSkipVerify) == "true"

	// Create OAuth2 user type detector from configuration
	var detector *jwt.Resolver
	if len(h.config.Security.UserTypes) > 0 {
		var err error
		detector, err = jwt.NewResolver(h.config.Security.UserTypes)
		if err != nil {
			h.logger.Error("Failed to create OAuth2 user type detector", "error", err)
			// Continue without detector - JWT middleware will still work but won't resolve SubjectContext
		}
	}

	// Configure JWT middleware
	jwtConfig := jwt.Config{
		Disabled:                     jwtDisabled,
		JWKSURL:                      jwksURL,
		ValidateIssuer:               jwtIssuer,
		ValidateAudience:             jwtAudience,
		JWKSURLTLSInsecureSkipVerify: jwksURLTLSInsecureSkipVerify,
		Detector:                     detector,
		Logger:                       h.logger,
	}

	return jwt.Middleware(jwtConfig)
}

func (h *Handler) initMCPMiddleware() func(http.Handler) http.Handler {
	// Get MCP configuration from environment variables
	serverBaseURL := os.Getenv(config.EnvServerBaseURL)
	if serverBaseURL == "" {
		serverBaseURL = config.DefaultServerBaseURL
	}
	resourceMetadataURL := serverBaseURL + "/.well-known/oauth-protected-resource"

	return mcpmiddleware.Auth401Interceptor(resourceMetadataURL)
}

// initAuditMiddleware initializes the audit logging middleware
func (h *Handler) initAuditMiddleware() func(http.Handler) http.Handler {
	auditLogger := audit.NewLogger(h.logger, "openchoreo-api")
	actionDefinitions := apiaudit.GetActionDefinitions()
	resolver := audit.NewActionResolver(actionDefinitions)
	auditMw := audit.NewMiddleware(auditLogger, resolver)
	return auditMw.Handler
}

// Health handles health check requests
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK")) // Ignore write errors for health checks
}

// Ready handles readiness check requests
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	// Add readiness checks (K8s connections, etc.)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Ready")) // Ignore write errors for health checks
}

// Version handles version information requests
func (h *Handler) Version(w http.ResponseWriter, r *http.Request) {
	v := version.Get()
	response := models.VersionResponse{
		Name:        v.Name,
		Version:     v.Version,
		GitRevision: v.GitRevision,
		BuildTime:   v.BuildTime,
		GoOS:        v.GoOS,
		GoArch:      v.GoArch,
		GoVersion:   v.GoVersion,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func getMCPServerToolsets(h *Handler) *tools.Toolsets {
	// Read toolsets from environment variable
	toolsetsEnv := os.Getenv(config.EnvMCPToolsets)
	if toolsetsEnv == "" {
		// Default to all toolsets if not specified
		toolsetsEnv = string(tools.ToolsetNamespace) + "," +
			string(tools.ToolsetProject) + "," +
			string(tools.ToolsetComponent) + "," +
			string(tools.ToolsetBuild) + "," +
			string(tools.ToolsetDeployment) + "," +
			string(tools.ToolsetInfrastructure) + "," +
			string(tools.ToolsetSchema) + "," +
			string(tools.ToolsetResource)
	}

	// Parse toolsets
	toolsetsMap := parseToolsets(toolsetsEnv)

	// Log enabled toolsets
	enabledToolsets := make([]string, 0, len(toolsetsMap))
	for ts := range toolsetsMap {
		enabledToolsets = append(enabledToolsets, string(ts))
	}
	h.logger.Info("Initializing MCP server",
		slog.Any("enabled_toolsets", enabledToolsets))

	handler := &mcphandlers.MCPHandler{Services: h.services}

	// Create toolsets struct and enable based on configuration
	toolsets := &tools.Toolsets{}

	for toolsetType := range toolsetsMap {
		switch toolsetType {
		case tools.ToolsetNamespace:
			toolsets.NamespaceToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "namespace"))
		case tools.ToolsetProject:
			toolsets.ProjectToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "project"))
		case tools.ToolsetComponent:
			toolsets.ComponentToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "component"))
		case tools.ToolsetBuild:
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "build"))
		case tools.ToolsetDeployment:
			toolsets.DeploymentToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "deployment"))
		case tools.ToolsetInfrastructure:
			toolsets.InfrastructureToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "infrastructure"))
		case tools.ToolsetSchema:
			toolsets.SchemaToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "schema"))
		case tools.ToolsetResource:
			toolsets.ResourceToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "resource"))
		default:
			h.logger.Warn("Unknown toolset type", slog.String("toolset", string(toolsetType)))
		}
	}
	return toolsets
}

func parseToolsets(toolsetsStr string) map[tools.ToolsetType]bool {
	toolsetsMap := make(map[tools.ToolsetType]bool)
	if toolsetsStr == "" {
		return toolsetsMap
	}

	toolsets := strings.Split(toolsetsStr, ",")
	for _, ts := range toolsets {
		ts = strings.TrimSpace(ts)
		if ts != "" {
			toolsetsMap[tools.ToolsetType(ts)] = true
		}
	}
	return toolsetsMap
}
