// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"testing"
)

func TestValidateLimit(t *testing.T) {
	tests := []struct {
		name    string
		limit   int
		wantErr bool
		errMsg  string
	}{
		{
			name:    "Valid limit - 1",
			limit:   1,
			wantErr: false,
		},
		{
			name:    "Valid limit - 100",
			limit:   100,
			wantErr: false,
		},
		{
			name:    "Valid limit - 10000 (max allowed)",
			limit:   10000,
			wantErr: false,
		},
		{
			name:    "Invalid limit - zero",
			limit:   0,
			wantErr: true,
			errMsg:  "limit must be a positive integer",
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
			err := validateLimit(tt.limit)

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
			}
		})
	}
}

func TestValidateSortOrder(t *testing.T) {
	tests := []struct {
		name      string
		sortOrder string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "Valid sort order - asc",
			sortOrder: "asc",
			wantErr:   false,
		},
		{
			name:      "Valid sort order - desc",
			sortOrder: "desc",
			wantErr:   false,
		},
		{
			name:      "Invalid sort order - empty string",
			sortOrder: "",
			wantErr:   true,
			errMsg:    "sortOrder must be either 'asc' or 'desc'",
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
			sortOrder: "invalid",
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
			err := validateSortOrder(tt.sortOrder)

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
