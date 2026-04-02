// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
)

// projectBundle holds the real HTTP handler wired to a fake K8s client so tests
// can both drive the handler through HTTP and inspect the resulting K8s state.
type projectBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newProjectBundle builds a projectBundle seeded with the given objects and using
// the supplied PDP for authorization decisions.
func newProjectBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) projectBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := projectsvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{ProjectService: svc}
	return projectBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedProject is a convenience constructor for an openchoreov1alpha1.Project object.
// A DeploymentPipelineRef is set to satisfy the OpenAPI schema's minLength constraint
// on spec.deploymentPipelineRef.name.
func seedProject(name string) *openchoreov1alpha1.Project {
	return &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
				Kind: openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline,
				Name: "default",
			},
		},
	}
}

// --- List ---

func TestProjectHTTPList(t *testing.T) {
	bundle := newProjectBundle(t, []client.Object{
		seedProject("proj-a"),
		seedProject("proj-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/projects", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ProjectList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded projects")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"proj-a", "proj-b"}, names)

	// Concern 2: response must conform to the OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestProjectHTTPListEmpty(t *testing.T) {
	bundle := newProjectBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/projects", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ProjectList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestProjectHTTPGet(t *testing.T) {
	bundle := newProjectBundle(t, []client.Object{seedProject("my-proj")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/projects/my-proj", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Project
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "my-proj", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestProjectHTTPGetNotFound(t *testing.T) {
	bundle := newProjectBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/projects/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProjectHTTPGetForbidden(t *testing.T) {
	bundle := newProjectBundle(t, []client.Object{seedProject("my-proj")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/projects/my-proj", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestProjectHTTPCreate(t *testing.T) {
	bundle := newProjectBundle(t, nil, &allowAllPDP{})

	kind := gen.ProjectSpecDeploymentPipelineRefKindDeploymentPipeline
	body, _ := json.Marshal(gen.Project{
		Metadata: gen.ObjectMeta{Name: "new-proj"},
		Spec: &gen.ProjectSpec{
			DeploymentPipelineRef: &struct {
				Kind *gen.ProjectSpecDeploymentPipelineRefKind `json:"kind,omitempty"`
				Name string                                    `json:"name"`
			}{Kind: &kind, Name: "default"},
		},
	})
	req, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/projects", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Project
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "new-proj", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sObj openchoreov1alpha1.Project
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "new-proj", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "project must be persisted to K8s after creation")
	assert.Equal(t, "new-proj", k8sObj.Name)
	assert.Equal(t, "default", k8sObj.Spec.DeploymentPipelineRef.Name,
		"deployment pipeline ref name must be persisted to K8s")

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestProjectHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newProjectBundle(t, []client.Object{seedProject("existing-proj")}, &allowAllPDP{})

	body, _ := json.Marshal(gen.Project{
		Metadata: gen.ObjectMeta{Name: "existing-proj"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/projects", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestProjectHTTPCreateForbidden(t *testing.T) {
	bundle := newProjectBundle(t, nil, &denyAllPDP{})

	body, _ := json.Marshal(gen.Project{
		Metadata: gen.ObjectMeta{Name: "new-proj"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/projects", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestProjectHTTPUpdate(t *testing.T) {
	bundle := newProjectBundle(t, []client.Object{seedProject("my-proj")}, &allowAllPDP{})

	// Include a label so we can assert the updated value is persisted.
	kind := gen.ProjectSpecDeploymentPipelineRefKindDeploymentPipeline
	body, _ := json.Marshal(gen.Project{
		Metadata: gen.ObjectMeta{
			Name:   "my-proj",
			Labels: &map[string]string{"tier": "updated"},
		},
		Spec: &gen.ProjectSpec{
			DeploymentPipelineRef: &struct {
				Kind *gen.ProjectSpecDeploymentPipelineRefKind `json:"kind,omitempty"`
				Name string                                    `json:"name"`
			}{Kind: &kind, Name: "default"},
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/projects/my-proj", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Project
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "my-proj", resp.Metadata.Name)

	// Concern 3: verify the label mutation is reflected in the fake K8s store.
	var k8sObj openchoreov1alpha1.Project
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "my-proj", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "project must still exist in K8s after update")
	assert.Equal(t, "updated", k8sObj.Labels["tier"],
		"updated label must be persisted to K8s")

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestProjectHTTPUpdateNotFound(t *testing.T) {
	bundle := newProjectBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.Project{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/projects/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProjectHTTPUpdateForbidden(t *testing.T) {
	bundle := newProjectBundle(t, []client.Object{seedProject("my-proj")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.Project{Metadata: gen.ObjectMeta{Name: "my-proj"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/projects/my-proj", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestProjectHTTPDelete(t *testing.T) {
	bundle := newProjectBundle(t, []client.Object{seedProject("my-proj")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/projects/my-proj", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.Project
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "my-proj", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"project must be removed from K8s after deletion, got err: %v", err)
}

func TestProjectHTTPDeleteNotFound(t *testing.T) {
	bundle := newProjectBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/projects/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProjectHTTPDeleteForbidden(t *testing.T) {
	bundle := newProjectBundle(t, []client.Object{seedProject("my-proj")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/projects/my-proj", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
