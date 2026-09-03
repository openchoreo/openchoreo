// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

var (
	// ErrSpanNotFound is returned when a requested span does not exist.
	ErrSpanNotFound = errors.New("span not found")

	ErrTracesResolveSearchScope = errors.New("traces search scope resolution failed")
	ErrTracesRetrieval          = errors.New("traces retrieval failed")
	ErrTracesInvalidRequest     = errors.New("invalid traces request")
)

type TracesService struct {
	tracingAdapter observability.TracingAdapter
	config         *config.Config
	resolver       *ResourceUIDResolver
	logger         *slog.Logger
}

func NewTracesService(
	tracingAdapter observability.TracingAdapter,
	resolver *ResourceUIDResolver,
	cfg *config.Config,
	logger *slog.Logger,
) (*TracesService, error) {
	if tracingAdapter == nil {
		return nil, fmt.Errorf("tracing adapter is required")
	}
	return &TracesService{
		tracingAdapter: tracingAdapter,
		config:         cfg,
		resolver:       resolver,
		logger:         logger,
	}, nil
}

func (s *TracesService) QueryTraces(ctx context.Context, req *types.TracesQueryRequest) (*types.TracesQueryResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is required", ErrTracesInvalidRequest)
	}

	s.logger.Info("QueryTraces called",
		"startTime", req.StartTime,
		"endTime", req.EndTime)

	// Resolve search scope to UIDs
	projectUID, componentUID, environmentUID, err := s.resolveSearchScope(ctx, &req.SearchScope)
	if err != nil {
		s.logger.Error("Failed to resolve search scope", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrTracesResolveSearchScope, err)
	}

	params := observability.TracesQueryParams{
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		Namespace:     req.SearchScope.Namespace,
		ProjectID:     projectUID,
		ComponentID:   componentUID,
		EnvironmentID: environmentUID,
		Limit:         req.Limit,
		SortOrder:     req.SortOrder,
	}

	result, err := s.tracingAdapter.GetTraces(ctx, params)
	if err != nil {
		s.logger.Error("Failed to retrieve traces", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrTracesRetrieval, err)
	}

	return s.convertToResponse(result), nil
}

func (s *TracesService) resolveSearchScope(ctx context.Context, scope *types.ComponentSearchScope) (projectUID, componentUID, environmentUID string, err error) {
	// Guard against nil resolver when scope fields are provided
	if s.resolver == nil && (scope.Project != "" || scope.Component != "" || scope.Environment != "") {
		return "", "", "", fmt.Errorf("%w: resolver not initialized", ErrTracesResolveSearchScope)
	}

	if scope.Project != "" {
		projectUID, err = s.resolver.GetProjectUID(ctx, scope.Namespace, scope.Project)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to resolve project UID: %w", err)
		}
	}

	if scope.Component != "" {
		if scope.Project == "" {
			return "", "", "", fmt.Errorf("%w: component specified without project", ErrTracesResolveSearchScope)
		}
		componentUID, err = s.resolver.GetComponentUID(ctx, scope.Namespace, scope.Project, scope.Component)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to resolve component UID: %w", err)
		}
	}

	if scope.Environment != "" {
		environmentUID, err = s.resolver.GetEnvironmentUID(ctx, scope.Namespace, scope.Environment)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to resolve environment UID: %w", err)
		}
	}

	return projectUID, componentUID, environmentUID, nil
}

// QuerySpans queries spans within a specific trace.
func (s *TracesService) QuerySpans(ctx context.Context, traceID string, req *types.TracesQueryRequest) (*types.SpansQueryResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is required", ErrTracesInvalidRequest)
	}
	if traceID == "" {
		return nil, fmt.Errorf("%w: traceId is required", ErrTracesInvalidRequest)
	}

	s.logger.Info("QuerySpans called",
		"traceId", traceID,
		"startTime", req.StartTime,
		"endTime", req.EndTime)

	// Resolve search scope to UIDs to enforce access control
	projectUID, componentUID, environmentUID, err := s.resolveSearchScope(ctx, &req.SearchScope)
	if err != nil {
		s.logger.Error("Failed to resolve search scope", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrTracesResolveSearchScope, err)
	}

	params := observability.TracesQueryParams{
		StartTime:         req.StartTime,
		EndTime:           req.EndTime,
		Namespace:         req.SearchScope.Namespace,
		ProjectID:         projectUID,
		ComponentID:       componentUID,
		EnvironmentID:     environmentUID,
		TraceID:           traceID,
		Limit:             req.Limit,
		SortOrder:         req.SortOrder,
		IncludeAttributes: req.IncludeAttributes,
	}

	spansResult, err := s.tracingAdapter.GetSpans(ctx, traceID, params)
	if err != nil {
		s.logger.Error("Failed to retrieve spans", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrTracesRetrieval, err)
	}
	return s.convertAdapterSpansToResponse(spansResult), nil
}

// GetSpanDetails retrieves detailed information about a specific span.
func (s *TracesService) GetSpanDetails(ctx context.Context, traceID string, spanID string) (*types.SpanInfo, error) {
	if traceID == "" {
		return nil, fmt.Errorf("%w: traceId is required", ErrTracesInvalidRequest)
	}
	if spanID == "" {
		return nil, fmt.Errorf("%w: spanId is required", ErrTracesInvalidRequest)
	}

	s.logger.Info("GetSpanDetails called",
		"traceId", traceID,
		"spanId", spanID)

	detail, err := s.tracingAdapter.GetSpanDetails(ctx, traceID, spanID)
	if err != nil {
		s.logger.Error("Failed to retrieve span details", "error", err)
		if errors.Is(err, ErrSpanNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", ErrTracesRetrieval, err)
	}
	return &types.SpanInfo{
		SpanID:             detail.SpanID,
		SpanName:           detail.SpanName,
		SpanKind:           detail.SpanKind,
		ParentSpanID:       detail.ParentSpanID,
		StartTime:          &detail.StartTime,
		EndTime:            &detail.EndTime,
		DurationNs:         detail.DurationNs,
		Status:             detail.Status,
		Attributes:         detail.Attributes,
		ResourceAttributes: detail.ResourceAttributes,
	}, nil
}

func (s *TracesService) convertToResponse(result *observability.TracesQueryResult) *types.TracesQueryResponse {
	traces := make([]types.TraceInfo, len(result.Traces))
	for i, trace := range result.Traces {
		traces[i] = types.TraceInfo{
			TraceID:      trace.TraceID,
			TraceName:    trace.TraceName,
			SpanCount:    trace.SpanCount,
			RootSpanID:   trace.RootSpanID,
			RootSpanName: trace.RootSpanName,
			RootSpanKind: trace.RootSpanKind,
			StartTime:    &trace.StartTime,
			EndTime:      &trace.EndTime,
			DurationNs:   trace.DurationNs,
			HasErrors:    trace.HasErrors,
		}
	}

	return &types.TracesQueryResponse{
		Traces: traces,
		Total:  result.TotalCount,
		TookMs: result.Took,
	}
}

func (s *TracesService) convertAdapterSpansToResponse(result *observability.SpansResult) *types.SpansQueryResponse {
	spans := make([]types.SpanInfo, 0, len(result.Spans))
	for _, span := range result.Spans {
		startTime := span.StartTime
		endTime := span.EndTime
		spans = append(spans, types.SpanInfo{
			SpanID:             span.SpanID,
			SpanName:           span.Name,
			SpanKind:           span.SpanKind,
			ParentSpanID:       span.ParentSpanID,
			StartTime:          &startTime,
			EndTime:            &endTime,
			DurationNs:         span.DurationNs,
			Status:             span.Status,
			Attributes:         span.Attributes,
			ResourceAttributes: span.ResourceAttributes,
		})
	}

	return &types.SpansQueryResponse{
		Spans:  spans,
		Total:  result.TotalCount,
		TookMs: result.Took,
	}
}
