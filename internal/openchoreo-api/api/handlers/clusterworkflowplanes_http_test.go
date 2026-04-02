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
	clusterworkflowplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterworkflowplane"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

// cwpBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type cwpBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newCWPBundle builds a cwpBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newCWPBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) cwpBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := clusterworkflowplanesvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{ClusterWorkflowPlaneService: svc}
	return cwpBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedCWP is a convenience constructor for an openchoreov1alpha1.ClusterWorkflowPlane
// object. ClusterWorkflowPlane is cluster-scoped so no Namespace is set.
func seedCWP(name string) *openchoreov1alpha1.ClusterWorkflowPlane {
	return &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			PlaneID: name,
		},
	}
}

// TestClusterWorkflowPlaneRoutesInSpec verifies that all ClusterWorkflowPlane endpoints
// are registered in the OpenAPI spec, so assertConformsToSpec does not silently skip
// schema validation due to a missing route.
func TestClusterWorkflowPlaneRoutesInSpec(t *testing.T) {
	requireRouteInSpec(t, http.MethodGet, "/api/v1/clusterworkflowplanes")
	requireRouteInSpec(t, http.MethodPost, "/api/v1/clusterworkflowplanes")
	requireRouteInSpec(t, http.MethodGet, "/api/v1/clusterworkflowplanes/cwp-1")
	requireRouteInSpec(t, http.MethodPut, "/api/v1/clusterworkflowplanes/cwp-1")
	requireRouteInSpec(t, http.MethodDelete, "/api/v1/clusterworkflowplanes/cwp-1")
}

// --- List ---

func TestClusterWorkflowPlaneHTTPList(t *testing.T) {
	bundle := newCWPBundle(t, []client.Object{
		seedCWP("cwp-a"),
		seedCWP("cwp-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/clusterworkflowplanes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterWorkflowPlaneList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded cluster workflow planes")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"cwp-a", "cwp-b"}, names)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestClusterWorkflowPlaneHTTPListEmpty(t *testing.T) {
	bundle := newCWPBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/clusterworkflowplanes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterWorkflowPlaneList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestClusterWorkflowPlaneHTTPGet(t *testing.T) {
	bundle := newCWPBundle(t, []client.Object{seedCWP("cwp-1")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/clusterworkflowplanes/cwp-1", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterWorkflowPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "cwp-1", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestClusterWorkflowPlaneHTTPGetNotFound(t *testing.T) {
	bundle := newCWPBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/clusterworkflowplanes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestClusterWorkflowPlaneHTTPGetForbidden(t *testing.T) {
	bundle := newCWPBundle(t, []client.Object{seedCWP("cwp-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/clusterworkflowplanes/cwp-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestClusterWorkflowPlaneHTTPCreate(t *testing.T) {
	bundle := newCWPBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.ClusterWorkflowPlane{
		Metadata: gen.ObjectMeta{Name: "new-cwp"},
	})
	req, rec := doRequest(t, bundle.handler, http.MethodPost, "/api/v1/clusterworkflowplanes", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterWorkflowPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "new-cwp", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	// ClusterWorkflowPlane is cluster-scoped so no Namespace in the lookup key.
	var k8sCWP openchoreov1alpha1.ClusterWorkflowPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "new-cwp"}, &k8sCWP)
	require.NoError(t, err, "cluster workflow plane must be persisted to K8s after creation")
	assert.Equal(t, "new-cwp", k8sCWP.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestClusterWorkflowPlaneHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newCWPBundle(t, []client.Object{seedCWP("cwp-1")}, &allowAllPDP{})

	body, _ := json.Marshal(gen.ClusterWorkflowPlane{
		Metadata: gen.ObjectMeta{Name: "cwp-1"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost, "/api/v1/clusterworkflowplanes", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestClusterWorkflowPlaneHTTPCreateForbidden(t *testing.T) {
	bundle := newCWPBundle(t, nil, &denyAllPDP{})

	body, _ := json.Marshal(gen.ClusterWorkflowPlane{
		Metadata: gen.ObjectMeta{Name: "new-cwp"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost, "/api/v1/clusterworkflowplanes", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestClusterWorkflowPlaneHTTPUpdate(t *testing.T) {
	bundle := newCWPBundle(t, []client.Object{seedCWP("cwp-1")}, &allowAllPDP{})

	// Include a label so we can assert the updated value is persisted.
	body, _ := json.Marshal(gen.ClusterWorkflowPlane{
		Metadata: gen.ObjectMeta{
			Name:   "cwp-1",
			Labels: &map[string]string{"tier": "prod"},
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/clusterworkflowplanes/cwp-1", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterWorkflowPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "cwp-1", resp.Metadata.Name)

	// Concern 3: verify the label mutation is reflected in the fake K8s store.
	var k8sCWP openchoreov1alpha1.ClusterWorkflowPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "cwp-1"}, &k8sCWP)
	require.NoError(t, err, "cluster workflow plane must still exist in K8s after update")
	assert.Equal(t, "prod", k8sCWP.Labels["tier"], "updated label must be persisted to K8s")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestClusterWorkflowPlaneHTTPUpdateNotFound(t *testing.T) {
	bundle := newCWPBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/clusterworkflowplanes/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestClusterWorkflowPlaneHTTPUpdateForbidden(t *testing.T) {
	bundle := newCWPBundle(t, []client.Object{seedCWP("cwp-1")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "cwp-1"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/clusterworkflowplanes/cwp-1", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestClusterWorkflowPlaneHTTPDelete(t *testing.T) {
	bundle := newCWPBundle(t, []client.Object{seedCWP("cwp-1")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/clusterworkflowplanes/cwp-1", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.ClusterWorkflowPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "cwp-1"}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"cluster workflow plane must be removed from K8s after deletion, got err: %v", err)
}

func TestClusterWorkflowPlaneHTTPDeleteNotFound(t *testing.T) {
	bundle := newCWPBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/clusterworkflowplanes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestClusterWorkflowPlaneHTTPDeleteForbidden(t *testing.T) {
	bundle := newCWPBundle(t, []client.Object{seedCWP("cwp-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/clusterworkflowplanes/cwp-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
