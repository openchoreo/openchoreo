// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
)

// newTestSchemeWithCore creates a scheme with OpenChoreo types and core k8s types registered
func newTestSchemeWithCore() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = openchoreov1alpha1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	return scheme
}

// newTestNamespaceHandler creates a Handler with a fake k8s client for namespace tests
func newTestNamespaceHandler(namespaces ...corev1.Namespace) *Handler {
	scheme := newTestSchemeWithCore()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range namespaces {
		builder = builder.WithObjects(&namespaces[i])
	}
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	nsSvc := services.NewNamespaceService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.NamespaceService = nsSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- ListNamespaces tests ----

func TestListNamespaces_Empty(t *testing.T) {
	h := newTestNamespaceHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces", nil)
	rr := httptest.NewRecorder()
	h.ListNamespaces(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListNamespaces empty: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestListNamespaces_WithNamespaces(t *testing.T) {
	h := newTestNamespaceHandler(
		corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ns-a",
				Labels: map[string]string{
					"openchoreo.dev/controlplane-namespace": "true",
				},
			},
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces", nil)
	rr := httptest.NewRecorder()
	h.ListNamespaces(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListNamespaces with items: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---- GetNamespace tests ----

func TestGetNamespace_MissingName(t *testing.T) {
	h := newTestNamespaceHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/", nil)
	// namespaceName is empty
	rr := httptest.NewRecorder()
	h.GetNamespace(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetNamespace missing name: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetNamespace_NotFound(t *testing.T) {
	h := newTestNamespaceHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/nonexistent", nil)
	req.SetPathValue("namespaceName", "nonexistent")
	rr := httptest.NewRecorder()
	h.GetNamespace(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GetNamespace not found: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestGetNamespace_Success(t *testing.T) {
	h := newTestNamespaceHandler(corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "my-ns"},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/my-ns", nil)
	req.SetPathValue("namespaceName", "my-ns")
	rr := httptest.NewRecorder()
	h.GetNamespace(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GetNamespace success: got %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}
}

// ---- CreateNamespace tests ----

func TestCreateNamespace_InvalidJSON(t *testing.T) {
	h := newTestNamespaceHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces", bytes.NewReader([]byte(`{bad}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.CreateNamespace(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("CreateNamespace invalid JSON: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCreateNamespace_MissingName(t *testing.T) {
	h := newTestNamespaceHandler()
	body := `{"displayName":"My Namespace"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.CreateNamespace(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("CreateNamespace missing name: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCreateNamespace_Success(t *testing.T) {
	h := newTestNamespaceHandler()
	body := `{"name":"new-ns","displayName":"New Namespace"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.CreateNamespace(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("CreateNamespace success: got %d, want %d (body: %s)", rr.Code, http.StatusCreated, rr.Body.String())
	}
}

func TestCreateNamespace_Duplicate(t *testing.T) {
	h := newTestNamespaceHandler(corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "existing-ns"},
	})
	body := `{"name":"existing-ns"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.CreateNamespace(rr, req)
	if rr.Code != http.StatusConflict {
		t.Errorf("CreateNamespace duplicate: got %d, want %d (body: %s)", rr.Code, http.StatusConflict, rr.Body.String())
	}
}
