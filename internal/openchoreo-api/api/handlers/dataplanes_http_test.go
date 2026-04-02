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
	dataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/dataplane"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

// dataPlaneBundle holds the real HTTP handler wired to a fake K8s client so
// tests can both drive the handler through HTTP and inspect the resulting K8s
// state.
type dataPlaneBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newDataPlaneBundle builds a dataPlaneBundle seeded with the given objects and
// using the supplied PDP for authorization decisions.
func newDataPlaneBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) dataPlaneBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := dataplanesvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{DataPlaneService: svc}
	return dataPlaneBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedDataPlane is a convenience constructor for an openchoreov1alpha1.DataPlane object.
func seedDataPlane(name string) *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
	}
}

// --- List ---

func TestDataPlaneHTTPList(t *testing.T) {
	bundle := newDataPlaneBundle(t, []client.Object{
		seedDataPlane("dp-a"),
		seedDataPlane("dp-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/dataplanes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.DataPlaneList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded data planes")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"dp-a", "dp-b"}, names)

	// Concern 2: response must conform to the OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestDataPlaneHTTPListEmpty(t *testing.T) {
	bundle := newDataPlaneBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/dataplanes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.DataPlaneList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestDataPlaneHTTPGet(t *testing.T) {
	bundle := newDataPlaneBundle(t, []client.Object{seedDataPlane("my-dp")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/dataplanes/my-dp", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.DataPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "my-dp", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestDataPlaneHTTPGetNotFound(t *testing.T) {
	bundle := newDataPlaneBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/dataplanes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDataPlaneHTTPGetForbidden(t *testing.T) {
	bundle := newDataPlaneBundle(t, []client.Object{seedDataPlane("my-dp")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/dataplanes/my-dp", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestDataPlaneHTTPCreate(t *testing.T) {
	bundle := newDataPlaneBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.DataPlane{
		Metadata: gen.ObjectMeta{Name: "new-dp"},
	})
	req, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/dataplanes", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.DataPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "new-dp", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sObj openchoreov1alpha1.DataPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "new-dp", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "data plane must be persisted to K8s after creation")
	assert.Equal(t, "new-dp", k8sObj.Name)

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestDataPlaneHTTPCreateEmptyName(t *testing.T) {
	bundle := newDataPlaneBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.DataPlane{
		Metadata: gen.ObjectMeta{Name: "  "},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/dataplanes", body)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDataPlaneHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newDataPlaneBundle(t, []client.Object{seedDataPlane("existing-dp")}, &allowAllPDP{})

	body, _ := json.Marshal(gen.DataPlane{
		Metadata: gen.ObjectMeta{Name: "existing-dp"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/dataplanes", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestDataPlaneHTTPCreateForbidden(t *testing.T) {
	bundle := newDataPlaneBundle(t, nil, &denyAllPDP{})

	body, _ := json.Marshal(gen.DataPlane{
		Metadata: gen.ObjectMeta{Name: "new-dp"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/dataplanes", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestDataPlaneHTTPUpdate(t *testing.T) {
	bundle := newDataPlaneBundle(t, []client.Object{seedDataPlane("my-dp")}, &allowAllPDP{})

	// Include a label so we can assert the updated value is persisted.
	body, _ := json.Marshal(gen.DataPlane{
		Metadata: gen.ObjectMeta{
			Name:   "my-dp",
			Labels: &map[string]string{"tier": "updated"},
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/dataplanes/my-dp", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.DataPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "my-dp", resp.Metadata.Name)

	// Concern 3: verify the label mutation is reflected in the fake K8s store.
	var k8sObj openchoreov1alpha1.DataPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "my-dp", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "data plane must still exist in K8s after update")
	assert.Equal(t, "updated", k8sObj.Labels["tier"],
		"updated label must be persisted to K8s")

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestDataPlaneHTTPUpdateNotFound(t *testing.T) {
	bundle := newDataPlaneBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.DataPlane{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/dataplanes/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDataPlaneHTTPUpdateForbidden(t *testing.T) {
	bundle := newDataPlaneBundle(t, []client.Object{seedDataPlane("my-dp")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.DataPlane{Metadata: gen.ObjectMeta{Name: "my-dp"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/dataplanes/my-dp", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestDataPlaneHTTPDelete(t *testing.T) {
	bundle := newDataPlaneBundle(t, []client.Object{seedDataPlane("my-dp")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/dataplanes/my-dp", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.DataPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "my-dp", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"data plane must be removed from K8s after deletion, got err: %v", err)
}

func TestDataPlaneHTTPDeleteNotFound(t *testing.T) {
	bundle := newDataPlaneBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/dataplanes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDataPlaneHTTPDeleteForbidden(t *testing.T) {
	bundle := newDataPlaneBundle(t, []client.Object{seedDataPlane("my-dp")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/dataplanes/my-dp", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
