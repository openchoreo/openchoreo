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
	componentreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentrelease"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

// crBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type crBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newCRBundle builds a crBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newCRBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) crBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := componentreleasesvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{ComponentReleaseService: svc}
	return crBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedComponentRelease returns an openchoreov1alpha1.ComponentRelease seeded with Name and Namespace.
func seedComponentRelease(name string) *openchoreov1alpha1.ComponentRelease {
	return &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
	}
}

// --- List ---

func TestComponentReleaseHTTPList(t *testing.T) {
	bundle := newCRBundle(t, []client.Object{
		seedComponentRelease("cr-a"),
		seedComponentRelease("cr-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componentreleases", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentReleaseList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded component releases")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"cr-a", "cr-b"}, names)

	// Concern 2: response must conform to the OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestComponentReleaseHTTPListEmpty(t *testing.T) {
	bundle := newCRBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componentreleases", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentReleaseList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestComponentReleaseHTTPGet(t *testing.T) {
	bundle := newCRBundle(t, []client.Object{seedComponentRelease("cr-1")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componentreleases/cr-1", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentRelease
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "cr-1", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestComponentReleaseHTTPGetNotFound(t *testing.T) {
	bundle := newCRBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componentreleases/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestComponentReleaseHTTPGetForbidden(t *testing.T) {
	bundle := newCRBundle(t, []client.Object{seedComponentRelease("cr-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet,
		"/api/v1/namespaces/"+testNS+"/componentreleases/cr-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestComponentReleaseHTTPCreate(t *testing.T) {
	bundle := newCRBundle(t, nil, &allowAllPDP{})

	body, _ := json.Marshal(gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: "cr-1"},
	})
	req, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/componentreleases", body)

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ComponentRelease
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "cr-1", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sCR openchoreov1alpha1.ComponentRelease
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "cr-1", Namespace: testNS}, &k8sCR)
	require.NoError(t, err, "component release must be persisted to K8s after creation")
	assert.Equal(t, "cr-1", k8sCR.Name)

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestComponentReleaseHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newCRBundle(t, []client.Object{seedComponentRelease("cr-1")}, &allowAllPDP{})

	body, _ := json.Marshal(gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: "cr-1"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/componentreleases", body)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestComponentReleaseHTTPCreateForbidden(t *testing.T) {
	bundle := newCRBundle(t, nil, &denyAllPDP{})

	body, _ := json.Marshal(gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: "cr-1"},
	})
	_, rec := doRequest(t, bundle.handler, http.MethodPost,
		"/api/v1/namespaces/"+testNS+"/componentreleases", body)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestComponentReleaseHTTPDelete(t *testing.T) {
	bundle := newCRBundle(t, []client.Object{seedComponentRelease("cr-1")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/componentreleases/cr-1", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.ComponentRelease
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "cr-1", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"component release must be removed from K8s after deletion, got err: %v", err)
}

func TestComponentReleaseHTTPDeleteNotFound(t *testing.T) {
	bundle := newCRBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/componentreleases/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestComponentReleaseHTTPDeleteForbidden(t *testing.T) {
	bundle := newCRBundle(t, []client.Object{seedComponentRelease("cr-1")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/componentreleases/cr-1", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
