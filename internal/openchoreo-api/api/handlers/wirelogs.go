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
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// RFC1123 DNS label (the k8s name form accepted by the gateway).
var wirelogsNameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

const wirelogsMaxNameLen = 63

// WirelogsHandler streams Cilium Hubble flow events for an environment (optionally filtered by project/component)
// as a Server-Sent Events response.
type WirelogsHandler struct {
	k8sClient      client.Client
	gatewayClient  *gatewayClient.Client
	gatewayURL     string
	gatewayTLSConf *tls.Config
	authzChecker   *svcpkg.AuthzChecker
	httpClient     *http.Client
	logger         *slog.Logger
}

// NewWirelogsHandler creates a new wirelogs handler and uses its own *http.Client
// (rather than the shared gatewayClient httpClient)
// because the gateway client applies a request-level timeout that is incompatible with the long-lived SSE stream
func NewWirelogsHandler(k8sClient client.Client, gwClient *gatewayClient.Client, gatewayURL string, gwTLSConf *tls.Config, authzChecker *svcpkg.AuthzChecker, logger *slog.Logger) *WirelogsHandler {
	return &WirelogsHandler{
		k8sClient:      k8sClient,
		gatewayClient:  gwClient,
		gatewayURL:     gatewayURL,
		gatewayTLSConf: gwTLSConf,
		authzChecker:   authzChecker,
		httpClient: &http.Client{
			// No Client.Timeout: SSE streams are long-lived and a request-level deadline
			// would abort them mid-stream. Bound the pre-stream phases via Transport timeouts.
			Transport: &http.Transport{
				TLSClientConfig: gwTLSConf,
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   5 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				IdleConnTimeout:       90 * time.Second,
			},
		},
		logger: logger.With("component", "wirelogs-handler"),
	}
}

// ServeHTTP authorizes the caller, resolves the target data plane, and proxies
// the gateway's SSE stream to the client.
// URL: /namespaces/{namespace}/environments/{environment}/wirelogs?project=&component=
func (h *WirelogsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	namespace, environment, ok := parseWirelogsPath(r)
	if !ok {
		http.Error(w, "invalid wirelogs URL: expected /namespaces/{namespace}/environments/{environment}/wirelogs", http.StatusBadRequest)
		return
	}

	project := r.URL.Query().Get("project")
	component := r.URL.Query().Get("component")

	if component != "" && project == "" {
		http.Error(w, "component filter requires project filter", http.StatusBadRequest)
		return
	}

	if len(namespace) > wirelogsMaxNameLen || !wirelogsNameRE.MatchString(namespace) {
		http.Error(w, "invalid namespace parameter", http.StatusBadRequest)
		return
	}
	if len(environment) > wirelogsMaxNameLen || !wirelogsNameRE.MatchString(environment) {
		http.Error(w, "invalid environment parameter", http.StatusBadRequest)
		return
	}
	if project != "" && (len(project) > wirelogsMaxNameLen || !wirelogsNameRE.MatchString(project)) {
		http.Error(w, "invalid project parameter", http.StatusBadRequest)
		return
	}
	if component != "" && (len(component) > wirelogsMaxNameLen || !wirelogsNameRE.MatchString(component)) {
		http.Error(w, "invalid component parameter", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	logger := h.logger.With(
		"namespace", namespace,
		"environment", environment,
		"project", project,
		"component", component,
	)

	if h.authzChecker == nil {
		logger.Error("Authorization checker not configured")
		http.Error(w, "authorization not configured", http.StatusInternalServerError)
		return
	}
	if err := h.authzChecker.Check(ctx, wirelogsCheckRequest(namespace, environment, project, component)); err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			http.Error(w, "you do not have permission to view wirelogs for this scope", http.StatusForbidden)
			return
		}
		logger.Error("Authorization check failed", "error", err)
		http.Error(w, "authorization check failed", http.StatusInternalServerError)
		return
	}

	plane, err := h.resolvePlane(ctx, namespace, environment)
	if err != nil {
		logger.Error("Failed to resolve data plane for wirelogs", "error", err)
		http.Error(w, fmt.Sprintf("failed to resolve data plane: %v", err), http.StatusBadRequest)
		return
	}

	logger = logger.With("planeType", plane.planeType, "planeID", plane.planeID)

	flusher, ok := w.(http.Flusher)
	if !ok {
		logger.Error("ResponseWriter does not support flushing; cannot stream SSE")
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	// The http.Server's WriteTimeout is an absolute deadline from when request
	// headers are read; for a long-lived SSE stream it would kill the connection
	// after that deadline regardless of activity. Hence, clear the deadline on this connection only
	// Other endpoints keep the server's default protection.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		logger.Warn("Failed to disable write deadline for SSE stream", "error", err)
	}

	gwURL, err := h.buildGatewayWirelogsURL(plane, namespace, environment, project, component)
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

	// The gateway already emits valid SSE framing; flush after every chunk so events arrive without buffering.
	if _, err := io.Copy(&sseFlushingWriter{w: w, flusher: flusher}, resp.Body); err != nil {
		if !errors.Is(err, context.Canceled) {
			logger.Debug("Wirelogs stream ended with error", "error", err)
		}
	}
}

// sseFlushingWriter flushes the underlying ResponseWriter after each write.
type sseFlushingWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (fw *sseFlushingWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if err == nil {
		fw.flusher.Flush()
	}
	return n, err
}

// wirelogsCheckRequest derives the most-specific authz scope from the supplied filters. The environment
// is always exposed via Context so CEL policies can scope per-environment.
func wirelogsCheckRequest(namespace, environment, project, component string) svcpkg.CheckRequest {
	envAttr := svcpkg.FormatDualScopedResourceName(namespace, environment, false)
	authzCtx := authz.Context{Resource: authz.ResourceAttribute{Environment: envAttr}}

	switch {
	case component != "":
		return svcpkg.CheckRequest{
			Action:       authz.ActionViewWirelogs,
			ResourceType: "component",
			ResourceID:   component,
			Hierarchy: authz.ResourceHierarchy{
				Namespace: namespace,
				Project:   project,
				Component: component,
			},
			Context: authzCtx,
		}
	case project != "":
		return svcpkg.CheckRequest{
			Action:       authz.ActionViewWirelogs,
			ResourceType: "project",
			ResourceID:   project,
			Hierarchy: authz.ResourceHierarchy{
				Namespace: namespace,
				Project:   project,
			},
			Context: authzCtx,
		}
	default:
		return svcpkg.CheckRequest{
			Action:       authz.ActionViewWirelogs,
			ResourceType: "environment",
			ResourceID:   environment,
			Hierarchy:    authz.ResourceHierarchy{Namespace: namespace},
			Context:      authzCtx,
		}
	}
}

// resolvePlane resolves the data plane for an environment.
func (h *WirelogsHandler) resolvePlane(ctx context.Context, namespace, environment string) (execPlaneInfo, error) {
	env := &openchoreov1alpha1.Environment{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: environment}, env); err != nil {
		return execPlaneInfo{}, fmt.Errorf("environment %q not found: %w", environment, err)
	}
	if env.Spec.DataPlaneRef == nil {
		return execPlaneInfo{}, fmt.Errorf("environment %q has no data plane reference", environment)
	}

	dpResult, err := controller.GetDataPlaneFromRef(ctx, h.k8sClient, env.Namespace, env.Spec.DataPlaneRef)
	if err != nil {
		return execPlaneInfo{}, fmt.Errorf("failed to resolve data plane: %w", err)
	}

	plane := resolveExecPlaneInfo(dpResult)
	if plane.planeID == "" {
		return execPlaneInfo{}, fmt.Errorf("failed to determine plane ID for environment %q", environment)
	}
	return plane, nil
}

// buildGatewayWirelogsURL constructs the HTTPS URL for the gateway wirelogs SSE endpoint.
func (h *WirelogsHandler) buildGatewayWirelogsURL(plane execPlaneInfo, namespace, environment, project, component string) (string, error) {
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
	q.Set("namespace", namespace)
	q.Set("environment", environment)
	if project != "" {
		q.Set("project", project)
	}
	if component != "" {
		q.Set("component", component)
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// parseWirelogsPath extracts (namespace, environment) from the request path.
// Expected form: /namespaces/{namespace}/environments/{environment}/wirelogs
func parseWirelogsPath(r *http.Request) (namespace, environment string, ok bool) {
	namespace = r.PathValue("namespace")
	environment = r.PathValue("environment")
	if namespace != "" && environment != "" {
		return namespace, environment, true
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 5 {
		return "", "", false
	}
	if parts[0] != "namespaces" || parts[2] != "environments" || parts[4] != "wirelogs" {
		return "", "", false
	}
	if parts[1] == "" || parts[3] == "" {
		return "", "", false
	}
	return parts[1], parts[3], true
}
