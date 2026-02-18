// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
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

// ---- ComponentType handler helpers ----

func newTestComponentTypeHandler(cts ...openchoreov1alpha1.ComponentType) *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range cts {
		builder = builder.WithObjects(&cts[i])
	}
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	ctSvc := services.NewComponentTypeService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.ComponentTypeService = ctSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- ListComponentTypes tests ----

func TestListComponentTypes_MissingNamespace(t *testing.T) {
	h := newTestComponentTypeHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//componenttypes", nil)
	rr := httptest.NewRecorder()
	h.ListComponentTypes(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("ListComponentTypes missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestListComponentTypes_Empty(t *testing.T) {
	h := newTestComponentTypeHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/componenttypes", nil)
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.ListComponentTypes(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListComponentTypes empty: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestListComponentTypes_WithItems(t *testing.T) {
	h := newTestComponentTypeHandler(
		openchoreov1alpha1.ComponentType{
			ObjectMeta: metav1.ObjectMeta{Name: "ct-1", Namespace: "ns1"},
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/ns1/componenttypes", nil)
	req.SetPathValue("namespaceName", "ns1")
	rr := httptest.NewRecorder()
	h.ListComponentTypes(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListComponentTypes with items: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---- GetComponentTypeSchema tests ----

func TestGetComponentTypeSchema_MissingParams(t *testing.T) {
	h := newTestComponentTypeHandler()
	tests := []struct {
		name          string
		namespaceName string
		ctName        string
	}{
		{"missing both", "", ""},
		{"missing namespace", "", "ct-1"},
		{"missing ct name", "default", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/"+tt.namespaceName+"/componenttypes/"+tt.ctName+"/schema", nil)
			req.SetPathValue("namespaceName", tt.namespaceName)
			req.SetPathValue("ctName", tt.ctName)
			rr := httptest.NewRecorder()
			h.GetComponentTypeSchema(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("GetComponentTypeSchema %s: got %d, want %d", tt.name, rr.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestGetComponentTypeSchema_NotFound(t *testing.T) {
	h := newTestComponentTypeHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/componenttypes/nonexistent/schema", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("ctName", "nonexistent")
	rr := httptest.NewRecorder()
	h.GetComponentTypeSchema(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GetComponentTypeSchema not found: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ---- Trait handler helpers ----

func newTestTraitHandler(traits ...openchoreov1alpha1.Trait) *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range traits {
		builder = builder.WithObjects(&traits[i])
	}
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	traitSvc := services.NewTraitService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.TraitService = traitSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- ListTraits tests ----

func TestListTraits_MissingNamespace(t *testing.T) {
	h := newTestTraitHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//traits", nil)
	rr := httptest.NewRecorder()
	h.ListTraits(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("ListTraits missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestListTraits_Empty(t *testing.T) {
	h := newTestTraitHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/traits", nil)
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.ListTraits(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListTraits empty: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestListTraits_WithItems(t *testing.T) {
	h := newTestTraitHandler(
		openchoreov1alpha1.Trait{
			ObjectMeta: metav1.ObjectMeta{Name: "trait-1", Namespace: "ns1"},
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/ns1/traits", nil)
	req.SetPathValue("namespaceName", "ns1")
	rr := httptest.NewRecorder()
	h.ListTraits(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListTraits with items: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---- GetTraitSchema tests ----

func TestGetTraitSchema_MissingParams(t *testing.T) {
	h := newTestTraitHandler()
	tests := []struct {
		name          string
		namespaceName string
		traitName     string
	}{
		{"missing both", "", ""},
		{"missing namespace", "", "trait-1"},
		{"missing trait name", "default", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/"+tt.namespaceName+"/traits/"+tt.traitName+"/schema", nil)
			req.SetPathValue("namespaceName", tt.namespaceName)
			req.SetPathValue("traitName", tt.traitName)
			rr := httptest.NewRecorder()
			h.GetTraitSchema(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("GetTraitSchema %s: got %d, want %d", tt.name, rr.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestGetTraitSchema_NotFound(t *testing.T) {
	h := newTestTraitHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/traits/nonexistent/schema", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("traitName", "nonexistent")
	rr := httptest.NewRecorder()
	h.GetTraitSchema(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GetTraitSchema not found: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ---- ObservabilityPlane handler helpers ----

func newTestObservabilityPlaneHandler(ops ...openchoreov1alpha1.ObservabilityPlane) *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range ops {
		builder = builder.WithObjects(&ops[i])
	}
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	opSvc := services.NewObservabilityPlaneService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.ObservabilityPlaneService = opSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- ListObservabilityPlanes tests ----

func TestListObservabilityPlanes_MissingNamespace(t *testing.T) {
	h := newTestObservabilityPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//observabilityplanes", nil)
	rr := httptest.NewRecorder()
	h.ListObservabilityPlanes(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("ListObservabilityPlanes missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestListObservabilityPlanes_Empty(t *testing.T) {
	h := newTestObservabilityPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/observabilityplanes", nil)
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.ListObservabilityPlanes(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListObservabilityPlanes empty: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---- ClusterBuildPlane handler helpers ----

func newTestClusterBuildPlaneHandler(cbps ...openchoreov1alpha1.ClusterBuildPlane) *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range cbps {
		builder = builder.WithObjects(&cbps[i])
	}
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	cbpSvc := services.NewClusterBuildPlaneService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.ClusterBuildPlaneService = cbpSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- ListClusterBuildPlanes tests ----

func TestListClusterBuildPlanes_Empty(t *testing.T) {
	h := newTestClusterBuildPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusterbuildplanes", nil)
	rr := httptest.NewRecorder()
	h.ListClusterBuildPlanes(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListClusterBuildPlanes empty: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestListClusterBuildPlanes_WithItems(t *testing.T) {
	h := newTestClusterBuildPlaneHandler(
		openchoreov1alpha1.ClusterBuildPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "cbp-1"},
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusterbuildplanes", nil)
	rr := httptest.NewRecorder()
	h.ListClusterBuildPlanes(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListClusterBuildPlanes with items: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---- SecretReference handler helpers ----

func newTestSecretReferenceHandler(refs ...openchoreov1alpha1.SecretReference) *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range refs {
		builder = builder.WithObjects(&refs[i])
	}
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	srSvc := services.NewSecretReferenceService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.SecretReferenceService = srSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- ListSecretReferences tests ----

func TestListSecretReferences_MissingNamespace(t *testing.T) {
	h := newTestSecretReferenceHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//secret-references", nil)
	rr := httptest.NewRecorder()
	h.ListSecretReferences(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("ListSecretReferences missing namespace: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestListSecretReferences_Empty(t *testing.T) {
	h := newTestSecretReferenceHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/secret-references", nil)
	req.SetPathValue("namespaceName", "default")
	rr := httptest.NewRecorder()
	h.ListSecretReferences(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListSecretReferences empty: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---- DeploymentPipeline handler helpers ----

func newTestDeploymentPipelineHandler(projects []*openchoreov1alpha1.Project, pipelines []*openchoreov1alpha1.DeploymentPipeline) *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range projects {
		builder = builder.WithObjects(projects[i])
	}
	for i := range pipelines {
		builder = builder.WithObjects(pipelines[i])
	}
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	projectSvc := services.NewProjectService(fakeClient, slog.Default(), pdp)
	pipelineSvc := services.NewDeploymentPipelineService(fakeClient, projectSvc, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.DeploymentPipelineService = pipelineSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- GetProjectDeploymentPipeline tests ----

func TestGetProjectDeploymentPipeline_MissingParams(t *testing.T) {
	h := newTestDeploymentPipelineHandler(nil, nil)
	tests := []struct {
		name          string
		namespaceName string
		projectName   string
	}{
		{"missing both", "", ""},
		{"missing namespace", "", "my-project"},
		{"missing project", "default", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/"+tt.namespaceName+"/projects/"+tt.projectName+"/deployment-pipeline", nil)
			req.SetPathValue("namespaceName", tt.namespaceName)
			req.SetPathValue("projectName", tt.projectName)
			rr := httptest.NewRecorder()
			h.GetProjectDeploymentPipeline(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("GetProjectDeploymentPipeline %s: got %d, want %d", tt.name, rr.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestGetProjectDeploymentPipeline_ProjectNotFound(t *testing.T) {
	h := newTestDeploymentPipelineHandler(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/projects/nonexistent/deployment-pipeline", nil)
	req.SetPathValue("namespaceName", "default")
	req.SetPathValue("projectName", "nonexistent")
	rr := httptest.NewRecorder()
	h.GetProjectDeploymentPipeline(rr, req)
	// Should be 404 since project doesn't exist
	if rr.Code != http.StatusNotFound {
		t.Errorf("GetProjectDeploymentPipeline project not found: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ---- ClusterObservabilityPlane handler helpers ----

func newTestClusterObservabilityPlaneHandler(cops ...openchoreov1alpha1.ClusterObservabilityPlane) *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range cops {
		builder = builder.WithObjects(&cops[i])
	}
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	copSvc := services.NewClusterObservabilityPlaneService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.ClusterObservabilityPlaneService = copSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- ListClusterObservabilityPlanes tests ----

func TestListClusterObservabilityPlanes_Empty(t *testing.T) {
	h := newTestClusterObservabilityPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusterobservabilityplanes", nil)
	rr := httptest.NewRecorder()
	h.ListClusterObservabilityPlanes(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListClusterObservabilityPlanes empty: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestListClusterObservabilityPlanes_WithItems(t *testing.T) {
	h := newTestClusterObservabilityPlaneHandler(
		openchoreov1alpha1.ClusterObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "cop-1"},
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusterobservabilityplanes", nil)
	rr := httptest.NewRecorder()
	h.ListClusterObservabilityPlanes(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListClusterObservabilityPlanes with items: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---- ClusterDataPlane handler helpers ----

func newTestClusterDataPlaneHandler() *Handler {
	scheme := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	fakeClient := builder.Build()
	pdp := &mockPDPAllow{}
	cdpSvc := services.NewClusterDataPlaneService(fakeClient, slog.Default(), pdp)
	svcs := &services.Services{}
	svcs.ClusterDataPlaneService = cdpSvc
	return New(svcs, &config.Config{}, slog.Default())
}

// ---- ListClusterDataPlanes tests ----

func TestListClusterDataPlanes_Empty(t *testing.T) {
	h := newTestClusterDataPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusterdataplanes", nil)
	rr := httptest.NewRecorder()
	h.ListClusterDataPlanes(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListClusterDataPlanes empty: got %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestListClusterDataPlanes_InvalidLimit(t *testing.T) {
	h := newTestClusterDataPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusterdataplanes?limit=abc", nil)
	rr := httptest.NewRecorder()
	h.ListClusterDataPlanes(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("ListClusterDataPlanes invalid limit: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestListClusterDataPlanes_LimitTooHigh(t *testing.T) {
	h := newTestClusterDataPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusterdataplanes?limit=999", nil)
	rr := httptest.NewRecorder()
	h.ListClusterDataPlanes(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("ListClusterDataPlanes limit too high: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// ---- GetClusterDataPlane tests ----

func TestGetClusterDataPlane_MissingName(t *testing.T) {
	h := newTestClusterDataPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusterdataplanes/", nil)
	rr := httptest.NewRecorder()
	h.GetClusterDataPlane(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetClusterDataPlane missing name: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetClusterDataPlane_NotFound(t *testing.T) {
	h := newTestClusterDataPlaneHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusterdataplanes/nonexistent", nil)
	req.SetPathValue("cdpName", "nonexistent")
	rr := httptest.NewRecorder()
	h.GetClusterDataPlane(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("GetClusterDataPlane not found: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}
