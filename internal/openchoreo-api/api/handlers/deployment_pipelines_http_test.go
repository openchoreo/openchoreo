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
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	deploymentpipelinesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/deploymentpipeline"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

// dpBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type dpBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newDPBundle builds a dpBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newDPBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) dpBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := deploymentpipelinesvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{DeploymentPipelineService: svc}
	return dpBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedDP2 is a convenience constructor for an openchoreov1alpha1.DeploymentPipeline object.
// Named seedDP2 to avoid collision with seedDP (DataPlane helper) in environments_http_test.go.
// Service-assigned labels (namespace and name) are pre-seeded to test that updates preserve them.
func seedDP2(name string) *openchoreov1alpha1.DeploymentPipeline {
	return &openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
			Labels: map[string]string{
				labels.LabelKeyNamespaceName: testNS,
				labels.LabelKeyName:          name,
			},
		},
	}
}

// --- List ---

func TestDeploymentPipelineHTTPList(t *testing.T) {
	bundle := newDPBundle(t, []client.Object{
		seedDP2("dp-a"),
		seedDP2("dp-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.DeploymentPipelineList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded deployment pipelines")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"dp-a", "dp-b"}, names)

	// Concern 2: response must conform to the OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestDeploymentPipelineHTTPListEmpty(t *testing.T) {
	bundle := newDPBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.DeploymentPipelineList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestDeploymentPipelineHTTPGet(t *testing.T) {
	bundle := newDPBundle(t, []client.Object{seedDP2("dp-1")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines/dp-1", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.DeploymentPipeline
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "dp-1", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestDeploymentPipelineHTTPGetNotFound(t *testing.T) {
	bundle := newDPBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeploymentPipelineHTTPGetForbidden(t *testing.T) {
	bundle := newDPBundle(t, []client.Object{seedDP2("dp-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines/dp-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestDeploymentPipelineHTTPCreate(t *testing.T) {
	bundle := newDPBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.DeploymentPipeline{
		Metadata: gen.ObjectMeta{Name: "new-dp"},
	})
	req, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.DeploymentPipeline
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "new-dp", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sObj openchoreov1alpha1.DeploymentPipeline
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "new-dp", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "deployment pipeline must be persisted to K8s after creation")
	assert.Equal(t, "new-dp", k8sObj.Name)

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestDeploymentPipelineHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newDPBundle(t, []client.Object{seedDP2("new-dp")}, &allowAllPDP{})

	body, _ := json.Marshal(gen.DeploymentPipeline{
		Metadata: gen.ObjectMeta{Name: "new-dp"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestDeploymentPipelineHTTPCreateForbidden(t *testing.T) {
	bundle := newDPBundle(t, nil, &denyAllPDP{})

	body, _ := json.Marshal(gen.DeploymentPipeline{
		Metadata: gen.ObjectMeta{Name: "new-dp"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestDeploymentPipelineHTTPUpdate(t *testing.T) {
	bundle := newDPBundle(t, []client.Object{seedDP2("dp-1")}, &allowAllPDP{})

	// Preserve existing service labels and add a new "tier" label to assert persistence.
	body, _ := json.Marshal(gen.DeploymentPipeline{
		Metadata: gen.ObjectMeta{
			Name: "dp-1",
			Labels: &map[string]string{
				labels.LabelKeyNamespaceName: testNS,
				labels.LabelKeyName:          "dp-1",
				"tier":                       "updated",
			},
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines/dp-1", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.DeploymentPipeline
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "dp-1", resp.Metadata.Name)

	// Concern 3: verify all three labels are reflected in the fake K8s store.
	var k8sObj openchoreov1alpha1.DeploymentPipeline
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "dp-1", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "deployment pipeline must still exist in K8s after update")
	assert.Equal(t, testNS, k8sObj.Labels[labels.LabelKeyNamespaceName],
		"namespace label must be preserved after update")
	assert.Equal(t, "dp-1", k8sObj.Labels[labels.LabelKeyName],
		"name label must be preserved after update")
	assert.Equal(t, "updated", k8sObj.Labels["tier"],
		"updated label must be persisted to K8s")

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestDeploymentPipelineHTTPUpdateNotFound(t *testing.T) {
	bundle := newDPBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.DeploymentPipeline{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeploymentPipelineHTTPUpdateForbidden(t *testing.T) {
	bundle := newDPBundle(t, []client.Object{seedDP2("dp-1")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.DeploymentPipeline{Metadata: gen.ObjectMeta{Name: "dp-1"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines/dp-1", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestDeploymentPipelineHTTPDelete(t *testing.T) {
	bundle := newDPBundle(t, []client.Object{seedDP2("dp-1")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines/dp-1", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.DeploymentPipeline
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "dp-1", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"deployment pipeline must be removed from K8s after deletion, got err: %v", err)
}

func TestDeploymentPipelineHTTPDeleteNotFound(t *testing.T) {
	bundle := newDPBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeploymentPipelineHTTPDeleteForbidden(t *testing.T) {
	bundle := newDPBundle(t, []client.Object{seedDP2("dp-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/deploymentpipelines/dp-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
