// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

type TracingAdapter struct {
	client *gen.ClientWithResponses
}

type TracingAdapterConfig struct {
	BaseURL string
	Timeout time.Duration
}

func NewTracingAdapter(config TracingAdapterConfig) (*TracingAdapter, error) {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	client, err := gen.NewClientWithResponses(config.BaseURL, gen.WithHTTPClient(&http.Client{
		Timeout: config.Timeout,
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to create tracing adapter client: %w", err)
	}

	return &TracingAdapter{
		client: client,
	}, nil
}

// GetTraces implements observability.TracingAdapter interface
func (t *TracingAdapter) GetTraces(ctx context.Context, params observability.TracesQueryParams) (*observability.TracesQueryResult, error) {
	reqBody := gen.QueryTracesJSONRequestBody{
		StartTime: params.StartTime,
		EndTime:   params.EndTime,
		SearchScope: gen.ComponentSearchScope{
			Namespace: params.Namespace,
		},
	}

	if params.Limit > 0 {
		reqBody.Limit = &params.Limit
	}
	if params.SortOrder != "" {
		sort := gen.TracesQueryRequestSort(params.SortOrder)
		reqBody.Sort = &sort
	}
	if params.ProjectID != "" {
		reqBody.SearchScope.Project = &params.ProjectID
	}
	if params.ComponentID != "" {
		reqBody.SearchScope.Component = &params.ComponentID
	}
	if params.EnvironmentID != "" {
		reqBody.SearchScope.Environment = &params.EnvironmentID
	}

	resp, err := t.client.QueryTracesWithResponse(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected nil response body")
	}

	return convertTracesResponse(resp.JSON200), nil
}

// GetSpans implements observability.TracingAdapter interface
func (t *TracingAdapter) GetSpans(ctx context.Context, traceID string, params observability.TracesQueryParams) (*observability.SpansResult, error) {
	reqBody := gen.QuerySpansForTraceJSONRequestBody{
		StartTime: params.StartTime,
		EndTime:   params.EndTime,
		SearchScope: gen.ComponentSearchScope{
			Namespace: params.Namespace,
		},
	}

	if params.Limit > 0 {
		reqBody.Limit = &params.Limit
	}
	if params.SortOrder != "" {
		sort := gen.TracesQueryRequestSort(params.SortOrder)
		reqBody.Sort = &sort
	}
	if params.ProjectID != "" {
		reqBody.SearchScope.Project = &params.ProjectID
	}
	if params.ComponentID != "" {
		reqBody.SearchScope.Component = &params.ComponentID
	}
	if params.EnvironmentID != "" {
		reqBody.SearchScope.Environment = &params.EnvironmentID
	}

	resp, err := t.client.QuerySpansForTraceWithResponse(ctx, traceID, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected nil response body")
	}

	return convertSpansAdapterResponse(resp.JSON200), nil
}

// GetSpanDetails implements observability.TracingAdapter interface
func (t *TracingAdapter) GetSpanDetails(ctx context.Context, traceID string, spanID string) (*observability.SpanDetail, error) {
	resp, err := t.client.GetSpanDetailsForTraceWithResponse(ctx, traceID, spanID)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return nil, ErrSpanNotFound
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected nil response body")
	}

	return convertSpanDetailResponse(resp.JSON200), nil
}

func convertSpanDetailResponse(resp *gen.TraceSpanDetailsResponse) *observability.SpanDetail {
	detail := &observability.SpanDetail{}

	if resp.SpanId != nil {
		detail.SpanID = *resp.SpanId
	}
	if resp.SpanName != nil {
		detail.SpanName = *resp.SpanName
	}
	if resp.ParentSpanId != nil {
		detail.ParentSpanID = *resp.ParentSpanId
	}
	if resp.StartTime != nil {
		detail.StartTime = *resp.StartTime
	}
	if resp.EndTime != nil {
		detail.EndTime = *resp.EndTime
	}
	if resp.DurationNs != nil {
		detail.DurationNs = *resp.DurationNs
	}

	if resp.Attributes != nil {
		detail.Attributes = make(map[string]interface{}, len(*resp.Attributes))
		for _, attr := range *resp.Attributes {
			if attr.Key != nil && attr.Value != nil {
				detail.Attributes[*attr.Key] = *attr.Value
			}
		}
	}

	return detail
}

func convertSpansAdapterResponse(resp *gen.TraceSpansQueryResponse) *observability.SpansResult {
	result := &observability.SpansResult{}

	if resp.TookMs != nil {
		result.Took = *resp.TookMs
	}
	if resp.Total != nil {
		result.TotalCount = *resp.Total
	}

	if resp.Spans != nil {
		for _, s := range *resp.Spans {
			span := observability.TraceSpan{}
			if s.SpanId != nil {
				span.SpanID = *s.SpanId
			}
			if s.SpanName != nil {
				span.Name = *s.SpanName
			}
			if s.ParentSpanId != nil {
				span.ParentSpanID = *s.ParentSpanId
			}
			if s.StartTime != nil {
				span.StartTime = *s.StartTime
			}
			if s.EndTime != nil {
				span.EndTime = *s.EndTime
			}
			if s.DurationNs != nil {
				span.DurationNs = *s.DurationNs
			}
			result.Spans = append(result.Spans, span)
		}
	}

	return result
}

func convertTracesResponse(resp *gen.TracesQueryResponse) *observability.TracesQueryResult {
	result := &observability.TracesQueryResult{}

	if resp.TookMs != nil {
		result.Took = *resp.TookMs
	}
	if resp.Total != nil {
		result.TotalCount = *resp.Total
	}

	if resp.Traces != nil {
		for _, t := range *resp.Traces {
			trace := observability.Trace{}
			if t.TraceId != nil {
				trace.TraceID = *t.TraceId
			}
			if t.TraceName != nil {
				trace.TraceName = *t.TraceName
			}
			if t.SpanCount != nil {
				trace.SpanCount = *t.SpanCount
			}
			if t.RootSpanId != nil {
				trace.RootSpanID = *t.RootSpanId
			}
			if t.RootSpanName != nil {
				trace.RootSpanName = *t.RootSpanName
			}
			if t.RootSpanKind != nil {
				trace.RootSpanKind = *t.RootSpanKind
			}
			if t.StartTime != nil {
				trace.StartTime = *t.StartTime
			}
			if t.EndTime != nil {
				trace.EndTime = *t.EndTime
			}
			if t.DurationNs != nil {
				trace.DurationNs = *t.DurationNs
			}
			result.Traces = append(result.Traces, trace)
		}
	}

	return result
}
