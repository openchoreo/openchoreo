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
	workloadsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workload"
)

// wlBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type wlBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newWLBundle builds a wlBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newWLBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) wlBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := workloadsvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{WorkloadService: svc}
	return wlBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedWorkload returns an openchoreov1alpha1.Workload seeded with Name and Namespace.
func seedWorkload(name string) *openchoreov1alpha1.Workload {
	return &openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
	}
}

// newWorkloadBody returns a gen.Workload body suitable for create/update requests.
func newWorkloadBody() *gen.Workload {
	return &gen.Workload{
		Metadata: gen.ObjectMeta{Name: "wl-1"},
		Spec: &gen.WorkloadSpec{
			Owner: &struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{
				ComponentName: "test-comp",
				ProjectName:   "test-proj",
			},
		},
	}
}

// --- List ---

func TestWorkloadHTTPList(t *testing.T) {
	bundle := newWLBundle(t, []client.Object{
		seedWorkload("wl-a"),
		seedWorkload("wl-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/workloads", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.WorkloadList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded workloads")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"wl-a", "wl-b"}, names)

	// Concern 2: response must conform to the OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestWorkloadHTTPListEmpty(t *testing.T) {
	bundle := newWLBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/workloads", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.WorkloadList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestWorkloadHTTPGet(t *testing.T) {
	bundle := newWLBundle(t, []client.Object{seedWorkload("wl-1")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/workloads/wl-1", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Workload
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "wl-1", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestWorkloadHTTPGetNotFound(t *testing.T) {
	bundle := newWLBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/workloads/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestWorkloadHTTPGetForbidden(t *testing.T) {
	bundle := newWLBundle(t, []client.Object{seedWorkload("wl-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/workloads/wl-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestWorkloadHTTPCreate(t *testing.T) {
	// Seed a Component so the service can resolve the component reference.
	seedComponent := &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: "test-comp", Namespace: testNS},
	}
	bundle := newWLBundle(t, []client.Object{seedComponent}, &allowAllPDP{})

	body, _ := json.Marshal(newWorkloadBody())
	req, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/workloads", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Workload
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "wl-1", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sWL openchoreov1alpha1.Workload
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "wl-1", Namespace: testNS}, &k8sWL)
	require.NoError(t, err, "workload must be persisted to K8s after creation")
	assert.Equal(t, "wl-1", k8sWL.Name)

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestWorkloadHTTPCreateComponentNotFound(t *testing.T) {
	// No component seeded — the service should return a 400.
	bundle := newWLBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(newWorkloadBody())
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/workloads", body)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestWorkloadHTTPCreateAlreadyExists(t *testing.T) {
	seedComponent := &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: "test-comp", Namespace: testNS},
	}
	bundle := newWLBundle(t, []client.Object{seedComponent, seedWorkload("wl-1")}, &allowAllPDP{})

	body, _ := json.Marshal(newWorkloadBody())
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/workloads", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestWorkloadHTTPCreateForbidden(t *testing.T) {
	seedComponent := &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: "test-comp", Namespace: testNS},
	}
	bundle := newWLBundle(t, []client.Object{seedComponent}, &denyAllPDP{})

	body, _ := json.Marshal(newWorkloadBody())
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/workloads", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestWorkloadHTTPUpdate(t *testing.T) {
	bundle := newWLBundle(t, []client.Object{seedWorkload("wl-1")}, &allowAllPDP{})

	// Use newWorkloadBody so spec.owner is included; add a label to assert persistence.
	wl := newWorkloadBody()
	wl.Metadata.Labels = &map[string]string{"tier": "updated"}
	body, _ := json.Marshal(wl)

	req, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/workloads/wl-1", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.Workload
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "wl-1", resp.Metadata.Name)

	// Concern 3: verify the label mutation and spec.owner are reflected in the fake K8s store.
	var k8sWL openchoreov1alpha1.Workload
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "wl-1", Namespace: testNS}, &k8sWL)
	require.NoError(t, err, "workload must still exist in K8s after update")
	assert.Equal(t, "updated", k8sWL.Labels["tier"],
		"updated label must be persisted to K8s")
	assert.Equal(t, "test-comp", k8sWL.Spec.Owner.ComponentName,
		"owner.componentName must be preserved after update")

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestWorkloadHTTPUpdateNotFound(t *testing.T) {
	bundle := newWLBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.Workload{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/workloads/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestWorkloadHTTPUpdateForbidden(t *testing.T) {
	bundle := newWLBundle(t, []client.Object{seedWorkload("wl-1")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.Workload{Metadata: gen.ObjectMeta{Name: "wl-1"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/workloads/wl-1", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestWorkloadHTTPDelete(t *testing.T) {
	bundle := newWLBundle(t, []client.Object{seedWorkload("wl-1")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/workloads/wl-1", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.Workload
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "wl-1", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"workload must be removed from K8s after deletion, got err: %v", err)
}

func TestWorkloadHTTPDeleteNotFound(t *testing.T) {
	bundle := newWLBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/workloads/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestWorkloadHTTPDeleteForbidden(t *testing.T) {
	bundle := newWLBundle(t, []client.Object{seedWorkload("wl-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/workloads/wl-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
