// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

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
