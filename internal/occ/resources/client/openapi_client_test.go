// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"testing"
)

func TestApiError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       []byte
		wantMsg    string
	}{
		{
			name:       "structured error with message",
			statusCode: 404,
			body:       []byte(`{"code":"NOT_FOUND","error":"component \"foo\" not found"}`),
			wantMsg:    `component "foo" not found`,
		},
		{
			name:       "structured error with details",
			statusCode: 400,
			body:       []byte(`{"code":"VALIDATION_ERROR","error":"validation failed","details":[{"field":"name","message":"must not be empty"}]}`),
			wantMsg:    `validation failed; name: must not be empty`,
		},
		{
			name:       "structured error with multiple details",
			statusCode: 400,
			body:       []byte(`{"code":"VALIDATION_ERROR","error":"validation failed","details":[{"field":"name","message":"must not be empty"},{"field":"project","message":"is required"}]}`),
			wantMsg:    `validation failed; name: must not be empty; project: is required`,
		},
		{
			name:       "non-JSON body",
			statusCode: 502,
			body:       []byte(`Bad Gateway`),
			wantMsg:    `unexpected response (HTTP 502): Bad Gateway`,
		},
		{
			name:       "empty body",
			statusCode: 500,
			body:       nil,
			wantMsg:    `unexpected response status: 500`,
		},
		{
			name:       "JSON without error field",
			statusCode: 503,
			body:       []byte(`{"status":"unavailable"}`),
			wantMsg:    `unexpected response (HTTP 503): {"status":"unavailable"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := apiError(tt.statusCode, tt.body)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); got != tt.wantMsg {
				t.Errorf("apiError() = %q, want %q", got, tt.wantMsg)
			}
		})
	}
}
