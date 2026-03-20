// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package adaptor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

var (
	ErrSpanNotFound = errors.New("span not found")
)

type DefaultTracesAdaptor struct {
	osClient     *opensearch.Client
	queryBuilder *opensearch.QueryBuilder
	logger       *slog.Logger
}

func NewDefaultTracesAdaptor(cfg *config.OpenSearchConfig, logger *slog.Logger) (*DefaultTracesAdaptor, error) {
	osClient, err := opensearch.NewClient(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenSearch client: %w", err)
	}

	return &DefaultTracesAdaptor{
		osClient:     osClient,
		queryBuilder: opensearch.NewQueryBuilder(cfg.IndexPrefix),
		logger:       logger,
	}, nil
}

func (a *DefaultTracesAdaptor) GetTraces(ctx context.Context, params observability.TracesQueryParams) (*observability.TracesQueryResult, error) {
	a.logger.Debug("Getting traces from OpenSearch")

	// Build OpenSearch query params from observability params
	osParams := opensearch.TracesRequestParams{
		StartTime:      params.StartTime.Format(time.RFC3339),
		EndTime:        params.EndTime.Format(time.RFC3339),
		ProjectUID:     params.ProjectID,
		EnvironmentUID: params.EnvironmentID,
		TraceID:        params.TraceID,
		Limit:          params.Limit,
		SortOrder:      params.SortOrder,
	}
	// Only add ComponentUID if it's not empty
	if params.ComponentID != "" {
		osParams.ComponentUIDs = []string{params.ComponentID}
	}

	// Build and execute query
	query := a.queryBuilder.BuildTracesQuery(osParams)
	response, err := a.osClient.Search(ctx, []string{"otel-traces-*"}, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute traces search: %w", err)
	}

	// Parse and group spans by trace ID
	traceMap := make(map[string][]opensearch.Span)
	for _, hit := range response.Hits.Hits {
		span := opensearch.ParseSpanEntry(hit)
		traceID := opensearch.GetTraceID(hit)
		if traceID != "" {
			traceMap[traceID] = append(traceMap[traceID], span)
		}
	}

	// Convert to observability.Trace format
	traces := make([]observability.Trace, 0, len(traceMap))
	traceIDs := make([]string, 0, len(traceMap))
	for traceID := range traceMap {
		traceIDs = append(traceIDs, traceID)
	}
	sort.Strings(traceIDs)

	for _, traceID := range traceIDs {
		spans := traceMap[traceID]
		trace := convertToObservabilityTrace(traceID, spans)
		traces = append(traces, trace)
	}

	return &observability.TracesQueryResult{
		Traces:     traces,
		TotalCount: min(response.Hits.Total.Value, config.MaxLimit),
		Took:       response.Took,
	}, nil
}

// GetSpanDetails retrieves details for a specific span
func (a *DefaultTracesAdaptor) GetSpanDetails(ctx context.Context, traceID string, spanID string) (*opensearch.Span, error) {
	a.logger.Debug("Getting span details from OpenSearch", "traceId", traceID, "spanId", spanID)

	// Build query to get the specific span
	query := a.queryBuilder.BuildSpanDetailsQuery(traceID, spanID)
	response, err := a.osClient.Search(ctx, []string{"otel-traces-*"}, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute span search: %w", err)
	}

	// Check if span was found
	if len(response.Hits.Hits) == 0 {
		return nil, ErrSpanNotFound
	}

	// Parse and return the span
	span := opensearch.ParseSpanEntry(response.Hits.Hits[0])
	return &span, nil
}

func convertToObservabilityTrace(traceID string, spans []opensearch.Span) observability.Trace {
	// Find root span and calculate trace metadata
	var rootSpan *opensearch.Span
	var minStartTime, maxEndTime time.Time

	for i := range spans {
		span := &spans[i]
		if rootSpan == nil || span.ParentSpanID == "" {
			rootSpan = span
		}
		if minStartTime.IsZero() || span.StartTime.Before(minStartTime) {
			minStartTime = span.StartTime
		}
		if maxEndTime.IsZero() || span.EndTime.After(maxEndTime) {
			maxEndTime = span.EndTime
		}
	}

	// Convert opensearch spans to observability spans
	traceSpans := make([]observability.TraceSpan, len(spans))
	for i, span := range spans {
		traceSpans[i] = observability.TraceSpan{
			SpanID:             span.SpanID,
			Name:               span.Name,
			ParentSpanID:       span.ParentSpanID,
			StartTime:          span.StartTime,
			EndTime:            span.EndTime,
			DurationNs:         span.DurationNanoseconds,
			Attributes:         span.Attributes,
			ResourceAttributes: span.ResourceAttributes,
		}
	}

	trace := observability.Trace{
		TraceID:    traceID,
		SpanCount:  len(spans),
		StartTime:  minStartTime,
		EndTime:    maxEndTime,
		DurationNs: maxEndTime.Sub(minStartTime).Nanoseconds(),
		Spans:      traceSpans,
	}

	if rootSpan != nil {
		trace.RootSpanID = rootSpan.SpanID
		trace.RootSpanName = rootSpan.Name
		trace.TraceName = rootSpan.Name
	}

	return trace
}
