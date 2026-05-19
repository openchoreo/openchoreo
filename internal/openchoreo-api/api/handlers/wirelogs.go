// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// WirelogsHandler streams Cilium Hubble flow events for a component as a
// Server-Sent Events response. It proxies an SSE stream from the cluster-gateway
// which in turn fans the request out to the data-plane cluster-agent.
type WirelogsHandler struct {
	k8sClient      client.Client
	gatewayClient  *gatewayClient.Client
	gatewayURL     string
	gatewayTLSConf *tls.Config
	authzChecker   *svcpkg.AuthzChecker
	httpClient     *http.Client
	logger         *slog.Logger
}

// NewWirelogsHandler creates a new wirelogs handler.
//
// The handler uses its own *http.Client (rather than the shared gatewayClient
// httpClient) because the gateway client applies a request-level timeout that
// is incompatible with the long-lived SSE stream; we rely on the inbound
// request's context for cancellation instead.
func NewWirelogsHandler(k8sClient client.Client, gwClient *gatewayClient.Client, gatewayURL string, gwTLSConf *tls.Config, authzChecker *svcpkg.AuthzChecker, logger *slog.Logger) *WirelogsHandler {
	return &WirelogsHandler{
		k8sClient:      k8sClient,
		gatewayClient:  gwClient,
		gatewayURL:     gatewayURL,
		gatewayTLSConf: gwTLSConf,
		authzChecker:   authzChecker,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: gwTLSConf,
			},
		},
		logger: logger.With("component", "wirelogs-handler"),
	}
}

// ServeHTTP authorizes the caller, resolves the target data plane, and proxies
// the gateway's SSE stream to the client.
// URL: /wirelogs/namespaces/{namespace}/projects/{project}/components/{component}?environment=...
func (h *WirelogsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	namespace, project, componentName, ok := parseWirelogsPath(r.URL.Path)
	if !ok {
		http.Error(w, "invalid wirelogs URL: expected /wirelogs/namespaces/{ns}/projects/{proj}/components/{name}", http.StatusBadRequest)
		return
	}

	envName := r.URL.Query().Get("environment")

	ctx := r.Context()
	logger := h.logger.With("namespace", namespace, "component", componentName)

	// Authorize: caller must have logs:view on the component.
	if h.authzChecker == nil {
		logger.Error("Authorization checker not configured")
		http.Error(w, "authorization not configured", http.StatusInternalServerError)
		return
	}
	if err := h.authzChecker.Check(ctx, svcpkg.CheckRequest{
		Action:       authz.ActionViewLogs,
		ResourceType: "component",
		ResourceID:   componentName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespace,
			Project:   project,
		},
	}); err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			http.Error(w, "you do not have permission to view wirelogs for this component", http.StatusForbidden)
			return
		}
		logger.Error("Authorization check failed", "error", err)
		http.Error(w, "authorization check failed", http.StatusInternalServerError)
		return
	}

	plane, err := h.resolvePlane(ctx, namespace, componentName, project, &envName)
	if err != nil {
		logger.Error("Failed to resolve data plane for wirelogs", "error", err)
		http.Error(w, fmt.Sprintf("failed to resolve data plane: %v", err), http.StatusBadRequest)
		return
	}

	logger = logger.With("environment", envName,
		"planeType", plane.planeType, "planeID", plane.planeID)

	flusher, ok := w.(http.Flusher)
	if !ok {
		logger.Error("ResponseWriter does not support flushing; cannot stream SSE")
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// The http.Server's WriteTimeout is an absolute deadline from when request
	// headers are read; for a long-lived SSE stream it would kill the connection
	// after that deadline regardless of activity (the classic "curl: (18) transfer
	// closed with outstanding read data remaining" once nothing has been sent for
	// a while). Clear the deadline on this connection only — other endpoints keep
	// the server's default protection.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		logger.Warn("Failed to disable write deadline for SSE stream", "error", err)
	}

	gwURL, err := h.buildGatewayWirelogsURL(plane, componentName, project, envName, namespace)
	if err != nil {
		logger.Error("Failed to build gateway wirelogs URL", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	gwReq, err := http.NewRequestWithContext(ctx, http.MethodGet, gwURL, nil)
	if err != nil {
		logger.Error("Failed to build gateway request", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	gwReq.Header.Set("Accept", "text/event-stream")

	resp, err := h.httpClient.Do(gwReq)
	if err != nil {
		logger.Error("Failed to connect to gateway wirelogs endpoint", "error", err)
		http.Error(w, fmt.Sprintf("failed to connect to data plane: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		logger.Error("Gateway returned non-OK status", "status", resp.StatusCode, "body", string(body))
		// Surface the gateway's status code unless it's something we shouldn't pass through.
		status := resp.StatusCode
		if status < 400 || status >= 600 {
			status = http.StatusBadGateway
		}
		http.Error(w, fmt.Sprintf("gateway error: %s", strings.TrimSpace(string(body))), status)
		return
	}

	// Commit to SSE: write headers (no Content-Length, force flush on each chunk).
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache, no-transform")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	logger.Info("Wirelogs SSE stream started")

	// The gateway already emits valid SSE framing — passing bytes through
	// verbatim preserves event boundaries. Flush after every read so events
	// reach the client immediately rather than being buffered.
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				logger.Debug("Client write failed; ending stream", "error", werr)
				return
			}
			flusher.Flush()
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) && !errors.Is(readErr, context.Canceled) {
				logger.Debug("Gateway stream ended with error", "error", readErr)
			}
			return
		}
	}
}

// resolvePlane resolves the data plane for a component+environment without
// requiring a specific pod (unlike exec). The flow filter targets pods by label.
func (h *WirelogsHandler) resolvePlane(ctx context.Context, namespace, componentName, project string, envName *string) (execPlaneInfo, error) {
	comp := &openchoreov1alpha1.Component{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, comp); err != nil {
		return execPlaneInfo{}, fmt.Errorf("component %q not found: %w", componentName, err)
	}

	if *envName == "" {
		if project == "" {
			return execPlaneInfo{}, fmt.Errorf("--project or --environment is required")
		}
		resolved, err := h.resolveLowestEnvironment(ctx, namespace, project)
		if err != nil {
			return execPlaneInfo{}, err
		}
		*envName = resolved
	}

	env := &openchoreov1alpha1.Environment{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: *envName}, env); err != nil {
		return execPlaneInfo{}, fmt.Errorf("environment %q not found: %w", *envName, err)
	}
	if env.Spec.DataPlaneRef == nil {
		return execPlaneInfo{}, fmt.Errorf("environment %q has no data plane reference", *envName)
	}

	dpResult, err := controller.GetDataPlaneFromRef(ctx, h.k8sClient, env.Namespace, env.Spec.DataPlaneRef)
	if err != nil {
		return execPlaneInfo{}, fmt.Errorf("failed to resolve data plane: %w", err)
	}

	plane := resolveExecPlaneInfo(dpResult)
	if plane.planeID == "" {
		return execPlaneInfo{}, fmt.Errorf("failed to determine plane ID for environment %q", *envName)
	}
	return plane, nil
}

// resolveLowestEnvironment finds the root environment from the project's deployment pipeline.
func (h *WirelogsHandler) resolveLowestEnvironment(ctx context.Context, namespace, projectName string) (string, error) {
	proj := &openchoreov1alpha1.Project{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: projectName}, proj); err != nil {
		return "", fmt.Errorf("project %q not found: %w", projectName, err)
	}

	if proj.Spec.DeploymentPipelineRef.Name == "" {
		return "", fmt.Errorf("project %q has no deployment pipeline configured", projectName)
	}

	pipeline := &openchoreov1alpha1.DeploymentPipeline{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: proj.Spec.DeploymentPipelineRef.Name}, pipeline); err != nil {
		return "", fmt.Errorf("deployment pipeline not found: %w", err)
	}

	if len(pipeline.Spec.PromotionPaths) == 0 {
		return "", fmt.Errorf("deployment pipeline has no promotion paths")
	}

	// Find root environment (source that is never a target)
	targets := make(map[string]bool)
	for _, path := range pipeline.Spec.PromotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			targets[target.Name] = true
		}
	}

	for _, path := range pipeline.Spec.PromotionPaths {
		if path.SourceEnvironmentRef.Name != "" && !targets[path.SourceEnvironmentRef.Name] {
			return path.SourceEnvironmentRef.Name, nil
		}
	}

	return "", fmt.Errorf("no root environment found in deployment pipeline")
}

// buildGatewayWirelogsURL constructs the HTTPS URL for the gateway wirelogs SSE
// endpoint. The gateway↔agent hop remains WebSocket-based, but the api↔gateway
// hop is now plain HTTP with SSE framing.
func (h *WirelogsHandler) buildGatewayWirelogsURL(plane execPlaneInfo, component, project, environment, namespace string) (string, error) {
	u, err := url.Parse(h.gatewayURL)
	if err != nil {
		return "", err
	}

	// Normalize any leftover ws/wss schemes to their HTTP equivalents so callers
	// passing the gateway base URL in either form Just Work.
	switch u.Scheme {
	case "wss":
		u.Scheme = "https"
	case "ws":
		u.Scheme = "http"
	}

	u.Path = fmt.Sprintf("/api/wirelogs/%s/%s/%s/%s",
		plane.planeType, plane.planeID, plane.crNamespace, plane.crName)

	q := u.Query()
	q.Set("component", component)
	q.Set("project", project)
	q.Set("environment", environment)
	q.Set("namespace", namespace)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// parseWirelogsPath extracts (namespace, project, component) from the request path.
// Expected form: /wirelogs/namespaces/{ns}/projects/{proj}/components/{comp}
func parseWirelogsPath(p string) (namespace, project, component string, ok bool) {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) != 7 {
		return "", "", "", false
	}
	if parts[0] != "wirelogs" || parts[1] != "namespaces" || parts[3] != "projects" || parts[5] != "components" {
		return "", "", "", false
	}
	if parts[2] == "" || parts[4] == "" || parts[6] == "" {
		return "", "", "", false
	}
	return parts[2], parts[4], parts[6], true
}
