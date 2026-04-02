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
	observabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityplane"
)

// obsPBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type obsPBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newObsPBundle builds an obsPBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newObsPBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) obsPBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := observabilityplanesvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{ObservabilityPlaneService: svc}
	return obsPBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedObservabilityPlane is a convenience constructor for an
// openchoreov1alpha1.ObservabilityPlane object.
func seedObservabilityPlane(name string) *openchoreov1alpha1.ObservabilityPlane {
	return &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
	}
}

// --- List ---

func TestObservabilityPlaneHTTPList(t *testing.T) {
	bundle := newObsPBundle(t, []client.Object{
		seedObservabilityPlane("obs-a"),
		seedObservabilityPlane("obs-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ObservabilityPlaneList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded observability planes")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"obs-a", "obs-b"}, names)

	// Concern 2: response must conform to the OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestObservabilityPlaneHTTPListEmpty(t *testing.T) {
	bundle := newObsPBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ObservabilityPlaneList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestObservabilityPlaneHTTPGet(t *testing.T) {
	bundle := newObsPBundle(t, []client.Object{seedObservabilityPlane("my-obs")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes/my-obs", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ObservabilityPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "my-obs", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestObservabilityPlaneHTTPGetNotFound(t *testing.T) {
	bundle := newObsPBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestObservabilityPlaneHTTPGetForbidden(t *testing.T) {
	bundle := newObsPBundle(t, []client.Object{seedObservabilityPlane("my-obs")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes/my-obs", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestObservabilityPlaneHTTPCreate(t *testing.T) {
	bundle := newObsPBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.ObservabilityPlane{
		Metadata: gen.ObjectMeta{Name: "new-obs"},
	})
	req, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ObservabilityPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "new-obs", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sObj openchoreov1alpha1.ObservabilityPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "new-obs", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "observability plane must be persisted to K8s after creation")
	assert.Equal(t, "new-obs", k8sObj.Name)

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestObservabilityPlaneHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newObsPBundle(t, []client.Object{seedObservabilityPlane("existing-obs")}, &allowAllPDP{})

	body, _ := json.Marshal(gen.ObservabilityPlane{
		Metadata: gen.ObjectMeta{Name: "existing-obs"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestObservabilityPlaneHTTPCreateForbidden(t *testing.T) {
	bundle := newObsPBundle(t, nil, &denyAllPDP{})

	body, _ := json.Marshal(gen.ObservabilityPlane{
		Metadata: gen.ObjectMeta{Name: "new-obs"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestObservabilityPlaneHTTPUpdate(t *testing.T) {
	bundle := newObsPBundle(t, []client.Object{seedObservabilityPlane("my-obs")}, &allowAllPDP{})

	// Include a label so we can assert the updated value is persisted.
	body, _ := json.Marshal(gen.ObservabilityPlane{
		Metadata: gen.ObjectMeta{
			Name:   "my-obs",
			Labels: &map[string]string{"tier": "updated"},
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes/my-obs", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ObservabilityPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "my-obs", resp.Metadata.Name)

	// Concern 3: verify the label mutation is reflected in the fake K8s store.
	var k8sObj openchoreov1alpha1.ObservabilityPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "my-obs", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "observability plane must still exist in K8s after update")
	assert.Equal(t, "updated", k8sObj.Labels["tier"],
		"updated label must be persisted to K8s")

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestObservabilityPlaneHTTPUpdateNotFound(t *testing.T) {
	bundle := newObsPBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.ObservabilityPlane{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestObservabilityPlaneHTTPUpdateForbidden(t *testing.T) {
	bundle := newObsPBundle(t, []client.Object{seedObservabilityPlane("my-obs")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.ObservabilityPlane{Metadata: gen.ObjectMeta{Name: "my-obs"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes/my-obs", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestObservabilityPlaneHTTPDelete(t *testing.T) {
	bundle := newObsPBundle(t, []client.Object{seedObservabilityPlane("my-obs")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes/my-obs", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.ObservabilityPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "my-obs", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"observability plane must be removed from K8s after deletion, got err: %v", err)
}

func TestObservabilityPlaneHTTPDeleteNotFound(t *testing.T) {
	bundle := newObsPBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestObservabilityPlaneHTTPDeleteForbidden(t *testing.T) {
	bundle := newObsPBundle(t, []client.Object{seedObservabilityPlane("my-obs")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/observabilityplanes/my-obs", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
