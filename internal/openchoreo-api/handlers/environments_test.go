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

// newTestEnvironmentHandler creates a Handler with a fake k8s client for environment tests
func newTestEnvironmentHandler(objects ...openchoreov1alpha1.Environment) *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range objects {
		builder = builder.WithObjects(&objects[i])
	}
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	envSvc := services.NewEnvironmentService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.EnvironmentService = envSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// newTestEnvironmentHandlerWithDataPlane creates a Handler with pre-populated environments and a DataPlane
func newTestEnvironmentHandlerWithDataPlane(ns string, dpName string, envNames ...string) *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	// Add DataPlane
	builder = builder.WithObjects(&openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpName,
			Namespace: ns,
		},
	})
	// Add environments
	for _, name := range envNames {
		builder = builder.WithObjects(&openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
		})
	}
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	envSvc := services.NewEnvironmentService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.EnvironmentService = envSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- ListEnvironments tests ----

func TestListEnvironments_MissingNamespace(t *testing.T) {
	h := newTestEnvironmentHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//environments", nil)
	// namespaceName is empty (not set)
	rr := httptest.NewRecorder()
	h.ListEnvironments(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("ListEnvironments missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestListEnvironments_EmptyNamespace(t *testing.T) {
	h := newTestEnvironmentHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/environments", nil)
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.ListEnvironments(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListEnvironments empty: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestListEnvironments_WithEnvironments(t *testing.T) {
	h := newTestEnvironmentHandler(
		openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "env-a", Namespace: "ns1"},
		},
		openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "env-b", Namespace: "ns1"},
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/ns1/environments", nil)
	req.SetPathValue("namespaceName", "ns1")
	rr := httptest.NewRecorder()
	h.ListEnvironments(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListEnvironments with environments: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---- GetEnvironment tests ----

func TestGetEnvironment_MissingNamespace(t *testing.T) {
	h := newTestEnvironmentHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//environments/my-env", nil)
	req.SetPathValue("envName", "my-env")
	rr := httptest.NewRecorder()
	h.GetEnvironment(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetEnvironment missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetEnvironment_MissingEnvName(t *testing.T) {
	h := newTestEnvironmentHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/environments/", nil)
	req.SetPathValue("namespaceName", "default")
	// envName is empty
	rr := httptest.NewRecorder()
	h.GetEnvironment(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetEnvironment missing envName: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetEnvironment_NotFound(t *testing.T) {
	h := newTestEnvironmentHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/environments/nonexistent", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("envName", "nonexistent")
	rr := httptest.NewRecorder()
	h.GetEnvironment(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GetEnvironment not found: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestGetEnvironment_Success(t *testing.T) {
	h := newTestEnvironmentHandler(openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-env", Namespace: "default"},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/environments/my-env", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("envName", "my-env")
	rr := httptest.NewRecorder()
	h.GetEnvironment(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GetEnvironment success: got %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}
}

// ---- CreateEnvironment tests ----

func TestCreateEnvironment_MissingNamespace(t *testing.T) {
	h := newTestEnvironmentHandler()
	body := `{"name":"my-env"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces//environments", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	// namespaceName not set
	rr := httptest.NewRecorder()
	h.CreateEnvironment(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("CreateEnvironment missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCreateEnvironment_InvalidJSON(t *testing.T) {
	h := newTestEnvironmentHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/environments", bytes.NewReader([]byte(`{invalid}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.CreateEnvironment(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("CreateEnvironment invalid JSON: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCreateEnvironment_NoDataPlane_Fails(t *testing.T) {
	// No DataPlane exists, and no dataPlaneRef provided → ErrDataPlaneNotFound → 500 or 404
	h := newTestEnvironmentHandler()
	body := `{"name":"my-env"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/environments", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.CreateEnvironment(rr, req)
	// The service returns ErrDataPlaneNotFound (500 internal since the handler doesn't map that error)
	if rr.Code == http.StatusCreated {
		t.Errorf("CreateEnvironment with no DataPlane should not succeed, got %d", rr.Code)
	}
}

func TestCreateEnvironment_WithDataPlane_Success(t *testing.T) {
	// Provide a DataPlane in the fake store and reference it explicitly
	h := newTestEnvironmentHandlerWithDataPlane("default", "default")
	body := `{"name":"my-env","dataPlaneRef":{"name":"default","kind":"DataPlane"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/environments", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.CreateEnvironment(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("CreateEnvironment success: got %d, want %d (body: %s)", rr.Code, http.StatusCreated, rr.Body.String())
	}
}

func TestCreateEnvironment_Duplicate(t *testing.T) {
	h := newTestEnvironmentHandlerWithDataPlane("default", "default", "existing-env")
	body := `{"name":"existing-env","dataPlaneRef":{"name":"default","kind":"DataPlane"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/environments", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.CreateEnvironment(rr, req)
	if rr.Code != http.StatusConflict {
		t.Errorf("CreateEnvironment duplicate: got %d, want %d (body: %s)", rr.Code, http.StatusConflict, rr.Body.String())
	}
}

// ---- GetEnvironmentObserverURL tests ----

func TestGetEnvironmentObserverURL_MissingNamespace(t *testing.T) {
	h := newTestEnvironmentHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//environments/my-env/observer-url", nil)
	req.SetPathValue("envName", "my-env")
	rr := httptest.NewRecorder()
	h.GetEnvironmentObserverURL(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetEnvironmentObserverURL missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetEnvironmentObserverURL_MissingEnvName(t *testing.T) {
	h := newTestEnvironmentHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/environments//observer-url", nil)
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.GetEnvironmentObserverURL(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetEnvironmentObserverURL missing envName: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetEnvironmentObserverURL_EnvNotFound(t *testing.T) {
	h := newTestEnvironmentHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/environments/nonexistent/observer-url", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("envName", "nonexistent")
	rr := httptest.NewRecorder()
	h.GetEnvironmentObserverURL(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GetEnvironmentObserverURL env not found: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ---- GetRCAAgentURL tests ----

func TestGetRCAAgentURL_MissingNamespace(t *testing.T) {
	h := newTestEnvironmentHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//environments/my-env/rca-agent-url", nil)
	req.SetPathValue("envName", "my-env")
	rr := httptest.NewRecorder()
	h.GetRCAAgentURL(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetRCAAgentURL missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetRCAAgentURL_MissingEnvName(t *testing.T) {
	h := newTestEnvironmentHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/environments//rca-agent-url", nil)
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.GetRCAAgentURL(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetRCAAgentURL missing envName: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetRCAAgentURL_EnvNotFound(t *testing.T) {
	h := newTestEnvironmentHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/environments/nonexistent/rca-agent-url", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("envName", "nonexistent")
	rr := httptest.NewRecorder()
	h.GetRCAAgentURL(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GetRCAAgentURL env not found: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}
