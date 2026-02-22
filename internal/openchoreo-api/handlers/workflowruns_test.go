// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// mockWorkflowRunService is a test double for WorkflowRunServiceInterface.
// Only GetWorkflowRunLogs is exercised by the tests below; the other methods
// panic if unexpectedly called so test failures surface immediately.
type mockWorkflowRunService struct {
	getLogsResult   []models.ComponentWorkflowRunLogEntry
	getLogsErr      error
	getEventsResult []models.ComponentWorkflowRunEventEntry
	getEventsErr    error
}

func (m *mockWorkflowRunService) ListWorkflowRuns(_ context.Context, _, _, _ string) ([]*models.WorkflowRunResponse, error) {
	panic("unexpected call to ListWorkflowRuns")
}
func (m *mockWorkflowRunService) GetWorkflowRun(_ context.Context, _, _ string) (*models.WorkflowRunResponse, error) {
	panic("unexpected call to GetWorkflowRun")
}
func (m *mockWorkflowRunService) GetWorkflowRunStatus(_ context.Context, _, _, _ string) (*models.ComponentWorkflowRunStatusResponse, error) {
	panic("unexpected call to GetWorkflowRunStatus")
}
func (m *mockWorkflowRunService) CreateWorkflowRun(_ context.Context, _ string, _ *models.CreateWorkflowRunRequest) (*models.WorkflowRunResponse, error) {
	panic("unexpected call to CreateWorkflowRun")
}
func (m *mockWorkflowRunService) GetWorkflowRunLogs(_ context.Context, _, _, _, _ string, _ *int64) ([]models.ComponentWorkflowRunLogEntry, error) {
	return m.getLogsResult, m.getLogsErr
}
func (m *mockWorkflowRunService) GetWorkflowRunEvents(_ context.Context, _, _, _, _ string) ([]models.ComponentWorkflowRunEventEntry, error) {
	return m.getEventsResult, m.getEventsErr
}
func (m *mockWorkflowRunService) TriggerWorkflow(_ context.Context, _, _, _, _ string) (*models.ComponentWorkflowResponse, error) {
	panic("unexpected call to TriggerWorkflow")
}

// newHandlerWithMock returns a Handler wired with the provided mock service.
func newHandlerWithMock(mock services.WorkflowRunServiceInterface) *Handler {
	svc := &services.Services{}
	svc.WorkflowRunService = mock
	cfg := &config.Config{
		ClusterGateway: config.ClusterGatewayConfig{URL: "http://test-gateway"},
	}
	return &Handler{
		services: svc,
		config:   cfg,
		logger:   slog.Default(),
	}
}

// ========== GetWorkflowRunLogs – query parameter validation ==========

func TestGetWorkflowRunLogs_InvalidSinceSeconds(t *testing.T) {
	tests := []struct {
		name         string
		sinceSeconds string
		wantStatus   int
		wantCode     string
	}{
		{
			name:         "non-integer sinceSeconds",
			sinceSeconds: "abc",
			wantStatus:   http.StatusBadRequest,
			wantCode:     services.CodeInvalidInput,
		},
		{
			name:         "negative sinceSeconds",
			sinceSeconds: "-5",
			wantStatus:   http.StatusBadRequest,
			wantCode:     services.CodeInvalidInput,
		},
	}

	h := newHandlerWithMock(&mockWorkflowRunService{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/namespaces/test-ns/workflow-runs/test-run/logs?sinceSeconds="+tt.sinceSeconds, nil)
			req.SetPathValue("namespaceName", "test-ns")
			req.SetPathValue("runName", "test-run")

			w := httptest.NewRecorder()
			h.GetWorkflowRunLogs(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if !strings.Contains(w.Body.String(), tt.wantCode) {
				t.Errorf("body %q does not contain code %q", w.Body.String(), tt.wantCode)
			}
		})
	}
}

func TestGetWorkflowRunLogs_MissingPathValues(t *testing.T) {
	tests := []struct {
		name          string
		namespaceName string
		runName       string
		wantStatus    int
		wantCode      string
	}{
		{
			name:          "missing namespaceName",
			namespaceName: "",
			runName:       "test-run",
			wantStatus:    http.StatusBadRequest,
			wantCode:      services.CodeInvalidInput,
		},
		{
			name:          "missing runName",
			namespaceName: "test-ns",
			runName:       "",
			wantStatus:    http.StatusBadRequest,
			wantCode:      services.CodeInvalidInput,
		},
	}

	h := newHandlerWithMock(&mockWorkflowRunService{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/namespaces/"+tt.namespaceName+"/workflow-runs/"+tt.runName+"/logs", nil)
			req.SetPathValue("namespaceName", tt.namespaceName)
			req.SetPathValue("runName", tt.runName)

			w := httptest.NewRecorder()
			h.GetWorkflowRunLogs(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if !strings.Contains(w.Body.String(), tt.wantCode) {
				t.Errorf("body %q does not contain code %q", w.Body.String(), tt.wantCode)
			}
		})
	}
}

// ========== GetWorkflowRunLogs – service error mapping ==========

func TestGetWorkflowRunLogs_ServiceErrors(t *testing.T) {
	tests := []struct {
		name       string
		serviceErr error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "ErrWorkflowRunNotFound → 404",
			serviceErr: services.ErrWorkflowRunNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   services.CodeWorkflowRunNotFound,
		},
		{
			name:       "ErrForbidden → 403",
			serviceErr: services.ErrForbidden,
			wantStatus: http.StatusForbidden,
			wantCode:   services.CodeForbidden,
		},
		{
			name:       "ErrWorkflowRunReferenceNotFound → 404",
			serviceErr: services.ErrWorkflowRunReferenceNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   services.CodeWorkflowRunReferenceNotFound,
		},
		{
			name:       "unexpected error → 500",
			serviceErr: errors.New("unexpected failure"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   services.CodeInternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHandlerWithMock(&mockWorkflowRunService{getLogsErr: tt.serviceErr})
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/namespaces/test-ns/workflow-runs/test-run/logs", nil)
			req.SetPathValue("namespaceName", "test-ns")
			req.SetPathValue("runName", "test-run")

			w := httptest.NewRecorder()
			h.GetWorkflowRunLogs(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if !strings.Contains(w.Body.String(), tt.wantCode) {
				t.Errorf("body %q does not contain code %q", w.Body.String(), tt.wantCode)
			}
		})
	}
}

// ========== GetWorkflowRunLogs – success response shape ==========

func TestGetWorkflowRunLogs_Success(t *testing.T) {
	entries := []models.ComponentWorkflowRunLogEntry{
		{Timestamp: "2025-01-06T10:00:00Z", Log: "step started"},
		{Timestamp: "", Log: "no timestamp line"},
	}
	h := newHandlerWithMock(&mockWorkflowRunService{getLogsResult: entries})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/namespaces/test-ns/workflow-runs/test-run/logs", nil)
	req.SetPathValue("namespaceName", "test-ns")
	req.SetPathValue("runName", "test-run")

	w := httptest.NewRecorder()
	h.GetWorkflowRunLogs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got []models.ComponentWorkflowRunLogEntry
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2", len(got))
	}
	if got[0].Log != "step started" {
		t.Errorf("got[0].Log = %q, want %q", got[0].Log, "step started")
	}
}
