// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"

	"golang.org/x/exp/slog"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
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

	// This is the promotion endpoint...
	mux.HandleFunc("POST "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/promote", h.PromoteComponent)

	// Build endpoints
	mux.HandleFunc("POST "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/builds", h.TriggerBuild)
	mux.HandleFunc("GET "+v1+"/orgs/{orgName}/projects/{projectName}/components/{componentName}/builds", h.ListBuilds)

	// Apply middleware
	return logger.LoggerMiddleware(h.logger)(mux)
}

// Health handles health check requests
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// Ready handles readiness check requests
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	// Add readiness checks (K8s connections, etc.)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ready"))
}
