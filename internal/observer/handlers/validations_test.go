// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"testing"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

const testInvalidValue = "invalid"

func TestValidateLimit(t *testing.T) {
	tests := []struct {
		name          string
		limit         int
		wantErr       bool
		errMsg        string
		expectedLimit int
	}{
		{
			name:          "Valid limit - 1",
			limit:         1,
			wantErr:       false,
			expectedLimit: 1,
		},
		{
			name:          "Valid limit - 100",
			limit:         100,
			wantErr:       false,
			expectedLimit: 100,
		},
		{
			name:          "Valid limit - 10000 (max allowed)",
			limit:         10000,
			wantErr:       false,
			expectedLimit: 10000,
		},
		{
			name:          "Zero limit - sets default to 100",
			limit:         0,
			wantErr:       false,
			expectedLimit: 100,
		},
		{
			name:    "Invalid limit - negative",
			limit:   -1,
			wantErr: true,
			errMsg:  "limit must be a positive integer",
		},
		{
			name:    "Invalid limit - exceeds maximum",
			limit:   10001,
			wantErr: true,
			errMsg:  "limit cannot exceed 10000. If you need to fetch more logs, please use pagination",
		},
		{
			name:    "Invalid limit - very large number",
			limit:   50000,
			wantErr: true,
			errMsg:  "limit cannot exceed 10000. If you need to fetch more logs, please use pagination",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit := tt.limit
			err := validateLimit(&limit)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateLimit() expected error but got none")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("validateLimit() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateLimit() unexpected error = %v", err)
				}
				if limit != tt.expectedLimit {
					t.Errorf("validateLimit() limit = %v, want %v", limit, tt.expectedLimit)
				}
			}
		})
	}
}

func TestValidateSortOrder(t *testing.T) {
	tests := []struct {
		name              string
		sortOrder         string
		wantErr           bool
		errMsg            string
		expectedSortOrder string // expected value after validation
	}{
		{
			name:              "Valid sort order - asc",
			sortOrder:         "asc",
			wantErr:           false,
			expectedSortOrder: "asc",
		},
		{
			name:              "Valid sort order - desc",
			sortOrder:         "desc",
			wantErr:           false,
			expectedSortOrder: "desc",
		},
		{
			name:              "Empty sort order - sets default to desc",
			sortOrder:         "",
			wantErr:           false,
			expectedSortOrder: "desc",
		},
		{
			name:      "Invalid sort order - ASC (uppercase)",
			sortOrder: "ASC",
			wantErr:   true,
			errMsg:    "sortOrder must be either 'asc' or 'desc'",
		},
		{
			name:      "Invalid sort order - DESC (uppercase)",
			sortOrder: "DESC",
			wantErr:   true,
			errMsg:    "sortOrder must be either 'asc' or 'desc'",
		},
		{
			name:      "Invalid sort order - ascending",
			sortOrder: "ascending",
			wantErr:   true,
			errMsg:    "sortOrder must be either 'asc' or 'desc'",
		},
		{
			name:      "Invalid sort order - random string",
			sortOrder: testInvalidValue,
			wantErr:   true,
			errMsg:    "sortOrder must be either 'asc' or 'desc'",
		},
		{
			name:      "Invalid sort order - mixed case",
			sortOrder: "Asc",
			wantErr:   true,
			errMsg:    "sortOrder must be either 'asc' or 'desc'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortOrder := tt.sortOrder
			err := validateSortOrder(&sortOrder)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateSortOrder() expected error but got none")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("validateSortOrder() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateSortOrder() unexpected error = %v", err)
				}
				if sortOrder != tt.expectedSortOrder {
					t.Errorf("validateSortOrder() sortOrder = %v, want %v", sortOrder, tt.expectedSortOrder)
				}
			}
		})
	}
}

func TestValidateTimes(t *testing.T) {
	tests := []struct {
		name      string
		startTime string
		endTime   string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "Valid times - basic RFC3339",
			startTime: "2024-01-01T00:00:00Z",
			endTime:   "2024-01-01T01:00:00Z",
			wantErr:   false,
		},
		{
			name:      "Valid times - with timezone",
			startTime: "2024-01-01T10:00:00+05:30",
			endTime:   "2024-01-01T12:00:00+05:30",
			wantErr:   false,
		},
		{
			name:      "Valid times - different dates",
			startTime: "2024-01-01T23:00:00Z",
			endTime:   "2024-01-02T01:00:00Z",
			wantErr:   false,
		},
		{
			name:      "Valid times - same time (edge case)",
			startTime: "2024-01-01T12:00:00Z",
			endTime:   "2024-01-01T12:00:00Z",
			wantErr:   false,
		},
		{
			name:      "Empty startTime",
			startTime: "",
			endTime:   "2024-01-01T01:00:00Z",
			wantErr:   true,
			errMsg:    "Required field startTime not found",
		},
		{
			name:      "Empty endTime",
			startTime: "2024-01-01T00:00:00Z",
			endTime:   "",
			wantErr:   true,
			errMsg:    "Required field endTime not found",
		},
		{
			name:      "Both times empty",
			startTime: "",
			endTime:   "",
			wantErr:   true,
			errMsg:    "Required field startTime not found",
		},
		{
			name:      "Invalid startTime format - no timezone",
			startTime: "2024-01-01T00:00:00",
			endTime:   "2024-01-01T01:00:00Z",
			wantErr:   true,
			errMsg:    "startTime must be in RFC3339 format (e.g., 2024-01-01T00:00:00Z): parsing time \"2024-01-01T00:00:00\" as \"2006-01-02T15:04:05Z07:00\": cannot parse \"\" as \"Z07:00\"",
		},
		{
			name:      "Invalid endTime format - no timezone",
			startTime: "2024-01-01T00:00:00Z",
			endTime:   "2024-01-01T01:00:00",
			wantErr:   true,
			errMsg:    "endTime must be in RFC3339 format (e.g., 2024-01-01T00:00:00Z): parsing time \"2024-01-01T01:00:00\" as \"2006-01-02T15:04:05Z07:00\": cannot parse \"\" as \"Z07:00\"",
		},
		{
			name:      "Invalid startTime format - wrong date format",
			startTime: "01-01-2024 00:00:00",
			endTime:   "2024-01-01T01:00:00Z",
			wantErr:   true,
			errMsg:    "startTime must be in RFC3339 format (e.g., 2024-01-01T00:00:00Z): parsing time \"01-01-2024 00:00:00\" as \"2006-01-02T15:04:05Z07:00\": cannot parse \"01-01-2024 00:00:00\" as \"2006\"",
		},
		{
			name:      "Invalid endTime format - wrong date format",
			startTime: "2024-01-01T00:00:00Z",
			endTime:   "01-01-2024 01:00:00",
			wantErr:   true,
			errMsg:    "endTime must be in RFC3339 format (e.g., 2024-01-01T00:00:00Z): parsing time \"01-01-2024 01:00:00\" as \"2006-01-02T15:04:05Z07:00\": cannot parse \"01-01-2024 01:00:00\" as \"2006\"",
		},
		{
			name:      "Invalid startTime format - malformed",
			startTime: "invalid-time",
			endTime:   "2024-01-01T01:00:00Z",
			wantErr:   true,
			errMsg:    "startTime must be in RFC3339 format (e.g., 2024-01-01T00:00:00Z): parsing time \"invalid-time\" as \"2006-01-02T15:04:05Z07:00\": cannot parse \"invalid-time\" as \"2006\"",
		},
		{
			name:      "Invalid endTime format - malformed",
			startTime: "2024-01-01T00:00:00Z",
			endTime:   "invalid-time",
			wantErr:   true,
			errMsg:    "endTime must be in RFC3339 format (e.g., 2024-01-01T00:00:00Z): parsing time \"invalid-time\" as \"2006-01-02T15:04:05Z07:00\": cannot parse \"invalid-time\" as \"2006\"",
		},
		{
			name:      "EndTime before startTime",
			startTime: "2024-01-01T02:00:00Z",
			endTime:   "2024-01-01T01:00:00Z",
			wantErr:   true,
			errMsg:    "endTime (2024-01-01 01:00:00 +0000 UTC) must be after startTime (2024-01-01 02:00:00 +0000 UTC)",
		},
		{
			name:      "EndTime significantly before startTime",
			startTime: "2024-01-02T00:00:00Z",
			endTime:   "2024-01-01T00:00:00Z",
			wantErr:   true,
			errMsg:    "endTime (2024-01-01 00:00:00 +0000 UTC) must be after startTime (2024-01-02 00:00:00 +0000 UTC)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTimes(tt.startTime, tt.endTime)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateTimes() expected error but got none")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("validateTimes() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateTimes() unexpected error = %v", err)
				}
			}
		})
	}
}

// ---- validateComponentUIDs ----

func TestValidateComponentUIDs(t *testing.T) {
	tests := []struct {
		name    string
		uids    []string
		wantErr bool
	}{
		{"nil slice", nil, false},
		{"empty slice", []string{}, false},
		{"valid single uid", []string{"abc123"}, false},
		{"valid multiple uids", []string{"uid-1", "uid-2"}, false},
		{"empty string in slice", []string{"valid", ""}, true},
		{"all empty strings", []string{"", ""}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateComponentUIDs(tt.uids)
			if tt.wantErr && err == nil {
				t.Error("validateComponentUIDs() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateComponentUIDs() unexpected error: %v", err)
			}
		})
	}
}

// ---- validateTraceID ----

func TestValidateTraceID(t *testing.T) {
	tests := []struct {
		name    string
		traceID string
		wantErr bool
	}{
		{"empty", "", false},
		{"valid hex lowercase", "abcdef0123456789", false},
		{"valid hex uppercase", "ABCDEF0123456789", false},
		{"with wildcard star", "abc*", false},
		{"with wildcard question", "abc?ef", false},
		{"invalid char g", "abcdefg", true},
		{"invalid char space", "abc def", true},
		{"invalid char colon", "abc:def", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTraceID(tt.traceID)
			if tt.wantErr && err == nil {
				t.Error("validateTraceID() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateTraceID() unexpected error: %v", err)
			}
		})
	}
}

// ---- validateAlertingRule ----

func makeValidAlertingRule() types.AlertingRuleRequest {
	return types.AlertingRuleRequest{
		Metadata: types.AlertingRuleMetadata{
			Name:           "my-alert",
			ComponentUID:   "comp-uid",
			ProjectUID:     "proj-uid",
			EnvironmentUID: "env-uid",
			Severity:       "critical",
		},
		Source: types.AlertingRuleSource{
			Type:  "log",
			Query: "level=error",
		},
		Condition: types.AlertingRuleCondition{
			Window:    "5m",
			Interval:  "1m",
			Operator:  "gt",
			Threshold: 10,
		},
	}
}

func TestValidateAlertingRule_Valid(t *testing.T) {
	req := makeValidAlertingRule()
	if err := validateAlertingRule(req); err != nil {
		t.Errorf("validateAlertingRule(valid) = %v, want nil", err)
	}
}

func TestValidateAlertingRule_MissingMetadata(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*types.AlertingRuleRequest)
	}{
		{"missing name", func(r *types.AlertingRuleRequest) { r.Metadata.Name = "" }},
		{"missing componentUID", func(r *types.AlertingRuleRequest) { r.Metadata.ComponentUID = "" }},
		{"missing projectUID", func(r *types.AlertingRuleRequest) { r.Metadata.ProjectUID = "" }},
		{"missing environmentUID", func(r *types.AlertingRuleRequest) { r.Metadata.EnvironmentUID = "" }},
		{"missing severity", func(r *types.AlertingRuleRequest) { r.Metadata.Severity = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := makeValidAlertingRule()
			tt.modify(&req)
			if err := validateAlertingRule(req); err == nil {
				t.Errorf("validateAlertingRule(%s) expected error, got nil", tt.name)
			}
		})
	}
}

func TestValidateAlertingRule_InvalidSourceType(t *testing.T) {
	req := makeValidAlertingRule()
	req.Source.Type = testInvalidValue
	if err := validateAlertingRule(req); err == nil {
		t.Error("validateAlertingRule(invalid source type) expected error, got nil")
	}
}

func TestValidateAlertingRule_LogSourceMissingQuery(t *testing.T) {
	req := makeValidAlertingRule()
	req.Source.Type = "log"
	req.Source.Query = ""
	if err := validateAlertingRule(req); err == nil {
		t.Error("validateAlertingRule(log without query) expected error, got nil")
	}
}

func TestValidateAlertingRule_MetricSource(t *testing.T) {
	req := makeValidAlertingRule()
	req.Source.Type = "metric"
	req.Source.Query = ""
	req.Source.Metric = "cpu_usage"
	if err := validateAlertingRule(req); err != nil {
		t.Errorf("validateAlertingRule(metric/cpu_usage) = %v, want nil", err)
	}
}

func TestValidateAlertingRule_MetricSourceInvalidMetric(t *testing.T) {
	req := makeValidAlertingRule()
	req.Source.Type = "metric"
	req.Source.Metric = "bad_metric"
	if err := validateAlertingRule(req); err == nil {
		t.Error("validateAlertingRule(metric/bad_metric) expected error, got nil")
	}
}

func TestValidateAlertingRule_InvalidCondition(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*types.AlertingRuleRequest)
	}{
		{"invalid window", func(r *types.AlertingRuleRequest) { r.Condition.Window = testInvalidValue }},
		{"invalid interval", func(r *types.AlertingRuleRequest) { r.Condition.Interval = testInvalidValue }},
		{"interval > window", func(r *types.AlertingRuleRequest) {
			r.Condition.Window = "1m"
			r.Condition.Interval = "5m"
		}},
		{"invalid operator", func(r *types.AlertingRuleRequest) { r.Condition.Operator = "bad" }},
		{"zero threshold", func(r *types.AlertingRuleRequest) { r.Condition.Threshold = 0 }},
		{"negative threshold", func(r *types.AlertingRuleRequest) { r.Condition.Threshold = -1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := makeValidAlertingRule()
			tt.modify(&req)
			if err := validateAlertingRule(req); err == nil {
				t.Errorf("validateAlertingRule(%s) expected error, got nil", tt.name)
			}
		})
	}
}
