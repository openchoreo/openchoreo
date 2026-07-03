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
	componentcomponentreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentreleasebinding"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

// crbBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type crbBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newCRBBundle builds a crbBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newCRBBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) crbBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := componentcomponentreleasebindingsvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{ComponentReleaseBindingService: svc}
	return crbBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedComponentReleaseBinding returns an openchoreov1alpha1.ComponentReleaseBinding seeded with Name,
// Namespace, and an Owner pointing to "test-comp".
func seedComponentReleaseBinding(name string) *openchoreov1alpha1.ComponentReleaseBinding {
	return &openchoreov1alpha1.ComponentReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
		Spec: openchoreov1alpha1.ComponentReleaseBindingSpec{
			Owner: openchoreov1alpha1.ComponentReleaseBindingOwner{
				ComponentName: "test-comp",
				ProjectName:   "test-proj",
			},
			Environment: "dev",
		},
	}
}

// newComponentReleaseBindingBody returns a gen.ComponentReleaseBinding body suitable for HTTP create/update requests.
func newComponentReleaseBindingBody(name string) *gen.ComponentReleaseBinding {
	return &gen.ComponentReleaseBinding{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.ComponentReleaseBindingSpec{
			Owner: struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{
				ComponentName: "test-comp",
				ProjectName:   "test-proj",
			},
			Environment: "dev",
		},
	}
}

// seedComponentForCRB returns a Component object used to satisfy the componentreleasebinding
// service's component-existence validation on create/update.
func seedComponentForCRB() *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: "test-comp", Namespace: testNS},
	}
}

// --- List ---

func TestComponentReleaseBindingHTTPList(t *testing.T) {
	bundle := newCRBBundle(t, []client.Object{
		seedComponentReleaseBinding("rb-a"),
		seedComponentReleaseBinding("rb-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentReleaseBindingList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded release bindings")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"rb-a", "rb-b"}, names)

	// Concern 2: response must conform to the OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestComponentReleaseBindingHTTPListEmpty(t *testing.T) {
	bundle := newCRBBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentReleaseBindingList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestComponentReleaseBindingHTTPGet(t *testing.T) {
	bundle := newCRBBundle(t, []client.Object{seedComponentReleaseBinding("rb-1")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings/rb-1", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentReleaseBinding
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "rb-1", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestComponentReleaseBindingHTTPGetNotFound(t *testing.T) {
	bundle := newCRBBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestComponentReleaseBindingHTTPGetForbidden(t *testing.T) {
	bundle := newCRBBundle(t, []client.Object{seedComponentReleaseBinding("rb-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings/rb-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestComponentReleaseBindingHTTPCreate(t *testing.T) {
	// Seed a Component so the service can resolve the component reference.
	bundle := newCRBBundle(t, []client.Object{seedComponentForCRB()}, &allowAllPDP{})

	body, _ := json.Marshal(newComponentReleaseBindingBody("rb-1"))
	req, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentReleaseBinding
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "rb-1", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sRB openchoreov1alpha1.ComponentReleaseBinding
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "rb-1", Namespace: testNS}, &k8sRB)
	require.NoError(t, err, "release binding must be persisted to K8s after creation")
	assert.Equal(t, "rb-1", k8sRB.Name)

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestComponentReleaseBindingHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newCRBBundle(t, []client.Object{seedComponentForCRB(), seedComponentReleaseBinding("rb-1")}, &allowAllPDP{})

	body, _ := json.Marshal(newComponentReleaseBindingBody("rb-1"))
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestComponentReleaseBindingHTTPCreateForbidden(t *testing.T) {
	bundle := newCRBBundle(t, []client.Object{seedComponentForCRB()}, &denyAllPDP{})

	body, _ := json.Marshal(newComponentReleaseBindingBody("rb-1"))
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestComponentReleaseBindingHTTPUpdate(t *testing.T) {
	// Seed both the component (for validation) and the existing release binding.
	bundle := newCRBBundle(t, []client.Object{seedComponentForCRB(), seedComponentReleaseBinding("rb-1")}, &allowAllPDP{})

	// Include spec.owner + spec.environment so we can assert they are preserved,
	// and add a label to assert label persistence.
	body, _ := json.Marshal(gen.ComponentReleaseBinding{
		Metadata: gen.ObjectMeta{
			Name:   "rb-1",
			Labels: &map[string]string{"tier": "updated"},
		},
		Spec: &gen.ComponentReleaseBindingSpec{
			Owner: struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{
				ComponentName: "test-comp",
				ProjectName:   "test-proj",
			},
			Environment: "dev",
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings/rb-1", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentReleaseBinding
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "rb-1", resp.Metadata.Name)
	require.NotNil(t, resp.Spec, "response spec must not be nil")
	assert.Equal(t, "test-comp", resp.Spec.Owner.ComponentName,
		"owner.componentName must be preserved in response")
	assert.Equal(t, "dev", resp.Spec.Environment,
		"environment must be preserved in response")

	// Concern 3: verify label and spec fields are reflected in the fake K8s store.
	var k8sRB openchoreov1alpha1.ComponentReleaseBinding
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "rb-1", Namespace: testNS}, &k8sRB)
	require.NoError(t, err, "release binding must still exist in K8s after update")
	assert.Equal(t, "updated", k8sRB.Labels["tier"],
		"updated label must be persisted to K8s")
	assert.Equal(t, "test-comp", k8sRB.Spec.Owner.ComponentName,
		"owner.componentName must be persisted to K8s after update")
	assert.Equal(t, "dev", k8sRB.Spec.Environment,
		"environment must be persisted to K8s after update")

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestComponentReleaseBindingHTTPUpdateNotFound(t *testing.T) {
	bundle := newCRBBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.ComponentReleaseBinding{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestComponentReleaseBindingHTTPUpdateForbidden(t *testing.T) {
	bundle := newCRBBundle(t, []client.Object{seedComponentForCRB(), seedComponentReleaseBinding("rb-1")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.ComponentReleaseBinding{Metadata: gen.ObjectMeta{Name: "rb-1"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings/rb-1", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestComponentReleaseBindingHTTPDelete(t *testing.T) {
	bundle := newCRBBundle(t, []client.Object{seedComponentReleaseBinding("rb-1")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings/rb-1", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.ComponentReleaseBinding
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "rb-1", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"release binding must be removed from K8s after deletion, got err: %v", err)
}

func TestComponentReleaseBindingHTTPDeleteNotFound(t *testing.T) {
	bundle := newCRBBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestComponentReleaseBindingHTTPDeleteForbidden(t *testing.T) {
	bundle := newCRBBundle(t, []client.Object{seedComponentReleaseBinding("rb-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/componentcomponentreleasebindings/rb-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
