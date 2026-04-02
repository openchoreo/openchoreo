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
	clusterobservabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterobservabilityplane"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

// copBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type copBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newCOPBundle builds a copBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newCOPBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) copBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := clusterobservabilityplanesvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{ClusterObservabilityPlaneService: svc}
	return copBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedCOP is a convenience constructor for an openchoreov1alpha1.ClusterObservabilityPlane
// object. ClusterObservabilityPlane is cluster-scoped so no Namespace is set.
func seedCOP(name string) *openchoreov1alpha1.ClusterObservabilityPlane {
	return &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
			PlaneID: name,
		},
	}
}

// --- List ---

func TestClusterObservabilityPlaneHTTPList(t *testing.T) {
	bundle := newCOPBundle(t, []client.Object{
		seedCOP("cop-a"),
		seedCOP("cop-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/clusterobservabilityplanes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterObservabilityPlaneList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded cluster observability planes")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"cop-a", "cop-b"}, names)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestClusterObservabilityPlaneHTTPListEmpty(t *testing.T) {
	bundle := newCOPBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/clusterobservabilityplanes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterObservabilityPlaneList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestClusterObservabilityPlaneHTTPGet(t *testing.T) {
	bundle := newCOPBundle(t, []client.Object{seedCOP("cop-1")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/clusterobservabilityplanes/cop-1", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterObservabilityPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "cop-1", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestClusterObservabilityPlaneHTTPGetNotFound(t *testing.T) {
	bundle := newCOPBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/clusterobservabilityplanes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestClusterObservabilityPlaneHTTPGetForbidden(t *testing.T) {
	bundle := newCOPBundle(t, []client.Object{seedCOP("cop-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/clusterobservabilityplanes/cop-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestClusterObservabilityPlaneHTTPCreate(t *testing.T) {
	bundle := newCOPBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.ClusterObservabilityPlane{
		Metadata: gen.ObjectMeta{Name: "new-cop"},
	})
	req, rec := doRequest(t, bundle.handler, http.MethodPost, "/api/v1/clusterobservabilityplanes", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterObservabilityPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "new-cop", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	// ClusterObservabilityPlane is cluster-scoped so no Namespace in the lookup key.
	var k8sCOP openchoreov1alpha1.ClusterObservabilityPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "new-cop"}, &k8sCOP)
	require.NoError(t, err, "cluster observability plane must be persisted to K8s after creation")
	assert.Equal(t, "new-cop", k8sCOP.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestClusterObservabilityPlaneHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newCOPBundle(t, []client.Object{seedCOP("cop-1")}, &allowAllPDP{})

	body, _ := json.Marshal(gen.ClusterObservabilityPlane{
		Metadata: gen.ObjectMeta{Name: "cop-1"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost, "/api/v1/clusterobservabilityplanes", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestClusterObservabilityPlaneHTTPCreateForbidden(t *testing.T) {
	bundle := newCOPBundle(t, nil, &denyAllPDP{})

	body, _ := json.Marshal(gen.ClusterObservabilityPlane{
		Metadata: gen.ObjectMeta{Name: "new-cop"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost, "/api/v1/clusterobservabilityplanes", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestClusterObservabilityPlaneHTTPUpdate(t *testing.T) {
	bundle := newCOPBundle(t, []client.Object{seedCOP("cop-1")}, &allowAllPDP{})

	// Include a label so we can assert the updated value is persisted.
	body, _ := json.Marshal(gen.ClusterObservabilityPlane{
		Metadata: gen.ObjectMeta{
			Name:   "cop-1",
			Labels: &map[string]string{"tier": "prod"},
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/clusterobservabilityplanes/cop-1", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterObservabilityPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "cop-1", resp.Metadata.Name)

	// Concern 3: verify the label mutation is reflected in the fake K8s store.
	var k8sCOP openchoreov1alpha1.ClusterObservabilityPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "cop-1"}, &k8sCOP)
	require.NoError(t, err, "cluster observability plane must still exist in K8s after update")
	assert.Equal(t, "prod", k8sCOP.Labels["tier"], "updated label must be persisted to K8s")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestClusterObservabilityPlaneHTTPUpdateNotFound(t *testing.T) {
	bundle := newCOPBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.ClusterObservabilityPlane{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/clusterobservabilityplanes/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestClusterObservabilityPlaneHTTPUpdateForbidden(t *testing.T) {
	bundle := newCOPBundle(t, []client.Object{seedCOP("cop-1")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.ClusterObservabilityPlane{Metadata: gen.ObjectMeta{Name: "cop-1"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/clusterobservabilityplanes/cop-1", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestClusterObservabilityPlaneHTTPDelete(t *testing.T) {
	bundle := newCOPBundle(t, []client.Object{seedCOP("cop-1")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/clusterobservabilityplanes/cop-1", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.ClusterObservabilityPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "cop-1"}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"cluster observability plane must be removed from K8s after deletion, got err: %v", err)
}

func TestClusterObservabilityPlaneHTTPDeleteNotFound(t *testing.T) {
	bundle := newCOPBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/clusterobservabilityplanes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestClusterObservabilityPlaneHTTPDeleteForbidden(t *testing.T) {
	bundle := newCOPBundle(t, []client.Object{seedCOP("cop-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/clusterobservabilityplanes/cop-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
