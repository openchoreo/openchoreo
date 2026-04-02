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
	componenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

// ctBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type ctBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newCTBundle builds a ctBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newCTBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) ctBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := componenttypesvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{ComponentTypeService: svc}
	return ctBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedCT is a convenience constructor for an openchoreov1alpha1.ComponentType object.
// It includes the minimum spec fields required by the OpenAPI schema (workloadType
// and at least one resource with an ID and template).
func seedCT(name string) *openchoreov1alpha1.ComponentType {
	return &openchoreov1alpha1.ComponentType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
		Spec: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: "deployment",
			Resources: []openchoreov1alpha1.ResourceTemplate{
				{ID: "deployment", Template: &runtime.RawExtension{Raw: []byte(`{}`)}},
			},
		},
	}
}

// seedCTWithSchema returns a ComponentType with a Parameters schema set so the
// GetComponentTypeSchema endpoint returns a non-empty body.
func seedCTWithSchema(name string) *openchoreov1alpha1.ComponentType {
	paramsRaw, _ := json.Marshal(map[string]interface{}{"type": "object"})
	return &openchoreov1alpha1.ComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
		Spec: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: "deployment",
			Parameters: &openchoreov1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{Raw: paramsRaw},
			},
			Resources: []openchoreov1alpha1.ResourceTemplate{
				{ID: "deployment", Template: &runtime.RawExtension{Raw: []byte(`{}`)}},
			},
		},
	}
}

// --- List ---

func TestComponentTypeHTTPList(t *testing.T) {
	bundle := newCTBundle(t, []client.Object{
		seedCT("ct-a"),
		seedCT("ct-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componenttypes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentTypeList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded component types")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"ct-a", "ct-b"}, names)

	// Concern 2: response must conform to the OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestComponentTypeHTTPListEmpty(t *testing.T) {
	bundle := newCTBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componenttypes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentTypeList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestComponentTypeHTTPGet(t *testing.T) {
	bundle := newCTBundle(t, []client.Object{seedCT("ct-x")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componenttypes/ct-x", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentType
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "ct-x", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestComponentTypeHTTPGetNotFound(t *testing.T) {
	bundle := newCTBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componenttypes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestComponentTypeHTTPGetForbidden(t *testing.T) {
	bundle := newCTBundle(t, []client.Object{seedCT("ct-x")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componenttypes/ct-x", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

// newCTBody returns JSON bytes for a ComponentType request body that satisfies
// the OpenAPI schema — spec.resources requires minItems:1 with id+template.
func newCTBody(name string) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{"name": name},
		"spec": map[string]interface{}{
			"workloadType": "deployment",
			"resources": []map[string]interface{}{
				{"id": "deployment", "template": map[string]interface{}{}},
			},
		},
	})
	return b
}

func TestComponentTypeHTTPCreate(t *testing.T) {
	bundle := newCTBundle(t, nil, &allowAllPDP{})

	body := newCTBody("new-ct")
	req, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/componenttypes", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentType
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "new-ct", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sObj openchoreov1alpha1.ComponentType
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "new-ct", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "component type must be persisted to K8s after creation")
	assert.Equal(t, "new-ct", k8sObj.Name)

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestComponentTypeHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newCTBundle(t, []client.Object{seedCT("new-ct")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/componenttypes", newCTBody("new-ct"))

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestComponentTypeHTTPCreateForbidden(t *testing.T) {
	bundle := newCTBundle(t, nil, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/componenttypes", newCTBody("new-ct"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

// newCTUpdateBody returns JSON bytes for a ComponentType PUT body that includes
// the required spec.resources fields along with a label to assert persistence.
func newCTUpdateBody(name string) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":   name,
			"labels": map[string]string{"tier": "updated"},
		},
		"spec": map[string]interface{}{
			"workloadType": "deployment",
			"resources": []map[string]interface{}{
				{"id": "deployment", "template": map[string]interface{}{}},
			},
		},
	})
	return b
}

func TestComponentTypeHTTPUpdate(t *testing.T) {
	bundle := newCTBundle(t, []client.Object{seedCT("ct-y")}, &allowAllPDP{})

	// Include a label so we can assert the updated value is persisted.
	req, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/componenttypes/ct-y", newCTUpdateBody("ct-y"))

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentType
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "ct-y", resp.Metadata.Name)

	// Concern 3: verify the label mutation is reflected in the fake K8s store.
	var k8sObj openchoreov1alpha1.ComponentType
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "ct-y", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "component type must still exist in K8s after update")
	assert.Equal(t, "updated", k8sObj.Labels["tier"],
		"updated label must be persisted to K8s")

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestComponentTypeHTTPUpdateNotFound(t *testing.T) {
	bundle := newCTBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/componenttypes/nonexistent", newCTBody("nonexistent"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestComponentTypeHTTPUpdateForbidden(t *testing.T) {
	bundle := newCTBundle(t, []client.Object{seedCT("ct-y")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/componenttypes/ct-y", newCTBody("ct-y"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestComponentTypeHTTPDelete(t *testing.T) {
	bundle := newCTBundle(t, []client.Object{seedCT("ct-z")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/componenttypes/ct-z", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.ComponentType
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "ct-z", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"component type must be removed from K8s after deletion, got err: %v", err)
}

func TestComponentTypeHTTPDeleteNotFound(t *testing.T) {
	bundle := newCTBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/componenttypes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestComponentTypeHTTPDeleteForbidden(t *testing.T) {
	bundle := newCTBundle(t, []client.Object{seedCT("ct-z")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/componenttypes/ct-z", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- GetSchema ---

func TestComponentTypeHTTPGetSchema(t *testing.T) {
	bundle := newCTBundle(t, []client.Object{seedCTWithSchema("ct-with-schema")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componenttypes/ct-with-schema/schema", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestComponentTypeHTTPGetSchemaNotFound(t *testing.T) {
	bundle := newCTBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componenttypes/missing/schema", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestComponentTypeHTTPGetSchemaForbidden(t *testing.T) {
	bundle := newCTBundle(t, []client.Object{seedCTWithSchema("ct-with-schema")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componenttypes/ct-with-schema/schema", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
