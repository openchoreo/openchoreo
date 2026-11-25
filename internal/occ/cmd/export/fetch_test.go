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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const (
	testComponentKind        = "Component"
	testComponentReleaseKind = "ComponentRelease"
	testReleaseLineKind      = "ReleaseLine"
	testNamespaceKind        = "Namespace"
	testWorkloadKind         = "Workload"
)

// collectionFixture defines the expected API collection inventory.
type collectionFixture struct {
	kind string
	path string
}

type fetchContextKey struct{}

var requiredClusterCollections = []collectionFixture{
	{kind: "ClusterProjectType", path: "clusterprojecttypes"},
	{kind: "ClusterComponentType", path: "clustercomponenttypes"},
	{kind: "ClusterResourceType", path: "clusterresourcetypes"},
	{kind: "ClusterEndpointType", path: "clusterendpointtypes"},
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
	{kind: "EndpointType", path: "endpointtypes"},
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
	{kind: testComponentReleaseKind, path: "componentreleases"},
	{kind: "ReleaseBinding", path: "releasebindings"},
	{kind: "Resource", path: "resources"},
	{kind: "ResourceRelease", path: "resourcereleases"},
	{kind: "ResourceReleaseBinding", path: "resourcereleasebindings"},
	{kind: endpointKind, path: "endpoints"},
	{kind: endpointReleaseKind, path: "endpointreleases"},
	{kind: "EndpointReleaseBinding", path: "endpointreleasebindings"},
	{kind: "ObservabilityAlertsNotificationChannel", path: "observabilityalertsnotificationchannels"},
}

var requiredComponentCollections = []collectionFixture{
	{kind: testReleaseLineKind, path: "releaselines"},
}

type fetchFixture struct {
	t               *testing.T
	client          *client.Client
	host            string
	collections     map[string]collectionFixture
	calls           map[string][]string
	paginated       bool
	empty           bool
	override        testutil.RoundTripFunc
	profileOverride testutil.RoundTripFunc
	profileCalls    int
}

func newFetchFixture(t *testing.T, paginated bool) *fetchFixture {
	t.Helper()
	f := &fetchFixture{
		t:           t,
		host:        "api.example.test",
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
		components := 1
		if paginated {
			components = 2
		}
		for i := range components {
			for _, collection := range requiredComponentCollections {
				path := "/api/v1/namespaces/" + namespace + "/components/components-" + strconv.Itoa(i) + "/" + collection.path
				f.collections[path] = collection
			}
		}
	}
	f.collections["/api/v1/namespaces"] = collectionFixture{kind: testNamespaceKind, path: "namespaces"}

	testHome := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, testHome, config.StoredConfig{
		CurrentContext: "test",
		ControlPlanes:  []config.ControlPlane{{Name: "test", URL: "https://api.example.test"}},
		Credentials:    []config.Credential{{Name: "test", Token: testutil.NonExpiredJWT}},
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

// roundTrip intercepts HTTP requests from the OCC and generated clients.
func (f *fetchFixture) roundTrip(req *http.Request) (*http.Response, error) {
	f.t.Helper()
	assert.Equal(f.t, http.MethodGet, req.Method)
	assert.Equal(f.t, f.host, req.URL.Host)
	assert.Equal(f.t, "Bearer "+testutil.NonExpiredJWT, req.Header.Get("Authorization"))
	if req.URL.Path == "/api/v1/authz/profile" {
		f.profileCalls++
		assert.Empty(f.t, req.URL.RawQuery, "export must request an unscoped authorization profile")
		if f.profileOverride != nil {
			return f.profileOverride(req)
		}
		return testutil.JSONResp(http.StatusOK, fullExportProfile()), nil
	}
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
		} else if collection.kind == testReleaseLineKind {
			// Add the implicit main entry on the first page.
			if page == 0 {
				component := strings.Split(path, "/")[6]
				items = append(items, releaseLineTestObject(strings.Split(path, "/")[4], component, "main"))
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

func fullExportProfile() gen.UserCapabilitiesResponse {
	wildcard := "*"
	capabilities := make(map[string]gen.ActionCapability)
	collections := append([]collectionFixture{{kind: testNamespaceKind}}, requiredClusterCollections...)
	collections = append(collections, requiredNamespacedCollections...)
	collections = append(collections, requiredComponentCollections...)
	for _, collection := range collections {
		allowed := []gen.CapabilityResource{{Path: &wildcard}}
		capabilities[strings.ToLower(collection.kind)+":view"] = gen.ActionCapability{Allowed: &allowed}
	}
	return gen.UserCapabilitiesResponse{Capabilities: &capabilities}
}

func fixtureObject(collection collectionFixture, path string, page int) map[string]any {
	metadata := map[string]any{
		"name": collection.path + "-" + strconv.Itoa(page),
		"uid":  "keep-uid", "creationTimestamp": fixtureCreationTimestamp,
		"labels": map[string]string{"keep": "label"},
		"annotations": map[string]string{
			"keep": "annotation", lastAppliedAnnotation: "previous apply data", restartedAtAnnotation: "old-restart",
			"meta.helm.sh/release-name": "existing-release", "kustomize.toolkit.fluxcd.io/ssa": "Ignore",
		},
	}
	if strings.HasPrefix(path, "/api/v1/namespaces/") {
		metadata["namespace"] = strings.Split(path, "/")[4]
	}
	object := map[string]any{
		"apiVersion": "openchoreo.dev/v1alpha1", "kind": collection.kind, "metadata": metadata,
	}
	// Add ownership fields to application fixtures.
	switch collection.kind {
	case "ProjectRelease", "ProjectReleaseBinding", "Resource", endpointKind:
		object["spec"] = map[string]any{"owner": map[string]string{"projectName": "store"}}
	case testWorkloadKind, testComponentReleaseKind, "ReleaseBinding", testReleaseLineKind:
		object["spec"] = map[string]any{"owner": map[string]string{"projectName": "store", "componentName": "checkout"}}
	case "ResourceRelease", "ResourceReleaseBinding":
		object["spec"] = map[string]any{"owner": map[string]string{"projectName": "store", "resourceName": "database"}}
	case endpointReleaseKind, "EndpointReleaseBinding":
		object["spec"] = map[string]any{"owner": map[string]string{"projectName": "store", "endpointName": "public-api"}}
	case "ObservabilityAlertsNotificationChannel":
		object["spec"] = map[string]any{
			"environment": "development", "type": "webhook", "isEnvDefault": true,
			"webhookConfig": map[string]any{
				"url":             "https://hooks.example.test/alerts",
				"headers":         map[string]any{"Authorization": map[string]string{"value": "Bearer sample-token"}},
				"payloadTemplate": `{"message":"{{ .Message }}"}`,
			},
		}
	}
	endpointTypeSpec := map[string]any{"resources": []any{map[string]any{
		"id": "route", "template": map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
			"spec": map[string]any{"hostnames": []string{"api.example.test"}},
		},
	}}}
	switch collection.kind {
	case "EndpointType", "ClusterEndpointType":
		object["spec"] = endpointTypeSpec
	case endpointKind, endpointReleaseKind:
		spec := object["spec"].(map[string]any)
		spec["parameters"] = map[string]any{"path": "/api"}
		spec["visibility"] = []string{"external"}
		spec["backendRefs"] = []any{map[string]any{
			"name": "backend", "kind": "Component", "weight": 100,
			"component": map[string]any{"name": "checkout", "endpoint": "http"},
		}}
		if collection.kind == endpointKind {
			spec["type"] = map[string]any{"kind": "ClusterEndpointType", "name": "http-route"}
		} else {
			spec["endpointType"] = map[string]any{"kind": "ClusterEndpointType", "name": "http-route", "spec": endpointTypeSpec}
		}
	case "EndpointReleaseBinding":
		object["spec"].(map[string]any)["environment"] = "development"
	}
	if collection.kind == testComponentKind {
		object["spec"] = map[string]any{
			"owner":         map[string]string{"projectName": "store"},
			"componentType": map[string]string{"kind": "ClusterComponentType", "name": "deployment/service"},
			"autoDeploy":    true,
			"parameters":    map[string]any{"replicas": 3, "nested": map[string]any{"keep": "value"}},
		}
		object["status"] = map[string]any{"observedGeneration": 7, "latestRelease": map[string]string{"name": "checkout-old-release"}}
	}
	return object
}

func TestFetch_AllRequiredResourcesAndPages(t *testing.T) {
	f := newFetchFixture(t, true)
	resources, err := fetchAllTestResources(context.Background(), f.client)
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
		(*expected.Spec.Parameters)["replicas"] = json.Number("3")
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
	resources, err := fetchAllTestResources(context.Background(), f.client)
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
	resources, err := fetchAllTestResources(context.Background(), f.client)
	require.NoError(t, err)
	var names []string
	for _, resource := range resources {
		if resource.Kind == testComponentReleaseKind && resource.Namespace == "acme" {
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
		{name: "component collection", path: testComponentsPath, kind: testComponentKind, namespace: "acme", status: http.StatusForbidden},
		{name: "later component page", path: testComponentsPath, kind: testComponentKind, namespace: "acme", status: http.StatusServiceUnavailable, laterPage: true},
		{name: "component release lines", path: "/api/v1/namespaces/acme/components/components-0/releaselines", kind: testReleaseLineKind,
			namespace: "acme", status: http.StatusForbidden},
		{name: "later release line page", path: "/api/v1/namespaces/acme/components/components-0/releaselines", kind: testReleaseLineKind,
			namespace: "acme", status: http.StatusServiceUnavailable, laterPage: true},
		{name: "later release page", path: "/api/v1/namespaces/acme/resourcereleases", kind: "ResourceRelease", namespace: "acme", status: http.StatusServiceUnavailable, laterPage: true},
		{name: "cluster endpoint types", path: "/api/v1/clusterendpointtypes", kind: "ClusterEndpointType", status: http.StatusForbidden},
		{name: "endpoints", path: "/api/v1/namespaces/acme/endpoints", kind: endpointKind, namespace: "acme", status: http.StatusForbidden},
		{name: "endpoint types", path: "/api/v1/namespaces/acme/endpointtypes", kind: "EndpointType", namespace: "acme", status: http.StatusForbidden},
		{name: "notification channels", path: "/api/v1/namespaces/acme/observabilityalertsnotificationchannels", kind: "ObservabilityAlertsNotificationChannel", namespace: "acme", status: http.StatusForbidden},
		{name: "endpoint release bindings", path: "/api/v1/namespaces/acme/endpointreleasebindings", kind: "EndpointReleaseBinding", namespace: "acme", status: http.StatusForbidden},
		{name: "later endpoint release page", path: "/api/v1/namespaces/acme/endpointreleases", kind: endpointReleaseKind, namespace: "acme", status: http.StatusServiceUnavailable, laterPage: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFetchFixture(t, true)
			f.override = func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == tc.path && (!tc.laterPage || req.URL.Query().Get("cursor") != "") {
					return testutil.JSONResp(tc.status, map[string]string{"error": "cannot list collection"}), nil
				}
				return nil, nil
			}
			resources, err := fetchAllTestResources(context.Background(), f.client)
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
	resources, err := fetchAllTestResources(context.Background(), f.client)
	require.ErrorIs(t, err, wantErr)
	assert.Nil(t, resources)
	assert.Len(t, f.calls, 1)
}

func TestFetch_MalformedResponse(t *testing.T) {
	for _, tc := range []struct{ name, path, kind, body string }{
		{"cluster collection", "/api/v1/clustertraits", "ClusterTrait", "{invalid-json"},
		{"trailing cluster data", "/api/v1/clustertraits", "ClusterTrait", `{"items":[],"pagination":{}} {}`},
		{"namespace discovery", "/api/v1/namespaces", testNamespaceKind, "{invalid-json"},
		{"component collection", testComponentsPath, testComponentKind, "{invalid-json"},
		{"trailing component data", testComponentsPath, testComponentKind, `{"items":[],"pagination":{}} {}`},
		{"binding collection", "/api/v1/namespaces/acme/releasebindings", "ReleaseBinding", "{invalid-json"},
		{"trailing release data", "/api/v1/namespaces/acme/componentreleases", testComponentReleaseKind, `{"items":[],"pagination":{}} {}`},
		{"trailing release line data", "/api/v1/namespaces/acme/components/components-0/releaselines", testReleaseLineKind, `{"items":[],"pagination":{}} {}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFetchFixture(t, false)
			f.override = func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != tc.path {
					return nil, nil
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(tc.body)),
				}, nil
			}
			resources, err := fetchAllTestResources(context.Background(), f.client)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.kind)
			assert.Nil(t, resources)
		})
	}
}

func TestFetch_CanceledBeforeRequest(t *testing.T) {
	f := newFetchFixture(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resources, err := fetchAllTestResources(ctx, f.client)
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
	resources, err := fetchAllTestResources(ctx, f.client)
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
	resources, err := fetchAllTestResources(ctx, f.client)
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
			resources, err := fetchAllTestResources(context.Background(), f.client)
			require.ErrorContains(t, err, "metadata.name")
			assert.Nil(t, resources)
		})
	}
}

func TestFetch_NilListResponse(t *testing.T) {
	resources, err := fetchAll(context.Background(), "ClusterProjectType", "", func(int, string) (*resourcePage[gen.ClusterProjectType], error) {
		return nil, nil
	})
	require.ErrorContains(t, err, "nil list response")
	assert.Nil(t, resources)
}

func TestFetch_RequiresGeneratedClientAccess(t *testing.T) {
	for _, tc := range []struct {
		name string
		api  client.Interface
		want string
	}{
		{"missing accessor", mocks.NewMockInterface(t), "API client does not expose the generated client"},
		{"nil generated client", exportClientWithMissingGeneratedClient{}, "generated API client is missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resources, err := fetchAllTestResources(context.Background(), tc.api)
			require.ErrorContains(t, err, tc.want)
			assert.Nil(t, resources)
		})
	}
}

func TestDecodePage_InvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response *http.Response
		want     string
	}{
		{"nil response", nil, "nil list response"},
		{"nil body", &http.Response{StatusCode: http.StatusOK}, "nil list response or body"},
		{"plain error", &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("unavailable"))}, "HTTP 502"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := decodePage[gen.ReleaseBinding](tc.response, nil)
			require.ErrorContains(t, err, tc.want)
			assert.Nil(t, page)
		})
	}
}

func TestFetch_NilClient(t *testing.T) {
	resources, err := fetchAllTestResources(context.Background(), nil)
	require.EqualError(t, err, "API client is required")
	assert.Nil(t, resources)
}

// fetchAllTestResources fetches complete history for inventory tests.
func fetchAllTestResources(ctx context.Context, c client.Interface) ([]Resource, error) {
	result, err := Fetch(ctx, c, true)
	if err != nil {
		return nil, err
	}
	return result.Resources, nil
}

var fixtureCreationTimestamp = time.Now().UTC().Format(time.RFC3339)

type releaseTestCase struct {
	kind, collection, bindingKind, bindings, reference, directory string
}

var releaseTestCases = []releaseTestCase{
	{testComponentReleaseKind, "componentreleases", "ReleaseBinding", "releasebindings", "releaseName", "components/checkout/releases"},
	{"ProjectRelease", "projectreleases", "ProjectReleaseBinding", "projectreleasebindings", "projectRelease", "releases"},
	{"ResourceRelease", "resourcereleases", "ResourceReleaseBinding", "resourcereleasebindings", "resourceRelease", "resources/database/releases"},
	{endpointReleaseKind, "endpointreleases", "EndpointReleaseBinding", "endpointreleasebindings", "endpointRelease", "endpoints/public-api/releases"},
}

func releaseTestObject(tc releaseTestCase, namespace, name string, created *time.Time) map[string]any {
	object := fixtureObject(collectionFixture{kind: tc.kind, path: tc.collection}, "/api/v1/namespaces/"+namespace+"/"+tc.collection, 0)
	metadata := object["metadata"].(map[string]any)
	metadata["name"] = name
	delete(metadata, "creationTimestamp")
	if created != nil {
		metadata["creationTimestamp"] = created.Format(time.RFC3339Nano)
	}
	return object
}

func bindingTestObject(tc releaseTestCase, name string) map[string]any {
	object := fixtureObject(collectionFixture{kind: tc.bindingKind, path: tc.bindings}, "/api/v1/namespaces/acme/"+tc.bindings, 0)
	object["spec"].(map[string]any)[tc.reference] = name
	return object
}

func TestFetch_RecentAndReferencedReleasesAcrossPages(t *testing.T) {
	cutoff := time.Date(2026, time.August, 19, 10, 0, 0, 0, time.UTC)
	old, recent, before := cutoff.AddDate(-1, 0, 0), cutoff.AddDate(0, 1, 0), cutoff.Add(-time.Nanosecond)
	for _, tc := range releaseTestCases {
		for _, all := range []bool{false, true} {
			t.Run(tc.kind+"/all="+strconv.FormatBool(all), func(t *testing.T) {
				f := newFetchFixture(t, true)
				path := "/api/v1/namespaces/acme/" + tc.collection
				f.override = func(req *http.Request) (*http.Response, error) {
					if req.URL.Path == "/api/v1/namespaces/acme/"+tc.bindings {
						assert.Empty(t, f.calls[path], "bindings must be read before releases")
						page := map[string]any{"items": []any{bindingTestObject(tc, "old-referenced")}, "pagination": map[string]any{}}
						if req.URL.Query().Get("cursor") == "" {
							page["pagination"] = map[string]string{"nextCursor": "next " + req.URL.Path}
						}
						return testutil.JSONResp(http.StatusOK, page), nil
					}
					if req.URL.Path == "/api/v1/namespaces/payments/"+tc.collection {
						// Return the same release name in another namespace.
						return testutil.JSONResp(http.StatusOK, map[string]any{
							"items": []any{releaseTestObject(tc, "payments", "old-referenced", &old)}, "pagination": map[string]any{},
						}), nil
					}
					if req.URL.Path != path {
						return nil, nil
					}
					for _, binding := range []string{"releasebindings", "projectreleasebindings", "resourcereleasebindings"} {
						assert.NotEmpty(t, f.calls["/api/v1/namespaces/acme/"+binding])
					}
					if req.URL.Query().Get("cursor") == "" {
						// Return an excluded release with a next-page cursor.
						return testutil.JSONResp(http.StatusOK, map[string]any{
							"items":      []any{releaseTestObject(tc, "acme", "old-unreferenced", &old)},
							"pagination": map[string]string{"nextCursor": "next " + path},
						}), nil
					}
					deleting := releaseTestObject(tc, "acme", "deleting", &recent)
					deleting["metadata"].(map[string]any)["deletionTimestamp"] = recent.Format(time.RFC3339)
					return testutil.JSONResp(http.StatusOK, map[string]any{"items": []any{
						releaseTestObject(tc, "acme", "old-referenced", &old),
						releaseTestObject(tc, "acme", "recent", &recent),
						releaseTestObject(tc, "acme", "at-cutoff", &cutoff),
						releaseTestObject(tc, "acme", "before-cutoff", &before),
						releaseTestObject(tc, "acme", "unknown-age", nil), deleting,
					}, "pagination": map[string]any{}}), nil
				}
				result, err := fetchResources(context.Background(), f.client, all, cutoff)
				require.NoError(t, err)
				var names, otherNamespace []string
				for _, resource := range result.Resources {
					if resource.Kind == tc.kind {
						if resource.Namespace == "acme" {
							names = append(names, resource.Name)
						} else {
							otherNamespace = append(otherNamespace, resource.Name)
						}
					}
				}
				want := []string{"old-referenced", "recent", "at-cutoff", "unknown-age"}
				if all {
					assert.Nil(t, result.ReleaseCutoff)
					want = append(want, "old-unreferenced", "before-cutoff")
					assert.Equal(t, []string{"old-referenced"}, otherNamespace)
					assert.Empty(t, result.PreservedPaths)
				} else {
					require.NotNil(t, result.ReleaseCutoff)
					assert.Equal(t, cutoff, *result.ReleaseCutoff)
					assert.Empty(t, otherNamespace)
					assert.Len(t, result.PreservedPaths, 3)
					for _, name := range []string{"old-unreferenced", "before-cutoff"} {
						assert.Contains(t, result.PreservedPaths, "namespaces/acme/projects/store/"+tc.directory+"/"+name+".yaml")
					}
					assert.Contains(t, result.PreservedPaths, "namespaces/payments/projects/store/"+tc.directory+"/old-referenced.yaml")
				}
				assert.ElementsMatch(t, want, names)
				assert.Equal(t, []string{"", "next " + path}, f.calls[path])
			})
		}
	}
}

func TestFetch_MissingOrDeletingReferencedRelease(t *testing.T) {
	for _, tc := range releaseTestCases {
		for _, deleting := range []bool{false, true} {
			for _, all := range []bool{false, true} {
				t.Run(tc.kind+"/deleting="+strconv.FormatBool(deleting)+"/all="+strconv.FormatBool(all), func(t *testing.T) {
					f := newFetchFixture(t, false)
					f.override = func(req *http.Request) (*http.Response, error) {
						if req.URL.Path == "/api/v1/namespaces/acme/"+tc.bindings {
							return testutil.JSONResp(http.StatusOK, map[string]any{
								"items": []any{bindingTestObject(tc, "required-release")}, "pagination": map[string]any{},
							}), nil
						}
						if deleting && req.URL.Path == "/api/v1/namespaces/acme/"+tc.collection {
							object := releaseTestObject(tc, "acme", "required-release", nil)
							object["metadata"].(map[string]any)["deletionTimestamp"] = fixtureCreationTimestamp
							return testutil.JSONResp(http.StatusOK, map[string]any{"items": []any{object}, "pagination": map[string]any{}}), nil
						}
						return nil, nil
					}
					result, err := Fetch(context.Background(), f.client, all)
					require.NoError(t, err)
					assert.Equal(t, []string{"binding-referenced release is missing or marked for deletion: " + tc.kind + " \"required-release\" in namespace \"acme\""}, result.Warnings)
					var bindingFound bool
					for _, resource := range result.Resources {
						if resource.Kind == tc.bindingKind && resource.Namespace == testExportNamespace {
							bindingFound = true
						}
						if resource.Kind == tc.kind {
							assert.NotEqual(t, "required-release", resource.Name)
						}
					}
					assert.True(t, bindingFound, "the binding must remain in the export")
				})
			}
		}
	}
}

func TestReleaseCutoff_CalendarMonthsAndUTC(t *testing.T) {
	for _, tc := range []struct{ now, want string }{
		{"2026-11-19T10:20:30Z", "2026-08-19T10:20:30Z"},
		{"2026-05-31T10:20:30Z", "2026-02-28T10:20:30Z"},
		{"2024-05-31T10:20:30Z", "2024-02-29T10:20:30Z"},
		{"2026-01-01T00:30:00+05:30", "2025-09-30T19:00:00Z"},
	} {
		t.Run(tc.now, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tc.now)
			require.NoError(t, err)
			assert.Equal(t, tc.want, releaseCutoff(now).Format(time.RFC3339))
		})
	}
}

func releaseLineTestObject(namespace, component, name string) map[string]any {
	object := fixtureObject(collectionFixture{kind: testReleaseLineKind, path: "releaselines"},
		"/api/v1/namespaces/"+namespace+"/components/"+component+"/releaselines", 0)
	object["metadata"].(map[string]any)["name"] = name
	object["spec"].(map[string]any)["owner"] = map[string]string{"projectName": "store", "componentName": component}
	return object
}

func TestFetch_ReleaseLinesForEveryComponentAndPage(t *testing.T) {
	f := newFetchFixture(t, true)
	f.override = func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/releaselines") {
			return nil, nil
		}
		parts := strings.Split(req.URL.Path, "/")
		namespace, component := parts[4], parts[6]
		page := 0
		if req.URL.Query().Get("cursor") != "" {
			page = 1
		}
		line := releaseLineTestObject(namespace, component, component+"-v"+strconv.Itoa(page+2))
		line["metadata"].(map[string]any)["creationTimestamp"] = "2000-01-01T00:00:00Z"
		line["spec"].(map[string]any)["autoDeploy"] = false
		line["spec"].(map[string]any)["parameters"] = map[string]any{"replicas": 2}
		line["status"] = map[string]any{"observedGeneration": 1}
		items := []any{line}
		pagination := map[string]string{}
		if page == 0 {
			items = append([]any{releaseLineTestObject(namespace, component, "main")}, items...)
			pagination["nextCursor"] = "next " + req.URL.Path
		}
		return testutil.JSONResp(http.StatusOK, map[string]any{"items": items, "pagination": pagination}), nil
	}
	result, err := Fetch(context.Background(), f.client, false)
	require.NoError(t, err)
	var lines []Resource
	for _, resource := range result.Resources {
		if resource.Kind == testReleaseLineKind {
			lines = append(lines, resource)
		}
	}
	require.Len(t, lines, 8, "two namespaces, two components, and two release lines each")
	for _, resource := range lines {
		line, ok := resource.Object.(gen.ReleaseLine)
		require.True(t, ok, "fetch must retain the typed API object")
		require.NotNil(t, line.Spec)
		assert.NotEqual(t, "main", line.Metadata.Name)
		assert.Equal(t, "store", line.Spec.Owner.ProjectName)
		assert.Contains(t, []string{"components-0", "components-1"}, line.Spec.Owner.ComponentName)
		assert.Contains(t, []string{"acme", "payments"}, resource.Namespace)
		require.NotNil(t, line.Spec.AutoDeploy)
		assert.False(t, *line.Spec.AutoDeploy)
		assert.NotNil(t, line.Status)
		require.NotNil(t, line.Metadata.CreationTimestamp)
		assert.Equal(t, 2000, line.Metadata.CreationTimestamp.Year(), "the history cutoff must not exclude ReleaseLine configuration")
		path := "/api/v1/namespaces/" + resource.Namespace + "/components/" + line.Spec.Owner.ComponentName + "/releaselines"
		assert.Equal(t, []string{"", "next " + path}, f.calls[path])
	}
}

func TestFetch_NoReleaseLineCallsWithoutComponents(t *testing.T) {
	f := newFetchFixture(t, false)
	f.override = func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/components") {
			return testutil.JSONResp(http.StatusOK, map[string]any{"items": []any{}, "pagination": map[string]any{}}), nil
		}
		return nil, nil
	}
	result, err := Fetch(context.Background(), f.client, false)
	require.NoError(t, err)
	for _, resource := range result.Resources {
		assert.NotEqual(t, testReleaseLineKind, resource.Kind)
	}
	for path := range f.calls {
		assert.False(t, strings.HasSuffix(path, "/releaselines"), path)
	}
}
