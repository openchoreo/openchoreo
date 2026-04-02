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
	workflowplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowplane"
)

// wpBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type wpBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newWPBundle builds a wpBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newWPBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) wpBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := workflowplanesvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{WorkflowPlaneService: svc}
	return wpBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedWP is a convenience constructor for an openchoreov1alpha1.WorkflowPlane object.
func seedWP(name string) *openchoreov1alpha1.WorkflowPlane {
	return &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
	}
}

// --- List ---

func TestWorkflowPlaneHTTPList(t *testing.T) {
	bundle := newWPBundle(t, []client.Object{
		seedWP("wp-a"),
		seedWP("wp-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/workflowplanes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.WorkflowPlaneList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded workflow planes")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"wp-a", "wp-b"}, names)

	// Concern 2: response must conform to the OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestWorkflowPlaneHTTPListEmpty(t *testing.T) {
	bundle := newWPBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/workflowplanes", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.WorkflowPlaneList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestWorkflowPlaneHTTPGet(t *testing.T) {
	bundle := newWPBundle(t, []client.Object{seedWP("wp-1")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/workflowplanes/wp-1", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.WorkflowPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "wp-1", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestWorkflowPlaneHTTPGetNotFound(t *testing.T) {
	bundle := newWPBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/workflowplanes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestWorkflowPlaneHTTPGetForbidden(t *testing.T) {
	bundle := newWPBundle(t, []client.Object{seedWP("wp-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/workflowplanes/wp-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestWorkflowPlaneHTTPCreate(t *testing.T) {
	bundle := newWPBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.WorkflowPlane{
		Metadata: gen.ObjectMeta{Name: "new-wp"},
	})
	req, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/workflowplanes", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.WorkflowPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "new-wp", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sObj openchoreov1alpha1.WorkflowPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "new-wp", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "workflow plane must be persisted to K8s after creation")
	assert.Equal(t, "new-wp", k8sObj.Name)

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestWorkflowPlaneHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newWPBundle(t, []client.Object{seedWP("new-wp")}, &allowAllPDP{})

	body, _ := json.Marshal(gen.WorkflowPlane{
		Metadata: gen.ObjectMeta{Name: "new-wp"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/workflowplanes", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestWorkflowPlaneHTTPCreateForbidden(t *testing.T) {
	bundle := newWPBundle(t, nil, &denyAllPDP{})

	body, _ := json.Marshal(gen.WorkflowPlane{
		Metadata: gen.ObjectMeta{Name: "new-wp"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/workflowplanes", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestWorkflowPlaneHTTPUpdate(t *testing.T) {
	bundle := newWPBundle(t, []client.Object{seedWP("wp-1")}, &allowAllPDP{})

	// Include a label so we can assert the updated value is persisted.
	body, _ := json.Marshal(gen.WorkflowPlane{
		Metadata: gen.ObjectMeta{
			Name:   "wp-1",
			Labels: &map[string]string{"tier": "updated"},
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/workflowplanes/wp-1", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.WorkflowPlane
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "wp-1", resp.Metadata.Name)

	// Concern 3: verify the label mutation is reflected in the fake K8s store.
	var k8sObj openchoreov1alpha1.WorkflowPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "wp-1", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "workflow plane must still exist in K8s after update")
	assert.Equal(t, "updated", k8sObj.Labels["tier"],
		"updated label must be persisted to K8s")

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestWorkflowPlaneHTTPUpdateNotFound(t *testing.T) {
	bundle := newWPBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.WorkflowPlane{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/workflowplanes/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestWorkflowPlaneHTTPUpdateForbidden(t *testing.T) {
	bundle := newWPBundle(t, []client.Object{seedWP("wp-1")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.WorkflowPlane{Metadata: gen.ObjectMeta{Name: "wp-1"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/workflowplanes/wp-1", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestWorkflowPlaneHTTPDelete(t *testing.T) {
	bundle := newWPBundle(t, []client.Object{seedWP("wp-1")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/workflowplanes/wp-1", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.WorkflowPlane
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "wp-1", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"workflow plane must be removed from K8s after deletion, got err: %v", err)
}

func TestWorkflowPlaneHTTPDeleteNotFound(t *testing.T) {
	bundle := newWPBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/workflowplanes/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestWorkflowPlaneHTTPDeleteForbidden(t *testing.T) {
	bundle := newWPBundle(t, []client.Object{seedWP("wp-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/workflowplanes/wp-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
