// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
)

const (
	maxProxyResponseBytes = 10 * 1024 * 1024 // 10MB

	planeTypeDataPlane          = "dataplane"
	planeTypeBuildPlane         = "buildplane"
	planeTypeObservabilityPlane = "observabilityplane"

	queryValueTrue = "true"
)

// planeProxyConfig holds plane-specific configuration for the proxy helper.
type planeProxyConfig struct {
	planeType    string
	action       string
	resourceType string
}

var (
	dataPlaneProxyConfig = planeProxyConfig{
		planeType:    planeTypeDataPlane,
		action:       string(services.SystemActionViewDataPlaneResource),
		resourceType: string(services.ResourceTypeDataPlaneResource),
	}
	buildPlaneProxyConfig = planeProxyConfig{
		planeType:    planeTypeBuildPlane,
		action:       string(services.SystemActionViewBuildPlaneResource),
		resourceType: string(services.ResourceTypeBuildPlaneResource),
	}
	observabilityPlaneProxyConfig = planeProxyConfig{
		planeType:    planeTypeObservabilityPlane,
		action:       string(services.SystemActionViewObservabilityPlaneResource),
		resourceType: string(services.ResourceTypeObservabilityPlaneResource),
	}
)

// blockedPathSegments contains path segments that are blocked from proxying for security.
// These are matched as exact URL path segments to avoid false positives.
// NOTE: exec/attach/portforward are POST-only in the K8s API, but we block them
// defensively since WebSocket upgrades can bypass method restrictions.
// TODO: Consider switching to an allowlist approach for stricter control.
var blockedPathSegments = []string{
	"secrets",
	"serviceaccounts",
	"exec",
	"attach",
	"portforward",
}

// blockedNamespaces contains namespaces that are blocked from proxying for security.
var blockedNamespaces = []string{
	"kube-system",
	"kube-public",
	"kube-node-lease",
}

// ProxyDataPlaneK8s handles GET /api/v1/oc-namespaces/{ocNamespace}/dataplanes/{dpName}/{k8sPath...}
func (h *Handler) ProxyDataPlaneK8s(w http.ResponseWriter, r *http.Request) {
	ocNamespace := r.PathValue("ocNamespace")
	dpName := r.PathValue("dpName")
	k8sPath := r.PathValue("k8sPath")

	if ocNamespace == "" || dpName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "namespace and dataplane name are required", services.CodeInvalidInput)
		return
	}

	dp := &openchoreov1alpha1.DataPlane{}
	if err := h.services.GetKubernetesClient().Get(r.Context(), client.ObjectKey{Namespace: ocNamespace, Name: dpName}, dp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			writeErrorResponse(w, http.StatusNotFound, "DataPlane not found", services.CodeNotFound)
			return
		}
		h.logger.Error("Failed to get DataPlane", "error", err, "namespace", ocNamespace, "name", dpName)
		writeErrorResponse(w, http.StatusInternalServerError, "failed to look up DataPlane", services.CodeInternalError)
		return
	}

	planeID := dp.Spec.PlaneID
	if planeID == "" {
		planeID = dp.Name
	}

	h.proxyPlaneK8sRequest(w, r, dataPlaneProxyConfig, planeID, ocNamespace, dpName, k8sPath,
		authz.ResourceHierarchy{Namespace: ocNamespace})
}

// ProxyBuildPlaneK8s handles GET /api/v1/oc-namespaces/{ocNamespace}/buildplanes/{bpName}/{k8sPath...}
func (h *Handler) ProxyBuildPlaneK8s(w http.ResponseWriter, r *http.Request) {
	ocNamespace := r.PathValue("ocNamespace")
	bpName := r.PathValue("bpName")
	k8sPath := r.PathValue("k8sPath")

	if ocNamespace == "" || bpName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "namespace and buildplane name are required", services.CodeInvalidInput)
		return
	}

	bp := &openchoreov1alpha1.BuildPlane{}
	if err := h.services.GetKubernetesClient().Get(r.Context(), client.ObjectKey{Namespace: ocNamespace, Name: bpName}, bp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			writeErrorResponse(w, http.StatusNotFound, "BuildPlane not found", services.CodeNotFound)
			return
		}
		h.logger.Error("Failed to get BuildPlane", "error", err, "namespace", ocNamespace, "name", bpName)
		writeErrorResponse(w, http.StatusInternalServerError, "failed to look up BuildPlane", services.CodeInternalError)
		return
	}

	planeID := bp.Spec.PlaneID
	if planeID == "" {
		planeID = bp.Name
	}

	h.proxyPlaneK8sRequest(w, r, buildPlaneProxyConfig, planeID, ocNamespace, bpName, k8sPath,
		authz.ResourceHierarchy{Namespace: ocNamespace})
}

// ProxyObservabilityPlaneK8s handles GET /api/v1/oc-namespaces/{ocNamespace}/observabilityplanes/{opName}/{k8sPath...}
func (h *Handler) ProxyObservabilityPlaneK8s(w http.ResponseWriter, r *http.Request) {
	ocNamespace := r.PathValue("ocNamespace")
	opName := r.PathValue("opName")
	k8sPath := r.PathValue("k8sPath")

	if ocNamespace == "" || opName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "namespace and observabilityplane name are required", services.CodeInvalidInput)
		return
	}

	op := &openchoreov1alpha1.ObservabilityPlane{}
	if err := h.services.GetKubernetesClient().Get(r.Context(), client.ObjectKey{Namespace: ocNamespace, Name: opName}, op); err != nil {
		if client.IgnoreNotFound(err) == nil {
			writeErrorResponse(w, http.StatusNotFound, "ObservabilityPlane not found", services.CodeNotFound)
			return
		}
		h.logger.Error("Failed to get ObservabilityPlane", "error", err, "namespace", ocNamespace, "name", opName)
		writeErrorResponse(w, http.StatusInternalServerError, "failed to look up ObservabilityPlane", services.CodeInternalError)
		return
	}

	planeID := op.Spec.PlaneID
	if planeID == "" {
		planeID = op.Name
	}

	h.proxyPlaneK8sRequest(w, r, observabilityPlaneProxyConfig, planeID, ocNamespace, opName, k8sPath,
		authz.ResourceHierarchy{Namespace: ocNamespace})
}

// ProxyClusterDataPlaneK8s handles GET /api/v1/cluster-dataplanes/{cdpName}/{k8sPath...}
func (h *Handler) ProxyClusterDataPlaneK8s(w http.ResponseWriter, r *http.Request) {
	cdpName := r.PathValue("cdpName")
	k8sPath := r.PathValue("k8sPath")

	if cdpName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "cluster dataplane name is required", services.CodeInvalidInput)
		return
	}

	cdp := &openchoreov1alpha1.ClusterDataPlane{}
	if err := h.services.GetKubernetesClient().Get(r.Context(), client.ObjectKey{Name: cdpName}, cdp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			writeErrorResponse(w, http.StatusNotFound, "ClusterDataPlane not found", services.CodeNotFound)
			return
		}
		h.logger.Error("Failed to get ClusterDataPlane", "error", err, "name", cdpName)
		writeErrorResponse(w, http.StatusInternalServerError, "failed to look up ClusterDataPlane", services.CodeInternalError)
		return
	}

	planeID := cdp.Spec.PlaneID
	if planeID == "" {
		planeID = cdpName
	}

	h.proxyPlaneK8sRequest(w, r, dataPlaneProxyConfig, planeID, "_cluster", cdpName, k8sPath,
		authz.ResourceHierarchy{})
}

// ProxyClusterBuildPlaneK8s handles GET /api/v1/cluster-buildplanes/{cbpName}/{k8sPath...}
func (h *Handler) ProxyClusterBuildPlaneK8s(w http.ResponseWriter, r *http.Request) {
	cbpName := r.PathValue("cbpName")
	k8sPath := r.PathValue("k8sPath")

	if cbpName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "cluster buildplane name is required", services.CodeInvalidInput)
		return
	}

	cbp := &openchoreov1alpha1.ClusterBuildPlane{}
	if err := h.services.GetKubernetesClient().Get(r.Context(), client.ObjectKey{Name: cbpName}, cbp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			writeErrorResponse(w, http.StatusNotFound, "ClusterBuildPlane not found", services.CodeNotFound)
			return
		}
		h.logger.Error("Failed to get ClusterBuildPlane", "error", err, "name", cbpName)
		writeErrorResponse(w, http.StatusInternalServerError, "failed to look up ClusterBuildPlane", services.CodeInternalError)
		return
	}

	planeID := cbp.Spec.PlaneID
	if planeID == "" {
		planeID = cbpName
	}

	h.proxyPlaneK8sRequest(w, r, buildPlaneProxyConfig, planeID, "_cluster", cbpName, k8sPath,
		authz.ResourceHierarchy{})
}

// ProxyClusterObservabilityPlaneK8s handles GET /api/v1/cluster-observabilityplanes/{copName}/{k8sPath...}
func (h *Handler) ProxyClusterObservabilityPlaneK8s(w http.ResponseWriter, r *http.Request) {
	copName := r.PathValue("copName")
	k8sPath := r.PathValue("k8sPath")

	if copName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "cluster observabilityplane name is required", services.CodeInvalidInput)
		return
	}

	cop := &openchoreov1alpha1.ClusterObservabilityPlane{}
	if err := h.services.GetKubernetesClient().Get(r.Context(), client.ObjectKey{Name: copName}, cop); err != nil {
		if client.IgnoreNotFound(err) == nil {
			writeErrorResponse(w, http.StatusNotFound, "ClusterObservabilityPlane not found", services.CodeNotFound)
			return
		}
		h.logger.Error("Failed to get ClusterObservabilityPlane", "error", err, "name", copName)
		writeErrorResponse(w, http.StatusInternalServerError, "failed to look up ClusterObservabilityPlane", services.CodeInternalError)
		return
	}

	planeID := cop.Spec.PlaneID
	if planeID == "" {
		planeID = copName
	}

	h.proxyPlaneK8sRequest(w, r, observabilityPlaneProxyConfig, planeID, "_cluster", copName, k8sPath,
		authz.ResourceHierarchy{})
}

// proxyPlaneK8sRequest is the shared helper that handles authorization, validation,
// and proxying the K8s API request through the gateway.
func (h *Handler) proxyPlaneK8sRequest(
	w http.ResponseWriter,
	r *http.Request,
	cfg planeProxyConfig,
	planeID, crNamespace, crName, k8sPath string,
	hierarchy authz.ResourceHierarchy,
) {
	// Validate k8sPath
	if k8sPath == "" {
		writeErrorResponse(w, http.StatusBadRequest, "K8s API path is required", services.CodeInvalidInput)
		return
	}
	if err := validateK8sPath(k8sPath); err != nil {
		writeErrorResponse(w, http.StatusForbidden, err.Error(), services.CodeForbidden)
		return
	}

	// Reject streaming parameters
	query := r.URL.Query()
	if query.Get("watch") == queryValueTrue || query.Get("follow") == queryValueTrue {
		writeErrorResponse(w, http.StatusBadRequest, "streaming (watch/follow) is not supported", services.CodeInvalidInput)
		return
	}

	// Check authorization
	if err := h.services.CheckAuthorization(r.Context(), h.logger, cfg.action, cfg.resourceType, crName, hierarchy); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		h.logger.Error("Authorization check failed", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "authorization check failed", services.CodeInternalError)
		return
	}

	// Proxy the request through the gateway
	gwClient := h.services.GatewayClient
	if gwClient == nil {
		h.logger.Error("Gateway client is not configured")
		writeErrorResponse(w, http.StatusInternalServerError, "gateway client not available", services.CodeInternalError)
		return
	}

	resp, err := gwClient.ProxyK8sRequest(r.Context(), cfg.planeType, planeID, crNamespace, crName, k8sPath, r.URL.RawQuery)
	if err != nil {
		h.logger.Error("Failed to proxy K8s request", "error", err, "planeType", cfg.planeType, "planeID", planeID, "k8sPath", k8sPath)
		writeErrorResponse(w, http.StatusBadGateway, "failed to proxy request to plane", services.CodeInternalError)
		return
	}
	defer resp.Body.Close()

	// Forward response headers, skipping hop-by-hop headers per RFC 7230 §6.1
	for key, values := range resp.Header {
		if isHopByHopHeader(key) {
			continue
		}
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}

	// Forward status code
	w.WriteHeader(resp.StatusCode)

	// Stream response body with size limit
	//nolint:errcheck // Best-effort copy to response writer
	io.Copy(w, io.LimitReader(resp.Body, maxProxyResponseBytes))
}

// hopByHopHeaders are HTTP/1.1 hop-by-hop headers that must not be forwarded by proxies.
// See RFC 7230 §6.1.
var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

// isHopByHopHeader returns true if the header is a hop-by-hop header that
// should not be forwarded by a proxy.
func isHopByHopHeader(header string) bool {
	return hopByHopHeaders[http.CanonicalHeaderKey(header)]
}

// validateK8sPath checks that the K8s API path is safe to proxy.
func validateK8sPath(path string) error {
	// Block directory traversal
	if strings.Contains(path, "..") {
		return errors.New("path traversal is not allowed")
	}

	// Block null bytes
	if strings.ContainsRune(path, '\x00') {
		return errors.New("invalid path")
	}

	// Split into segments for exact matching to avoid false positives
	// (e.g., a pod named "my-secrets-pod" should not be blocked)
	segments := strings.Split(strings.ToLower(path), "/")

	// Block access to sensitive resource types (exact segment match)
	for _, seg := range segments {
		for _, blocked := range blockedPathSegments {
			if seg == blocked {
				return errors.New("access to this resource type is not allowed")
			}
		}
	}

	// Block access to sensitive namespaces
	for i, seg := range segments {
		if seg == "namespaces" && i+1 < len(segments) {
			for _, ns := range blockedNamespaces {
				if segments[i+1] == ns {
					return errors.New("access to this namespace is not allowed")
				}
			}
		}
	}

	return nil
}
