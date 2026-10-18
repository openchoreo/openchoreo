// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func endpointTypeKindPtr(k gen.EndpointTypeRefKind) *gen.EndpointTypeRefKind {
	return &k
}

func endpointInstance(name, typeKind, typeName string) gen.Endpoint {
	r := gen.Endpoint{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.EndpointSpec{
			Type:       gen.EndpointTypeRef{Name: typeName},
			Visibility: []gen.EndpointSpecVisibility{"internal", "external"},
		},
	}
	r.Spec.Owner.ProjectName = "online-store"
	if typeKind != "" {
		r.Spec.Type.Kind = endpointTypeKindPtr(gen.EndpointTypeRefKind(typeKind))
	}
	return r
}

// --- printList tests ---

func TestPrint_Nil(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(nil, true))
	})
	assert.Contains(t, out, "No endpoints found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.Endpoint{}, true))
	})
	assert.Contains(t, out, "No endpoints found")
}

func TestPrint_WithItems_ShowProject(t *testing.T) {
	now := time.Now()
	items := []gen.Endpoint{
		endpointInstance("orders-api", "EndpointType", "public-http"),
		endpointInstance("billing-api", "ClusterEndpointType", "internal-grpc"),
	}
	items[0].Metadata.CreationTimestamp = &now
	items[1].Metadata.CreationTimestamp = &now

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items, true))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "TYPE")
	assert.Contains(t, out, "VISIBILITY")
	assert.Contains(t, out, "PROJECT")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "orders-api")
	assert.Contains(t, out, "internal,external")
	assert.Contains(t, out, "EndpointType/public-http")
	assert.Contains(t, out, "ClusterEndpointType/internal-grpc")
	assert.Contains(t, out, "online-store")
}

func TestPrint_WithItems_HideProject(t *testing.T) {
	items := []gen.Endpoint{
		endpointInstance("orders-api", "EndpointType", "public-http"),
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items, false))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "TYPE")
	assert.NotContains(t, out, "PROJECT")
	assert.Contains(t, out, "orders-api")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.Endpoint{
		{
			Metadata: gen.ObjectMeta{Name: "no-spec", CreationTimestamp: &now},
			Spec:     nil,
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items, true))
	})

	assert.Contains(t, out, "no-spec")
}

// --- Validation tests ---

func TestList_ValidationError(t *testing.T) {
	r := New(mocks.NewMockInterface(t))
	assert.Error(t, r.List(ListParams{Namespace: ""}))
}

func TestGet_ValidationError(t *testing.T) {
	r := New(mocks.NewMockInterface(t))
	assert.Error(t, r.Get(GetParams{Namespace: ""}))
}

func TestDelete_ValidationError(t *testing.T) {
	r := New(mocks.NewMockInterface(t))
	assert.Error(t, r.Delete(DeleteParams{Namespace: "my-org", EndpointName: ""}))
}

func TestPromote_ValidationError_MissingNamespace(t *testing.T) {
	r := New(mocks.NewMockInterface(t))
	assert.Error(t, r.Promote(PromoteParams{Namespace: "", EndpointName: "db", Environment: "dev"}))
}

func TestPromote_ValidationError_MissingEndpoint(t *testing.T) {
	r := New(mocks.NewMockInterface(t))
	assert.Error(t, r.Promote(PromoteParams{Namespace: "my-org", EndpointName: "", Environment: "dev"}))
}

func TestPromote_ValidationError_MissingEnv(t *testing.T) {
	r := New(mocks.NewMockInterface(t))
	assert.Error(t, r.Promote(PromoteParams{Namespace: "my-org", EndpointName: "db", Environment: ""}))
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpoints(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	r := New(mc)
	assert.EqualError(t, r.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success_NoProjectFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpoints(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListEndpointsParams) bool {
		return p.Project == nil
	})).Return(&gen.EndpointList{
		Items: []gen.Endpoint{
			endpointInstance("orders-api", "EndpointType", "public-http"),
		},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "PROJECT")
	assert.Contains(t, out, "orders-api")
	assert.Contains(t, out, "online-store")
}

func TestList_Success_WithProjectFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpoints(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListEndpointsParams) bool {
		return p.Project != nil && *p.Project == "online-store"
	})).Return(&gen.EndpointList{
		Items: []gen.Endpoint{
			endpointInstance("orders-api", "EndpointType", "public-http"),
		},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.List(ListParams{Namespace: "my-org", Project: "online-store"}))
	})

	assert.NotContains(t, out, "PROJECT")
	assert.Contains(t, out, "orders-api")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListEndpoints(mock.Anything, "my-org", mock.Anything).Return(&gen.EndpointList{
		Items:      []gen.Endpoint{},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "No endpoints found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found: missing"))

	r := New(mc)
	assert.EqualError(t, r.Get(GetParams{Namespace: "my-org", EndpointName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "my-org", "orders-api").Return(&gen.Endpoint{
		Metadata: gen.ObjectMeta{Name: "orders-api"},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Get(GetParams{Namespace: "my-org", EndpointName: "orders-api"}))
	})

	assert.Contains(t, out, "name: orders-api")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteEndpoint(mock.Anything, "my-org", "orders-api").Return(fmt.Errorf("forbidden"))

	r := New(mc)
	assert.EqualError(t, r.Delete(DeleteParams{Namespace: "my-org", EndpointName: "orders-api"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteEndpoint(mock.Anything, "my-org", "orders-api").Return(nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Delete(DeleteParams{Namespace: "my-org", EndpointName: "orders-api"}))
	})

	assert.Contains(t, out, "Endpoint 'orders-api' deleted")
}

// --- Promote tests ---

func endpointWithLatestRelease() *gen.Endpoint {
	r := &gen.Endpoint{
		Metadata: gen.ObjectMeta{Name: "orders-api"},
		Status:   &gen.EndpointStatus{},
	}
	r.Status.LatestRelease = &struct {
		Hash string `json:"hash"`
		Name string `json:"name"`
	}{Hash: "abc123", Name: "orders-api-abc123"}
	return r
}

func bindingForEnv(bindingName, env string) gen.EndpointReleaseBinding {
	b := gen.EndpointReleaseBinding{
		Metadata: gen.ObjectMeta{Name: bindingName},
		Spec: &gen.EndpointReleaseBindingSpec{
			Environment: env,
		},
	}
	b.Spec.Owner.EndpointName = "orders-api"
	return b
}

func TestPromote_GetEndpointError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "my-org", "orders-api").Return(nil, fmt.Errorf("not found"))

	r := New(mc)
	err := r.Promote(PromoteParams{Namespace: "my-org", EndpointName: "orders-api", Environment: "dev"})
	assert.EqualError(t, err, "not found")
}

func TestPromote_EndpointHasNoLatestRelease(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "my-org", "orders-api").Return(&gen.Endpoint{
		Metadata: gen.ObjectMeta{Name: "orders-api"},
		Status:   &gen.EndpointStatus{},
	}, nil)

	r := New(mc)
	err := r.Promote(PromoteParams{Namespace: "my-org", EndpointName: "orders-api", Environment: "dev"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no released versions")
}

func TestPromote_ListBindingsError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "my-org", "orders-api").Return(
		endpointWithLatestRelease(), nil)
	mc.EXPECT().ListEndpointReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("api blew up"))

	r := New(mc)
	err := r.Promote(PromoteParams{Namespace: "my-org", EndpointName: "orders-api", Environment: "dev"})
	assert.EqualError(t, err, "api blew up")
}

func TestPromote_NoBindingForEnv(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "my-org", "orders-api").Return(
		endpointWithLatestRelease(), nil)
	mc.EXPECT().ListEndpointReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(&gen.EndpointReleaseBindingList{
		Items: []gen.EndpointReleaseBinding{
			bindingForEnv("orders-api-staging", "staging"),
			bindingForEnv("orders-api-prod", "prod"),
		},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	err := r.Promote(PromoteParams{Namespace: "my-org", EndpointName: "orders-api", Environment: "dev"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dev")
	assert.Contains(t, err.Error(), "orders-api")
}

func TestPromote_FiltersByEndpoint(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "my-org", "orders-api").Return(
		endpointWithLatestRelease(), nil)
	mc.EXPECT().ListEndpointReleaseBindings(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListEndpointReleaseBindingsParams) bool {
		return p.Endpoint != nil && *p.Endpoint == "orders-api"
	})).Return(&gen.EndpointReleaseBindingList{
		Items: []gen.EndpointReleaseBinding{
			bindingForEnv("orders-api-dev", "dev"),
		},
		Pagination: gen.Pagination{},
	}, nil)
	mc.EXPECT().UpdateEndpointReleaseBinding(
		mock.Anything,
		"my-org",
		"orders-api-dev",
		mock.MatchedBy(func(b gen.EndpointReleaseBinding) bool {
			return b.Spec != nil && b.Spec.EndpointRelease != nil && *b.Spec.EndpointRelease == "orders-api-abc123"
		}),
	).Return(&gen.EndpointReleaseBinding{Metadata: gen.ObjectMeta{Name: "orders-api-dev"}}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Promote(PromoteParams{Namespace: "my-org", EndpointName: "orders-api", Environment: "dev"}))
	})

	assert.Contains(t, out, "orders-api-dev")
	assert.Contains(t, out, "orders-api-abc123")
}

func TestPromote_UpdateError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetEndpoint(mock.Anything, "my-org", "orders-api").Return(
		endpointWithLatestRelease(), nil)
	mc.EXPECT().ListEndpointReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(&gen.EndpointReleaseBindingList{
		Items: []gen.EndpointReleaseBinding{
			bindingForEnv("orders-api-dev", "dev"),
		},
		Pagination: gen.Pagination{},
	}, nil)
	mc.EXPECT().UpdateEndpointReleaseBinding(mock.Anything, "my-org", "orders-api-dev", mock.Anything).
		Return(nil, fmt.Errorf("conflict"))

	r := New(mc)
	err := r.Promote(PromoteParams{Namespace: "my-org", EndpointName: "orders-api", Environment: "dev"})
	assert.EqualError(t, err, "conflict")
}
