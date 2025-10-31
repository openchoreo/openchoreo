// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"
	"os"
	"strings"

	"golang.org/x/exp/slog"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/mcphandlers"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/pkg/mcp"
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

	// Health endpoints
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /ready", h.Ready)

	// API versioning
	v1 := "/api/v1"

	// Apply endpoint (similar to kubectl apply)
	mux.HandleFunc("POST "+v1+"/apply", h.ApplyResource)

	// Delete endpoint (similar to kubectl delete)
	mux.HandleFunc("DELETE "+v1+"/delete", h.DeleteResource)

	// Organization endpoints
	mux.HandleFunc("GET "+v1+"/orgs", h.ListOrganizations)
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}", h.GetOrganization)

	// DataPlane endpoints
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/dataplanes", h.ListDataPlanes)
	mux.HandleFunc("POST "+v1+"/orgs/{orgName}/dataplanes", h.CreateDataPlane)
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/dataplanes/{dpName}", h.GetDataPlane)

	// Environment endpoints
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/environments", h.ListEnvironments)
	mux.HandleFunc("POST "+v1+"/orgs/{orgName}/environments", h.CreateEnvironment)
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/environments/{envName}", h.GetEnvironment)

	// BuildPlane endpoints
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/buildplanes", h.ListBuildPlanes)
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/build-templates", h.ListBuildTemplates)

	// Project endpoints
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/projects", h.ListProjects)
	mux.HandleFunc("POST "+v1+"/orgs/{orgName}/projects", h.CreateProject)
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}", h.GetProject)
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/deployment-pipeline", h.GetProjectDeploymentPipeline)

	// Component endpoints
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components", h.ListComponents)
	mux.HandleFunc("POST "+v1+"/orgs/{orgName}/projects/{projectName}/components", h.CreateComponent)
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}", h.GetComponent)

	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/bindings", h.GetComponentBinding)
	mux.HandleFunc("PATCH "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/bindings/{bindingName}", h.UpdateComponentBinding)

	// This is the promotion endpoint...
	mux.HandleFunc("POST "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/promote", h.PromoteComponent)

	// Build endpoints
	mux.HandleFunc("POST "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/builds", h.TriggerBuild)
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/builds", h.ListBuilds)

	// Observer URL endpoints
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/environments/{environmentName}/observer-url", h.GetComponentObserverURL)
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/observer-url", h.GetBuildObserverURL)

	// Workload endpoints
	mux.HandleFunc("POST "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/workloads", h.CreateWorkload)
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/workloads", h.GetWorkloads)

	// MCP endpoint
	toolsets := getMCPServerToolsets(h)
	mux.Handle("/mcp", mcp.NewHTTPServer(toolsets))

	// Apply middleware
	return logger.LoggerMiddleware(h.logger)(mux)
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

func getMCPServerToolsets(h *Handler) *mcp.Toolsets {
	// Read toolsets from environment variable
	toolsetsEnv := os.Getenv("MCP_TOOLSETS")
	if toolsetsEnv == "" {
		// Default to all toolsets if not specified
		toolsetsEnv = string(mcp.ToolsetOrganization) + "," +
			string(mcp.ToolsetProject) + "," +
			string(mcp.ToolsetComponent) + "," +
			string(mcp.ToolsetBuild) + "," +
			string(mcp.ToolsetDeployment) + "," +
			string(mcp.ToolsetInfrastructure)
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
	toolsets := &mcp.Toolsets{}

	for toolsetType := range toolsetsMap {
		switch toolsetType {
		case mcp.ToolsetOrganization:
			toolsets.OrganizationToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "organization"))
		case mcp.ToolsetProject:
			toolsets.ProjectToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "project"))
		case mcp.ToolsetComponent:
			toolsets.ComponentToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "component"))
		case mcp.ToolsetBuild:
			toolsets.BuildToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "build"))
		case mcp.ToolsetDeployment:
			toolsets.DeploymentToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "deployment"))
		case mcp.ToolsetInfrastructure:
			toolsets.InfrastructureToolset = handler
			h.logger.Debug("Enabled MCP toolset", slog.String("toolset", "infrastructure"))
		default:
			h.logger.Warn("Unknown toolset type", slog.String("toolset", string(toolsetType)))
		}
	}
	return toolsets
}

func parseToolsets(toolsetsStr string) map[mcp.ToolsetType]bool {
	toolsetsMap := make(map[mcp.ToolsetType]bool)
	if toolsetsStr == "" {
		return toolsetsMap
	}

	toolsets := strings.Split(toolsetsStr, ",")
	for _, ts := range toolsets {
		ts = strings.TrimSpace(ts)
		if ts != "" {
			toolsetsMap[mcp.ToolsetType(ts)] = true
		}
	}
	return toolsetsMap
}
