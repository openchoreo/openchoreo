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
	secretreferencesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secretreference"
)

// srBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type srBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newSRBundle builds an srBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newSRBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) srBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := secretreferencesvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{SecretReferenceService: svc}
	return srBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedSR is a convenience constructor for an openchoreov1alpha1.SecretReference object.
// spec.data requires minItems:1 per the OpenAPI schema.
func seedSR(name string) *openchoreov1alpha1.SecretReference {
	return &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			Data: []openchoreov1alpha1.SecretDataSource{
				{
					SecretKey: "my-key",
					RemoteRef: openchoreov1alpha1.RemoteReference{Key: "secret/path"},
				},
			},
		},
	}
}

// newSRBody returns a JSON body for creating/updating a SecretReference.
// spec.data requires minItems:1 per the OpenAPI schema.
func newSRBody(name string) []byte {
	b, _ := json.Marshal(gen.SecretReference{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.SecretReferenceSpec{
			Data: []gen.SecretDataSource{
				{SecretKey: "my-key", RemoteRef: gen.RemoteReference{Key: "secret/path"}},
			},
		},
	})
	return b
}

// --- List ---

func TestSecretReferenceHTTPList(t *testing.T) {
	bundle := newSRBundle(t, []client.Object{
		seedSR("sr-a"),
		seedSR("sr-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/secretreferences", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.SecretReferenceList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded secret references")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"sr-a", "sr-b"}, names)

	// Concern 2: response must conform to the OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestSecretReferenceHTTPListEmpty(t *testing.T) {
	bundle := newSRBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/secretreferences", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.SecretReferenceList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestSecretReferenceHTTPGet(t *testing.T) {
	bundle := newSRBundle(t, []client.Object{seedSR("sr-1")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/secretreferences/sr-1", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.SecretReference
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "sr-1", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestSecretReferenceHTTPGetNotFound(t *testing.T) {
	bundle := newSRBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/secretreferences/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSecretReferenceHTTPGetForbidden(t *testing.T) {
	bundle := newSRBundle(t, []client.Object{seedSR("sr-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/secretreferences/sr-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestSecretReferenceHTTPCreate(t *testing.T) {
	bundle := newSRBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/secretreferences", newSRBody("new-sr"))

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.SecretReference
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "new-sr", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sObj openchoreov1alpha1.SecretReference
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "new-sr", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "secret reference must be persisted to K8s after creation")
	assert.Equal(t, "new-sr", k8sObj.Name)

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestSecretReferenceHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newSRBundle(t, []client.Object{seedSR("new-sr")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/secretreferences", newSRBody("new-sr"))

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestSecretReferenceHTTPCreateForbidden(t *testing.T) {
	bundle := newSRBundle(t, nil, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/secretreferences", newSRBody("new-sr"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestSecretReferenceHTTPUpdate(t *testing.T) {
	bundle := newSRBundle(t, []client.Object{seedSR("sr-1")}, &allowAllPDP{})

	// Include a label so we can assert the updated value is persisted.
	body, _ := json.Marshal(gen.SecretReference{
		Metadata: gen.ObjectMeta{
			Name:   "sr-1",
			Labels: &map[string]string{"tier": "updated"},
		},
		Spec: &gen.SecretReferenceSpec{
			Data: []gen.SecretDataSource{
				{SecretKey: "my-key", RemoteRef: gen.RemoteReference{Key: "secret/path"}},
			},
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/secretreferences/sr-1", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.SecretReference
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "sr-1", resp.Metadata.Name)

	// Concern 3: verify the label mutation is reflected in the fake K8s store.
	var k8sObj openchoreov1alpha1.SecretReference
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "sr-1", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "secret reference must still exist in K8s after update")
	assert.Equal(t, "updated", k8sObj.Labels["tier"],
		"updated label must be persisted to K8s")

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestSecretReferenceHTTPUpdateNotFound(t *testing.T) {
	bundle := newSRBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/secretreferences/nonexistent", newSRBody("nonexistent"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSecretReferenceHTTPUpdateForbidden(t *testing.T) {
	bundle := newSRBundle(t, []client.Object{seedSR("sr-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/secretreferences/sr-1", newSRBody("sr-1"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestSecretReferenceHTTPDelete(t *testing.T) {
	bundle := newSRBundle(t, []client.Object{seedSR("sr-1")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/secretreferences/sr-1", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.SecretReference
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "sr-1", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"secret reference must be removed from K8s after deletion, got err: %v", err)
}

func TestSecretReferenceHTTPDeleteNotFound(t *testing.T) {
	bundle := newSRBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/secretreferences/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSecretReferenceHTTPDeleteForbidden(t *testing.T) {
	bundle := newSRBundle(t, []client.Object{seedSR("sr-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/secretreferences/sr-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
