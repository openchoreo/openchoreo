// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/openchoreo/openchoreo/internal/observer/api/finopsadapterclientgen"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
)

var (
	// ErrFinOpsInvalidRequest indicates that the inbound FinOps request is malformed.
	ErrFinOpsInvalidRequest = errors.New("invalid finops request")
	// ErrFinOpsResolveScope indicates a failure while resolving names to UIDs.
	ErrFinOpsResolveScope = errors.New("finops scope resolution failed")
	// ErrFinOpsRetrieval indicates a failure while retrieving data from the FinOps adapter.
	ErrFinOpsRetrieval = errors.New("finops retrieval failed")
)

// FinOpsAdapter forwards cost-insights queries to an external FinOps adapter service.
// It resolves human-readable names (environment, project, component) to UIDs before
// forwarding, so that the adapter receives the UIDs it expects. It implements the
// FinOpsQuerier interface.
type FinOpsAdapter struct {
	client   *finopsadapterclientgen.ClientWithResponses
	resolver *ResourceUIDResolver
	logger   *slog.Logger
}

var _ FinOpsQuerier = (*FinOpsAdapter)(nil)

// NewFinOpsAdapter creates a new FinOpsAdapter that forwards requests to the given base URL.
// The resolver is used to convert human-readable names to UIDs before forwarding.
func NewFinOpsAdapter(
	baseURL string,
	timeout time.Duration,
	resolver *ResourceUIDResolver,
	logger *slog.Logger,
) (*FinOpsAdapter, error) {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	httpClient := &http.Client{Timeout: timeout}
	client, err := finopsadapterclientgen.NewClientWithResponses(
		baseURL,
		finopsadapterclientgen.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create FinOps adapter client: %w", err)
	}
	return &FinOpsAdapter{
		client:   client,
		resolver: resolver,
		logger:   logger,
	}, nil
}

// GetComponentCosts resolves the scope names to UIDs and forwards the cost query
// to the FinOps adapter, returning the raw JSON response.
func (a *FinOpsAdapter) GetComponentCosts(ctx context.Context, req *types.CostQueryRequest) (any, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request must not be nil", ErrFinOpsInvalidRequest)
	}

	startTime, endTime, err := parseFinOpsWindow(req.StartTime, req.EndTime)
	if err != nil {
		return nil, err
	}

	environmentUID, projectUID, componentUID, err := a.resolveScope(
		ctx, req.Namespace, req.Environment, req.Project, req.Component,
	)
	if err != nil {
		return nil, err
	}

	params := &finopsadapterclientgen.GetComponentCostsParams{
		StartTime:    startTime,
		EndTime:      endTime,
		ProjectUid:   projectUID,
		ComponentUid: componentUID,
	}
	if req.Granularity != "" {
		granularity := req.Granularity
		params.Granularity = &granularity
	}

	a.logger.Debug("Forwarding cost query to finops adapter",
		"namespace", req.Namespace,
		"environmentUID", environmentUID,
		"projectUID", ptrUUIDString(projectUID),
		"componentUID", ptrUUIDString(componentUID),
		"granularity", req.Granularity,
	)

	// The costs endpoint is unauthenticated on the adapter, so no token is forwarded.
	resp, err := a.client.GetComponentCostsWithResponse(ctx, req.Namespace, environmentUID, params)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFinOpsRetrieval, err)
	}
	return rawFinOpsResponse(resp.StatusCode(), resp.Body)
}

// GetRecommendations resolves the scope names to UIDs and forwards the
// recommendation query to the FinOps adapter, returning the raw JSON response.
// The adapter needs the names forwarded alongside the UIDs, so both are sent.
func (a *FinOpsAdapter) GetRecommendations(
	ctx context.Context,
	req *types.RecommendationQueryRequest,
) (any, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request must not be nil", ErrFinOpsInvalidRequest)
	}

	startTime, endTime, err := parseFinOpsWindow(req.StartTime, req.EndTime)
	if err != nil {
		return nil, err
	}

	environmentUID, projectUID, componentUID, err := a.resolveScope(
		ctx, req.Namespace, req.Environment, req.Project, req.Component,
	)
	if err != nil {
		return nil, err
	}

	params := &finopsadapterclientgen.GetRecommendationsParams{
		StartTime:    startTime,
		EndTime:      endTime,
		ProjectUid:   projectUID,
		ComponentUid: componentUID,
		Environment:  req.Environment,
	}
	if req.Project != "" {
		project := req.Project
		params.Project = &project
	}
	if req.Component != "" {
		component := req.Component
		params.Component = &component
	}

	a.logger.Debug("Forwarding recommendation query to FinOps adapter",
		"namespace", req.Namespace,
		"environment", req.Environment,
		"environmentUID", environmentUID,
		"projectUID", ptrUUIDString(projectUID),
		"componentUID", ptrUUIDString(componentUID),
	)

	resp, err := a.client.GetRecommendationsWithResponse(ctx, req.Namespace, environmentUID, params, forwardBearerToken)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFinOpsRetrieval, err)
	}
	return rawFinOpsResponse(resp.StatusCode(), resp.Body)
}

// forwardBearerToken is a request editor that forwards the caller's JWT to the
// FinOps adapter. It is applied only to the recommendations endpoint, which
// requires it: the adapter uses the token to call back into the observer's
// metrics API for usage data. The costs endpoint is unauthenticated, so the
// token is not forwarded there.
func forwardBearerToken(ctx context.Context, req *http.Request) error {
	if token := jwt.GetTokenFromContext(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return nil
}

// resolveScope resolves the environment (required), project (optional), and
// component (optional) names to UIDs. Component requires project.
func (a *FinOpsAdapter) resolveScope(
	ctx context.Context,
	namespace, environment, project, component string,
) (environmentUID openapi_types.UUID, projectUID, componentUID *openapi_types.UUID, err error) {
	envUIDStr, err := a.resolver.GetEnvironmentUID(ctx, namespace, environment)
	if err != nil {
		return environmentUID, nil, nil, fmt.Errorf("%w: failed to get environment UID: %w", ErrFinOpsResolveScope, err)
	}
	environmentUID, err = uuid.Parse(envUIDStr)
	if err != nil {
		return environmentUID, nil, nil, fmt.Errorf("%w: invalid environment UID %q: %w", ErrFinOpsResolveScope, envUIDStr, err)
	}

	if project != "" {
		projUIDStr, projErr := a.resolver.GetProjectUID(ctx, namespace, project)
		if projErr != nil {
			return environmentUID, nil, nil, fmt.Errorf("%w: failed to get project UID: %w", ErrFinOpsResolveScope, projErr)
		}
		projUID, parseErr := uuid.Parse(projUIDStr)
		if parseErr != nil {
			return environmentUID, nil, nil, fmt.Errorf("%w: invalid project UID %q: %w", ErrFinOpsResolveScope, projUIDStr, parseErr)
		}
		projectUID = &projUID
	}

	if component != "" {
		compUIDStr, compErr := a.resolver.GetComponentUID(ctx, namespace, project, component)
		if compErr != nil {
			return environmentUID, nil, nil, fmt.Errorf("%w: failed to get component UID: %w", ErrFinOpsResolveScope, compErr)
		}
		compUID, parseErr := uuid.Parse(compUIDStr)
		if parseErr != nil {
			return environmentUID, nil, nil, fmt.Errorf("%w: invalid component UID %q: %w", ErrFinOpsResolveScope, compUIDStr, parseErr)
		}
		componentUID = &compUID
	}

	return environmentUID, projectUID, componentUID, nil
}

// parseFinOpsWindow validates and parses the RFC 3339 start/end times.
func parseFinOpsWindow(startTime, endTime string) (time.Time, time.Time, error) {
	if startTime == "" || endTime == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: startTime and endTime are required", ErrFinOpsInvalidRequest)
	}
	start, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: invalid startTime: %w", ErrFinOpsInvalidRequest, err)
	}
	end, err := time.Parse(time.RFC3339, endTime)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: invalid endTime: %w", ErrFinOpsInvalidRequest, err)
	}
	return start, end, nil
}

// rawFinOpsResponse returns the adapter body verbatim on a 2xx, or an error
// carrying the status and body otherwise.
func rawFinOpsResponse(statusCode int, body []byte) (any, error) {
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("%w: finops adapter returned HTTP %d: %s", ErrFinOpsRetrieval, statusCode, string(body))
	}
	return json.RawMessage(body), nil
}

func ptrUUIDString(u *openapi_types.UUID) string {
	if u == nil {
		return ""
	}
	return u.String()
}
