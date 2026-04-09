// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	apimocks "github.com/openchoreo/openchoreo/internal/rca-agent/api/mocks"
	"github.com/openchoreo/openchoreo/internal/rca-agent/service"
	"github.com/openchoreo/openchoreo/internal/rca-agent/store"
	storemocks "github.com/openchoreo/openchoreo/internal/rca-agent/store/mocks"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// These are simple structs that implement the authzcore.PDP interface.
// We use hand-written stubs instead of mockery here because the behavior
// is always the same: allow everything, deny everything, or return an error.

type allowAllPDP struct{}

func (a *allowAllPDP) Evaluate(_ context.Context, _ *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return &authzcore.Decision{Decision: true, Context: &authzcore.DecisionContext{}}, nil
}

func (a *allowAllPDP) BatchEvaluate(_ context.Context, _ *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	return nil, nil
}

func (a *allowAllPDP) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

type denyAllPDP struct{}

func (d *denyAllPDP) Evaluate(_ context.Context, _ *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return &authzcore.Decision{Decision: false, Context: &authzcore.DecisionContext{}}, nil
}

func (d *denyAllPDP) BatchEvaluate(_ context.Context, _ *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	return nil, nil
}

func (d *denyAllPDP) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

const (
	applyFirstActionBody = `{"appliedIndices": [0]}`
	validChatBody        = `{"reportId":"rpt-1","namespace":"ns","project":"proj","environment":"dev","messages":[{"role":"user","content":"hi"}]}`
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// authedContext returns a context with a SubjectContext set, simulating
// a request that passed through the JWT auth middleware.
func authedContext() context.Context {
	return auth.SetSubjectContext(context.Background(), &auth.SubjectContext{
		ID:                "test-user",
		Type:              "user",
		EntitlementClaim:  "groups",
		EntitlementValues: []string{"org-admins"},
	})
}

// newTestHandler creates a Handler wired to the given mocks.
func newTestHandler(mockStore *storemocks.MockReportStore, pdp authzcore.PDP, mockSvc *apimocks.MockAgentService) *Handler {
	return &Handler{
		logger:            testLogger(),
		reportStore:       mockStore,
		authzClient:       pdp,
		service:           mockSvc,
		streamWriteTimeout: 10 * time.Minute,
	}
}

// doRequest creates an HTTP request, sets the authed context, calls the handler,
// and returns the recorded response.
func doGet(t *testing.T, handler http.HandlerFunc, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func doPost(t *testing.T, handler http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1alpha1/rca-agent/analyze", strings.NewReader(body))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestGetReport_Success(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	reportJSON := `{"summary":"OOM detected","result":{"type":"root_cause_identified"},"alert_context":{"alert_id":"a1","alert_name":"high-mem","triggered_at":"2026-03-07T10:00:00Z","trigger_value":95,"component":"api","project":"myproj","environment":"prod"},"investigation_path":[]}`

	mockStore.On("GetReport", mock.Anything, "rpt-1").Return(&store.ReportEntry{
		ReportID:      "rpt-1",
		AlertID:       "alert-1",
		Status:        "completed",
		Timestamp:     "2026-03-07T10:00:00Z",
		NamespaceName: "ns-1",
		ProjectName:   "proj-1",
		Report:        &reportJSON,
	}, nil)

	// Use http.NewServeMux to wire path parameters (Go 1.22+ pattern).
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/rca-agent/reports/{report_id}", h.GetReport)
	req := httptest.NewRequest("GET", "/api/v1/rca-agent/reports/rpt-1", nil)
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp RCAReportDetailed
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "rpt-1", resp.ReportID)
	assert.Equal(t, "alert-1", resp.AlertID)
	assert.Equal(t, "completed", resp.Status)
	require.NotNil(t, resp.Report)
	assert.Equal(t, "OOM detected", resp.Report.Summary)
}

func TestGetReport_NotFound(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	mockStore.On("GetReport", mock.Anything, "nonexistent").Return(nil, fmt.Errorf("failed to get report: %w", sql.ErrNoRows))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/rca-agent/reports/{report_id}", h.GetReport)
	req := httptest.NewRequest("GET", "/api/v1/rca-agent/reports/nonexistent", nil)
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetReport_Forbidden(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &denyAllPDP{}, nil)

	mockStore.On("GetReport", mock.Anything, "rpt-1").Return(&store.ReportEntry{
		ReportID:      "rpt-1",
		NamespaceName: "ns-1",
		ProjectName:   "proj-1",
	}, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/rca-agent/reports/{report_id}", h.GetReport)
	req := httptest.NewRequest("GET", "/api/v1/rca-agent/reports/rpt-1", nil)
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestGetReport_Unauthenticated(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	mockStore.On("GetReport", mock.Anything, "rpt-1").Return(&store.ReportEntry{
		ReportID:      "rpt-1",
		NamespaceName: "ns-1",
		ProjectName:   "proj-1",
	}, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/rca-agent/reports/{report_id}", h.GetReport)
	// No authedContext — simulates missing JWT
	req := httptest.NewRequest("GET", "/api/v1/rca-agent/reports/rpt-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestGetReport_WithoutReportContent(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	mockStore.On("GetReport", mock.Anything, "rpt-1").Return(&store.ReportEntry{
		ReportID:      "rpt-1",
		AlertID:       "alert-1",
		Status:        "pending",
		Timestamp:     "2026-03-07T10:00:00Z",
		NamespaceName: "ns-1",
		ProjectName:   "proj-1",
		Report:        nil,
	}, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/rca-agent/reports/{report_id}", h.GetReport)
	req := httptest.NewRequest("GET", "/api/v1/rca-agent/reports/rpt-1", nil)
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp RCAReportDetailed
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "pending", resp.Status)
	assert.Nil(t, resp.Report)
}

func TestGetReport_StoreError(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	mockStore.On("GetReport", mock.Anything, "rpt-1").Return(nil, fmt.Errorf("connection refused"))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/rca-agent/reports/{report_id}", h.GetReport)
	req := httptest.NewRequest("GET", "/api/v1/rca-agent/reports/rpt-1", nil)
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestUpdateReport_Success(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	reportJSON := `{"result":{"recommendations":{"recommended_actions":[{"description":"restart pod","status":"revised"}]}}}`
	mockStore.On("GetReport", mock.Anything, "rpt-1").Return(&store.ReportEntry{
		ReportID:      "rpt-1",
		NamespaceName: "ns-1",
		ProjectName:   "proj-1",
		Report:        &reportJSON,
	}, nil)
	mockStore.On("UpdateActionStatuses", mock.Anything, "rpt-1", mock.Anything).Return(nil)

	body := applyFirstActionBody

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v1/rca-agent/reports/{report_id}", h.UpdateReport)
	req := httptest.NewRequest("PUT", "/api/v1/rca-agent/reports/rpt-1", strings.NewReader(body))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp StatusResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp.Status)
}

func TestUpdateReport_NotFound(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	mockStore.On("GetReport", mock.Anything, "nonexistent").Return(nil, fmt.Errorf("failed: %w", sql.ErrNoRows))

	body := applyFirstActionBody

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v1/rca-agent/reports/{report_id}", h.UpdateReport)
	req := httptest.NewRequest("PUT", "/api/v1/rca-agent/reports/nonexistent", strings.NewReader(body))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestUpdateReport_Forbidden(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &denyAllPDP{}, nil)

	reportJSON := `{"result":{}}`
	mockStore.On("GetReport", mock.Anything, "rpt-1").Return(&store.ReportEntry{
		ReportID:      "rpt-1",
		NamespaceName: "ns-1",
		ProjectName:   "proj-1",
		Report:        &reportJSON,
	}, nil)

	body := applyFirstActionBody

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v1/rca-agent/reports/{report_id}", h.UpdateReport)
	req := httptest.NewRequest("PUT", "/api/v1/rca-agent/reports/rpt-1", strings.NewReader(body))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestUpdateReport_OverlappingIndices(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	body := `{"appliedIndices": [0, 1], "dismissedIndices": [1, 2]}`

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v1/rca-agent/reports/{report_id}", h.UpdateReport)
	req := httptest.NewRequest("PUT", "/api/v1/rca-agent/reports/rpt-1", strings.NewReader(body))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUpdateReport_NoReportContent(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	mockStore.On("GetReport", mock.Anything, "rpt-1").Return(&store.ReportEntry{
		ReportID:      "rpt-1",
		NamespaceName: "ns-1",
		ProjectName:   "proj-1",
		Report:        nil,
	}, nil)

	body := applyFirstActionBody

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v1/rca-agent/reports/{report_id}", h.UpdateReport)
	req := httptest.NewRequest("PUT", "/api/v1/rca-agent/reports/rpt-1", strings.NewReader(body))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUpdateReport_InvalidBody(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v1/rca-agent/reports/{report_id}", h.UpdateReport)
	req := httptest.NewRequest("PUT", "/api/v1/rca-agent/reports/rpt-1", strings.NewReader("not json"))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListReports_MissingRequiredParams(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	rec := doGet(t, h.ListReports, "/api/v1/rca-agent/reports")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestListReports_InvalidStartTime(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	rec := doGet(t, h.ListReports,
		"/api/v1/rca-agent/reports?project=p&environment=e&namespace=n&startTime=bad&endTime=2026-03-08T00:00:00Z")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListReports_InvalidEndTime(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	rec := doGet(t, h.ListReports,
		"/api/v1/rca-agent/reports?project=p&environment=e&namespace=n&startTime=2026-03-07T00:00:00Z&endTime=bad")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListReports_StartTimeAfterEndTime(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	rec := doGet(t, h.ListReports,
		"/api/v1/rca-agent/reports?project=p&environment=e&namespace=n&startTime=2026-03-09T00:00:00Z&endTime=2026-03-08T00:00:00Z")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListReports_InvalidSort(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	mockSvc := apimocks.NewMockAgentService(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, mockSvc)

	mockSvc.On("ResolveProjectScope", mock.Anything, "n", "p", "e").
		Return(&service.Scope{ProjectUID: "p-uid", EnvironmentUID: "e-uid"}, nil)

	rec := doGet(t, h.ListReports,
		"/api/v1/rca-agent/reports?project=p&environment=e&namespace=n&startTime=2026-03-07T00:00:00Z&endTime=2026-03-08T00:00:00Z&sort=invalid")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListReports_InvalidStatus(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	mockSvc := apimocks.NewMockAgentService(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, mockSvc)

	mockSvc.On("ResolveProjectScope", mock.Anything, "n", "p", "e").
		Return(&service.Scope{ProjectUID: "p-uid", EnvironmentUID: "e-uid"}, nil)

	rec := doGet(t, h.ListReports,
		"/api/v1/rca-agent/reports?project=p&environment=e&namespace=n&startTime=2026-03-07T00:00:00Z&endTime=2026-03-08T00:00:00Z&status=invalid")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListReports_InvalidLimit(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	mockSvc := apimocks.NewMockAgentService(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, mockSvc)

	mockSvc.On("ResolveProjectScope", mock.Anything, "n", "p", "e").
		Return(&service.Scope{ProjectUID: "p-uid", EnvironmentUID: "e-uid"}, nil)

	rec := doGet(t, h.ListReports,
		"/api/v1/rca-agent/reports?project=p&environment=e&namespace=n&startTime=2026-03-07T00:00:00Z&endTime=2026-03-08T00:00:00Z&limit=-1")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListReports_Forbidden(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &denyAllPDP{}, nil)

	rec := doGet(t, h.ListReports,
		"/api/v1/rca-agent/reports?project=p&environment=e&namespace=n&startTime=2026-03-07T00:00:00Z&endTime=2026-03-08T00:00:00Z")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestListReports_Success(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	mockSvc := apimocks.NewMockAgentService(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, mockSvc)

	mockSvc.On("ResolveProjectScope", mock.Anything, "ns-1", "proj-1", "dev").
		Return(&service.Scope{ProjectUID: "proj-uid-1", EnvironmentUID: "env-uid-1"}, nil)

	summary := "high error rate"
	mockStore.On("ListReports", mock.Anything, mock.MatchedBy(func(p store.QueryParams) bool {
		return p.ProjectUID == "proj-uid-1" && p.EnvironmentUID == "env-uid-1"
	})).Return([]store.ReportEntry{
		{
			ReportID:  "rpt-1",
			AlertID:   "alert-1",
			Status:    "completed",
			Timestamp: "2026-03-07T10:00:00Z",
			Summary:   &summary,
		},
	}, 1, nil)

	rec := doGet(t, h.ListReports,
		"/api/v1/rca-agent/reports?project=proj-1&environment=dev&namespace=ns-1&startTime=2026-03-07T00:00:00Z&endTime=2026-03-08T00:00:00Z")

	require.Equal(t, http.StatusOK, rec.Code)

	var resp RCAReportsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.TotalCount)
	require.Len(t, resp.Reports, 1)
	assert.Equal(t, "rpt-1", resp.Reports[0].ReportID)
	assert.Equal(t, "high error rate", *resp.Reports[0].Summary)
}

// This is a pure function — no mocks needed, just input/output.

func TestApplyActionStatusUpdates_RevisedToApplied(t *testing.T) {
	t.Parallel()
	report := map[string]any{
		"result": map[string]any{
			"recommendations": map[string]any{
				"recommended_actions": []any{
					map[string]any{"description": "restart", "status": "revised"},
					map[string]any{"description": "scale up", "status": "suggested"},
				},
			},
		},
	}

	changed := applyActionStatusUpdates(report, []int{0}, nil)

	require.True(t, changed)
	actions := report["result"].(map[string]any)["recommendations"].(map[string]any)["recommended_actions"].([]any)
	assert.Equal(t, "applied", actions[0].(map[string]any)["status"])
	assert.Equal(t, "suggested", actions[1].(map[string]any)["status"])
}

func TestApplyActionStatusUpdates_SuggestedToDismissed(t *testing.T) {
	t.Parallel()
	report := map[string]any{
		"result": map[string]any{
			"recommendations": map[string]any{
				"recommended_actions": []any{
					map[string]any{"description": "restart", "status": "suggested"},
				},
			},
		},
	}

	changed := applyActionStatusUpdates(report, nil, []int{0})

	require.True(t, changed)
	actions := report["result"].(map[string]any)["recommendations"].(map[string]any)["recommended_actions"].([]any)
	assert.Equal(t, "dismissed", actions[0].(map[string]any)["status"])
}

func TestApplyActionStatusUpdates_InvalidTransitionIgnored(t *testing.T) {
	t.Parallel()
	report := map[string]any{
		"result": map[string]any{
			"recommendations": map[string]any{
				"recommended_actions": []any{
					map[string]any{"description": "restart", "status": "suggested"},
				},
			},
		},
	}

	// "suggested" → "applied" is not a valid transition
	changed := applyActionStatusUpdates(report, []int{0}, nil)

	assert.False(t, changed)
	actions := report["result"].(map[string]any)["recommendations"].(map[string]any)["recommended_actions"].([]any)
	assert.Equal(t, "suggested", actions[0].(map[string]any)["status"])
}

func TestApplyActionStatusUpdates_NoResult(t *testing.T) {
	t.Parallel()
	report := map[string]any{}
	changed := applyActionStatusUpdates(report, []int{0}, nil)
	assert.False(t, changed)
}

func TestHealth(t *testing.T) {
	t.Parallel()
	h := newTestHandler(nil, nil, nil)

	rec := doGet(t, h.Health, "/health")

	require.Equal(t, http.StatusOK, rec.Code)

	var resp StatusResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "healthy", resp.Status)
}

func TestChat_InvalidRole(t *testing.T) {
	t.Parallel()
	h := newTestHandler(storemocks.NewMockReportStore(t), &allowAllPDP{}, apimocks.NewMockAgentService(t))

	body := `{"reportId":"rpt-1","namespace":"ns","project":"proj","environment":"dev","messages":[{"role":"system","content":"you are helpful"}]}`
	req := httptest.NewRequest("POST", "/api/v1alpha1/rca-agent/chat", strings.NewReader(body))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	h.Chat(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid role")
}

func TestChat_ToolRoleRejected(t *testing.T) {
	t.Parallel()
	h := newTestHandler(storemocks.NewMockReportStore(t), &allowAllPDP{}, apimocks.NewMockAgentService(t))

	body := `{"reportId":"rpt-1","namespace":"ns","project":"proj","environment":"dev","messages":[{"role":"tool","content":"result"}]}`
	req := httptest.NewRequest("POST", "/api/v1alpha1/rca-agent/chat", strings.NewReader(body))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	h.Chat(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid role")
}

func TestChat_ValidRolesAccepted(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	mockSvc := apimocks.NewMockAgentService(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, mockSvc)

	reportJSON := `{"summary":"test"}`
	mockStore.On("GetReport", mock.Anything, "rpt-1").Return(&store.ReportEntry{
		ReportID: "rpt-1",
		Status:   "completed",
		Report:   &reportJSON,
	}, nil)

	// Mock StreamChat to return a channel that closes immediately.
	mockSvc.On("StreamChat", mock.Anything, mock.Anything, mock.Anything).Return(func() <-chan service.ChatEvent {
		ch := make(chan service.ChatEvent)
		close(ch)
		return ch
	}())

	body := `{"reportId":"rpt-1","namespace":"ns","project":"proj","environment":"dev","messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"}]}`
	req := httptest.NewRequest("POST", "/api/v1alpha1/rca-agent/chat", strings.NewReader(body))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	h.Chat(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAnalyze_Success(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	mockSvc := apimocks.NewMockAgentService(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, mockSvc)

	mockSvc.On("ResolveComponentScope", mock.Anything, "ns", "proj", "comp", "dev").
		Return(&service.Scope{
			ProjectUID:     "proj-uid",
			EnvironmentUID: "env-uid",
		}, nil)

	mockStore.On("UpsertReport", mock.Anything, mock.MatchedBy(func(e *store.ReportEntry) bool {
		return e.Status == "pending" && e.NamespaceName == "ns" && e.ProjectName == "proj"
	})).Return(nil)

	mockSvc.On("RunAnalysis", mock.Anything).Maybe().Return()

	body := `{"namespace":"ns","project":"proj","component":"comp","environment":"dev","alert":{"id":"alert-1","value":95}}`
	rec := doPost(t, h.Analyze, body)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp RCAResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "pending", resp.Status)
	assert.NotEmpty(t, resp.ReportID)
}

func TestAnalyze_MissingFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing namespace",
			body: `{"project":"p","component":"c","environment":"e","alert":{"id":"a"}}`,
			want: "namespace, project, component, and environment are required",
		},
		{
			name: "missing alert id",
			body: `{"namespace":"n","project":"p","component":"c","environment":"e","alert":{}}`,
			want: "alert.id is required",
		},
		{
			name: "invalid body",
			body: `{bad`,
			want: "invalid request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(storemocks.NewMockReportStore(t), &allowAllPDP{}, nil)
			rec := doPost(t, h.Analyze, tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.want)
		})
	}
}

func TestAnalyze_ScopeResolutionFailure(t *testing.T) {
	t.Parallel()
	mockSvc := apimocks.NewMockAgentService(t)
	h := newTestHandler(storemocks.NewMockReportStore(t), &allowAllPDP{}, mockSvc)

	mockSvc.On("ResolveComponentScope", mock.Anything, "ns", "proj", "comp", "dev").
		Return(nil, fmt.Errorf("API unreachable"))

	body := `{"namespace":"ns","project":"proj","component":"comp","environment":"dev","alert":{"id":"alert-1"}}`
	rec := doPost(t, h.Analyze, body)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "failed to resolve component scope")
}

func TestAnalyze_StoreFailure(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	mockSvc := apimocks.NewMockAgentService(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, mockSvc)

	mockSvc.On("ResolveComponentScope", mock.Anything, "ns", "proj", "comp", "dev").
		Return(&service.Scope{ProjectUID: "p", EnvironmentUID: "e"}, nil)

	mockStore.On("UpsertReport", mock.Anything, mock.Anything).
		Return(fmt.Errorf("db connection lost"))

	body := `{"namespace":"ns","project":"proj","component":"comp","environment":"dev","alert":{"id":"alert-1"}}`
	rec := doPost(t, h.Analyze, body)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "failed to create report")
}

func TestChat_ReportNotFound(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	mockStore.On("GetReport", mock.Anything, "missing").Return(nil, sql.ErrNoRows)

	body := `{"reportId":"missing","namespace":"ns","project":"proj","environment":"dev","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest("POST", "/api/v1alpha1/rca-agent/chat", strings.NewReader(body))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	h.Chat(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestChat_ProjectMismatch(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, nil)

	mockStore.On("GetReport", mock.Anything, "rpt-1").Return(&store.ReportEntry{
		ReportID:    "rpt-1",
		Status:      "completed",
		ProjectName: "other-project",
	}, nil)

	body := validChatBody
	req := httptest.NewRequest("POST", "/api/v1alpha1/rca-agent/chat", strings.NewReader(body))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	h.Chat(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "does not belong")
}

func TestChat_StreamsEvents(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	mockSvc := apimocks.NewMockAgentService(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, mockSvc)

	reportJSON := `{"summary":"test"}`
	mockStore.On("GetReport", mock.Anything, "rpt-1").Return(&store.ReportEntry{
		ReportID: "rpt-1",
		Status:   "completed",
		Report:   &reportJSON,
	}, nil)

	mockSvc.On("StreamChat", mock.Anything, mock.Anything, mock.Anything).Return(func() <-chan service.ChatEvent {
		ch := make(chan service.ChatEvent, 3)
		ch <- service.ChatEvent{Type: "message_chunk", Content: "hello"}
		ch <- service.ChatEvent{Type: "message_chunk", Content: " world"}
		ch <- service.ChatEvent{Type: "done", Message: "hello world"}
		close(ch)
		return ch
	}())

	body := validChatBody
	req := httptest.NewRequest("POST", "/api/v1alpha1/rca-agent/chat", strings.NewReader(body))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	h.Chat(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/x-ndjson", rec.Header().Get("Content-Type"))

	// NDJSON: each line is a JSON object.
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	require.Len(t, lines, 3)

	var first service.ChatEvent
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
	assert.Equal(t, "message_chunk", first.Type)
	assert.Equal(t, "hello", first.Content)

	var last service.ChatEvent
	require.NoError(t, json.Unmarshal([]byte(lines[2]), &last))
	assert.Equal(t, "done", last.Type)
	assert.Equal(t, "hello world", last.Message)
}

func TestChat_Forbidden(t *testing.T) {
	t.Parallel()
	h := newTestHandler(storemocks.NewMockReportStore(t), &denyAllPDP{}, nil)

	body := validChatBody
	req := httptest.NewRequest("POST", "/api/v1alpha1/rca-agent/chat", strings.NewReader(body))
	req = req.WithContext(authedContext())
	rec := httptest.NewRecorder()
	h.Chat(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestListReports_ScopeResolutionFailure(t *testing.T) {
	t.Parallel()
	mockSvc := apimocks.NewMockAgentService(t)
	h := newTestHandler(storemocks.NewMockReportStore(t), &allowAllPDP{}, mockSvc)

	mockSvc.On("ResolveProjectScope", mock.Anything, "n", "p", "e").
		Return(nil, fmt.Errorf("API unreachable"))

	rec := doGet(t, h.ListReports,
		"/api/v1/rca-agent/reports?project=p&environment=e&namespace=n&startTime=2026-03-07T00:00:00Z&endTime=2026-03-08T00:00:00Z")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "failed to resolve project scope")
}

func TestListReports_StoreError(t *testing.T) {
	t.Parallel()
	mockStore := storemocks.NewMockReportStore(t)
	mockSvc := apimocks.NewMockAgentService(t)
	h := newTestHandler(mockStore, &allowAllPDP{}, mockSvc)

	mockSvc.On("ResolveProjectScope", mock.Anything, "n", "p", "e").
		Return(&service.Scope{ProjectUID: "p-uid", EnvironmentUID: "e-uid"}, nil)

	mockStore.On("ListReports", mock.Anything, mock.Anything).
		Return(nil, 0, fmt.Errorf("db error"))

	rec := doGet(t, h.ListReports,
		"/api/v1/rca-agent/reports?project=p&environment=e&namespace=n&startTime=2026-03-07T00:00:00Z&endTime=2026-03-08T00:00:00Z")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "failed to list reports")
}
