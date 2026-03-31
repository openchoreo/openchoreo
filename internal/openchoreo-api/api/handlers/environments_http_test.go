// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

// HTTP-layer integration tests for the environments resource.
//
// These tests go beyond the direct-call tests in environments_test.go by
// exercising all three behavioral concerns raised in code review:
//
//  1. HTTP/router layer — requests flow through gen.NewStrictHandler and the
//     real net/http mux, so route matching, path-parameter extraction, content-
//     type negotiation, and JSON serialization are all exercised.
//
//  2. OpenAPI contract — every success response is validated against the spec
//     generated from openapi/openchoreo-api.yaml via assertConformsToSpec, so a
//     schema drift is caught without needing a live server.
//
//  3. K8s side effects — create/update/delete operations verify the expected
//     object state in the fake client after the HTTP call returns, confirming that
//     the handler actually mutates the store rather than just returning the right
//     status code.

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
	environmentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/environment"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

// envBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type envBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newEnvBundle builds an envBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newEnvBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) envBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := environmentsvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{EnvironmentService: svc}
	return envBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

const defaultPlaneName = "default"

// seedEnv is a convenience constructor for an openchoreov1alpha1.Environment
// object with a DataPlaneRef already set (the create path requires it or a
// default DataPlane to be present).
func seedEnv(name string) *openchoreov1alpha1.Environment {
	return &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: defaultPlaneName,
			},
		},
	}
}

// seedDP is a convenience constructor for a DataPlane object that the
// CreateEnvironment service resolves when no DataPlaneRef is provided.
func seedDP(namespace string) *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultPlaneName,
			Namespace: namespace,
		},
	}
}

const testNS = "test-ns"

// --- List ---

func TestEnvironmentHTTPList(t *testing.T) {
	bundle := newEnvBundle(t, []client.Object{
		seedEnv("env-a"),
		seedEnv("env-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/environments", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.EnvironmentList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded environments")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"env-a", "env-b"}, names)

	// Concern 2: response must conform to the OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestEnvironmentHTTPListEmpty(t *testing.T) {
	bundle := newEnvBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/environments", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.EnvironmentList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestEnvironmentHTTPListInvalidLabelSelector(t *testing.T) {
	bundle := newEnvBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/environments?labelSelector===invalid", nil)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- Get ---

func TestEnvironmentHTTPGet(t *testing.T) {
	bundle := newEnvBundle(t, []client.Object{seedEnv("staging")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/environments/staging", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Environment
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "staging", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestEnvironmentHTTPGetNotFound(t *testing.T) {
	bundle := newEnvBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/environments/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestEnvironmentHTTPGetForbidden(t *testing.T) {
	bundle := newEnvBundle(t, []client.Object{seedEnv("staging")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/environments/staging", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestEnvironmentHTTPCreate(t *testing.T) {
	// Seed a DataPlane so the service can resolve the default DataPlaneRef.
	bundle := newEnvBundle(t, []client.Object{seedDP(testNS)}, &allowAllPDP{})

	body, _ := json.Marshal(gen.Environment{
		Metadata: gen.ObjectMeta{Name: "prod"},
	})
	req, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/environments", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Environment
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "prod", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sEnv openchoreov1alpha1.Environment
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "prod", Namespace: testNS}, &k8sEnv)
	require.NoError(t, err, "environment must be persisted to K8s after creation")
	assert.Equal(t, "prod", k8sEnv.Name)

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestEnvironmentHTTPCreateEmptyName(t *testing.T) {
	bundle := newEnvBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.Environment{
		Metadata: gen.ObjectMeta{Name: "  "},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/environments", body)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEnvironmentHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newEnvBundle(t, []client.Object{seedEnv("prod"), seedDP(testNS)}, &allowAllPDP{})

	body, _ := json.Marshal(gen.Environment{
		Metadata: gen.ObjectMeta{Name: "prod"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/environments", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestEnvironmentHTTPCreateForbidden(t *testing.T) {
	bundle := newEnvBundle(t, []client.Object{seedDP(testNS)}, &denyAllPDP{})

	body, _ := json.Marshal(gen.Environment{
		Metadata: gen.ObjectMeta{Name: "prod"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/environments", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestEnvironmentHTTPUpdate(t *testing.T) {
	bundle := newEnvBundle(t, []client.Object{seedEnv("dev")}, &allowAllPDP{})

	// Include a label so we can assert the updated value is persisted.
	body, _ := json.Marshal(gen.Environment{
		Metadata: gen.ObjectMeta{
			Name:   "dev",
			Labels: &map[string]string{"env-tier": "staging"},
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/environments/dev", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Environment
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "dev", resp.Metadata.Name)

	// Concern 3: verify the label mutation is reflected in the fake K8s store,
	// not just that the object still exists.
	var k8sEnv openchoreov1alpha1.Environment
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "dev", Namespace: testNS}, &k8sEnv)
	require.NoError(t, err, "environment must still exist in K8s after update")
	assert.Equal(t, "staging", k8sEnv.Labels["env-tier"],
		"updated label must be persisted to K8s")

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestEnvironmentHTTPUpdateNotFound(t *testing.T) {
	bundle := newEnvBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.Environment{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/environments/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestEnvironmentHTTPUpdateForbidden(t *testing.T) {
	bundle := newEnvBundle(t, []client.Object{seedEnv("dev")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.Environment{Metadata: gen.ObjectMeta{Name: "dev"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/environments/dev", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestEnvironmentHTTPDelete(t *testing.T) {
	bundle := newEnvBundle(t, []client.Object{seedEnv("dev")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/environments/dev", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.Environment
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "dev", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"environment must be removed from K8s after deletion, got err: %v", err)
}

func TestEnvironmentHTTPDeleteNotFound(t *testing.T) {
	bundle := newEnvBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/environments/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestEnvironmentHTTPDeleteForbidden(t *testing.T) {
	bundle := newEnvBundle(t, []client.Object{seedEnv("dev")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/environments/dev", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
