// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterendpointtypemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterendpointtype/mocks"
	endpointmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpoint/mocks"
	endpointreleasemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointrelease/mocks"
	endpointreleasebindingmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointreleasebinding/mocks"
	endpointtypemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointtype/mocks"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

const (
	testEndpointName               = "orders-api"
	testEndpointTypeName           = "http-route"
	testEndpointReleaseName        = "orders-api-abc123"
	testEndpointReleaseBindingName = "orders-api-development"
	testEndpointEnvironment        = "development"
)

func sampleEndpoint() *openchoreov1alpha1.Endpoint {
	return &openchoreov1alpha1.Endpoint{
		ObjectMeta: metav1.ObjectMeta{Name: testEndpointName, Namespace: testNS},
		Spec: openchoreov1alpha1.EndpointSpec{
			Owner: openchoreov1alpha1.EndpointOwner{ProjectName: testProject},
			Type: openchoreov1alpha1.EndpointTypeRef{
				Kind: openchoreov1alpha1.EndpointTypeRefKindClusterEndpointType,
				Name: testEndpointTypeName,
			},
			Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
			BackendRefs: []openchoreov1alpha1.EndpointBackendRef{{
				Name:      "primary",
				Kind:      openchoreov1alpha1.EndpointBackendKindComponent,
				Component: &openchoreov1alpha1.EndpointComponentBackend{Name: "orders", Endpoint: "http"},
				Weight:    ptr.To[int32](100),
			}},
		},
	}
}

func sampleEndpointReleaseBinding() *openchoreov1alpha1.EndpointReleaseBinding {
	return &openchoreov1alpha1.EndpointReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: testEndpointReleaseBindingName, Namespace: testNS},
		Spec: openchoreov1alpha1.EndpointReleaseBindingSpec{
			Owner: openchoreov1alpha1.EndpointReleaseBindingOwner{
				ProjectName:  testProject,
				EndpointName: testEndpointName,
			},
			Environment:     testEndpointEnvironment,
			EndpointRelease: testEndpointReleaseName,
		},
	}
}

// ---------------------------------------------------------------------------
// Endpoint
// ---------------------------------------------------------------------------

func TestListEndpoints(t *testing.T) {
	ctx := context.Background()

	t.Run("returns wrapped list with backends", func(t *testing.T) {
		svc := endpointmocks.NewMockService(t)
		svc.EXPECT().
			ListEndpoints(mock.Anything, testNS, testProject, mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.Endpoint]{
				Items:      []openchoreov1alpha1.Endpoint{*sampleEndpoint()},
				NextCursor: "next-token",
			}, nil)

		h := newTestHandler(withEndpointService(svc))
		result, err := h.ListEndpoints(ctx, testNS, testProject, tools.ListOpts{Limit: 5})
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "next-token", m["next_cursor"])
		items, ok := m["endpoints"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, items, 1)
		assert.Equal(t, testEndpointName, items[0]["name"])
		assert.Equal(t, []string{"external"}, items[0]["visibility"])
		backends, ok := items[0]["backends"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, backends, 1)
		assert.Equal(t, "orders", backends[0]["component"])
		assert.Equal(t, int32(100), backends[0]["weight"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("list failed")
		svc := endpointmocks.NewMockService(t)
		svc.EXPECT().
			ListEndpoints(mock.Anything, testNS, testProject, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withEndpointService(svc))
		_, err := h.ListEndpoints(ctx, testNS, testProject, tools.ListOpts{})
		require.ErrorIs(t, err, expected)
	})
}

func TestCreateEndpoint(t *testing.T) {
	ctx := context.Background()

	newReq := func(owner string) *gen.CreateEndpointJSONRequestBody {
		spec := &gen.EndpointSpec{
			Type:       gen.EndpointTypeRef{Name: testEndpointTypeName},
			Visibility: []gen.EndpointSpecVisibility{"external"},
			BackendRefs: []gen.EndpointBackendRef{{
				Name:      "primary",
				Component: &gen.EndpointComponentBackend{Name: "orders", Endpoint: "http"},
			}},
		}
		spec.Owner.ProjectName = owner
		return &gen.CreateEndpointJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testEndpointName},
			Spec:     spec,
		}
	}

	t.Run("fills owner project from the argument", func(t *testing.T) {
		svc := endpointmocks.NewMockService(t)
		var sent *openchoreov1alpha1.Endpoint
		svc.EXPECT().
			CreateEndpoint(mock.Anything, testNS, mock.Anything).
			Run(func(_ context.Context, _ string, ep *openchoreov1alpha1.Endpoint) { sent = ep }).
			Return(sampleEndpoint(), nil)

		h := newTestHandler(withEndpointService(svc))
		result, err := h.CreateEndpoint(ctx, testNS, testProject, newReq(""))
		require.NoError(t, err)

		require.NotNil(t, sent)
		assert.Equal(t, testProject, sent.Spec.Owner.ProjectName)
		assert.Equal(t, testNS, sent.Namespace)
		require.Len(t, sent.Spec.BackendRefs, 1)
		assert.Equal(t, "orders", sent.Spec.BackendRefs[0].Component.Name)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
	})

	t.Run("rejects a mismatched owner project", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.CreateEndpoint(ctx, testNS, testProject, newReq("other-project"))
		require.Error(t, err)
	})

	t.Run("rejects a missing spec", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.CreateEndpoint(ctx, testNS, testProject, &gen.CreateEndpointJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testEndpointName},
		})
		require.Error(t, err)
	})
}

func TestUpdateEndpoint(t *testing.T) {
	ctx := context.Background()

	t.Run("replaces backends and keeps immutable fields", func(t *testing.T) {
		existing := sampleEndpoint()
		svc := endpointmocks.NewMockService(t)
		svc.EXPECT().GetEndpoint(mock.Anything, testNS, testEndpointName).Return(existing, nil)
		var sent *openchoreov1alpha1.Endpoint
		svc.EXPECT().
			UpdateEndpoint(mock.Anything, testNS, mock.Anything).
			Run(func(_ context.Context, _ string, ep *openchoreov1alpha1.Endpoint) { sent = ep }).
			Return(existing, nil)

		params := map[string]any{"timeout": "5s"}
		req := &gen.UpdateEndpointJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testEndpointName},
			Spec: &gen.EndpointSpec{
				BackendRefs: []gen.EndpointBackendRef{
					{
						Name:      "primary",
						Component: &gen.EndpointComponentBackend{Name: "orders", Endpoint: "http"},
						Weight:    ptr.To[int32](90),
					},
					{
						Name:      "canary",
						Component: &gen.EndpointComponentBackend{Name: "orders-v2", Endpoint: "http"},
						Weight:    ptr.To[int32](10),
					},
				},
				Parameters: &params,
			},
		}

		h := newTestHandler(withEndpointService(svc))
		_, err := h.UpdateEndpoint(ctx, testNS, req)
		require.NoError(t, err)

		require.NotNil(t, sent)
		require.Len(t, sent.Spec.BackendRefs, 2)
		assert.Equal(t, "canary", sent.Spec.BackendRefs[1].Name)
		assert.Equal(t, int32(10), *sent.Spec.BackendRefs[1].Weight)
		// Omitted visibility and the immutable owner/type survive the update.
		assert.Equal(t, []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
			sent.Spec.Visibility)
		assert.Equal(t, testProject, sent.Spec.Owner.ProjectName)
		assert.Equal(t, openchoreov1alpha1.EndpointTypeRefKindClusterEndpointType, sent.Spec.Type.Kind)
		require.NotNil(t, sent.Spec.Parameters)
		var got map[string]any
		require.NoError(t, json.Unmarshal(sent.Spec.Parameters.Raw, &got))
		assert.Equal(t, "5s", got["timeout"])
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.UpdateEndpoint(ctx, testNS, nil)
		require.Error(t, err)
	})
}

func TestDeleteEndpoint(t *testing.T) {
	svc := endpointmocks.NewMockService(t)
	svc.EXPECT().DeleteEndpoint(mock.Anything, testNS, testEndpointName).Return(nil)

	h := newTestHandler(withEndpointService(svc))
	result, err := h.DeleteEndpoint(context.Background(), testNS, testEndpointName)
	require.NoError(t, err)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "deleted", m["action"])
}

// ---------------------------------------------------------------------------
// EndpointType / ClusterEndpointType
// ---------------------------------------------------------------------------

func TestUpdateEndpointType(t *testing.T) {
	existing := &openchoreov1alpha1.EndpointType{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testEndpointTypeName,
			Namespace:   testNS,
			Annotations: map[string]string{"keep-me": "value"},
		},
	}
	svc := endpointtypemocks.NewMockService(t)
	svc.EXPECT().GetEndpointType(mock.Anything, testNS, testEndpointTypeName).Return(existing, nil)
	var sent *openchoreov1alpha1.EndpointType
	svc.EXPECT().
		UpdateEndpointType(mock.Anything, testNS, mock.Anything).
		Run(func(_ context.Context, _ string, et *openchoreov1alpha1.EndpointType) { sent = et }).
		Return(existing, nil)

	anns := map[string]string{"openchoreo.dev/description": "Routes HTTP"}
	h := newTestHandler(withEndpointTypeService(svc))
	_, err := h.UpdateEndpointType(context.Background(), testNS, &gen.UpdateEndpointTypeJSONRequestBody{
		Metadata: gen.ObjectMeta{Name: testEndpointTypeName, Annotations: &anns},
	})
	require.NoError(t, err)
	require.NotNil(t, sent)
	assert.Equal(t, "value", sent.Annotations["keep-me"])
	assert.Equal(t, "Routes HTTP", sent.Annotations["openchoreo.dev/description"])
}

func TestGetClusterEndpointType(t *testing.T) {
	svc := clusterendpointtypemocks.NewMockService(t)
	svc.EXPECT().GetClusterEndpointType(mock.Anything, testEndpointTypeName).
		Return(&openchoreov1alpha1.ClusterEndpointType{
			ObjectMeta: metav1.ObjectMeta{Name: testEndpointTypeName},
		}, nil)

	h := newTestHandler(withClusterEndpointTypeService(svc))
	result, err := h.GetClusterEndpointType(context.Background(), testEndpointTypeName)
	require.NoError(t, err)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, testEndpointTypeName, m["name"])
}

// ---------------------------------------------------------------------------
// EndpointRelease
// ---------------------------------------------------------------------------

func TestListEndpointReleases(t *testing.T) {
	svc := endpointreleasemocks.NewMockService(t)
	svc.EXPECT().
		ListEndpointReleases(mock.Anything, testNS, testEndpointName, mock.Anything).
		Return(&services.ListResult[openchoreov1alpha1.EndpointRelease]{
			Items: []openchoreov1alpha1.EndpointRelease{{
				ObjectMeta: metav1.ObjectMeta{Name: testEndpointReleaseName, Namespace: testNS},
				Spec: openchoreov1alpha1.EndpointReleaseSpec{
					Owner: openchoreov1alpha1.EndpointReleaseOwner{
						ProjectName:  testProject,
						EndpointName: testEndpointName,
					},
				},
			}},
		}, nil)

	h := newTestHandler(withEndpointReleaseService(svc))
	result, err := h.ListEndpointReleases(context.Background(), testNS, testEndpointName, tools.ListOpts{})
	require.NoError(t, err)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	items, ok := m["endpoint_releases"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	assert.Equal(t, testEndpointName, items[0]["endpointName"])
}

// ---------------------------------------------------------------------------
// EndpointReleaseBinding
// ---------------------------------------------------------------------------

func TestUpdateEndpointReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("advances the pin and keeps owner and environment", func(t *testing.T) {
		existing := sampleEndpointReleaseBinding()
		svc := endpointreleasebindingmocks.NewMockService(t)
		svc.EXPECT().
			GetEndpointReleaseBinding(mock.Anything, testNS, testEndpointReleaseBindingName).
			Return(existing, nil)
		var sent *openchoreov1alpha1.EndpointReleaseBinding
		svc.EXPECT().
			UpdateEndpointReleaseBinding(mock.Anything, testNS, mock.Anything).
			Run(func(_ context.Context, _ string, b *openchoreov1alpha1.EndpointReleaseBinding) { sent = b }).
			Return(existing, nil)

		envConfigs := map[string]any{"hostname": "orders.example.com"}
		req := &gen.UpdateEndpointReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testEndpointReleaseBindingName},
			Spec: &gen.EndpointReleaseBindingSpec{
				EndpointRelease:                ptr.To("orders-api-def456"),
				EndpointTypeEnvironmentConfigs: &envConfigs,
			},
		}

		h := newTestHandler(withEndpointReleaseBindingService(svc))
		_, err := h.UpdateEndpointReleaseBinding(ctx, testNS, req)
		require.NoError(t, err)

		require.NotNil(t, sent)
		assert.Equal(t, "orders-api-def456", sent.Spec.EndpointRelease)
		assert.Equal(t, testEndpointEnvironment, sent.Spec.Environment)
		assert.Equal(t, testEndpointName, sent.Spec.Owner.EndpointName)
		require.NotNil(t, sent.Spec.EndpointTypeEnvironmentConfigs)
		var got map[string]any
		require.NoError(t, json.Unmarshal(sent.Spec.EndpointTypeEnvironmentConfigs.Raw, &got))
		assert.Equal(t, "orders.example.com", got["hostname"])
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.UpdateEndpointReleaseBinding(ctx, testNS, nil)
		require.Error(t, err)
	})
}

func TestEndpointReleaseBindingDetail(t *testing.T) {
	b := sampleEndpointReleaseBinding()
	b.Spec.EndpointTypeEnvironmentConfigs = &runtime.RawExtension{Raw: []byte(`{"hostname":"orders.example.com"}`)}
	b.Status.Backends = []openchoreov1alpha1.ResolvedEndpointBackend{
		{
			Name:     "primary",
			Kind:     openchoreov1alpha1.EndpointBackendKindComponent,
			Weight:   100,
			Resolved: true,
			ServiceURL: &openchoreov1alpha1.EndpointURL{
				Scheme: "http", Host: "orders.dp-ns.svc.cluster.local", Port: 8080,
			},
		},
		{
			Name:    "canary",
			Kind:    openchoreov1alpha1.EndpointBackendKindComponent,
			Reason:  "BackendNotExposed",
			Message: "workload endpoint http has project visibility",
		},
	}

	m := endpointReleaseBindingDetail(b)
	assert.Equal(t, testEndpointEnvironment, m["environment"])
	assert.Equal(t, map[string]any{"hostname": "orders.example.com"}, m["endpointTypeEnvironmentConfigs"])

	backends, ok := m["backends"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, backends, 2)
	assert.Equal(t, true, backends[0]["resolved"])
	assert.Contains(t, backends[0], "serviceURL")
	assert.NotContains(t, backends[0], "reason")
	assert.Equal(t, false, backends[1]["resolved"])
	assert.Equal(t, "BackendNotExposed", backends[1]["reason"])
	assert.NotContains(t, backends[1], "serviceURL")
}
