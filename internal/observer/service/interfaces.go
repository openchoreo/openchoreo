// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
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

// AlertsQuerier is the interface for querying alerts.
type AlertsQuerier interface {
	QueryAlerts(ctx context.Context, req gen.AlertsQueryRequest) (*gen.AlertsQueryResponse, error)
}

// IncidentsQuerier is the interface for querying incidents.
type IncidentsQuerier interface {
	QueryIncidents(ctx context.Context, req gen.IncidentsQueryRequest) (*gen.IncidentsQueryResponse, error)
}

// IncidentsUpdater is the interface for updating incidents.
type IncidentsUpdater interface {
	UpdateIncident(ctx context.Context, incidentID string, req gen.IncidentPutRequest) (*gen.IncidentPutResponse, error)
}

// AlertIncidentService is a composite interface combining alert query, incident query,
// and incident update operations. The concrete *AlertService satisfies this interface.
// The individual sub-interfaces are kept for consumers that only need a subset.
type AlertIncidentService interface {
	AlertsQuerier
	IncidentsQuerier
	IncidentsUpdater
}
