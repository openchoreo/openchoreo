// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"k8s.io/client-go/rest"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// RouteConfig represents the configuration for a backend route
type RouteConfig struct {
	Name     string
	Endpoint string
	Auth     AuthConfig
}

// AuthConfig represents authentication configuration for a route
type AuthConfig struct {
	Type     string // "serviceaccount", "bearer", "basic", "none"
	Token    string
	Username string
	Password string
}

// Route represents a configured backend service route. This is used to route requests to different backend services.
type Route struct {
	Name      string
	Backend   string // "kubernetes", "http", "https"
	Endpoint  string
	Auth      AuthConfig
	Transport http.RoundTripper
}

// Router routes HTTP tunnel requests to different backend services
type Router struct {
	routes    map[string]*Route
	k8sConfig *rest.Config
	logger    *slog.Logger
}

// NewRouter creates a new router with configured routes
func NewRouter(k8sConfig *rest.Config, routeConfigs []RouteConfig, logger *slog.Logger) (*Router, error) {
	router := &Router{
		routes:    make(map[string]*Route),
		k8sConfig: k8sConfig,
		logger:    logger.With("component", "router"),
	}

	// Add default k8s route using agent's ServiceAccount
	k8sRoute, err := createK8sRoute(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s route: %w", err)
	}
	router.routes["k8s"] = k8sRoute

	logger.Info("registered default k8s route",
		"endpoint", k8sConfig.Host,
		"auth", "serviceaccount",
	)

	// Add configured routes. This has been added to allow the agent to route requests to external services.
	for _, cfg := range routeConfigs {
		route := createRoute(cfg)
		router.routes[cfg.Name] = route
		logger.Info("registered route",
			"name", cfg.Name,
			"endpoint", cfg.Endpoint,
			"auth", cfg.Auth.Type,
		)
	}

	return router, nil
}

// Route routes an HTTP tunnel request to the appropriate backend service
func (r *Router) Route(req *messaging.HTTPTunnelRequest) *messaging.HTTPTunnelResponse {
	logger := r.logger
	if req.GatewayRequestID != "" {
		logger = r.logger.With("requestId", req.GatewayRequestID)
	}

	route, exists := r.routes[req.Target]
	if !exists {
		logger.Warn("unknown target requested",
			"target", req.Target,
			"availableTargets", r.getAvailableTargets(),
		)
		return messaging.NewHTTPTunnelErrorResponse(req, http.StatusNotFound,
			fmt.Sprintf("unknown target: %s", req.Target))
	}

	fullPath := req.Path
	if req.Query != "" {
		fullPath += "?" + req.Query
	}

	targetURL := route.Endpoint + fullPath

	logger.Info("agent routing request to backend",
		"target", req.Target,
		"method", req.Method,
		"path", req.Path,
		"url", targetURL,
	)

	// Create HTTP request
	httpReq, err := http.NewRequest(req.Method, targetURL, bytes.NewReader(req.Body))
	if err != nil {
		return messaging.NewHTTPTunnelErrorResponse(req, http.StatusInternalServerError,
			fmt.Sprintf("failed to create request: %v", err))
	}

	httpReq.Header = req.Headers

	route.applyAuth(httpReq)

	resp, err := route.Transport.RoundTrip(httpReq)
	if err != nil {
		logger.Error("backend request failed",
			"target", req.Target,
			"url", targetURL,
			"error", err,
		)
		return messaging.NewHTTPTunnelErrorResponse(req, http.StatusBadGateway,
			fmt.Sprintf("backend request failed: %v", err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read response body",
			"target", req.Target,
			"error", err,
		)
		return messaging.NewHTTPTunnelErrorResponse(req, http.StatusBadGateway,
			fmt.Sprintf("failed to read response: %v", err))
	}

	logger.Info("agent request completed",
		"target", req.Target,
		"statusCode", resp.StatusCode,
		"bodySize", len(body),
	)

	return messaging.NewHTTPTunnelSuccessResponse(req, resp.StatusCode, resp.Header, body)
}

func (r *Router) getAvailableTargets() []string {
	targets := make([]string, 0, len(r.routes))
	for name := range r.routes {
		targets = append(targets, name)
	}
	return targets
}

func createK8sRoute(config *rest.Config) (*Route, error) {
	transport, err := rest.TransportFor(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s transport: %w", err)
	}

	return &Route{
		Name:     "k8s",
		Backend:  "kubernetes",
		Endpoint: config.Host,
		Auth: AuthConfig{
			Type:  "serviceaccount",
			Token: config.BearerToken,
		},
		Transport: transport,
	}, nil
}

// createRoute creates a route from configuration
func createRoute(cfg RouteConfig) *Route {
	// Create HTTP transport
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false, //nolint:gosec // Can be configured per route if needed
		},
	}

	// For HTTPS endpoints with self-signed certs, we might need to skip verification
	// This should be configurable per route in the future
	if cfg.Auth.Type == "none" {
		transport.TLSClientConfig.InsecureSkipVerify = true //nolint:gosec // Intentional for internal services
	}

	return &Route{
		Name:      cfg.Name,
		Backend:   "http",
		Endpoint:  cfg.Endpoint,
		Auth:      cfg.Auth,
		Transport: transport,
	}
}

// applyAuth applies authentication to an HTTP request
func (r *Route) applyAuth(req *http.Request) {
	switch r.Auth.Type {
	case "serviceaccount", "bearer":
		if r.Auth.Token != "" {
			req.Header.Set("Authorization", "Bearer "+r.Auth.Token)
		}
	case "basic":
		if r.Auth.Username != "" && r.Auth.Password != "" {
			req.SetBasicAuth(r.Auth.Username, r.Auth.Password)
		}
	case "none":
		// No authentication
	}
}
