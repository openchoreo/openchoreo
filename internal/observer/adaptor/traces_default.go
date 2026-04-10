// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package adaptor

import (
	"context"
	"encoding/json"
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

	// When listing traces (no specific traceID), use aggregation query so that
	// the limit controls the number of distinct traces, not individual spans.
	if params.TraceID == "" {
		return a.getTracesWithAggregation(ctx, osParams)
	}

	return a.getTracesWithSpanQuery(ctx, osParams)
}

// getTracesWithAggregation uses a terms aggregation on traceId to return the
// requested number of distinct traces.
func (a *DefaultTracesAdaptor) getTracesWithAggregation(ctx context.Context, osParams opensearch.TracesRequestParams) (*observability.TracesQueryResult, error) {
	query := a.queryBuilder.BuildTracesAggregationQuery(osParams)
	response, err := a.osClient.Search(ctx, []string{"otel-traces-*"}, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute traces aggregation search: %w", err)
	}

	aggResult, err := parseTracesAggregation(response.Aggregations)
	if err != nil {
		return nil, fmt.Errorf("failed to parse traces aggregation: %w", err)
	}

	traces := make([]observability.Trace, 0, len(aggResult.Traces.Buckets))
	for _, bucket := range aggResult.Traces.Buckets {
		trace := buildTraceFromBucket(bucket)
		traces = append(traces, trace)
	}

	totalCount := aggResult.TraceCount.Value
	if totalCount > config.MaxLimit {
		totalCount = config.MaxLimit
	}

	return &observability.TracesQueryResult{
		Traces:     traces,
		TotalCount: totalCount,
		Took:       response.Took,
	}, nil
}

// getTracesWithSpanQuery uses the span-level query (for fetching spans of a specific trace).
func (a *DefaultTracesAdaptor) getTracesWithSpanQuery(ctx context.Context, osParams opensearch.TracesRequestParams) (*observability.TracesQueryResult, error) {
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

// parseTracesAggregation parses the raw aggregation JSON from the OpenSearch response.
func parseTracesAggregation(raw json.RawMessage) (*opensearch.TracesAggregationResult, error) {
	if len(raw) == 0 {
		return &opensearch.TracesAggregationResult{}, nil
	}
	var result opensearch.TracesAggregationResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal aggregation result: %w", err)
	}
	return &result, nil
}

// buildTraceFromBucket converts an aggregation bucket into an observability.Trace.
func buildTraceFromBucket(bucket opensearch.TraceBucket) observability.Trace {
	trace := observability.Trace{
		TraceID:   bucket.Key,
		SpanCount: bucket.DocCount,
	}

	// Extract startTime from the earliest span (top_hits sorted by startTime asc)
	if len(bucket.EarliestSpan.Hits.Hits) > 0 {
		src := bucket.EarliestSpan.Hits.Hits[0].Source
		if src != nil {
			if ts, ok := src["startTime"].(string); ok {
				if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
					trace.StartTime = parsed
				}
			}
		}
	}

	// Extract root span info from the root_span filter aggregation (parentSpanId == "")
	// Falls back to the earliest span if no root span is found
	if len(bucket.RootSpan.Hit.Hits.Hits) > 0 {
		src := bucket.RootSpan.Hit.Hits.Hits[0].Source
		if src != nil {
			if spanID, ok := src["spanId"].(string); ok {
				trace.RootSpanID = spanID
			}
			if name, ok := src["name"].(string); ok {
				trace.RootSpanName = name
				trace.TraceName = name
			}
			if spanKind, ok := src["kind"].(string); ok {
				trace.RootSpanKind = spanKind
			}
		}
	} else if len(bucket.EarliestSpan.Hits.Hits) > 0 {
		src := bucket.EarliestSpan.Hits.Hits[0].Source
		if src != nil {
			if spanID, ok := src["spanId"].(string); ok {
				trace.RootSpanID = spanID
			}
			if name, ok := src["name"].(string); ok {
				trace.RootSpanName = name
				trace.TraceName = name
			}
			if spanKind, ok := src["kind"].(string); ok {
				trace.RootSpanKind = spanKind
			}
		}
	}

	// Extract endTime from the latest span (top_hits sorted by endTime desc)
	if len(bucket.LatestSpan.Hits.Hits) > 0 {
		src := bucket.LatestSpan.Hits.Hits[0].Source
		if src != nil {
			if ts, ok := src["endTime"].(string); ok {
				if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
					trace.EndTime = parsed
				}
			}
		}
	}

	// Calculate duration
	if !trace.StartTime.IsZero() && !trace.EndTime.IsZero() {
		trace.DurationNs = trace.EndTime.Sub(trace.StartTime).Nanoseconds()
	}

	// Determine trace status from error_span sub-aggregation
	if bucket.ErrorSpanCount.DocCount > 0 {
		trace.HasErrors = true
	} else {
		trace.HasErrors = false
	}

	return trace
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
			SpanKind:           span.SpanKind,
			ParentSpanID:       span.ParentSpanID,
			StartTime:          span.StartTime,
			EndTime:            span.EndTime,
			DurationNs:         span.DurationNanoseconds,
			Status:             span.Status,
			Attributes:         span.Attributes,
			ResourceAttributes: span.ResourceAttributes,
		}
	}

	// Derive trace hasErrors from span statuses
	traceHasErrors := false
	for _, s := range traceSpans {
		if s.Status == opensearch.SpanStatusError {
			traceHasErrors = true
			break
		}
	}

	trace := observability.Trace{
		TraceID:    traceID,
		SpanCount:  len(spans),
		StartTime:  minStartTime,
		EndTime:    maxEndTime,
		DurationNs: maxEndTime.Sub(minStartTime).Nanoseconds(),
		HasErrors:  traceHasErrors,
		Spans:      traceSpans,
	}

	if rootSpan != nil {
		trace.RootSpanID = rootSpan.SpanID
		trace.RootSpanName = rootSpan.Name
		trace.RootSpanKind = rootSpan.SpanKind
		trace.TraceName = rootSpan.Name
	}

	return trace
}
