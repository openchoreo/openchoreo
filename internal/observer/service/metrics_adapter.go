// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// runtimeTopologyAdapterRequest matches the metrics-adapter runtime-topology spec.
type runtimeTopologyAdapterRequest struct {
	SearchScope     runtimeTopologyAdapterScope `json:"searchScope"`
	StartTime       string                      `json:"startTime"`
	EndTime         string                      `json:"endTime"`
	IncludeGateways *bool                       `json:"includeGateways,omitempty"`
	IncludeExternal *bool                       `json:"includeExternal,omitempty"`
}

type runtimeTopologyAdapterScope struct {
	Namespace      string  `json:"namespace"`
	ComponentUID   *string `json:"componentUid,omitempty"`
	ProjectUID     *string `json:"projectUid,omitempty"`
	EnvironmentUID *string `json:"environmentUid,omitempty"`
}

type runtimeTopologyAdapterResponse struct {
	Nodes   []runtimeTopologyAdapterNode  `json:"nodes,omitempty"`
	Edges   []runtimeTopologyAdapterEdge  `json:"edges,omitempty"`
	Summary runtimeTopologyAdapterSummary `json:"summary"`
}

type runtimeTopologyAdapterNode struct {
	Kind         string                         `json:"kind"`
	Component    string                         `json:"component,omitempty"`
	ComponentUID string                         `json:"componentUid,omitempty"`
	ProjectUID   string                         `json:"projectUid,omitempty"`
	Namespace    string                         `json:"namespace,omitempty"`
	GatewayName  string                         `json:"gatewayName,omitempty"`
	ExternalHost string                         `json:"externalHost,omitempty"`
	Metrics      *runtimeTopologyAdapterMetrics `json:"metrics,omitempty"`
}

type runtimeTopologyAdapterEdge struct {
	ID       string                         `json:"id"`
	Source   runtimeTopologyAdapterNodeRef  `json:"source"`
	Target   runtimeTopologyAdapterNodeRef  `json:"target"`
	Protocol string                         `json:"protocol,omitempty"`
	Metrics  *runtimeTopologyAdapterMetrics `json:"metrics,omitempty"`
}

type runtimeTopologyAdapterNodeRef struct {
	Kind         string `json:"kind"`
	Component    string `json:"component,omitempty"`
	ComponentUID string `json:"componentUid,omitempty"`
	ProjectUID   string `json:"projectUid,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
	GatewayName  string `json:"gatewayName,omitempty"`
	ExternalHost string `json:"externalHost,omitempty"`
}

type runtimeTopologyAdapterMetrics struct {
	RequestCount             *float64 `json:"requestCount,omitempty"`
	UnsuccessfulRequestCount *float64 `json:"unsuccessfulRequestCount,omitempty"`
	MeanLatency              *float64 `json:"meanLatency,omitempty"`
	LatencyP50               *float64 `json:"latencyP50,omitempty"`
	LatencyP90               *float64 `json:"latencyP90,omitempty"`
	LatencyP99               *float64 `json:"latencyP99,omitempty"`
}

type runtimeTopologyAdapterSummary struct {
	StartTime   time.Time `json:"startTime"`
	EndTime     time.Time `json:"endTime"`
	GeneratedAt time.Time `json:"generatedAt"`
}

// metricsAdapterRequest is the request payload sent to the external metrics adapter.
// It uses the field names expected by the adapter's OpenAPI spec (componentUid, projectUid,
// environmentUid) rather than the Observer's API field names (component, project, environment).
type metricsAdapterRequest struct {
	Metric      string                    `json:"metric"`
	StartTime   string                    `json:"startTime"`
	EndTime     string                    `json:"endTime"`
	Step        *string                   `json:"step,omitempty"`
	SearchScope metricsAdapterSearchScope `json:"searchScope"`
}

// metricsAdapterSearchScope matches the adapter's ComponentSearchScope schema.
type metricsAdapterSearchScope struct {
	Namespace      string  `json:"namespace"`
	ComponentUID   *string `json:"componentUid,omitempty"`
	ProjectUID     *string `json:"projectUid,omitempty"`
	EnvironmentUID *string `json:"environmentUid,omitempty"`
}

// MetricsAdapter forwards metrics queries to an external metrics adapter service.
// It resolves human-readable names (project, component, environment) to UIDs before
// forwarding, so that the adapter receives the UIDs it expects for Prometheus label filtering.
// It implements the MetricsQuerier interface.
type MetricsAdapter struct {
	baseURL    string
	httpClient *http.Client
	resolver   *ResourceUIDResolver
	logger     *slog.Logger
}

var _ MetricsQuerier = (*MetricsAdapter)(nil)

// Sentinel errors for the runtime topology queries. The handler maps these to
// specific HTTP status codes / error codes.
var (
	// ErrRuntimeTopologyInvalidRequest indicates the request payload is
	// malformed or violates a precondition (e.g., unparseable time, missing
	// project). Maps to HTTP 400.
	ErrRuntimeTopologyInvalidRequest = errors.New("invalid runtime topology request")
	// ErrRuntimeTopologyResolveSearchScope indicates a failure while resolving
	// a name in searchScope to a UID. Typically a 5xx from the OpenChoreo API.
	ErrRuntimeTopologyResolveSearchScope = errors.New("runtime topology search scope resolution failed")
	// ErrRuntimeTopologyRetrieval indicates the underlying metrics backend
	// failed to return data.
	ErrRuntimeTopologyRetrieval = errors.New("runtime topology retrieval failed")
)

// NewMetricsAdapter creates a new MetricsAdapter that forwards requests to the given base URL.
// The resolver is used to convert human-readable names to UIDs before forwarding.
func NewMetricsAdapter(baseURL string, timeout time.Duration, resolver *ResourceUIDResolver, logger *slog.Logger) *MetricsAdapter {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &MetricsAdapter{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		resolver: resolver,
		logger:   logger,
	}
}

// QueryMetrics resolves search scope names to UIDs and forwards the metrics query
// request to the external adapter, returning the raw JSON response.
func (a *MetricsAdapter) QueryMetrics(ctx context.Context, req *types.MetricsQueryRequest) (any, error) {
	if req == nil {
		return nil, fmt.Errorf("request must not be nil")
	}

	scope := &req.SearchScope

	// Resolve human-readable names to UIDs
	var projectUID, componentUID, environmentUID string
	var err error
	if scope.Project != "" {
		projectUID, err = a.resolver.GetProjectUID(ctx, scope.Namespace, scope.Project)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to get project UID: %w", ErrMetricsResolveSearchScope, err)
		}
	}
	if scope.Component != "" {
		componentUID, err = a.resolver.GetComponentUID(ctx, scope.Namespace, scope.Project, scope.Component)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to get component UID: %w", ErrMetricsResolveSearchScope, err)
		}
	}
	if scope.Environment != "" {
		environmentUID, err = a.resolver.GetEnvironmentUID(ctx, scope.Namespace, scope.Environment)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to get environment UID: %w", ErrMetricsResolveSearchScope, err)
		}
	}

	// Build adapter request with resolved UIDs
	adapterReq := metricsAdapterRequest{
		Metric:    req.Metric,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Step:      req.Step,
		SearchScope: metricsAdapterSearchScope{
			Namespace: scope.Namespace,
		},
	}
	if projectUID != "" {
		adapterReq.SearchScope.ProjectUID = &projectUID
	}
	if componentUID != "" {
		adapterReq.SearchScope.ComponentUID = &componentUID
	}
	if environmentUID != "" {
		adapterReq.SearchScope.EnvironmentUID = &environmentUID
	}

	a.logger.Debug("Forwarding metrics query to adapter",
		"metric", req.Metric,
		"namespace", scope.Namespace,
		"projectUID", projectUID,
		"componentUID", componentUID,
		"environmentUID", environmentUID,
	)

	body, err := json.Marshal(adapterReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metrics query request: %w", err)
	}

	url := a.baseURL + "/api/v1/metrics/query"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics adapter request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMetricsRetrieval, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read metrics adapter response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: metrics adapter returned HTTP %d: %s", ErrMetricsRetrieval, resp.StatusCode, string(respBody))
	}

	var result json.RawMessage
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("%w: failed to decode metrics adapter response: %w", ErrMetricsRetrieval, err)
	}

	return result, nil
}

// QueryRuntimeTopology resolves search scope names to UIDs and forwards the
// runtime topology query to the metrics-adapter's
// /api/v1alpha1/metrics/runtime-topology endpoint.
func (a *MetricsAdapter) QueryRuntimeTopology(
	ctx context.Context,
	req *types.RuntimeTopologyRequest,
) (*types.RuntimeTopologyResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request must not be nil", ErrRuntimeTopologyInvalidRequest)
	}

	scope := &req.SearchScope
	if scope.Namespace == "" {
		return nil, fmt.Errorf("%w: searchScope.namespace is required", ErrRuntimeTopologyInvalidRequest)
	}
	if scope.Project == "" {
		return nil, fmt.Errorf("%w: searchScope.project is required", ErrRuntimeTopologyInvalidRequest)
	}
	if scope.Environment == "" {
		return nil, fmt.Errorf("%w: searchScope.environment is required", ErrRuntimeTopologyInvalidRequest)
	}
	if req.StartTime == "" || req.EndTime == "" {
		return nil, fmt.Errorf("%w: startTime and endTime are required", ErrRuntimeTopologyInvalidRequest)
	}
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid startTime: %w", ErrRuntimeTopologyInvalidRequest, err)
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid endTime: %w", ErrRuntimeTopologyInvalidRequest, err)
	}

	// Resolve names to UIDs for filtering.
	projectUID, err := a.resolver.GetProjectUID(ctx, scope.Namespace, scope.Project)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get project UID: %w", ErrRuntimeTopologyResolveSearchScope, err)
	}
	environmentUID, err := a.resolver.GetEnvironmentUID(ctx, scope.Namespace, scope.Environment)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get environment UID: %w", ErrRuntimeTopologyResolveSearchScope, err)
	}
	var componentUID string
	if scope.Component != "" {
		componentUID, err = a.resolver.GetComponentUID(ctx, scope.Namespace, scope.Project, scope.Component)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to get component UID: %w", ErrRuntimeTopologyResolveSearchScope, err)
		}
	}

	adapterReq := runtimeTopologyAdapterRequest{
		SearchScope: runtimeTopologyAdapterScope{
			Namespace:      scope.Namespace,
			ProjectUID:     ptrStringIfNonEmpty(projectUID),
			EnvironmentUID: ptrStringIfNonEmpty(environmentUID),
			ComponentUID:   ptrStringIfNonEmpty(componentUID),
		},
		StartTime:       req.StartTime,
		EndTime:         req.EndTime,
		IncludeGateways: req.IncludeGateways,
		IncludeExternal: req.IncludeExternal,
	}

	a.logger.Debug("Forwarding runtime topology query to adapter",
		"namespace", scope.Namespace,
		"project", scope.Project,
		"projectUID", projectUID,
		"environment", scope.Environment,
		"environmentUID", environmentUID,
		"component", scope.Component,
		"startTime", req.StartTime,
		"endTime", req.EndTime,
	)

	body, err := json.Marshal(adapterReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal runtime topology request: %w", err)
	}

	url := a.baseURL + "/api/v1alpha1/metrics/runtime-topology"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create runtime topology adapter request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRuntimeTopologyRetrieval, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read response: %w", ErrRuntimeTopologyRetrieval, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf(
			"%w: metrics adapter returned HTTP %d: %s",
			ErrRuntimeTopologyRetrieval, resp.StatusCode, string(respBody),
		)
	}

	var adapterResp runtimeTopologyAdapterResponse
	if err := json.Unmarshal(respBody, &adapterResp); err != nil {
		return nil, fmt.Errorf("%w: failed to decode response: %w", ErrRuntimeTopologyRetrieval, err)
	}

	out := &types.RuntimeTopologyResponse{
		Nodes: convertTopologyAdapterNodes(adapterResp.Nodes),
		Edges: convertTopologyAdapterEdges(adapterResp.Edges),
		Summary: types.RuntimeTopologySummary{
			StartTime:   coalesceTime(adapterResp.Summary.StartTime, startTime),
			EndTime:     coalesceTime(adapterResp.Summary.EndTime, endTime),
			GeneratedAt: coalesceTime(adapterResp.Summary.GeneratedAt, time.Now().UTC()),
		},
	}
	return out, nil
}

func convertTopologyAdapterEdges(in []runtimeTopologyAdapterEdge) []types.RuntimeTopologyEdge {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.RuntimeTopologyEdge, 0, len(in))
	for _, e := range in {
		out = append(out, types.RuntimeTopologyEdge{
			ID:       e.ID,
			Source:   convertTopologyAdapterNodeRef(e.Source),
			Target:   convertTopologyAdapterNodeRef(e.Target),
			Protocol: types.RuntimeTopologyProtocol(e.Protocol),
			Metrics:  convertTopologyAdapterMetrics(e.Metrics),
		})
	}
	return out
}

func convertTopologyAdapterNodes(in []runtimeTopologyAdapterNode) []types.RuntimeTopologyNode {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.RuntimeTopologyNode, 0, len(in))
	for _, n := range in {
		out = append(out, types.RuntimeTopologyNode{
			RuntimeTopologyNodeRef: convertTopologyAdapterNodeRef(
				runtimeTopologyAdapterNodeRef{
					Kind:         n.Kind,
					Component:    n.Component,
					ComponentUID: n.ComponentUID,
					ProjectUID:   n.ProjectUID,
					Namespace:    n.Namespace,
					GatewayName:  n.GatewayName,
					ExternalHost: n.ExternalHost,
				},
			),
			Metrics: convertTopologyAdapterMetrics(n.Metrics),
		})
	}
	return out
}

func convertTopologyAdapterNodeRef(in runtimeTopologyAdapterNodeRef) types.RuntimeTopologyNodeRef {
	out := types.RuntimeTopologyNodeRef{
		Kind:      types.RuntimeTopologyNodeKind(in.Kind),
		Namespace: in.Namespace,
	}
	switch out.Kind {
	case types.RuntimeTopologyNodeKindComponent:
		out.Component = in.Component
		out.ComponentUID = in.ComponentUID
		out.ProjectUID = in.ProjectUID
	case types.RuntimeTopologyNodeKindGateway:
		out.Name = in.GatewayName
		out.ProjectUID = in.ProjectUID
	case types.RuntimeTopologyNodeKindExternal:
		out.Host = in.ExternalHost
		out.Component = in.Component
		out.ComponentUID = in.ComponentUID
		out.ProjectUID = in.ProjectUID
	}
	return out
}

func convertTopologyAdapterMetrics(in *runtimeTopologyAdapterMetrics) *types.RuntimeTopologyMetrics {
	if in == nil {
		return nil
	}
	out := &types.RuntimeTopologyMetrics{}
	if in.RequestCount != nil {
		out.RequestCount = *in.RequestCount
	}
	if in.UnsuccessfulRequestCount != nil {
		out.UnsuccessfulRequestCount = *in.UnsuccessfulRequestCount
	}
	if in.MeanLatency != nil {
		out.MeanLatency = *in.MeanLatency
	}
	if in.LatencyP50 != nil {
		out.LatencyP50 = *in.LatencyP50
	}
	if in.LatencyP90 != nil {
		out.LatencyP90 = *in.LatencyP90
	}
	if in.LatencyP99 != nil {
		out.LatencyP99 = *in.LatencyP99
	}
	return out
}

func ptrStringIfNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func coalesceTime(t time.Time, fallback time.Time) time.Time {
	if t.IsZero() {
		return fallback
	}
	return t
}
