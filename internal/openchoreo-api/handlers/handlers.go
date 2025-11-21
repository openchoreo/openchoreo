// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/mcphandlers"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server/middleware"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
	mcpmiddleware "github.com/openchoreo/openchoreo/internal/server/middleware/mcp"
	"github.com/openchoreo/openchoreo/pkg/mcp"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// Handler holds the services and provides HTTP handlers
type Handler struct {
	services *services.Services
	logger   *slog.Logger
}

// New creates a new Handler instance
func New(services *services.Services, logger *slog.Logger) *Handler {
	return &Handler{
		services: services,
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

	// OAuth Protected Resource Metadata endpoint
	routes.HandleFunc("GET /.well-known/oauth-protected-resource", h.OAuthProtectedResourceMetadata)

	// ===== Protected API Routes (JWT Authentication Required) =====

	// JWT authentication middleware - applies to protected routes only
	jwtAuth := h.initJWTMiddleware()

	// MCP endpoint
	toolsets := getMCPServerToolsets(h)

	// MCP middleware
	mcpMiddleware := h.initMCPMiddleware()

	// MCP endpoint with chained middleware (logger -> auth401 -> jwt -> handler)
	mcpRoutes := routes.Group(mcpMiddleware, jwtAuth)
	mcpRoutes.Handle("/mcp", mcp.NewHTTPServer(toolsets))

	// Create protected route group with JWT auth
	api := routes.With(jwtAuth)

	// Organization operations
	api.HandleFunc("GET "+v1+"/orgs", h.ListOrganizations)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}", h.GetOrganization)

	// Apply/Delete operations (kubectl-like)
	api.HandleFunc("POST "+v1+"/apply", h.ApplyResource)
	api.HandleFunc("DELETE "+v1+"/delete", h.DeleteResource)

	// DataPlane management
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/dataplanes", h.ListDataPlanes)
	api.HandleFunc("POST "+v1+"/orgs/{orgName}/dataplanes", h.CreateDataPlane)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/dataplanes/{dpName}", h.GetDataPlane)

	// Environment management
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/environments", h.ListEnvironments)
	api.HandleFunc("POST "+v1+"/orgs/{orgName}/environments", h.CreateEnvironment)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/environments/{envName}", h.GetEnvironment)

	// BuildPlane & Build Templates
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/buildplanes", h.ListBuildPlanes)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/build-templates", h.ListBuildTemplates)

	// ComponentType endpoints
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/component-types", h.ListComponentTypes)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/component-types/{ctName}/schema", h.GetComponentTypeSchema)

	// Workflow endpoints
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/workflows", h.ListWorkflows)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/workflows/{workflowName}/schema", h.GetWorkflowSchema)

	// Trait endpoints
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/traits", h.ListTraits)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/traits/{traitName}/schema", h.GetTraitSchema)

	// Project management
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects", h.ListProjects)
	api.HandleFunc("POST "+v1+"/orgs/{orgName}/projects", h.CreateProject)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}", h.GetProject)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/deployment-pipeline", h.GetProjectDeploymentPipeline)

	// Component management
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components", h.ListComponents)
	api.HandleFunc("POST "+v1+"/orgs/{orgName}/projects/{projectName}/components", h.CreateComponent)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}", h.GetComponent)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/schema", h.GetComponentSchema)
	api.HandleFunc("PATCH "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/workflow-schema", h.UpdateComponentWorkflowSchema)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/environments/{environmentName}/release", h.GetEnvironmentRelease)

	// Component bindings
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/bindings", h.GetComponentBinding)
	api.HandleFunc("PATCH "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/bindings/{bindingName}", h.UpdateComponentBinding)

	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/component-releases", h.ListComponentReleases)
	api.HandleFunc("POST "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/component-releases", h.CreateComponentRelease)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/component-releases/{releaseName}", h.GetComponentRelease)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/component-releases/{releaseName}/schema", h.GetComponentReleaseSchema)

	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/release-bindings", h.ListReleaseBindings)
	api.HandleFunc("PATCH "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/release-bindings/{bindingName}", h.PatchReleaseBinding)

	// Deployment endpoint
	api.HandleFunc("POST "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/deploy", h.DeployRelease)

	// Promotion endpoint
	api.HandleFunc("POST "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/promote", h.PromoteComponent)

	// Build operations
	api.HandleFunc("POST "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/builds", h.TriggerBuild)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/builds", h.ListBuilds)

	// Observer URL endpoints
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/environments/{environmentName}/observer-url", h.GetComponentObserverURL)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/observer-url", h.GetBuildObserverURL)

	// Workload management
	api.HandleFunc("POST "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/workloads", h.CreateWorkload)
	api.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/workloads", h.GetWorkloads)

	return mux
}

// initJWTMiddleware initializes the JWT authentication middleware with configuration from environment
func (h *Handler) initJWTMiddleware() func(http.Handler) http.Handler {
	// Get JWT configuration from environment variables
	jwtDisabled := os.Getenv(config.EnvJWTDisabled) == "true"
	jwksURL := os.Getenv(config.EnvJWKSURL)
	jwtIssuer := os.Getenv(config.EnvJWTIssuer)
	jwtAudience := os.Getenv(config.EnvJWTAudience) // Optional

	// Configure JWT middleware
	config := jwt.Config{
		Disabled:         jwtDisabled,
		JWKSURL:          jwksURL,
		ValidateIssuer:   jwtIssuer,
		ValidateAudience: jwtAudience, // Only validates if set
		Logger:           h.logger,
	}

	return jwt.Middleware(config)
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

func getMCPServerToolsets(h *Handler) *tools.Toolsets {
	// Read toolsets from environment variable
	toolsetsEnv := os.Getenv(config.EnvMCPToolsets)
	if toolsetsEnv == "" {
		// Default to all toolsets if not specified
		toolsetsEnv = string(tools.ToolsetOrganization) + "," +
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
		case tools.ToolsetOrganization:
			toolsets.OrganizationToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "organization"))
		case tools.ToolsetProject:
			toolsets.ProjectToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "project"))
		case tools.ToolsetComponent:
			toolsets.ComponentToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "component"))
		case tools.ToolsetBuild:
			toolsets.BuildToolset = handler
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
