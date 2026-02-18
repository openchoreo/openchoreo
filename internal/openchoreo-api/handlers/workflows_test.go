// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
)

// ---- Workflow handler helpers ----

func newTestWorkflowHandler(workflows ...openchoreov1alpha1.Workflow) *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range workflows {
		builder = builder.WithObjects(&workflows[i])
	}
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	wfSvc := services.NewWorkflowService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.WorkflowService = wfSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- ListWorkflows tests ----

func TestListWorkflows_MissingNamespace(t *testing.T) {
	h := newTestWorkflowHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//workflows", nil)
	rr := httptest.NewRecorder()
	h.ListWorkflows(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("ListWorkflows missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestListWorkflows_Empty(t *testing.T) {
	h := newTestWorkflowHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/workflows", nil)
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.ListWorkflows(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListWorkflows empty: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestListWorkflows_WithItems(t *testing.T) {
	h := newTestWorkflowHandler(
		openchoreov1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "wf-1", Namespace: "ns1"},
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/ns1/workflows", nil)
	req.SetPathValue("namespaceName", "ns1")
	rr := httptest.NewRecorder()
	h.ListWorkflows(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListWorkflows with items: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---- GetWorkflowSchema tests ----

func TestGetWorkflowSchema_MissingNamespace(t *testing.T) {
	h := newTestWorkflowHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//workflows/my-wf/schema", nil)
	req.SetPathValue("workflowName", "my-wf")
	rr := httptest.NewRecorder()
	h.GetWorkflowSchema(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetWorkflowSchema missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetWorkflowSchema_MissingWorkflowName(t *testing.T) {
	h := newTestWorkflowHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/workflows//schema", nil)
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.GetWorkflowSchema(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetWorkflowSchema missing workflow name: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetWorkflowSchema_NotFound(t *testing.T) {
	h := newTestWorkflowHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/workflows/nonexistent/schema", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("workflowName", "nonexistent")
	rr := httptest.NewRecorder()
	h.GetWorkflowSchema(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GetWorkflowSchema not found: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ---- WorkflowRun handler helpers ----

func newTestWorkflowRunHandler() *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	wfrSvc := services.NewWorkflowRunService(fakeClient, slog.Default(), pdp, nil, nil)
	svcs := &services.Services{}
	svcs.WorkflowRunService = wfrSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- ListWorkflowRuns tests ----

func TestListWorkflowRuns_MissingNamespace(t *testing.T) {
	h := newTestWorkflowRunHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//workflowruns", nil)
	rr := httptest.NewRecorder()
	h.ListWorkflowRuns(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("ListWorkflowRuns missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestListWorkflowRuns_Empty(t *testing.T) {
	h := newTestWorkflowRunHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/workflowruns", nil)
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.ListWorkflowRuns(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListWorkflowRuns empty: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---- GetWorkflowRun tests ----

func TestGetWorkflowRun_MissingParams(t *testing.T) {
	tests := []struct {
		name          string
		namespaceName string
		runName       string
		wantCode      int
	}{
		{"missing both", "", "", http.StatusBadRequest},
		{"missing namespace", "", "run-1", http.StatusBadRequest},
		{"missing run name", "default", "", http.StatusBadRequest},
	}
	h := newTestWorkflowRunHandler()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/"+tt.namespaceName+"/workflowruns/"+tt.runName, nil)
			req.SetPathValue("namespaceName", tt.namespaceName)
			req.SetPathValue("runName", tt.runName)
			rr := httptest.NewRecorder()
			h.GetWorkflowRun(rr, req)
			if rr.Code != tt.wantCode {
				t.Errorf("GetWorkflowRun %s: got %d, want %d", tt.name, rr.Code, tt.wantCode)
			}
		})
	}
}

func TestGetWorkflowRun_NotFound(t *testing.T) {
	h := newTestWorkflowRunHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/workflowruns/nonexistent", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("runName", "nonexistent")
	rr := httptest.NewRecorder()
	h.GetWorkflowRun(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GetWorkflowRun not found: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ---- CreateWorkflowRun tests ----

func TestCreateWorkflowRun_MissingNamespace(t *testing.T) {
	h := newTestWorkflowRunHandler()
	body := `{"workflowName":"my-wf"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces//workflowruns", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.CreateWorkflowRun(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("CreateWorkflowRun missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCreateWorkflowRun_InvalidJSON(t *testing.T) {
	h := newTestWorkflowRunHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/workflowruns", bytes.NewReader([]byte(`{invalid}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.CreateWorkflowRun(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("CreateWorkflowRun invalid JSON: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}
