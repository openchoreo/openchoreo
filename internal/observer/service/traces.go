// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/openchoreo/openchoreo/internal/observer/adaptor"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

var (
	ErrTracesResolveSearchScope = errors.New("traces search scope resolution failed")
	ErrTracesRetrieval          = errors.New("traces retrieval failed")
	ErrTracesInvalidRequest     = errors.New("invalid traces request")
)

type TracesService struct {
	tracesBackend    observability.TracesBackend
	defaultAdaptor   *adaptor.DefaultTracesAdaptor
	config           *config.Config
	resolver         *ResourceUIDResolver
	logger           *slog.Logger
}

func NewTracesService(
	tracesBackend observability.TracesBackend,
	resolver *ResourceUIDResolver,
	cfg *config.Config,
	logger *slog.Logger,
) (*TracesService, error) {
	var defaultAdaptor *adaptor.DefaultTracesAdaptor

	// Initialize default traces adaptor (queries OpenSearch when backend is not enabled)
	if !cfg.Experimental.UseTracesBackend || tracesBackend == nil {
		var err error
		defaultAdaptor, err = adaptor.NewDefaultTracesAdaptor(&cfg.OpenSearch, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize default traces adaptor: %w", err)
		}
	}

	return &TracesService{
		tracesBackend:  tracesBackend,
		defaultAdaptor: defaultAdaptor,
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
		"endTime", req.EndTime,
		"useTracesBackend", s.config.Experimental.UseTracesBackend)

	// Resolve search scope to UIDs
	projectUID, componentUID, environmentUID, err := s.resolveSearchScope(ctx, &req.SearchScope)
	if err != nil {
		s.logger.Error("Failed to resolve search scope", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrTracesResolveSearchScope, err)
	}

	// Build query params (handler already converted defaults)
	params := observability.TracesQueryParams{
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		Namespace:     req.SearchScope.Namespace,
		ProjectID:     projectUID,
		ComponentID:   componentUID,
		EnvironmentID: environmentUID,
		Limit:         req.Limit,
		SortOrder:     req.Sort,
	}

	// Route to backend or OpenSearch
	var result *observability.TracesQueryResult
	if s.config.Experimental.UseTracesBackend && s.tracesBackend != nil {
		s.logger.Debug("Using traces backend for query")
		result, err = s.tracesBackend.GetTraces(ctx, params)
	} else {
		s.logger.Debug("Using default adaptor (OpenSearch) for query")
		result, err = s.defaultAdaptor.GetTraces(ctx, params)
	}

	if err != nil {
		s.logger.Error("Failed to retrieve traces", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrTracesRetrieval, err)
	}

	return s.convertToResponse(result), nil
}

func (s *TracesService) resolveSearchScope(ctx context.Context, scope *types.ComponentSearchScope) (projectUID, componentUID, environmentUID string, err error) {
	if scope.Project != "" {
		projectUID, err = s.resolver.GetProjectUID(ctx, scope.Namespace, scope.Project)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to resolve project UID: %w", err)
		}
	}

	if scope.Component != "" {
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

// QuerySpans queries spans within a specific trace
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

	// Build query params for spans with the specific trace ID
	params := observability.TracesQueryParams{
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		TraceID:   traceID,
		Limit:     req.Limit,
		SortOrder: req.Sort,
	}

	// Query spans using the default adaptor (for now, external backend not supported for span queries)
	result, err := s.defaultAdaptor.GetTraces(ctx, params)
	if err != nil {
		s.logger.Error("Failed to retrieve spans", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrTracesRetrieval, err)
	}

	return s.convertSpansToResponse(result), nil
}

// GetSpanDetails retrieves detailed information about a specific span
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

	// Query using the default adaptor
	span, err := s.defaultAdaptor.GetSpanDetails(ctx, traceID, spanID)
	if err != nil {
		s.logger.Error("Failed to retrieve span details", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrTracesRetrieval, err)
	}

	return &types.SpanInfo{
		SpanID:             span.SpanID,
		SpanName:           span.Name,
		ParentSpanID:       span.ParentSpanID,
		StartTime:          &span.StartTime,
		EndTime:            &span.EndTime,
		DurationNs:         float64(span.DurationNanoseconds),
		Attributes:         span.Attributes,
		ResourceAttributes: span.ResourceAttributes,
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
			DurationNs:   float64(trace.DurationNs),
		}
	}

	return &types.TracesQueryResponse{
		Traces: traces,
		Total:  result.TotalCount,
		TookMs: result.Took,
	}
}

func (s *TracesService) convertSpansToResponse(result *observability.TracesQueryResult) *types.SpansQueryResponse {
	spans := make([]types.SpanInfo, len(result.Traces))
	for i, trace := range result.Traces {
		spans[i] = types.SpanInfo{
			SpanID:       trace.RootSpanID,
			SpanName:     trace.RootSpanName,
			StartTime:    &trace.StartTime,
			EndTime:      &trace.EndTime,
			DurationNs:   float64(trace.DurationNs),
		}
	}

	return &types.SpansQueryResponse{
		Spans:  spans,
		Total:  len(spans),
		TookMs: result.Took,
	}
}
