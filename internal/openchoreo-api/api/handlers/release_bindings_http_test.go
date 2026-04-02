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
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
)

// rbBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type rbBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newRBBundle builds an rbBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newRBBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) rbBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := releasebindingsvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{ReleaseBindingService: svc}
	return rbBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedReleaseBinding returns an openchoreov1alpha1.ReleaseBinding seeded with Name,
// Namespace, and an Owner pointing to "test-comp".
func seedReleaseBinding(name string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ComponentName: "test-comp",
				ProjectName:   "test-proj",
			},
			Environment: "dev",
		},
	}
}

// newReleaseBindingBody returns a gen.ReleaseBinding body suitable for HTTP create/update requests.
func newReleaseBindingBody(name string) *gen.ReleaseBinding {
	return &gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.ReleaseBindingSpec{
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

// seedComponentForRB returns a Component object used to satisfy the releasebinding
// service's component-existence validation on create/update.
func seedComponentForRB() *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: "test-comp", Namespace: testNS},
	}
}

// --- List ---

func TestReleaseBindingHTTPList(t *testing.T) {
	bundle := newRBBundle(t, []client.Object{
		seedReleaseBinding("rb-a"),
		seedReleaseBinding("rb-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/releasebindings", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ReleaseBindingList
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

func TestReleaseBindingHTTPListEmpty(t *testing.T) {
	bundle := newRBBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/releasebindings", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ReleaseBindingList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestReleaseBindingHTTPGet(t *testing.T) {
	bundle := newRBBundle(t, []client.Object{seedReleaseBinding("rb-1")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/releasebindings/rb-1", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ReleaseBinding
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "rb-1", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestReleaseBindingHTTPGetNotFound(t *testing.T) {
	bundle := newRBBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/releasebindings/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestReleaseBindingHTTPGetForbidden(t *testing.T) {
	bundle := newRBBundle(t, []client.Object{seedReleaseBinding("rb-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/releasebindings/rb-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestReleaseBindingHTTPCreate(t *testing.T) {
	// Seed a Component so the service can resolve the component reference.
	bundle := newRBBundle(t, []client.Object{seedComponentForRB()}, &allowAllPDP{})

	body, _ := json.Marshal(newReleaseBindingBody("rb-1"))
	req, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/releasebindings", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ReleaseBinding
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "rb-1", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sRB openchoreov1alpha1.ReleaseBinding
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "rb-1", Namespace: testNS}, &k8sRB)
	require.NoError(t, err, "release binding must be persisted to K8s after creation")
	assert.Equal(t, "rb-1", k8sRB.Name)

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestReleaseBindingHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newRBBundle(t, []client.Object{seedComponentForRB(), seedReleaseBinding("rb-1")}, &allowAllPDP{})

	body, _ := json.Marshal(newReleaseBindingBody("rb-1"))
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/releasebindings", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestReleaseBindingHTTPCreateForbidden(t *testing.T) {
	bundle := newRBBundle(t, []client.Object{seedComponentForRB()}, &denyAllPDP{})

	body, _ := json.Marshal(newReleaseBindingBody("rb-1"))
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/releasebindings", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestReleaseBindingHTTPUpdate(t *testing.T) {
	// Seed both the component (for validation) and the existing release binding.
	bundle := newRBBundle(t, []client.Object{seedComponentForRB(), seedReleaseBinding("rb-1")}, &allowAllPDP{})

	// Include spec.owner + spec.environment so we can assert they are preserved,
	// and add a label to assert label persistence.
	body, _ := json.Marshal(gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{
			Name:   "rb-1",
			Labels: &map[string]string{"tier": "updated"},
		},
		Spec: &gen.ReleaseBindingSpec{
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
		"/api/v1/namespaces/"+testNS+"/releasebindings/rb-1", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ReleaseBinding
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "rb-1", resp.Metadata.Name)
	require.NotNil(t, resp.Spec, "response spec must not be nil")
	assert.Equal(t, "test-comp", resp.Spec.Owner.ComponentName,
		"owner.componentName must be preserved in response")
	assert.Equal(t, "dev", resp.Spec.Environment,
		"environment must be preserved in response")

	// Concern 3: verify label and spec fields are reflected in the fake K8s store.
	var k8sRB openchoreov1alpha1.ReleaseBinding
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

func TestReleaseBindingHTTPUpdateNotFound(t *testing.T) {
	bundle := newRBBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: "nonexistent"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/releasebindings/nonexistent", body)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestReleaseBindingHTTPUpdateForbidden(t *testing.T) {
	bundle := newRBBundle(t, []client.Object{seedComponentForRB(), seedReleaseBinding("rb-1")}, &denyAllPDP{})

	body, _ := json.Marshal(gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: "rb-1"}})
	_, rec := doRequest(t, bundle.handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/releasebindings/rb-1", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestReleaseBindingHTTPDelete(t *testing.T) {
	bundle := newRBBundle(t, []client.Object{seedReleaseBinding("rb-1")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/releasebindings/rb-1", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.ReleaseBinding
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "rb-1", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"release binding must be removed from K8s after deletion, got err: %v", err)
}

func TestReleaseBindingHTTPDeleteNotFound(t *testing.T) {
	bundle := newRBBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/releasebindings/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestReleaseBindingHTTPDeleteForbidden(t *testing.T) {
	bundle := newRBBundle(t, []client.Object{seedReleaseBinding("rb-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/releasebindings/rb-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
