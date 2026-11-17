// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const (
	testComponentKind = "Component"
	testNamespaceKind = "Namespace"
	testWorkloadKind  = "Workload"
)

// The collection inventory comes from the export requirements, rather than the
// fetch implementation, so tests can detect missing or accidentally added kinds.
type collectionFixture struct {
	kind string
	path string
}

type fetchContextKey struct{}

var requiredClusterCollections = []collectionFixture{
	{kind: "ClusterProjectType", path: "clusterprojecttypes"},
	{kind: "ClusterComponentType", path: "clustercomponenttypes"},
	{kind: "ClusterResourceType", path: "clusterresourcetypes"},
	{kind: "ClusterTrait", path: "clustertraits"},
	{kind: "ClusterWorkflow", path: "clusterworkflows"},
	{kind: "ClusterDataPlane", path: "clusterdataplanes"},
	{kind: "ClusterWorkflowPlane", path: "clusterworkflowplanes"},
	{kind: "ClusterObservabilityPlane", path: "clusterobservabilityplanes"},
	{kind: "ClusterAuthzRole", path: "clusterauthzroles"},
	{kind: "ClusterAuthzRoleBinding", path: "clusterauthzrolebindings"},
}

var requiredNamespacedCollections = []collectionFixture{
	{kind: "DeploymentPipeline", path: "deploymentpipelines"},
	{kind: "Environment", path: "environments"},
	{kind: "DataPlane", path: "dataplanes"},
	{kind: "WorkflowPlane", path: "workflowplanes"},
	{kind: "ObservabilityPlane", path: "observabilityplanes"},
	{kind: "ProjectType", path: "projecttypes"},
	{kind: "ComponentType", path: "componenttypes"},
	{kind: "ResourceType", path: "resourcetypes"},
	{kind: "Trait", path: "traits"},
	{kind: "Workflow", path: "workflows"},
	{kind: "AuthzRole", path: "authzroles"},
	{kind: "AuthzRoleBinding", path: "authzrolebindings"},
	{kind: "SecretReference", path: "secretreferences"},
	{kind: "Project", path: "projects"},
	{kind: "ProjectRelease", path: "projectreleases"},
	{kind: "ProjectReleaseBinding", path: "projectreleasebindings"},
	{kind: testComponentKind, path: "components"},
	{kind: testWorkloadKind, path: "workloads"},
	{kind: "ComponentRelease", path: "componentreleases"},
	{kind: "ReleaseBinding", path: "releasebindings"},
	{kind: "Resource", path: "resources"},
	{kind: "ResourceRelease", path: "resourcereleases"},
	{kind: "ResourceReleaseBinding", path: "resourcereleasebindings"},
}

type fetchFixture struct {
	t           *testing.T
	client      *client.Client
	collections map[string]collectionFixture
	calls       map[string][]string
	paginated   bool
	empty       bool
	override    testutil.RoundTripFunc
}

func newFetchFixture(t *testing.T, paginated bool) *fetchFixture {
	t.Helper()
	f := &fetchFixture{
		t:           t,
		collections: make(map[string]collectionFixture),
		calls:       make(map[string][]string),
		paginated:   paginated,
	}
	for _, collection := range requiredClusterCollections {
		f.collections["/api/v1/"+collection.path] = collection
	}
	for _, namespace := range []string{"acme", "payments"} {
		for _, collection := range requiredNamespacedCollections {
			f.collections["/api/v1/namespaces/"+namespace+"/"+collection.path] = collection
		}
	}
	f.collections["/api/v1/namespaces"] = collectionFixture{kind: testNamespaceKind, path: "namespaces"}

	testHome := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, testHome, config.StoredConfig{
		CurrentContext: "test",
		ControlPlanes:  []config.ControlPlane{{Name: "test", URL: "https://api.example.test"}},
		Credentials:    []config.Credential{{Name: "test", Token: testutil.NonExpiredJWT}},
		// Default CLI scope must not restrict this complete-inventory fetch.
		Contexts: []config.Context{{
			Name: "test", ControlPlane: "test", Credentials: "test",
			Namespace: "unrelated", Project: "unrelated", Component: "unrelated", Resource: "unrelated",
		}},
	})
	testutil.SetTransport(t, testutil.RoundTripFunc(f.roundTrip))
	var err error
	f.client, err = client.NewClient()
	require.NoError(t, err)
	return f
}

// roundTrip exercises the real OCC and generated clients while intercepting the
// HTTP boundary, without requiring a cluster or local listening socket.
func (f *fetchFixture) roundTrip(req *http.Request) (*http.Response, error) {
	f.t.Helper()
	assert.Equal(f.t, http.MethodGet, req.Method)
	assert.Equal(f.t, "api.example.test", req.URL.Host)
	assert.Equal(f.t, "Bearer "+testutil.NonExpiredJWT, req.Header.Get("Authorization"))
	query := req.URL.Query()
	for key := range query {
		assert.Contains(f.t, []string{"limit", "cursor"}, key, "unexpected selector on %s", req.URL.Path)
	}
	limit, err := strconv.Atoi(query.Get("limit"))
	require.NoError(f.t, err)
	assert.Positive(f.t, limit)
	path := req.URL.Path
	f.calls[path] = append(f.calls[path], query.Get("cursor"))
	if f.override != nil {
		response, err := f.override(req)
		if response != nil || err != nil {
			return response, err
		}
	}
	collection, ok := f.collections[path]
	if !ok {
		f.t.Errorf("unexpected collection requested: %s", path)
		return testutil.JSONResp(http.StatusNotFound, map[string]string{"error": "unexpected collection"}), nil
	}
	page := 0
	if query.Get("cursor") != "" {
		require.True(f.t, f.paginated)
		require.Equal(f.t, "next "+path, query.Get("cursor"), "cursor must belong to the requested collection")
		page = 1
	}
	items := make([]any, 0)
	pagination := make(map[string]any)
	if !f.empty {
		if collection.kind == testNamespaceKind {
			for i, namespace := range []string{"acme", "payments"} {
				if !f.paginated || i == page {
					items = append(items, map[string]any{"metadata": map[string]any{"name": namespace}})
				}
			}
		} else {
			items = append(items, fixtureObject(collection, path, page))
		}
		if f.paginated && page == 0 {
			pagination["nextCursor"] = "next " + path
		}
	}
	return testutil.JSONResp(http.StatusOK, map[string]any{"items": items, "pagination": pagination}), nil
}

func fixtureObject(collection collectionFixture, path string, page int) map[string]any {
	metadata := map[string]any{
		"name": collection.path + "-" + strconv.Itoa(page),
		"uid":  "keep-uid", "creationTimestamp": "2026-01-01T00:00:00Z",
		"labels": map[string]string{"keep": "label"}, "annotations": map[string]string{"keep": "annotation"},
	}
	if strings.HasPrefix(path, "/api/v1/namespaces/") {
		metadata["namespace"] = strings.Split(path, "/")[4]
	}
	object := map[string]any{
		"apiVersion": "openchoreo.dev/v1alpha1", "kind": collection.kind, "metadata": metadata,
	}
	// Application fixtures include ownership because serialization uses these
	// fields to place releases, bindings, and workloads in the repository tree.
	switch collection.kind {
	case "ProjectRelease", "ProjectReleaseBinding", "Resource":
		object["spec"] = map[string]any{"owner": map[string]string{"projectName": "store"}}
	case testWorkloadKind, "ComponentRelease", "ReleaseBinding":
		object["spec"] = map[string]any{"owner": map[string]string{"projectName": "store", "componentName": "checkout"}}
	case "ResourceRelease", "ResourceReleaseBinding":
		object["spec"] = map[string]any{"owner": map[string]string{"projectName": "store", "resourceName": "database"}}
	}
	if collection.kind == testComponentKind {
		object["spec"] = map[string]any{
			"owner":         map[string]string{"projectName": "store"},
			"componentType": map[string]string{"kind": "ClusterComponentType", "name": "deployment/service"},
			"parameters":    map[string]any{"replicas": 3, "nested": map[string]any{"keep": "value"}},
		}
		object["status"] = map[string]any{"observedGeneration": 7, "latestRelease": map[string]string{"name": "checkout-old-release"}}
	}
	return object
}

func TestFetch_AllRequiredResourcesAndPages(t *testing.T) {
	f := newFetchFixture(t, true)
	resources, err := Fetch(context.Background(), f.client)
	require.NoError(t, err)
	require.Len(t, resources, 2*len(requiredClusterCollections)+2+4*len(requiredNamespacedCollections))
	require.Len(t, f.calls, len(f.collections))
	for path := range f.collections {
		assert.Equal(t, []string{"", "next " + path}, f.calls[path], path)
	}

	counts := make(map[string]int)
	for _, resource := range resources {
		counts[resource.Kind+"/"+resource.Namespace]++
		assert.NotEmpty(t, resource.Name)
		if resource.Kind != testComponentKind {
			continue
		}
		component, ok := resource.Object.(gen.Component)
		require.True(t, ok, "the payload must remain a generated API value")
		assert.Equal(t, resource.Name, component.Metadata.Name)
		require.NotNil(t, component.Metadata.Namespace)
		assert.Equal(t, resource.Namespace, *component.Metadata.Namespace)
		page, err := strconv.Atoi(strings.TrimPrefix(resource.Name, "components-"))
		require.NoError(t, err)
		body, err := json.Marshal(fixtureObject(collectionFixture{kind: testComponentKind, path: "components"},
			"/api/v1/namespaces/"+resource.Namespace+"/components", page))
		require.NoError(t, err)
		var expected gen.Component
		require.NoError(t, json.Unmarshal(body, &expected))
		assert.Equal(t, expected, component, "fetch must preserve spec, status, metadata, and nested parameters")
	}
	assert.Equal(t, 2, counts["Namespace/"])
	for _, collection := range requiredClusterCollections {
		assert.Equal(t, 2, counts[collection.kind+"/"], collection.kind)
	}
	for _, namespace := range []string{"acme", "payments"} {
		for _, collection := range requiredNamespacedCollections {
			assert.Equal(t, 2, counts[collection.kind+"/"+namespace], collection.kind+"/"+namespace)
		}
	}
}

func TestFetch_EmptyInventory(t *testing.T) {
	f := newFetchFixture(t, false)
	f.empty = true
	resources, err := Fetch(context.Background(), f.client)
	require.NoError(t, err)
	assert.Empty(t, resources)
	assert.Len(t, f.calls, len(requiredClusterCollections)+1)
}

func TestFetch_EmptyPageWithNextCursor(t *testing.T) {
	f := newFetchFixture(t, true)
	path := "/api/v1/namespaces/acme/componentreleases"
	f.override = func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == path && req.URL.Query().Get("cursor") == "" {
			return testutil.JSONResp(http.StatusOK, map[string]any{
				"items": []any{}, "pagination": map[string]string{"nextCursor": "next " + path},
			}), nil
		}
		return nil, nil
	}
	resources, err := Fetch(context.Background(), f.client)
	require.NoError(t, err)
	var names []string
	for _, resource := range resources {
		if resource.Kind == "ComponentRelease" && resource.Namespace == "acme" {
			names = append(names, resource.Name)
		}
	}
	assert.Equal(t, []string{"componentreleases-1"}, names)
	assert.Equal(t, []string{"", "next " + path}, f.calls[path])
}

func TestFetch_APIErrorsDiscardPartialResults(t *testing.T) {
	for _, tc := range []struct {
		name, path, kind, namespace string
		status                      int
		laterPage                   bool
	}{
		{name: "cluster collection", path: "/api/v1/clustertraits", kind: "ClusterTrait", status: http.StatusForbidden},
		{name: "namespace discovery", path: "/api/v1/namespaces", kind: testNamespaceKind, status: http.StatusForbidden},
		{name: "namespaced collection", path: "/api/v1/namespaces/acme/resources", kind: "Resource", namespace: "acme", status: http.StatusInternalServerError},
		{name: "later release page", path: "/api/v1/namespaces/acme/resourcereleases", kind: "ResourceRelease", namespace: "acme", status: http.StatusServiceUnavailable, laterPage: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFetchFixture(t, true)
			f.override = func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == tc.path && (!tc.laterPage || req.URL.Query().Get("cursor") != "") {
					return testutil.JSONResp(tc.status, map[string]string{"error": "cannot list collection"}), nil
				}
				return nil, nil
			}
			resources, err := Fetch(context.Background(), f.client)
			require.Error(t, err)
			assert.Nil(t, resources)
			assert.Contains(t, err.Error(), tc.kind)
			assert.Contains(t, err.Error(), "cannot list collection")
			if tc.namespace != "" {
				assert.Contains(t, err.Error(), tc.namespace)
			}
			assert.NotContains(t, f.calls, "/api/v1/namespaces/payments/deploymentpipelines")
		})
	}
}

func TestFetch_TransportError(t *testing.T) {
	f := newFetchFixture(t, false)
	wantErr := errors.New("connection reset")
	f.override = func(_ *http.Request) (*http.Response, error) { return nil, wantErr }
	resources, err := Fetch(context.Background(), f.client)
	require.ErrorIs(t, err, wantErr)
	assert.Nil(t, resources)
	assert.Len(t, f.calls, 1)
}

func TestFetch_MalformedResponse(t *testing.T) {
	f := newFetchFixture(t, false)
	f.override = func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader("{invalid-json")),
		}, nil
	}
	resources, err := Fetch(context.Background(), f.client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ClusterProjectType")
	assert.Nil(t, resources)
}

func TestFetch_CanceledBeforeRequest(t *testing.T) {
	f := newFetchFixture(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resources, err := Fetch(ctx, f.client)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, resources)
	assert.Empty(t, f.calls)
}

func TestFetch_CancellationReachesHTTPClient(t *testing.T) {
	f := newFetchFixture(t, true)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), fetchContextKey{}, "fetch"))
	defer cancel()
	f.override = func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "fetch", req.Context().Value(fetchContextKey{}))
		cancel()
		return nil, req.Context().Err()
	}
	resources, err := Fetch(ctx, f.client)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, resources)
	assert.Equal(t, []string{""}, f.calls["/api/v1/clusterprojecttypes"])
	assert.Len(t, f.calls, 1)
}

func TestFetch_CancellationAfterPage(t *testing.T) {
	f := newFetchFixture(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.override = func(_ *http.Request) (*http.Response, error) {
		cancel()
		return testutil.JSONResp(http.StatusOK, map[string]any{
			"items": []any{}, "pagination": map[string]string{"nextCursor": "next-page"},
		}), nil
	}
	resources, err := Fetch(ctx, f.client)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, resources)
	assert.Equal(t, []string{""}, f.calls["/api/v1/clusterprojecttypes"])
}

func TestFetch_InvalidIdentity(t *testing.T) {
	for _, path := range []string{"/api/v1/clusterprojecttypes", "/api/v1/namespaces"} {
		t.Run(path, func(t *testing.T) {
			f := newFetchFixture(t, false)
			f.override = func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == path {
					return testutil.JSONResp(http.StatusOK, map[string]any{
						"items": []any{map[string]any{"metadata": map[string]any{}}}, "pagination": map[string]any{},
					}), nil
				}
				return nil, nil
			}
			resources, err := Fetch(context.Background(), f.client)
			require.ErrorContains(t, err, "metadata.name")
			assert.Nil(t, resources)
		})
	}
}

func TestFetch_NilListResponse(t *testing.T) {
	c := mocks.NewMockInterface(t)
	c.EXPECT().ListClusterProjectTypes(mock.Anything, mock.Anything).Return(nil, nil)
	resources, err := Fetch(context.Background(), c)
	require.ErrorContains(t, err, "nil list response")
	assert.Nil(t, resources)
}

func TestFetch_NilClient(t *testing.T) {
	resources, err := Fetch(context.Background(), nil)
	require.EqualError(t, err, "API client is required")
	assert.Nil(t, resources)
}
