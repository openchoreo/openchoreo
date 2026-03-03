// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// LogsQuerier is the interface for querying logs.
type LogsQuerier interface {
	QueryLogs(ctx context.Context, req *types.LogsQueryRequest) (*types.LogsQueryResponse, error)
}

// MetricsQuerier is the interface for querying metrics.
type MetricsQuerier interface {
	QueryMetrics(ctx context.Context, req *types.MetricsQueryRequest) (any, error)
}

// TracesQuerier is the interface for querying traces and spans.
type TracesQuerier interface {
	QueryTraces(ctx context.Context, req *types.TracesQueryRequest) (*types.TracesQueryResponse, error)
	QuerySpans(ctx context.Context, traceID string, req *types.TracesQueryRequest) (*types.SpansQueryResponse, error)
	GetSpanDetails(ctx context.Context, traceID string, spanID string) (*types.SpanInfo, error)
}
