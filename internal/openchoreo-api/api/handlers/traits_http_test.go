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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	traitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/trait"
)

// traitBundle holds the real HTTP handler wired to a fake K8s client so tests
// can both drive the handler through HTTP and inspect the resulting K8s state.
type traitBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newTraitBundle builds a traitBundle seeded with the given objects and using
// the supplied PDP for authorization decisions.
func newTraitBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) traitBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := traitsvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{TraitService: svc}
	return traitBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedTrait is a convenience constructor for an openchoreov1alpha1.Trait object.
func seedTrait(name string) *openchoreov1alpha1.Trait {
	return &openchoreov1alpha1.Trait{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
	}
}

// seedTraitWithSchema returns a Trait with a Parameters schema set so the
// GetTraitSchema endpoint returns a non-empty body.
func seedTraitWithSchema(name string) *openchoreov1alpha1.Trait {
	paramsRaw, _ := json.Marshal(map[string]interface{}{"type": "object"})
	return &openchoreov1alpha1.Trait{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
		Spec: openchoreov1alpha1.TraitSpec{
			Parameters: &openchoreov1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{Raw: paramsRaw},
			},
		},
	}
}

// --- List ---

func TestTraitHTTPList(t *testing.T) {
	bundle := newTraitBundle(t, []client.Object{
		seedTrait("trait-a"),
		seedTrait("trait-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/traits", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.TraitList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded traits")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"trait-a", "trait-b"}, names)

	// Concern 2: response must conform to the OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestTraitHTTPListEmpty(t *testing.T) {
	bundle := newTraitBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/traits", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.TraitList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestTraitHTTPGet(t *testing.T) {
	bundle := newTraitBundle(t, []client.Object{seedTrait("trait-x")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/traits/trait-x", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Trait
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "trait-x", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestTraitHTTPGetNotFound(t *testing.T) {
	bundle := newTraitBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/traits/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestTraitHTTPGetForbidden(t *testing.T) {
	bundle := newTraitBundle(t, []client.Object{seedTrait("trait-x")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/traits/trait-x", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestTraitHTTPCreate(t *testing.T) {
	bundle := newTraitBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.Trait{
		Metadata: gen.ObjectMeta{Name: "new-trait"},
	})
	req, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/traits", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Trait
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "new-trait", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sObj openchoreov1alpha1.Trait
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "new-trait", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "trait must be persisted to K8s after creation")
	assert.Equal(t, "new-trait", k8sObj.Name)

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestTraitHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newTraitBundle(t, []client.Object{seedTrait("new-trait")}, &allowAllPDP{})

	body, _ := json.Marshal(gen.Trait{
		Metadata: gen.ObjectMeta{Name: "new-trait"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/traits", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestTraitHTTPCreateForbidden(t *testing.T) {
	bundle := newTraitBundle(t, nil, &denyAllPDP{})

	body, _ := json.Marshal(gen.Trait{
		Metadata: gen.ObjectMeta{Name: "new-trait"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/traits", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestTraitHTTPUpdate(t *testing.T) {
	bundle := newTraitBundle(t, []client.Object{seedTrait("trait-y")}, &allowAllPDP{})

	// Include a label so we can assert the updated value is persisted.
	body, _ := json.Marshal(gen.Trait{
		Metadata: gen.ObjectMeta{
			Name:   "trait-y",
			Labels: &map[string]string{"tier": "updated"},
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/traits/trait-y", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Trait
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "trait-y", resp.Metadata.Name)

	// Concern 3: verify the label mutation is reflected in the fake K8s store.
	var k8sObj openchoreov1alpha1.Trait
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "trait-y", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "trait must still exist in K8s after update")
	assert.Equal(t, "updated", k8sObj.Labels["tier"],
		"updated label must be persisted to K8s")

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestTraitHTTPUpdateNotFound(t *testing.T) {
	bundle := newTraitBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.Trait{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/traits/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestTraitHTTPUpdateForbidden(t *testing.T) {
	bundle := newTraitBundle(t, []client.Object{seedTrait("trait-y")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.Trait{Metadata: gen.ObjectMeta{Name: "trait-y"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/traits/trait-y", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestTraitHTTPDelete(t *testing.T) {
	bundle := newTraitBundle(t, []client.Object{seedTrait("trait-z")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/traits/trait-z", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.Trait
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "trait-z", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"trait must be removed from K8s after deletion, got err: %v", err)
}

func TestTraitHTTPDeleteNotFound(t *testing.T) {
	bundle := newTraitBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/traits/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestTraitHTTPDeleteForbidden(t *testing.T) {
	bundle := newTraitBundle(t, []client.Object{seedTrait("trait-z")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/traits/trait-z", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- GetSchema ---

func TestTraitHTTPGetSchema(t *testing.T) {
	bundle := newTraitBundle(t, []client.Object{seedTraitWithSchema("trait-with-schema")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/traits/trait-with-schema/schema", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	// Verify the response body contains the seeded schema content {"type":"object"}.
	assert.Contains(t, string(bodyBytes), `"object"`,
		"schema response must contain the seeded type value")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestTraitHTTPGetSchemaNotFound(t *testing.T) {
	bundle := newTraitBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/traits/missing/schema", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestTraitHTTPGetSchemaForbidden(t *testing.T) {
	bundle := newTraitBundle(t, []client.Object{seedTraitWithSchema("trait-with-schema")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/traits/trait-with-schema/schema", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
