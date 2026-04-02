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
	clusterdataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterdataplane"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

// cdpBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type cdpBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newCDPBundle builds a cdpBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newCDPBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) cdpBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := clusterdataplanesvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{ClusterDataPlaneService: svc}
	return cdpBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedCDP is a convenience constructor for an openchoreov1alpha1.ClusterDataPlane
// object. ClusterDataPlane is cluster-scoped so no Namespace is set.
func seedCDP(name string) *openchoreov1alpha1.ClusterDataPlane {
	return &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID: name,
		},
	}
}

// --- List ---

func TestClusterDataPlaneHTTPList(t *testing.T) {
	bundle := newCDPBundle(t, []client.Object{
		seedCDP("cdp-a"),
		seedCDP("cdp-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/clusterdataplanes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterDataPlaneList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded cluster data planes")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"cdp-a", "cdp-b"}, names)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestClusterDataPlaneHTTPListEmpty(t *testing.T) {
	bundle := newCDPBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/clusterdataplanes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterDataPlaneList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestClusterDataPlaneHTTPGet(t *testing.T) {
	bundle := newCDPBundle(t, []client.Object{seedCDP("cdp-1")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/clusterdataplanes/cdp-1", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterDataPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "cdp-1", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestClusterDataPlaneHTTPGetNotFound(t *testing.T) {
	bundle := newCDPBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/clusterdataplanes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestClusterDataPlaneHTTPGetForbidden(t *testing.T) {
	bundle := newCDPBundle(t, []client.Object{seedCDP("cdp-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/clusterdataplanes/cdp-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestClusterDataPlaneHTTPCreate(t *testing.T) {
	bundle := newCDPBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.ClusterDataPlane{
		Metadata: gen.ObjectMeta{Name: "new-cdp"},
	})
	req, rec := doRequest(t, bundle.handler, http.MethodPost, "/api/v1/clusterdataplanes", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterDataPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "new-cdp", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	// ClusterDataPlane is cluster-scoped so no Namespace in the lookup key.
	var k8sCDP openchoreov1alpha1.ClusterDataPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "new-cdp"}, &k8sCDP)
	require.NoError(t, err, "cluster data plane must be persisted to K8s after creation")
	assert.Equal(t, "new-cdp", k8sCDP.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestClusterDataPlaneHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newCDPBundle(t, []client.Object{seedCDP("cdp-1")}, &allowAllPDP{})

	body, _ := json.Marshal(gen.ClusterDataPlane{
		Metadata: gen.ObjectMeta{Name: "cdp-1"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost, "/api/v1/clusterdataplanes", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestClusterDataPlaneHTTPCreateForbidden(t *testing.T) {
	bundle := newCDPBundle(t, nil, &denyAllPDP{})

	body, _ := json.Marshal(gen.ClusterDataPlane{
		Metadata: gen.ObjectMeta{Name: "new-cdp"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost, "/api/v1/clusterdataplanes", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestClusterDataPlaneHTTPUpdate(t *testing.T) {
	bundle := newCDPBundle(t, []client.Object{seedCDP("cdp-1")}, &allowAllPDP{})

	// Include a label so we can assert the updated value is persisted.
	body, _ := json.Marshal(gen.ClusterDataPlane{
		Metadata: gen.ObjectMeta{
			Name:   "cdp-1",
			Labels: &map[string]string{"tier": "prod"},
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut, "/api/v1/clusterdataplanes/cdp-1", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ClusterDataPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "cdp-1", resp.Metadata.Name)

	// Concern 3: verify the label mutation is reflected in the fake K8s store.
	var k8sCDP openchoreov1alpha1.ClusterDataPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "cdp-1"}, &k8sCDP)
	require.NoError(t, err, "cluster data plane must still exist in K8s after update")
	assert.Equal(t, "prod", k8sCDP.Labels["tier"], "updated label must be persisted to K8s")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestClusterDataPlaneHTTPUpdateNotFound(t *testing.T) {
	bundle := newCDPBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.ClusterDataPlane{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut, "/api/v1/clusterdataplanes/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestClusterDataPlaneHTTPUpdateForbidden(t *testing.T) {
	bundle := newCDPBundle(t, []client.Object{seedCDP("cdp-1")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.ClusterDataPlane{Metadata: gen.ObjectMeta{Name: "cdp-1"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut, "/api/v1/clusterdataplanes/cdp-1", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestClusterDataPlaneHTTPDelete(t *testing.T) {
	bundle := newCDPBundle(t, []client.Object{seedCDP("cdp-1")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete, "/api/v1/clusterdataplanes/cdp-1", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.ClusterDataPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "cdp-1"}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"cluster data plane must be removed from K8s after deletion, got err: %v", err)
}

func TestClusterDataPlaneHTTPDeleteNotFound(t *testing.T) {
	bundle := newCDPBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete, "/api/v1/clusterdataplanes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestClusterDataPlaneHTTPDeleteForbidden(t *testing.T) {
	bundle := newCDPBundle(t, []client.Object{seedCDP("cdp-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete, "/api/v1/clusterdataplanes/cdp-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
