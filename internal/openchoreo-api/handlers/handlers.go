// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	apiaudit "github.com/openchoreo/openchoreo/internal/openchoreo-api/audit"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/mcphandlers"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
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

	// MCP endpoint (only if enabled)
	if h.config.MCP.Enabled {
		toolsets := getMCPServerToolsets(h)
		mcpMiddleware := h.initMCPMiddleware()
		mcpRoutes := routes.Group(mcpMiddleware, jwtAuth)
		mcpRoutes.Handle("/mcp", mcp.NewHTTPServer(toolsets))
	}

	// Create protected route group with JWT auth and audit logging
	// Middleware order: logger -> jwt -> audit -> handler
	api := routes.With(jwtAuth, auditMiddleware)

	// Controlplane namespace operations
	api.HandleFunc("GET "+v1+"/namespaces", h.ListNamespaces)
	api.HandleFunc("POST "+v1+"/namespaces", h.CreateNamespace)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}", h.GetNamespace)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/secret-references", h.ListSecretReferences)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/git-secrets", h.ListGitSecrets)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/git-secrets", h.CreateGitSecret)
	api.HandleFunc("DELETE "+v1+"/namespaces/{namespaceName}/git-secrets/{secretName}", h.DeleteGitSecret)

	// Apply/Delete operations (kubectl-like)
	api.HandleFunc("POST "+v1+"/apply", h.ApplyResource)
	api.HandleFunc("DELETE "+v1+"/delete", h.DeleteResource)

	// TODO: Remove this generic resource GET endpoint after the API spec redesign
	// Generic resource GET endpoint
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/resources/{kind}/{resourceName}", h.GetResource)

	// DataPlane management
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/dataplanes", h.ListDataPlanes)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/dataplanes", h.CreateDataPlane)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/dataplanes/{dpName}", h.GetDataPlane)

	// Environment management
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/environments", h.ListEnvironments)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/environments", h.CreateEnvironment)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/environments/{envName}", h.GetEnvironment)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/environments/{envName}/observer-url", h.GetEnvironmentObserverURL)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/environments/{envName}/rca-agent-url", h.GetRCAAgentURL)

	// BuildPlane management
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/buildplanes", h.ListBuildPlanes)

	// ComponentType endpoints
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/component-types/definition", h.CreateComponentTypeDefinition)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/component-types", h.ListComponentTypes)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/component-types/{ctName}/schema", h.GetComponentTypeSchema)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/component-types/{ctName}/definition", h.GetComponentTypeDefinition)
	api.HandleFunc("PUT "+v1+"/namespaces/{namespaceName}/component-types/{ctName}/definition", h.UpdateComponentTypeDefinition)
	api.HandleFunc("DELETE "+v1+"/namespaces/{namespaceName}/component-types/{ctName}/definition", h.DeleteComponentTypeDefinition)

	// Workflow endpoints (generic workflows)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/workflows", h.ListWorkflows)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/workflows/{workflowName}/schema", h.GetWorkflowSchema)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/workflows/{workflowName}/definition", h.GetWorkflowDefinition)
	api.HandleFunc("PUT "+v1+"/namespaces/{namespaceName}/workflows/{workflowName}/definition", h.UpdateWorkflowDefinition)
	api.HandleFunc("DELETE "+v1+"/namespaces/{namespaceName}/workflows/{workflowName}/definition", h.DeleteWorkflowDefinition)

	// WorkflowRun endpoints (generic workflow executions)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/workflow-runs", h.ListWorkflowRuns)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/workflow-runs", h.CreateWorkflowRun)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/workflow-runs/{runName}", h.GetWorkflowRun)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/workflow-runs/{runName}/status", h.GetWorkflowRunStatus)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/workflow-runs/{runName}/logs", h.GetWorkflowRunLogs)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/workflow-runs/{runName}/events", h.GetWorkflowRunEvents)

	// ComponentWorkflow endpoints (component-specific workflows)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/component-workflows/definition", h.CreateComponentWorkflowDefinition)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/component-workflows", h.ListComponentWorkflows)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/component-workflows/{cwName}/schema", h.GetComponentWorkflowSchema)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/component-workflows/{cwName}/definition", h.GetComponentWorkflowDefinition)
	api.HandleFunc("PUT "+v1+"/namespaces/{namespaceName}/component-workflows/{cwName}/definition", h.UpdateComponentWorkflowDefinition)
	api.HandleFunc("DELETE "+v1+"/namespaces/{namespaceName}/component-workflows/{cwName}/definition", h.DeleteComponentWorkflowDefinition)
	api.HandleFunc("PATCH "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workflow-parameters", h.UpdateComponentWorkflowParameters)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workflow-runs", h.CreateComponentWorkflowRun)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workflow-runs", h.ListComponentWorkflowRuns)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workflow-runs/{runName}", h.GetComponentWorkflowRun)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workflow-runs/{runName}/status", h.GetComponentWorkflowRunStatus)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workflow-runs/{runName}/logs", h.GetComponentWorkflowRunLogs)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workflow-runs/{runName}/events", h.GetComponentWorkflowRunEvents)

	// Trait endpoints
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/traits/definition", h.CreateTraitDefinition)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/traits", h.ListTraits)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/traits/{traitName}/schema", h.GetTraitSchema)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/traits/{traitName}/definition", h.GetTraitDefinition)
	api.HandleFunc("PUT "+v1+"/namespaces/{namespaceName}/traits/{traitName}/definition", h.UpdateTraitDefinition)
	api.HandleFunc("DELETE "+v1+"/namespaces/{namespaceName}/traits/{traitName}/definition", h.DeleteTraitDefinition)

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
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/environments/{environmentName}/release/resources", h.GetReleaseResourceTree)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/environments/{environmentName}/release/resources/events", h.GetResourceEvents)

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

	// Authorization - Actions
	api.HandleFunc("GET "+v1+"/authz/actions", h.ListActions)

	// Authorization - Cluster Roles
	api.HandleFunc("GET "+v1+"/clusterroles", h.ListClusterRoles)
	api.HandleFunc("POST "+v1+"/clusterroles", h.CreateClusterRole)
	api.HandleFunc("GET "+v1+"/clusterroles/{name}", h.GetClusterRole)
	api.HandleFunc("PUT "+v1+"/clusterroles/{name}", h.UpdateClusterRole)
	api.HandleFunc("DELETE "+v1+"/clusterroles/{name}", h.DeleteClusterRole)

	// Authorization - Cluster Role Bindings
	api.HandleFunc("GET "+v1+"/clusterrolebindings", h.ListClusterRoleBindings)
	api.HandleFunc("POST "+v1+"/clusterrolebindings", h.CreateClusterRoleBinding)
	api.HandleFunc("GET "+v1+"/clusterrolebindings/{name}", h.GetClusterRoleBinding)
	api.HandleFunc("PUT "+v1+"/clusterrolebindings/{name}", h.UpdateClusterRoleBinding)
	api.HandleFunc("DELETE "+v1+"/clusterrolebindings/{name}", h.DeleteClusterRoleBinding)

	// Authorization - Namespace Roles
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/roles", h.ListNamespaceRoles)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/roles", h.CreateNamespaceRole)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/roles/{name}", h.GetNamespaceRole)
	api.HandleFunc("PUT "+v1+"/namespaces/{namespaceName}/roles/{name}", h.UpdateNamespaceRole)
	api.HandleFunc("DELETE "+v1+"/namespaces/{namespaceName}/roles/{name}", h.DeleteNamespaceRole)

	// Authorization - Namespace Role Bindings
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/rolebindings", h.ListNamespaceRoleBindings)
	api.HandleFunc("POST "+v1+"/namespaces/{namespaceName}/rolebindings", h.CreateNamespaceRoleBinding)
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/rolebindings/{name}", h.GetNamespaceRoleBinding)
	api.HandleFunc("PUT "+v1+"/namespaces/{namespaceName}/rolebindings/{name}", h.UpdateNamespaceRoleBinding)
	api.HandleFunc("DELETE "+v1+"/namespaces/{namespaceName}/rolebindings/{name}", h.DeleteNamespaceRoleBinding)

	// User types endpoint
	api.HandleFunc("GET "+v1+"/user-types", h.listUserTypes)

	// Authorization evaluation endpoints
	api.HandleFunc("POST "+v1+"/authz/evaluate", h.Evaluate)
	api.HandleFunc("POST "+v1+"/authz/batch-evaluate", h.BatchEvaluate)
	api.HandleFunc("GET "+v1+"/authz/profile", h.GetSubjectProfile)

	// ObservabilityPlane management
	api.HandleFunc("GET "+v1+"/namespaces/{namespaceName}/observabilityplanes", h.ListObservabilityPlanes)

	// ClusterDataPlane management
	api.HandleFunc("GET "+v1+"/clusterdataplanes", h.ListClusterDataPlanes)
	api.HandleFunc("POST "+v1+"/clusterdataplanes", h.CreateClusterDataPlane)
	api.HandleFunc("GET "+v1+"/clusterdataplanes/{cdpName}", h.GetClusterDataPlane)

	// ClusterBuildPlane management
	api.HandleFunc("GET "+v1+"/clusterbuildplanes", h.ListClusterBuildPlanes)

	// ClusterObservabilityPlane management
	api.HandleFunc("GET "+v1+"/clusterobservabilityplanes", h.ListClusterObservabilityPlanes)

	// Plane K8s resource proxy (read-only)
	api.HandleFunc("GET "+v1+"/oc-namespaces/{ocNamespace}/dataplanes/{dpName}/{k8sPath...}", h.ProxyDataPlaneK8s)
	api.HandleFunc("GET "+v1+"/oc-namespaces/{ocNamespace}/buildplanes/{bpName}/{k8sPath...}", h.ProxyBuildPlaneK8s)
	api.HandleFunc("GET "+v1+"/oc-namespaces/{ocNamespace}/observabilityplanes/{opName}/{k8sPath...}", h.ProxyObservabilityPlaneK8s)
	api.HandleFunc("GET "+v1+"/cluster-dataplanes/{cdpName}/{k8sPath...}", h.ProxyClusterDataPlaneK8s)
	api.HandleFunc("GET "+v1+"/cluster-buildplanes/{cbpName}/{k8sPath...}", h.ProxyClusterBuildPlaneK8s)
	api.HandleFunc("GET "+v1+"/cluster-observabilityplanes/{copName}/{k8sPath...}", h.ProxyClusterObservabilityPlaneK8s)

	return mux
}

func (h *Handler) listUserTypes(w http.ResponseWriter, r *http.Request) {
	userTypes := h.config.Security.ToSubjectUserTypeConfigs()
	writeSuccessResponse(w, http.StatusOK, userTypes)
}

// InitJWTMiddleware initializes the JWT authentication middleware from unified configuration.
// Exported for reuse by the new OpenAPI-generated server.
func (h *Handler) InitJWTMiddleware() func(http.Handler) http.Handler {
	jwtCfg := &h.config.Security.Authentication.JWT

	// Create OAuth2 user type resolver from configuration
	var resolver *jwt.Resolver
	subjectUserTypes := h.config.Security.ToSubjectUserTypeConfigs()
	if len(subjectUserTypes) > 0 {
		var err error
		resolver, err = jwt.NewResolver(subjectUserTypes)
		if err != nil {
			h.logger.Error("Failed to create OAuth2 user type resolver", "error", err)
			// Continue without resolver - JWT middleware will still work but won't resolve SubjectContext
		}
	}

	return jwt.Middleware(jwtCfg.ToJWTMiddlewareConfig(&h.config.Identity.OIDC, h.logger, resolver))
}

func (h *Handler) initMCPMiddleware() func(http.Handler) http.Handler {
	resourceMetadataURL := h.config.Server.PublicURL + "/.well-known/oauth-protected-resource"
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
	// Get enabled toolsets from config (defaults are set in MCPDefaults())
	toolsetsMap := h.config.MCP.ParseToolsets()

	// Log enabled toolsets
	h.logger.Info("Initializing MCP server", slog.Any("enabled_toolsets", h.config.MCP.Toolsets))

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
