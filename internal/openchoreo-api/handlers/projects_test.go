// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
)

// mockPDPAllow is a PDP that always allows all requests
type mockPDPAllow struct{}

func (m *mockPDPAllow) Evaluate(_ context.Context, _ *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return &authzcore.Decision{Decision: true, Context: &authzcore.DecisionContext{Reason: "allowed"}}, nil
}

func (m *mockPDPAllow) BatchEvaluate(_ context.Context, req *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	decisions := make([]authzcore.Decision, len(req.Requests))
	for i := range decisions {
		decisions[i] = authzcore.Decision{Decision: true, Context: &authzcore.DecisionContext{Reason: "allowed"}}
	}
	return &authzcore.BatchEvaluateResponse{Decisions: decisions}, nil
}

func (m *mockPDPAllow) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

// newTestScheme creates a scheme with OpenChoreo types registered
func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = openchoreov1alpha1.AddToScheme(scheme)
	return scheme
}

// newTestProjectHandler creates a Handler with a fake k8s client and permissive PDP
func newTestProjectHandler() *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	projectSvc := services.NewProjectService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.ProjectService = projectSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// newTestProjectHandlerWithObjects creates a handler with pre-populated OpenChoreo Project objects
func newTestProjectHandlerWithProjects(ns string, projectNames ...string) *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, name := range projectNames {
		builder = builder.WithObjects(&openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
		})
	}
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	projectSvc := services.NewProjectService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.ProjectService = projectSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- CreateProject tests ----

func TestCreateProject_MissingNamespace(t *testing.T) {
	h := newTestProjectHandler()
	body := `{"name": "my-project"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces//projects", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	// namespaceName is empty (not set)
	rr := httptest.NewRecorder()
	h.CreateProject(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("CreateProject missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCreateProject_InvalidJSON(t *testing.T) {
	h := newTestProjectHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/projects", bytes.NewReader([]byte(`{invalid json}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.CreateProject(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("CreateProject invalid JSON: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCreateProject_InvalidBuildPlaneRef(t *testing.T) {
	h := newTestProjectHandler()
	body := `{"name":"proj","buildPlaneRef":{"kind":"InvalidKind","name":"bp"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/projects", bytes.NewReader([]byte(body)))
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.CreateProject(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("CreateProject invalid buildPlaneRef: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCreateProject_Success(t *testing.T) {
	h := newTestProjectHandler()
	body := `{"name":"my-project","displayName":"My Project"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/projects", bytes.NewReader([]byte(body)))
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.CreateProject(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("CreateProject success: got %d, want %d (body: %s)", rr.Code, http.StatusCreated, rr.Body.String())
	}
}

func TestCreateProject_Duplicate(t *testing.T) {
	h := newTestProjectHandlerWithProjects("default", "existing-project")
	body := `{"name":"existing-project"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/projects", bytes.NewReader([]byte(body)))
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.CreateProject(rr, req)
	if rr.Code != http.StatusConflict {
		t.Errorf("CreateProject duplicate: got %d, want %d (body: %s)", rr.Code, http.StatusConflict, rr.Body.String())
	}
}

// ---- ListProjects tests ----

func TestListProjects_MissingNamespace(t *testing.T) {
	h := newTestProjectHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//projects", nil)
	// namespaceName is empty
	rr := httptest.NewRecorder()
	h.ListProjects(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("ListProjects missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestListProjects_EmptyNamespace(t *testing.T) {
	h := newTestProjectHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/projects", nil)
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.ListProjects(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListProjects empty: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestListProjects_WithProjects(t *testing.T) {
	h := newTestProjectHandlerWithProjects("ns1", "proj-a", "proj-b", "proj-c")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/ns1/projects", nil)
	req.SetPathValue("namespaceName", "ns1")
	rr := httptest.NewRecorder()
	h.ListProjects(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListProjects with projects: got %d, want %d", rr.Code, http.StatusOK)
	}
	// Verify response body is valid JSON with items
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Errorf("ListProjects response is not valid JSON: %v", err)
	}
}

// ---- GetProject tests ----

func TestGetProject_MissingParams(t *testing.T) {
	h := newTestProjectHandler()
	tests := []struct {
		name          string
		namespaceName string
		projectName   string
		wantCode      int
	}{
		{"missing both", "", "", http.StatusBadRequest},
		{"missing project", "default", "", http.StatusBadRequest},
		{"missing namespace", "", "proj", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/"+tt.namespaceName+"/projects/"+tt.projectName, nil)
			req.SetPathValue("namespaceName", tt.namespaceName)
			req.SetPathValue("projectName", tt.projectName)
			rr := httptest.NewRecorder()
			h.GetProject(rr, req)
			if rr.Code != tt.wantCode {
				t.Errorf("GetProject %s: got %d, want %d", tt.name, rr.Code, tt.wantCode)
			}
		})
	}
}

func TestGetProject_NotFound(t *testing.T) {
	h := newTestProjectHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/projects/nonexistent", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("projectName", "nonexistent")
	rr := httptest.NewRecorder()
	h.GetProject(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GetProject not found: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestGetProject_Success(t *testing.T) {
	h := newTestProjectHandlerWithProjects("default", "my-project")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/projects/my-project", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("projectName", "my-project")
	rr := httptest.NewRecorder()
	h.GetProject(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GetProject success: got %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}
}

// ---- DeleteProject tests ----

func TestDeleteProject_MissingParams(t *testing.T) {
	h := newTestProjectHandler()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/namespaces//projects/", nil)
	// namespaceName and projectName are empty
	rr := httptest.NewRecorder()
	h.DeleteProject(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("DeleteProject missing params: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestDeleteProject_NotFound(t *testing.T) {
	h := newTestProjectHandler()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/namespaces/default/projects/nonexistent", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("projectName", "nonexistent")
	rr := httptest.NewRecorder()
	h.DeleteProject(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("DeleteProject not found: got %d, want %d (body: %s)", rr.Code, http.StatusNotFound, rr.Body.String())
	}
}

func TestDeleteProject_Success(t *testing.T) {
	h := newTestProjectHandlerWithProjects("default", "to-delete")
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/namespaces/default/projects/to-delete", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("projectName", "to-delete")
	rr := httptest.NewRecorder()
	h.DeleteProject(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("DeleteProject success: got %d, want %d (body: %s)", rr.Code, http.StatusNoContent, rr.Body.String())
	}
}
