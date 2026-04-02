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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	namespacesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/namespace"
)

// nsBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type nsBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newNSBundle builds an nsBundle seeded with the given objects and using the
// supplied PDP for authorization decisions. It uses newTestSchemeWithCoreV1
// because Namespace objects live in the corev1 API group.
func newNSBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) nsBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestSchemeWithCoreV1(t)).
		WithObjects(objects...).
		Build()
	svc := namespacesvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{NamespaceService: svc}
	return nsBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedNS is a convenience constructor for a corev1.Namespace with the label
// required by the namespace service to include the object in its results.
// corev1.Namespace is cluster-scoped so no Namespace is set in ObjectMeta.
func seedNS(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"openchoreo.dev/control-plane": "true",
			},
		},
	}
}

// --- List ---

func TestNamespaceHTTPList(t *testing.T) {
	bundle := newNSBundle(t, []client.Object{
		seedNS("ns-a"),
		seedNS("ns-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/namespaces", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.NamespaceList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded namespaces")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"ns-a", "ns-b"}, names)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestNamespaceHTTPListFiltersUnlabeled(t *testing.T) {
	// Only namespaces with openchoreo.dev/control-plane=true should appear.
	unlabeled := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "unlabeled-ns"},
	}
	bundle := newNSBundle(t, []client.Object{seedNS("labeled-ns"), unlabeled}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/namespaces", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.NamespaceList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Len(t, resp.Items, 1, "only labeled namespaces must appear in list")
	assert.Equal(t, "labeled-ns", resp.Items[0].Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestNamespaceHTTPListEmpty(t *testing.T) {
	bundle := newNSBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/namespaces", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.NamespaceList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestNamespaceHTTPGet(t *testing.T) {
	bundle := newNSBundle(t, []client.Object{seedNS("ns-1")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/namespaces/ns-1", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Namespace
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "ns-1", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestNamespaceHTTPGetNotFound(t *testing.T) {
	bundle := newNSBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/namespaces/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestNamespaceHTTPGetForbidden(t *testing.T) {
	bundle := newNSBundle(t, []client.Object{seedNS("ns-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet, "/api/v1/namespaces/ns-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestNamespaceHTTPCreate(t *testing.T) {
	bundle := newNSBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.Namespace{
		Metadata: gen.ObjectMeta{Name: "new-ns"},
	})
	req, rec := doRequest(t, bundle.handler, http.MethodPost, "/api/v1/namespaces", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Namespace
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "new-ns", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	// corev1.Namespace is cluster-scoped so no Namespace field in the lookup key.
	var k8sNS corev1.Namespace
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "new-ns"}, &k8sNS)
	require.NoError(t, err, "namespace must be persisted to K8s after creation")
	assert.Equal(t, "new-ns", k8sNS.Name)
	assert.Equal(t, "true", k8sNS.Labels["openchoreo.dev/control-plane"],
		"control-plane label must be stamped on namespace creation")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestNamespaceHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newNSBundle(t, []client.Object{seedNS("ns-1")}, &allowAllPDP{})

	body, _ := json.Marshal(gen.Namespace{
		Metadata: gen.ObjectMeta{Name: "ns-1"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost, "/api/v1/namespaces", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestNamespaceHTTPCreateForbidden(t *testing.T) {
	bundle := newNSBundle(t, nil, &denyAllPDP{})

	body, _ := json.Marshal(gen.Namespace{
		Metadata: gen.ObjectMeta{Name: "new-ns"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost, "/api/v1/namespaces", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestNamespaceHTTPUpdate(t *testing.T) {
	bundle := newNSBundle(t, []client.Object{seedNS("ns-1")}, &allowAllPDP{})

	// Include a display-name annotation so we can assert the updated value is persisted.
	// The namespace service only propagates openchoreo.dev/display-name and
	// openchoreo.dev/description annotations — arbitrary label changes are not applied.
	body, _ := json.Marshal(gen.Namespace{
		Metadata: gen.ObjectMeta{
			Name:        "ns-1",
			Annotations: &map[string]string{"openchoreo.dev/display-name": "Updated NS"},
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut, "/api/v1/namespaces/ns-1", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Namespace
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "ns-1", resp.Metadata.Name)

	// Concern 3: verify the annotation mutation is reflected in the fake K8s store.
	// corev1.Namespace is cluster-scoped so no Namespace field in the lookup key.
	var k8sNS corev1.Namespace
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "ns-1"}, &k8sNS)
	require.NoError(t, err, "namespace must still exist in K8s after update")
	assert.Equal(t, "Updated NS", k8sNS.Annotations["openchoreo.dev/display-name"],
		"display-name annotation must be persisted to K8s")
	assert.Equal(t, "true", k8sNS.Labels["openchoreo.dev/control-plane"],
		"control-plane label must still be present after update")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestNamespaceHTTPUpdateNotFound(t *testing.T) {
	bundle := newNSBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.Namespace{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut, "/api/v1/namespaces/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestNamespaceHTTPUpdateForbidden(t *testing.T) {
	bundle := newNSBundle(t, []client.Object{seedNS("ns-1")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.Namespace{Metadata: gen.ObjectMeta{Name: "ns-1"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut, "/api/v1/namespaces/ns-1", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestNamespaceHTTPDelete(t *testing.T) {
	bundle := newNSBundle(t, []client.Object{seedNS("ns-1")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete, "/api/v1/namespaces/ns-1", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	// corev1.Namespace is cluster-scoped so no Namespace field in the lookup key.
	var gone corev1.Namespace
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "ns-1"}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"namespace must be removed from K8s after deletion, got err: %v", err)
}

func TestNamespaceHTTPDeleteNotFound(t *testing.T) {
	bundle := newNSBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete, "/api/v1/namespaces/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestNamespaceHTTPDeleteForbidden(t *testing.T) {
	bundle := newNSBundle(t, []client.Object{seedNS("ns-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete, "/api/v1/namespaces/ns-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
