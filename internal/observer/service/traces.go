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

// Re-export adaptor errors for use in handlers
var (
	ErrSpanNotFound = adaptor.ErrSpanNotFound
)

var (
	ErrTracesResolveSearchScope = errors.New("traces search scope resolution failed")
	ErrTracesRetrieval          = errors.New("traces retrieval failed")
	ErrTracesInvalidRequest     = errors.New("invalid traces request")
)

type TracesService struct {
	tracingAdapter observability.TracingAdapter
	defaultAdaptor *adaptor.DefaultTracesAdaptor
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
	// Always initialize default traces adaptor since QuerySpans and GetSpanDetails depend on it
	// even when the tracing adapter is enabled for QueryTraces.
	defaultAdaptor, err := adaptor.NewDefaultTracesAdaptor(&cfg.OpenSearch, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize default traces adaptor: %w", err)
	}

	return &TracesService{
		tracingAdapter: tracingAdapter,
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
		"useTracingAdapter", s.config.Adapters.TracingAdapterEnabled)

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
		SortOrder:     req.SortOrder,
	}

	// Route to tracing adapter or OpenSearch
	var result *observability.TracesQueryResult
	if s.config.Adapters.TracingAdapterEnabled && s.tracingAdapter != nil {
		s.logger.Debug("Using tracing adapter for query")
		result, err = s.tracingAdapter.GetTraces(ctx, params)
	} else {
		if s.defaultAdaptor == nil {
			return nil, fmt.Errorf("%w: default traces adaptor not initialized", ErrTracesRetrieval)
		}
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

	// Resolve search scope to UIDs to enforce access control
	projectUID, componentUID, environmentUID, err := s.resolveSearchScope(ctx, &req.SearchScope)
	if err != nil {
		s.logger.Error("Failed to resolve search scope", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrTracesResolveSearchScope, err)
	}

	// Build query params for spans with the specific trace ID and scope
	params := observability.TracesQueryParams{
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		Namespace:     req.SearchScope.Namespace,
		ProjectID:     projectUID,
		ComponentID:   componentUID,
		EnvironmentID: environmentUID,
		TraceID:       traceID,
		Limit:         req.Limit,
		SortOrder:     req.SortOrder,
	}

	// Route to tracing adapter or OpenSearch
	if s.config.Adapters.TracingAdapterEnabled && s.tracingAdapter != nil {
		s.logger.Debug("Using tracing adapter for span query")
		spansResult, err := s.tracingAdapter.GetSpans(ctx, traceID, params)
		if err != nil {
			s.logger.Error("Failed to retrieve spans", "error", err)
			return nil, fmt.Errorf("%w: %w", ErrTracesRetrieval, err)
		}
		return s.convertAdapterSpansToResponse(spansResult), nil
	}

	if s.defaultAdaptor == nil {
		return nil, fmt.Errorf("%w: default traces adaptor not initialized", ErrTracesRetrieval)
	}

	s.logger.Debug("Using default adaptor (OpenSearch) for span query")
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

	// Route to tracing adapter or OpenSearch
	if s.config.Adapters.TracingAdapterEnabled && s.tracingAdapter != nil {
		s.logger.Debug("Using tracing adapter for span details")
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
			ParentSpanID:       detail.ParentSpanID,
			StartTime:          &detail.StartTime,
			EndTime:            &detail.EndTime,
			DurationNs:         detail.DurationNs,
			Attributes:         detail.Attributes,
			ResourceAttributes: detail.ResourceAttributes,
		}, nil
	}

	if s.defaultAdaptor == nil {
		return nil, fmt.Errorf("%w: default traces adaptor not initialized", ErrTracesRetrieval)
	}

	s.logger.Debug("Using default adaptor (OpenSearch) for span details")
	span, err := s.defaultAdaptor.GetSpanDetails(ctx, traceID, spanID)
	if err != nil {
		s.logger.Error("Failed to retrieve span details", "error", err)
		// Pass through ErrSpanNotFound without wrapping so handlers can detect it
		if errors.Is(err, ErrSpanNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", ErrTracesRetrieval, err)
	}

	return &types.SpanInfo{
		SpanID:             span.SpanID,
		SpanName:           span.Name,
		ParentSpanID:       span.ParentSpanID,
		StartTime:          &span.StartTime,
		EndTime:            &span.EndTime,
		DurationNs:         span.DurationNanoseconds,
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
			DurationNs:   trace.DurationNs,
		}
	}

	return &types.TracesQueryResponse{
		Traces: traces,
		Total:  result.TotalCount,
		TookMs: result.Took,
	}
}

func (s *TracesService) convertSpansToResponse(result *observability.TracesQueryResult) *types.SpansQueryResponse {
	var spans []types.SpanInfo
	// Calculate total spans to preallocate
	totalSpans := 0
	for _, trace := range result.Traces {
		totalSpans += len(trace.Spans)
	}
	spans = make([]types.SpanInfo, 0, totalSpans)

	// Flatten spans from all traces
	for _, trace := range result.Traces {
		for _, traceSpan := range trace.Spans {
			spans = append(spans, types.SpanInfo{
				SpanID:             traceSpan.SpanID,
				SpanName:           traceSpan.Name,
				ParentSpanID:       traceSpan.ParentSpanID,
				StartTime:          &traceSpan.StartTime,
				EndTime:            &traceSpan.EndTime,
				DurationNs:         traceSpan.DurationNs,
				Attributes:         traceSpan.Attributes,
				ResourceAttributes: traceSpan.ResourceAttributes,
			})
		}
	}

	return &types.SpansQueryResponse{
		Spans:  spans,
		Total:  len(spans),
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
			ParentSpanID:       span.ParentSpanID,
			StartTime:          &startTime,
			EndTime:            &endTime,
			DurationNs:         span.DurationNs,
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
