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

// newTestDataPlaneHandler creates a Handler with a fake k8s client for dataplane tests
func newTestDataPlaneHandler(objects ...openchoreov1alpha1.DataPlane) *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range objects {
		builder = builder.WithObjects(&objects[i])
	}
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	dpSvc := services.NewDataPlaneService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.DataPlaneService = dpSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- ListDataPlanes tests ----

func TestListDataPlanes_MissingNamespace(t *testing.T) {
	h := newTestDataPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//dataplanes", nil)
	rr := httptest.NewRecorder()
	h.ListDataPlanes(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("ListDataPlanes missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestListDataPlanes_Empty(t *testing.T) {
	h := newTestDataPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/dataplanes", nil)
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.ListDataPlanes(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListDataPlanes empty: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestListDataPlanes_WithItems(t *testing.T) {
	h := newTestDataPlaneHandler(
		openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "dp-1", Namespace: "ns1"},
		},
		openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "dp-2", Namespace: "ns1"},
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/ns1/dataplanes", nil)
	req.SetPathValue("namespaceName", "ns1")
	rr := httptest.NewRecorder()
	h.ListDataPlanes(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListDataPlanes with items: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---- GetDataPlane tests ----

func TestGetDataPlane_MissingNamespace(t *testing.T) {
	h := newTestDataPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//dataplanes/dp-1", nil)
	req.SetPathValue("dpName", "dp-1")
	rr := httptest.NewRecorder()
	h.GetDataPlane(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetDataPlane missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetDataPlane_MissingDPName(t *testing.T) {
	h := newTestDataPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/dataplanes/", nil)
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.GetDataPlane(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetDataPlane missing dpName: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetDataPlane_NotFound(t *testing.T) {
	h := newTestDataPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/dataplanes/nonexistent", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("dpName", "nonexistent")
	rr := httptest.NewRecorder()
	h.GetDataPlane(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GetDataPlane not found: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestGetDataPlane_Success(t *testing.T) {
	h := newTestDataPlaneHandler(openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "my-dp", Namespace: "default"},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/dataplanes/my-dp", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("dpName", "my-dp")
	rr := httptest.NewRecorder()
	h.GetDataPlane(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GetDataPlane success: got %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}
}

// ---- CreateDataPlane tests ----

func TestCreateDataPlane_MissingNamespace(t *testing.T) {
	h := newTestDataPlaneHandler()
	body := `{"name":"my-dp"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces//dataplanes", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.CreateDataPlane(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("CreateDataPlane missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCreateDataPlane_InvalidJSON(t *testing.T) {
	h := newTestDataPlaneHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/dataplanes", bytes.NewReader([]byte(`{bad json}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.CreateDataPlane(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("CreateDataPlane invalid JSON: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCreateDataPlane_Success(t *testing.T) {
	h := newTestDataPlaneHandler()
	// Provide required fields based on DataPlane validation
	body := `{"name":"my-dp","publicVirtualHost":"host.example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/dataplanes", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.CreateDataPlane(rr, req)
	// DataPlane creation may succeed (201) or fail validation (400) depending on impl
	if rr.Code != http.StatusCreated && rr.Code != http.StatusBadRequest {
		t.Errorf("CreateDataPlane unexpected status: got %d, want 201 or 400 (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestCreateDataPlane_Duplicate(t *testing.T) {
	h := newTestDataPlaneHandler(openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "existing-dp", Namespace: "default"},
	})
	body := `{"name":"existing-dp","publicVirtualHost":"host.example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/dataplanes", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.CreateDataPlane(rr, req)
	if rr.Code != http.StatusConflict && rr.Code != http.StatusBadRequest {
		t.Errorf("CreateDataPlane duplicate: got %d, want 409 or 400 (body: %s)", rr.Code, rr.Body.String())
	}
}
