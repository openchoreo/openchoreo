// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	logger := slog.Default()
	healthService := service.NewHealthService(logger)
	h := NewHandler(nil, healthService, logger, nil)
	return h
}

func TestNewHandler(t *testing.T) {
	logger := slog.Default()
	healthService := service.NewHealthService(logger)

	h := NewHandler(nil, healthService, logger, nil)

	if h == nil {
		t.Fatal("expected non-nil Handler")
	}
	if h.healthService == nil {
		t.Error("expected healthService to be set")
	}
	if h.logger == nil {
		t.Error("expected logger to be set")
	}
}

func TestHandler_writeJSON(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		body           any
		wantStatus     int
		wantBodySubstr string
	}{
		{
			name:           "200 with string body",
			status:         http.StatusOK,
			body:           map[string]string{"status": "ok"},
			wantStatus:     http.StatusOK,
			wantBodySubstr: `"status":"ok"`,
		},
		{
			name:           "400 with error body",
			status:         http.StatusBadRequest,
			body:           map[string]string{"error": "bad"},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: `"error":"bad"`,
		},
		{
			name:       "nil body",
			status:     http.StatusOK,
			body:       nil,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t)
			w := httptest.NewRecorder()

			h.writeJSON(w, tt.status, tt.body)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if w.Header().Get("Content-Type") != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", w.Header().Get("Content-Type"))
			}
			if tt.wantBodySubstr != "" {
				body := w.Body.String()
				if !strings.Contains(body, tt.wantBodySubstr) {
					t.Errorf("body %q does not contain %q", body, tt.wantBodySubstr)
				}
			}
		})
	}
}

func TestHandler_writeErrorResponse(t *testing.T) {
	h := newTestHandler(t)
	w := httptest.NewRecorder()

	h.writeErrorResponse(w, http.StatusBadRequest,
		types.ErrorTypeValidation,
		types.ErrorCodeInvalidRequest,
		"some error message",
	)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}

	var resp types.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Title != types.ErrorTypeValidation {
		t.Errorf("Title = %q, want %q", resp.Title, types.ErrorTypeValidation)
	}
	if resp.ErrorCode != types.ErrorCodeInvalidRequest {
		t.Errorf("ErrorCode = %q, want %q", resp.ErrorCode, types.ErrorCodeInvalidRequest)
	}
	if resp.Message != "some error message" {
		t.Errorf("Message = %q, want %q", resp.Message, "some error message")
	}
}

func TestHandler_Health(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	h.Health(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}

	if resp["status"] != "healthy" {
		t.Errorf("status = %q, want %q", resp["status"], "healthy")
	}
	if resp["timestamp"] == "" {
		t.Error("expected non-empty timestamp in health response")
	}
}
