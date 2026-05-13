// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// nonZeroUUID returns a UUID with all bytes set to 1 — used to satisfy required UUID fields.
var nonZeroUUID = openapi_types.UUID{1}

func TestValidateTimeRange(t *testing.T) {
	t.Parallel()

	const (
		validStart = "2024-01-01T00:00:00Z"
		validEnd   = "2024-01-02T00:00:00Z"
	)

	tests := []struct {
		name        string
		start       string
		end         string
		wantErr     bool
		errContains string
	}{
		{name: "missing startTime", start: "", end: validEnd, wantErr: true, errContains: "startTime is required"},
		{name: "missing endTime", start: validStart, end: "", wantErr: true, errContains: "endTime is required"},
		{name: "invalid start format", start: "2024-01-01", end: validEnd, wantErr: true, errContains: "startTime must be in RFC3339"},
		{name: "invalid end format", start: validStart, end: "not-a-time", wantErr: true, errContains: "endTime must be in RFC3339"},
		{name: "end before start", start: "2024-01-02T00:00:00Z", end: "2024-01-01T00:00:00Z", wantErr: true, errContains: "endTime must be after startTime"},
		{name: "range exceeds 30 days", start: "2024-01-01T00:00:00Z", end: "2024-02-02T00:00:00Z", wantErr: true, errContains: "cannot exceed"},
		{name: "valid range", start: validStart, end: validEnd, wantErr: false},
		{name: "range exactly 30 days", start: "2024-01-01T00:00:00Z", end: "2024-01-31T00:00:00Z", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTimeRange(tt.start, tt.end)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateAndSetLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		limit       int
		wantErr     bool
		errContains string
		wantLimit   int
	}{
		{name: "zero sets default", limit: 0, wantErr: false, wantLimit: defaultLimit},
		{name: "negative returns error", limit: -1, wantErr: true, errContains: "positive integer"},
		{name: "exceeds max returns error", limit: config.MaxLimit + 1, wantErr: true, errContains: "cannot exceed"},
		{name: "valid positive limit", limit: 50, wantErr: false, wantLimit: 50},
		{name: "max limit is allowed", limit: config.MaxLimit, wantErr: false, wantLimit: config.MaxLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := tt.limit
			err := ValidateAndSetLimit(&l)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantLimit, l)
			}
		})
	}
}

func TestValidateAndSetSortOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantOrder string
	}{
		{name: "empty sets default desc", input: "", wantErr: false, wantOrder: "desc"},
		{name: "asc is valid", input: "asc", wantErr: false, wantOrder: "asc"},
		{name: "desc is valid", input: "desc", wantErr: false, wantOrder: "desc"},
		{name: "invalid order", input: "random", wantErr: true},
		{name: "case sensitive invalid", input: "ASC", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := tt.input
			err := ValidateAndSetSortOrder(&s)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "sortOrder must be either")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantOrder, s)
			}
		})
	}
}

func TestValidateLogLevels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		levels      []string
		wantErr     bool
		errContains string
	}{
		{name: "empty list is valid", levels: nil, wantErr: false},
		{name: "all valid levels", levels: []string{"DEBUG", "INFO", "WARN", "ERROR"}, wantErr: false},
		{name: "single valid level", levels: []string{"INFO"}, wantErr: false},
		{name: "invalid level", levels: []string{"TRACE"}, wantErr: true, errContains: "invalid log level"},
		{name: "lowercase invalid", levels: []string{"info"}, wantErr: true, errContains: "invalid log level"},
		{name: "duplicate level", levels: []string{"INFO", "INFO"}, wantErr: true, errContains: "duplicate log level"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateLogLevels(tt.levels)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateLogsQueryRequest(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	validStart := now.Add(-1 * time.Hour).Format(time.RFC3339)
	validEnd := now.Format(time.RFC3339)

	validComponentScope := &types.SearchScope{
		Component: &types.ComponentSearchScope{Namespace: "test-ns"},
	}
	validWorkflowScope := &types.SearchScope{
		Workflow: &types.WorkflowSearchScope{Namespace: "test-ns"},
	}

	tests := []struct {
		name        string
		req         *types.LogsQueryRequest
		wantErr     bool
		errContains string
	}{
		{name: "nil request", req: nil, wantErr: true, errContains: "required"},
		{
			name:        "nil searchScope",
			req:         &types.LogsQueryRequest{StartTime: validStart, EndTime: validEnd},
			wantErr:     true,
			errContains: "searchScope is required",
		},
		{
			name: "both component and workflow scope",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Component: &types.ComponentSearchScope{Namespace: "ns"},
					Workflow:  &types.WorkflowSearchScope{Namespace: "ns"},
				},
				StartTime: validStart, EndTime: validEnd,
			},
			wantErr:     true,
			errContains: "cannot be both",
		},
		{
			name: "neither component nor workflow scope",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{},
				StartTime:   validStart, EndTime: validEnd,
			},
			wantErr:     true,
			errContains: "must be either",
		},
		{
			name: "component scope missing namespace",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Component: &types.ComponentSearchScope{Namespace: ""},
				},
				StartTime: validStart, EndTime: validEnd,
			},
			wantErr:     true,
			errContains: "namespace is required",
		},
		{
			name: "component without project when component set",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Component: &types.ComponentSearchScope{Namespace: "ns", Component: "comp"},
				},
				StartTime: validStart, EndTime: validEnd,
			},
			wantErr:     true,
			errContains: "project is required when",
		},
		{
			name: "workflow scope missing namespace",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Workflow: &types.WorkflowSearchScope{Namespace: ""},
				},
				StartTime: validStart, EndTime: validEnd,
			},
			wantErr:     true,
			errContains: "namespace is required",
		},
		{
			name: "invalid time range",
			req: &types.LogsQueryRequest{
				SearchScope: validComponentScope,
				StartTime:   validEnd, EndTime: validStart, // reversed
			},
			wantErr:     true,
			errContains: "endTime must be after startTime",
		},
		{
			name: "invalid log level",
			req: &types.LogsQueryRequest{
				SearchScope: validComponentScope,
				StartTime:   validStart, EndTime: validEnd,
				LogLevels: []string{"INVALID"},
			},
			wantErr:     true,
			errContains: "invalid log level",
		},
		{
			name: "valid component scope",
			req: &types.LogsQueryRequest{
				SearchScope: validComponentScope,
				StartTime:   validStart, EndTime: validEnd,
			},
			wantErr: false,
		},
		{
			name: "valid workflow scope",
			req: &types.LogsQueryRequest{
				SearchScope: validWorkflowScope,
				StartTime:   validStart, EndTime: validEnd,
			},
			wantErr: false,
		},
		{
			name: "valid component with project",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Component: &types.ComponentSearchScope{Namespace: "ns", Project: "proj", Component: "comp"},
				},
				StartTime: validStart, EndTime: validEnd,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateLogsQueryRequest(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateMetricsQueryRequest(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	validStart := now.Add(-1 * time.Hour).Format(time.RFC3339)
	validEnd := now.Format(time.RFC3339)

	tests := []struct {
		name        string
		req         *types.MetricsQueryRequest
		wantErr     bool
		errContains string
	}{
		{name: "nil request", req: nil, wantErr: true, errContains: "must not be nil"},
		{
			name:        "missing metric",
			req:         &types.MetricsQueryRequest{StartTime: validStart, EndTime: validEnd, SearchScope: types.ComponentSearchScope{Namespace: "ns"}},
			wantErr:     true,
			errContains: "metric is required",
		},
		{
			name:        "invalid metric type",
			req:         &types.MetricsQueryRequest{Metric: "invalid", StartTime: validStart, EndTime: validEnd, SearchScope: types.ComponentSearchScope{Namespace: "ns"}},
			wantErr:     true,
			errContains: "metric must be either",
		},
		{
			name:        "missing namespace",
			req:         &types.MetricsQueryRequest{Metric: "resource", StartTime: validStart, EndTime: validEnd},
			wantErr:     true,
			errContains: "namespace is required",
		},
		{
			name: "component without project",
			req: &types.MetricsQueryRequest{
				Metric:    "resource",
				StartTime: validStart, EndTime: validEnd,
				SearchScope: types.ComponentSearchScope{Namespace: "ns", Component: "comp"},
			},
			wantErr:     true,
			errContains: "project is required when",
		},
		{
			name: "invalid step format",
			req: &types.MetricsQueryRequest{
				Metric:    "resource",
				StartTime: validStart, EndTime: validEnd,
				SearchScope: types.ComponentSearchScope{Namespace: "ns"},
				Step:        strPtr("not-a-duration"),
			},
			wantErr:     true,
			errContains: "valid duration",
		},
		{
			name: "zero step",
			req: &types.MetricsQueryRequest{
				Metric:    "resource",
				StartTime: validStart, EndTime: validEnd,
				SearchScope: types.ComponentSearchScope{Namespace: "ns"},
				Step:        strPtr("0s"),
			},
			wantErr:     true,
			errContains: "greater than 0",
		},
		{
			name: "valid resource metric",
			req: &types.MetricsQueryRequest{
				Metric:    "resource",
				StartTime: validStart, EndTime: validEnd,
				SearchScope: types.ComponentSearchScope{Namespace: "ns"},
			},
			wantErr: false,
		},
		{
			name: "valid http metric",
			req: &types.MetricsQueryRequest{
				Metric:    "http",
				StartTime: validStart, EndTime: validEnd,
				SearchScope: types.ComponentSearchScope{Namespace: "ns"},
			},
			wantErr: false,
		},
		{
			name: "valid with step",
			req: &types.MetricsQueryRequest{
				Metric:    "resource",
				StartTime: validStart, EndTime: validEnd,
				SearchScope: types.ComponentSearchScope{Namespace: "ns"},
				Step:        strPtr("5m"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateMetricsQueryRequest(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateTracesQueryRequest(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	start := now.Add(-1 * time.Hour)
	end := now

	ptrComponent := func(s string) *string { return &s }

	tests := []struct {
		name        string
		req         *gen.TracesQueryRequest
		wantErr     bool
		errContains string
	}{
		{name: "nil request", req: nil, wantErr: true, errContains: "required"},
		{
			name:        "zero startTime",
			req:         &gen.TracesQueryRequest{EndTime: end, SearchScope: gen.ComponentSearchScope{Namespace: "ns"}},
			wantErr:     true,
			errContains: "startTime is required",
		},
		{
			name:        "zero endTime",
			req:         &gen.TracesQueryRequest{StartTime: start, SearchScope: gen.ComponentSearchScope{Namespace: "ns"}},
			wantErr:     true,
			errContains: "endTime is required",
		},
		{
			name:        "missing namespace",
			req:         &gen.TracesQueryRequest{StartTime: start, EndTime: end, SearchScope: gen.ComponentSearchScope{}},
			wantErr:     true,
			errContains: "namespace is required",
		},
		{
			name: "component without project",
			req: &gen.TracesQueryRequest{
				StartTime: start, EndTime: end,
				SearchScope: gen.ComponentSearchScope{Namespace: "ns", Component: ptrComponent("comp")},
			},
			wantErr:     true,
			errContains: "project is required when",
		},
		{
			name: "negative limit",
			req: &gen.TracesQueryRequest{
				StartTime: start, EndTime: end,
				SearchScope: gen.ComponentSearchScope{Namespace: "ns"},
				Limit:       intPtr(-1),
			},
			wantErr:     true,
			errContains: "positive integer",
		},
		{
			name: "limit exceeds max",
			req: &gen.TracesQueryRequest{
				StartTime: start, EndTime: end,
				SearchScope: gen.ComponentSearchScope{Namespace: "ns"},
				Limit:       intPtr(config.MaxLimit + 1),
			},
			wantErr:     true,
			errContains: "cannot exceed",
		},
		{
			name: "invalid sort order",
			req: &gen.TracesQueryRequest{
				StartTime: start, EndTime: end,
				SearchScope: gen.ComponentSearchScope{Namespace: "ns"},
				SortOrder:   (*gen.TracesQueryRequestSortOrder)(strPtr("invalid")),
			},
			wantErr:     true,
			errContains: "sortOrder must be either",
		},
		{
			name: "valid minimal request",
			req: &gen.TracesQueryRequest{
				StartTime: start, EndTime: end,
				SearchScope: gen.ComponentSearchScope{Namespace: "ns"},
			},
			wantErr: false,
		},
		{
			name: "valid with all fields",
			req: &gen.TracesQueryRequest{
				StartTime: start, EndTime: end,
				SearchScope: gen.ComponentSearchScope{Namespace: "ns", Project: ptrComponent("proj"), Component: ptrComponent("comp")},
				Limit:       intPtr(50),
				SortOrder:   (*gen.TracesQueryRequestSortOrder)(strPtr("asc")),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTracesQueryRequest(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateAlertsQueryRequest(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	start := now.Add(-1 * time.Hour)
	end := now

	ptrComponent := func(s string) *string { return &s }

	tests := []struct {
		name        string
		req         *gen.AlertsQueryRequest
		wantErr     bool
		errContains string
	}{
		{name: "nil request", req: nil, wantErr: true, errContains: "required"},
		{
			name:        "zero startTime",
			req:         &gen.AlertsQueryRequest{EndTime: end, SearchScope: gen.ComponentSearchScope{Namespace: "ns"}},
			wantErr:     true,
			errContains: "startTime is required",
		},
		{
			name:        "zero endTime",
			req:         &gen.AlertsQueryRequest{StartTime: start, SearchScope: gen.ComponentSearchScope{Namespace: "ns"}},
			wantErr:     true,
			errContains: "endTime is required",
		},
		{
			name:        "missing namespace",
			req:         &gen.AlertsQueryRequest{StartTime: start, EndTime: end},
			wantErr:     true,
			errContains: "namespace is required",
		},
		{
			name:        "whitespace namespace",
			req:         &gen.AlertsQueryRequest{StartTime: start, EndTime: end, SearchScope: gen.ComponentSearchScope{Namespace: "   "}},
			wantErr:     true,
			errContains: "namespace is required",
		},
		{
			name: "component without project",
			req: &gen.AlertsQueryRequest{
				StartTime: start, EndTime: end,
				SearchScope: gen.ComponentSearchScope{Namespace: "ns", Component: ptrComponent("comp")},
			},
			wantErr:     true,
			errContains: "project is required when",
		},
		{
			name: "invalid limit",
			req: &gen.AlertsQueryRequest{
				StartTime: start, EndTime: end,
				SearchScope: gen.ComponentSearchScope{Namespace: "ns"},
				Limit:       intPtr(-1),
			},
			wantErr:     true,
			errContains: "positive integer",
		},
		{
			name: "invalid sort order",
			req: &gen.AlertsQueryRequest{
				StartTime: start, EndTime: end,
				SearchScope: gen.ComponentSearchScope{Namespace: "ns"},
				SortOrder:   (*gen.AlertsQueryRequestSortOrder)(strPtr("RANDOM")),
			},
			wantErr:     true,
			errContains: "sortOrder must be either",
		},
		{
			name: "valid minimal",
			req: &gen.AlertsQueryRequest{
				StartTime: start, EndTime: end,
				SearchScope: gen.ComponentSearchScope{Namespace: "ns"},
			},
			wantErr: false,
		},
		{
			name: "valid with project and component",
			req: &gen.AlertsQueryRequest{
				StartTime: start, EndTime: end,
				SearchScope: gen.ComponentSearchScope{Namespace: "ns", Project: ptrComponent("proj"), Component: ptrComponent("comp")},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateAlertsQueryRequest(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateIncidentsQueryRequest(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	start := now.Add(-1 * time.Hour)
	end := now

	// Mirror alerts — identical validation logic.
	tests := []struct {
		name        string
		req         *gen.IncidentsQueryRequest
		wantErr     bool
		errContains string
	}{
		{name: "nil request", req: nil, wantErr: true, errContains: "required"},
		{
			name:        "zero startTime",
			req:         &gen.IncidentsQueryRequest{EndTime: end, SearchScope: gen.ComponentSearchScope{Namespace: "ns"}},
			wantErr:     true,
			errContains: "startTime is required",
		},
		{
			name:        "missing namespace",
			req:         &gen.IncidentsQueryRequest{StartTime: start, EndTime: end},
			wantErr:     true,
			errContains: "namespace is required",
		},
		{
			name: "valid minimal",
			req: &gen.IncidentsQueryRequest{
				StartTime: start, EndTime: end,
				SearchScope: gen.ComponentSearchScope{Namespace: "ns"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateIncidentsQueryRequest(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateIncidentPutRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		req         *gen.IncidentPutRequest
		wantErr     bool
		errContains string
	}{
		{name: "nil request", req: nil, wantErr: true, errContains: "required"},
		{
			name:        "empty status",
			req:         &gen.IncidentPutRequest{Status: ""},
			wantErr:     true,
			errContains: "status is required",
		},
		{
			name:        "invalid status",
			req:         &gen.IncidentPutRequest{Status: "pending"},
			wantErr:     true,
			errContains: "status must be one of",
		},
		{
			name:    "status active",
			req:     &gen.IncidentPutRequest{Status: gen.IncidentPutRequestStatusActive},
			wantErr: false,
		},
		{
			name:    "status acknowledged",
			req:     &gen.IncidentPutRequest{Status: gen.IncidentPutRequestStatusAcknowledged},
			wantErr: false,
		},
		{
			name:    "status resolved",
			req:     &gen.IncidentPutRequest{Status: gen.IncidentPutRequestStatusResolved},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateIncidentPutRequest(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateAlertRuleRequest(t *testing.T) {
	t.Parallel()

	uid := nonZeroUUID
	logQuery := "ERROR"
	cpuMetric := gen.AlertRuleRequestSourceMetric("cpu_usage")
	memMetric := gen.AlertRuleRequestSourceMetric("memory_usage")
	invalidMetric := gen.AlertRuleRequestSourceMetric("disk_usage")

	// Helper: build a minimal valid log-based rule and let each test mutate a field.
	baseLogRule := func() gen.AlertRuleRequest {
		req := gen.AlertRuleRequest{}
		req.Metadata.Name = "test-rule"
		req.Metadata.ComponentUid = uid
		req.Metadata.ProjectUid = uid
		req.Metadata.EnvironmentUid = uid
		req.Source.Type = "log"
		req.Source.Query = &logQuery
		req.Condition.Window = "5m"
		req.Condition.Interval = "1m"
		req.Condition.Operator = "gt"
		req.Condition.Threshold = 1
		return req
	}

	tests := []struct {
		name        string
		req         func() gen.AlertRuleRequest
		wantErr     bool
		errContains string
	}{
		{
			name:        "missing name",
			req:         func() gen.AlertRuleRequest { r := baseLogRule(); r.Metadata.Name = ""; return r },
			wantErr:     true,
			errContains: "metadata.name is required",
		},
		{
			name: "missing componentUid",
			req: func() gen.AlertRuleRequest {
				r := baseLogRule()
				r.Metadata.ComponentUid = openapi_types.UUID{}
				return r
			},
			wantErr:     true,
			errContains: "metadata.componentUid is required",
		},
		{
			name: "missing projectUid",
			req: func() gen.AlertRuleRequest {
				r := baseLogRule()
				r.Metadata.ProjectUid = openapi_types.UUID{}
				return r
			},
			wantErr:     true,
			errContains: "metadata.projectUid is required",
		},
		{
			name: "missing environmentUid",
			req: func() gen.AlertRuleRequest {
				r := baseLogRule()
				r.Metadata.EnvironmentUid = openapi_types.UUID{}
				return r
			},
			wantErr:     true,
			errContains: "metadata.environmentUid is required",
		},
		{
			name:        "invalid source type",
			req:         func() gen.AlertRuleRequest { r := baseLogRule(); r.Source.Type = "trace"; return r },
			wantErr:     true,
			errContains: "source.type must be",
		},
		{
			name:        "log rule missing query",
			req:         func() gen.AlertRuleRequest { r := baseLogRule(); r.Source.Query = nil; return r },
			wantErr:     true,
			errContains: "source.query is required",
		},
		{
			name: "metric rule missing metric field",
			req: func() gen.AlertRuleRequest {
				r := baseLogRule()
				r.Source.Type = sourceTypeMetric
				r.Source.Query = nil
				r.Source.Metric = nil
				return r
			},
			wantErr:     true,
			errContains: "source.metric is required",
		},
		{
			name: "metric rule invalid metric value",
			req: func() gen.AlertRuleRequest {
				r := baseLogRule()
				r.Source.Type = sourceTypeMetric
				r.Source.Query = nil
				r.Source.Metric = &invalidMetric
				return r
			},
			wantErr:     true,
			errContains: "source.metric must be",
		},
		{
			name:        "invalid window duration",
			req:         func() gen.AlertRuleRequest { r := baseLogRule(); r.Condition.Window = "not-duration"; return r },
			wantErr:     true,
			errContains: "condition.window must be a valid duration",
		},
		{
			name:        "zero window duration",
			req:         func() gen.AlertRuleRequest { r := baseLogRule(); r.Condition.Window = "0s"; return r },
			wantErr:     true,
			errContains: "condition.window must be greater than zero",
		},
		{
			name:        "invalid interval duration",
			req:         func() gen.AlertRuleRequest { r := baseLogRule(); r.Condition.Interval = "bad"; return r },
			wantErr:     true,
			errContains: "condition.interval must be a valid duration",
		},
		{
			name:        "interval exceeds window",
			req:         func() gen.AlertRuleRequest { r := baseLogRule(); r.Condition.Interval = "10m"; return r },
			wantErr:     true,
			errContains: "interval must not exceed",
		},
		{
			name:        "invalid operator",
			req:         func() gen.AlertRuleRequest { r := baseLogRule(); r.Condition.Operator = "between"; return r },
			wantErr:     true,
			errContains: "condition.operator must be one of",
		},
		{
			name:        "threshold not positive",
			req:         func() gen.AlertRuleRequest { r := baseLogRule(); r.Condition.Threshold = 0; return r },
			wantErr:     true,
			errContains: "condition.threshold must be greater than zero",
		},
		{name: "valid log rule", req: baseLogRule, wantErr: false},
		{
			name: "valid metric rule cpu_usage",
			req: func() gen.AlertRuleRequest {
				r := baseLogRule()
				r.Source.Type = sourceTypeMetric
				r.Source.Query = nil
				r.Source.Metric = &cpuMetric
				return r
			},
			wantErr: false,
		},
		{
			name: "valid metric rule memory_usage",
			req: func() gen.AlertRuleRequest {
				r := baseLogRule()
				r.Source.Type = sourceTypeMetric
				r.Source.Query = nil
				r.Source.Metric = &memMetric
				return r
			},
			wantErr: false,
		},
		{
			name: "all valid operators",
			req: func() gen.AlertRuleRequest {
				r := baseLogRule()
				r.Condition.Operator = "gte"
				return r
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateAlertRuleRequest(tt.req())
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateSourceType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sourceType string
		wantErr    bool
	}{
		{name: "log is valid", sourceType: "log", wantErr: false},
		{name: "metric is valid", sourceType: sourceTypeMetric, wantErr: false},
		{name: "trace is invalid", sourceType: "trace", wantErr: true},
		{name: "empty is invalid", sourceType: "", wantErr: true},
		{name: "Log is invalid (case sensitive)", sourceType: "Log", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateSourceType(tt.sourceType)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateRuntimeTopologyRequest(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	validStart := now.Add(-1 * time.Hour).Format(time.RFC3339)
	validEnd := now.Format(time.RFC3339)

	tests := []struct {
		name        string
		req         *types.RuntimeTopologyRequest
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil request",
			req:         nil,
			wantErr:     true,
			errContains: "request must not be nil",
		},
		{
			name: "missing namespace",
			req: &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Project:     "proj",
					Environment: "env",
				},
				StartTime: validStart,
				EndTime:   validEnd,
			},
			wantErr:     true,
			errContains: "searchScope.namespace is required",
		},
		{
			name: "whitespace-only namespace",
			req: &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Namespace:   "   ",
					Project:     "proj",
					Environment: "env",
				},
				StartTime: validStart,
				EndTime:   validEnd,
			},
			wantErr:     true,
			errContains: "searchScope.namespace is required",
		},
		{
			name: "missing project",
			req: &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Namespace:   "ns",
					Environment: "env",
				},
				StartTime: validStart,
				EndTime:   validEnd,
			},
			wantErr:     true,
			errContains: "searchScope.project is required",
		},
		{
			name: "missing environment",
			req: &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Namespace: "ns",
					Project:   "proj",
				},
				StartTime: validStart,
				EndTime:   validEnd,
			},
			wantErr:     true,
			errContains: "searchScope.environment is required",
		},
		{
			name: "invalid time range",
			req: &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Namespace:   "ns",
					Project:     "proj",
					Environment: "env",
				},
				StartTime: validEnd,
				EndTime:   validStart,
			},
			wantErr:     true,
			errContains: "endTime must be after startTime",
		},
		{
			name: "valid request",
			req: &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Namespace:   "ns",
					Project:     "proj",
					Environment: "env",
				},
				StartTime: validStart,
				EndTime:   validEnd,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRuntimeTopologyRequest(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// strPtr is a helper to get a pointer to a string literal. Used in validations tests.
func strPtr(s string) *string { return &s }

// intPtr is a helper to get a pointer to an int literal. Used in validations tests.
func intPtr(i int) *int { return &i }
