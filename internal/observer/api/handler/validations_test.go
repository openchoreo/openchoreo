// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"testing"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

func TestValidateLogsQueryRequest(t *testing.T) {
	validComponentReq := func() *types.LogsQueryRequest {
		return &types.LogsQueryRequest{
			SearchScope: &types.SearchScope{
				Component: &types.ComponentSearchScope{
					Namespace: "ns1",
				},
			},
			StartTime: "2024-01-01T00:00:00Z",
			EndTime:   "2024-01-02T00:00:00Z",
			Limit:     100,
			SortOrder: "desc",
		}
	}

	tests := []struct {
		name    string
		req     *types.LogsQueryRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid component request",
			req:     validComponentReq(),
			wantErr: false,
		},
		{
			name: "valid workflow request",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Workflow: &types.WorkflowSearchScope{
						Namespace: "ns1",
					},
				},
				StartTime: "2024-01-01T00:00:00Z",
				EndTime:   "2024-01-02T00:00:00Z",
				Limit:     50,
				SortOrder: "asc",
			},
			wantErr: false,
		},
		{
			name: "nil search scope",
			req: &types.LogsQueryRequest{
				SearchScope: nil,
				StartTime:   "2024-01-01T00:00:00Z",
				EndTime:     "2024-01-02T00:00:00Z",
			},
			wantErr: true,
			errMsg:  "searchScope is required",
		},
		{
			name: "both component and workflow set",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Component: &types.ComponentSearchScope{Namespace: "ns1"},
					Workflow:  &types.WorkflowSearchScope{Namespace: "ns1"},
				},
				StartTime: "2024-01-01T00:00:00Z",
				EndTime:   "2024-01-02T00:00:00Z",
			},
			wantErr: true,
		},
		{
			name: "neither component nor workflow set",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{},
				StartTime:   "2024-01-01T00:00:00Z",
				EndTime:     "2024-01-02T00:00:00Z",
			},
			wantErr: true,
		},
		{
			name: "component with namespace missing",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Component: &types.ComponentSearchScope{Namespace: ""},
				},
				StartTime: "2024-01-01T00:00:00Z",
				EndTime:   "2024-01-02T00:00:00Z",
			},
			wantErr: true,
			errMsg:  "searchScope.namespace is required",
		},
		{
			name: "component with component set but no project",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Component: &types.ComponentSearchScope{
						Namespace: "ns1",
						Component: "comp1",
						// Project intentionally missing
					},
				},
				StartTime: "2024-01-01T00:00:00Z",
				EndTime:   "2024-01-02T00:00:00Z",
			},
			wantErr: true,
			errMsg:  "searchScope.project is required when searchScope.component is provided",
		},
		{
			name: "workflow with namespace missing",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Workflow: &types.WorkflowSearchScope{Namespace: ""},
				},
				StartTime: "2024-01-01T00:00:00Z",
				EndTime:   "2024-01-02T00:00:00Z",
			},
			wantErr: true,
			errMsg:  "searchScope.namespace is required",
		},
		{
			name: "missing start time",
			req: func() *types.LogsQueryRequest {
				r := validComponentReq()
				r.StartTime = ""
				return r
			}(),
			wantErr: true,
			errMsg:  "startTime is required",
		},
		{
			name: "missing end time",
			req: func() *types.LogsQueryRequest {
				r := validComponentReq()
				r.EndTime = ""
				return r
			}(),
			wantErr: true,
			errMsg:  "endTime is required",
		},
		{
			name: "end time before start time",
			req: func() *types.LogsQueryRequest {
				r := validComponentReq()
				r.StartTime = "2024-01-02T00:00:00Z"
				r.EndTime = "2024-01-01T00:00:00Z"
				return r
			}(),
			wantErr: true,
		},
		{
			name: "negative limit",
			req: func() *types.LogsQueryRequest {
				r := validComponentReq()
				r.Limit = -1
				return r
			}(),
			wantErr: true,
		},
		{
			name: "limit exceeds max",
			req: func() *types.LogsQueryRequest {
				r := validComponentReq()
				r.Limit = 99999
				return r
			}(),
			wantErr: true,
		},
		{
			name: "invalid sort order",
			req: func() *types.LogsQueryRequest {
				r := validComponentReq()
				r.SortOrder = "invalid"
				return r
			}(),
			wantErr: true,
		},
		{
			name: "invalid log level",
			req: func() *types.LogsQueryRequest {
				r := validComponentReq()
				r.LogLevels = []string{"TRACE"}
				return r
			}(),
			wantErr: true,
		},
		{
			name: "zero limit gets set to default",
			req: func() *types.LogsQueryRequest {
				r := validComponentReq()
				r.Limit = 0
				return r
			}(),
			wantErr: false,
		},
		{
			name: "empty sort order gets set to default",
			req: func() *types.LogsQueryRequest {
				r := validComponentReq()
				r.SortOrder = ""
				return r
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogsQueryRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLogsQueryRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if err.Error() != tt.errMsg {
					t.Errorf("ValidateLogsQueryRequest() error msg = %q, want %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestValidateTimeRange(t *testing.T) {
	tests := []struct {
		name      string
		startTime string
		endTime   string
		wantErr   bool
	}{
		{
			name:      "valid range",
			startTime: "2024-01-01T00:00:00Z",
			endTime:   "2024-01-02T00:00:00Z",
			wantErr:   false,
		},
		{
			name:      "same time - start equals end",
			startTime: "2024-01-01T00:00:00Z",
			endTime:   "2024-01-01T00:00:00Z",
			wantErr:   false,
		},
		{
			name:      "empty start time",
			startTime: "",
			endTime:   "2024-01-02T00:00:00Z",
			wantErr:   true,
		},
		{
			name:      "empty end time",
			startTime: "2024-01-01T00:00:00Z",
			endTime:   "",
			wantErr:   true,
		},
		{
			name:      "invalid start time format",
			startTime: "2024-01-01",
			endTime:   "2024-01-02T00:00:00Z",
			wantErr:   true,
		},
		{
			name:      "invalid end time format",
			startTime: "2024-01-01T00:00:00Z",
			endTime:   "not-a-time",
			wantErr:   true,
		},
		{
			name:      "end before start",
			startTime: "2024-01-02T00:00:00Z",
			endTime:   "2024-01-01T00:00:00Z",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimeRange(tt.startTime, tt.endTime)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTimeRange() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAndSetLimit(t *testing.T) {
	tests := []struct {
		name       string
		input      int
		wantErr    bool
		wantResult int
	}{
		{
			name:       "zero sets default",
			input:      0,
			wantErr:    false,
			wantResult: defaultLimit,
		},
		{
			name:       "valid positive limit",
			input:      500,
			wantErr:    false,
			wantResult: 500,
		},
		{
			name:       "max limit is valid",
			input:      maxLimit,
			wantErr:    false,
			wantResult: maxLimit,
		},
		{
			name:    "negative limit",
			input:   -1,
			wantErr: true,
		},
		{
			name:    "exceeds max",
			input:   maxLimit + 1,
			wantErr: true,
		},
		{
			name:       "limit of 1",
			input:      1,
			wantErr:    false,
			wantResult: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit := tt.input
			err := ValidateAndSetLimit(&limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAndSetLimit() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && limit != tt.wantResult {
				t.Errorf("ValidateAndSetLimit() limit = %d, want %d", limit, tt.wantResult)
			}
		})
	}
}

func TestValidateAndSetSortOrder(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantErr    bool
		wantResult string
	}{
		{
			name:       "empty sets default",
			input:      "",
			wantErr:    false,
			wantResult: defaultSortOrder,
		},
		{
			name:       "asc is valid",
			input:      "asc",
			wantErr:    false,
			wantResult: "asc",
		},
		{
			name:       "desc is valid",
			input:      "desc",
			wantErr:    false,
			wantResult: "desc",
		},
		{
			name:    "invalid value",
			input:   "random",
			wantErr: true,
		},
		{
			name:    "uppercase invalid",
			input:   "ASC",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := tt.input
			err := ValidateAndSetSortOrder(&order)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAndSetSortOrder() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && order != tt.wantResult {
				t.Errorf("ValidateAndSetSortOrder() order = %q, want %q", order, tt.wantResult)
			}
		})
	}
}

func TestValidateLogLevels(t *testing.T) {
	tests := []struct {
		name      string
		logLevels []string
		wantErr   bool
	}{
		{
			name:      "nil levels is valid",
			logLevels: nil,
			wantErr:   false,
		},
		{
			name:      "empty slice is valid",
			logLevels: []string{},
			wantErr:   false,
		},
		{
			name:      "DEBUG is valid",
			logLevels: []string{"DEBUG"},
			wantErr:   false,
		},
		{
			name:      "INFO is valid",
			logLevels: []string{"INFO"},
			wantErr:   false,
		},
		{
			name:      "WARN is valid",
			logLevels: []string{"WARN"},
			wantErr:   false,
		},
		{
			name:      "ERROR is valid",
			logLevels: []string{"ERROR"},
			wantErr:   false,
		},
		{
			name:      "multiple valid levels",
			logLevels: []string{"DEBUG", "INFO", "WARN", "ERROR"},
			wantErr:   false,
		},
		{
			name:      "TRACE is invalid",
			logLevels: []string{"TRACE"},
			wantErr:   true,
		},
		{
			name:      "lowercase invalid",
			logLevels: []string{"info"},
			wantErr:   true,
		},
		{
			name:      "mix of valid and invalid",
			logLevels: []string{"INFO", "FATAL"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogLevels(tt.logLevels)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLogLevels() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLogsQueryRequest_DefaultsAreSet(t *testing.T) {
	req := &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{
			Component: &types.ComponentSearchScope{Namespace: "ns1"},
		},
		StartTime: "2024-01-01T00:00:00Z",
		EndTime:   "2024-01-02T00:00:00Z",
		Limit:     0,  // should be set to defaultLimit
		SortOrder: "", // should be set to defaultSortOrder
	}

	if err := ValidateLogsQueryRequest(req); err != nil {
		t.Fatalf("ValidateLogsQueryRequest() unexpected error: %v", err)
	}

	if req.Limit != defaultLimit {
		t.Errorf("Expected Limit to be set to %d, got %d", defaultLimit, req.Limit)
	}
	if req.SortOrder != defaultSortOrder {
		t.Errorf("Expected SortOrder to be set to %q, got %q", defaultSortOrder, req.SortOrder)
	}
}
