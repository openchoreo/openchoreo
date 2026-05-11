// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

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

func resourceTypeKindPtr(k gen.ResourceTypeRefKind) *gen.ResourceTypeRefKind {
	return &k
}

func resourceInstance(name, typeKind, typeName string) gen.ResourceInstance {
	r := gen.ResourceInstance{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.ResourceInstanceSpec{
			Type: gen.ResourceTypeRef{Name: typeName},
		},
	}
	r.Spec.Owner.ProjectName = "online-store"
	if typeKind != "" {
		r.Spec.Type.Kind = resourceTypeKindPtr(gen.ResourceTypeRefKind(typeKind))
	}
	return r
}

// --- printList tests ---

func TestPrint_Nil(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(nil, true))
	})
	assert.Contains(t, out, "No resources found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ResourceInstance{}, true))
	})
	assert.Contains(t, out, "No resources found")
}

func TestPrint_WithItems_ShowProject(t *testing.T) {
	now := time.Now()
	items := []gen.ResourceInstance{
		resourceInstance("analytics-db", "ResourceType", "mysql"),
		resourceInstance("billing-cache", "ClusterResourceType", "redis"),
	}
	items[0].Metadata.CreationTimestamp = &now
	items[1].Metadata.CreationTimestamp = &now

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items, true))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "TYPE")
	assert.Contains(t, out, "PROJECT")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "analytics-db")
	assert.Contains(t, out, "ResourceType/mysql")
	assert.Contains(t, out, "ClusterResourceType/redis")
	assert.Contains(t, out, "online-store")
}

func TestPrint_WithItems_HideProject(t *testing.T) {
	items := []gen.ResourceInstance{
		resourceInstance("analytics-db", "ResourceType", "mysql"),
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items, false))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "TYPE")
	assert.NotContains(t, out, "PROJECT")
	assert.Contains(t, out, "analytics-db")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.ResourceInstance{
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
	assert.Error(t, r.Delete(DeleteParams{Namespace: "my-org", ResourceName: ""}))
}

func TestPromote_ValidationError_MissingNamespace(t *testing.T) {
	r := New(mocks.NewMockInterface(t))
	assert.Error(t, r.Promote(PromoteParams{Namespace: "", ResourceName: "db", Environment: "dev"}))
}

func TestPromote_ValidationError_MissingResource(t *testing.T) {
	r := New(mocks.NewMockInterface(t))
	assert.Error(t, r.Promote(PromoteParams{Namespace: "my-org", ResourceName: "", Environment: "dev"}))
}

func TestPromote_ValidationError_MissingEnv(t *testing.T) {
	r := New(mocks.NewMockInterface(t))
	assert.Error(t, r.Promote(PromoteParams{Namespace: "my-org", ResourceName: "db", Environment: ""}))
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResources(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	r := New(mc)
	assert.EqualError(t, r.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success_NoProjectFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResources(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListResourcesParams) bool {
		return p.Project == nil
	})).Return(&gen.ResourceInstanceList{
		Items: []gen.ResourceInstance{
			resourceInstance("analytics-db", "ResourceType", "mysql"),
		},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "PROJECT")
	assert.Contains(t, out, "analytics-db")
	assert.Contains(t, out, "online-store")
}

func TestList_Success_WithProjectFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResources(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListResourcesParams) bool {
		return p.Project != nil && *p.Project == "online-store"
	})).Return(&gen.ResourceInstanceList{
		Items: []gen.ResourceInstance{
			resourceInstance("analytics-db", "ResourceType", "mysql"),
		},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.List(ListParams{Namespace: "my-org", Project: "online-store"}))
	})

	assert.NotContains(t, out, "PROJECT")
	assert.Contains(t, out, "analytics-db")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResources(mock.Anything, "my-org", mock.Anything).Return(&gen.ResourceInstanceList{
		Items:      []gen.ResourceInstance{},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "No resources found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResource(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found: missing"))

	r := New(mc)
	assert.EqualError(t, r.Get(GetParams{Namespace: "my-org", ResourceName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResource(mock.Anything, "my-org", "analytics-db").Return(&gen.ResourceInstance{
		Metadata: gen.ObjectMeta{Name: "analytics-db"},
	}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Get(GetParams{Namespace: "my-org", ResourceName: "analytics-db"}))
	})

	assert.Contains(t, out, "name: analytics-db")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteResource(mock.Anything, "my-org", "analytics-db").Return(fmt.Errorf("forbidden"))

	r := New(mc)
	assert.EqualError(t, r.Delete(DeleteParams{Namespace: "my-org", ResourceName: "analytics-db"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteResource(mock.Anything, "my-org", "analytics-db").Return(nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Delete(DeleteParams{Namespace: "my-org", ResourceName: "analytics-db"}))
	})

	assert.Contains(t, out, "Resource 'analytics-db' deleted")
}

// --- Promote tests ---

func resourceWithLatestRelease() *gen.ResourceInstance {
	r := &gen.ResourceInstance{
		Metadata: gen.ObjectMeta{Name: "analytics-db"},
		Status:   &gen.ResourceInstanceStatus{},
	}
	r.Status.LatestRelease = &struct {
		Hash string `json:"hash"`
		Name string `json:"name"`
	}{Hash: "abc123", Name: "analytics-db-abc123"}
	return r
}

func bindingForEnv(bindingName, env string) gen.ResourceReleaseBinding {
	b := gen.ResourceReleaseBinding{
		Metadata: gen.ObjectMeta{Name: bindingName},
		Spec: &gen.ResourceReleaseBindingSpec{
			Environment: env,
		},
	}
	b.Spec.Owner.ResourceName = "analytics-db"
	return b
}

func TestPromote_GetResourceError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResource(mock.Anything, "my-org", "analytics-db").Return(nil, fmt.Errorf("not found"))

	r := New(mc)
	err := r.Promote(PromoteParams{Namespace: "my-org", ResourceName: "analytics-db", Environment: "dev"})
	assert.EqualError(t, err, "not found")
}

func TestPromote_ResourceHasNoLatestRelease(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResource(mock.Anything, "my-org", "analytics-db").Return(&gen.ResourceInstance{
		Metadata: gen.ObjectMeta{Name: "analytics-db"},
		Status:   &gen.ResourceInstanceStatus{},
	}, nil)

	r := New(mc)
	err := r.Promote(PromoteParams{Namespace: "my-org", ResourceName: "analytics-db", Environment: "dev"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no released versions")
}

func TestPromote_ListBindingsError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResource(mock.Anything, "my-org", "analytics-db").Return(
		resourceWithLatestRelease(), nil)
	mc.EXPECT().ListResourceReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("api blew up"))

	r := New(mc)
	err := r.Promote(PromoteParams{Namespace: "my-org", ResourceName: "analytics-db", Environment: "dev"})
	assert.EqualError(t, err, "api blew up")
}

func TestPromote_NoBindingForEnv(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResource(mock.Anything, "my-org", "analytics-db").Return(
		resourceWithLatestRelease(), nil)
	mc.EXPECT().ListResourceReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(&gen.ResourceReleaseBindingList{
		Items: []gen.ResourceReleaseBinding{
			bindingForEnv("analytics-db-staging", "staging"),
			bindingForEnv("analytics-db-prod", "prod"),
		},
		Pagination: gen.Pagination{},
	}, nil)

	r := New(mc)
	err := r.Promote(PromoteParams{Namespace: "my-org", ResourceName: "analytics-db", Environment: "dev"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dev")
	assert.Contains(t, err.Error(), "analytics-db")
}

func TestPromote_FiltersByResource(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResource(mock.Anything, "my-org", "analytics-db").Return(
		resourceWithLatestRelease(), nil)
	mc.EXPECT().ListResourceReleaseBindings(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListResourceReleaseBindingsParams) bool {
		return p.Resource != nil && *p.Resource == "analytics-db"
	})).Return(&gen.ResourceReleaseBindingList{
		Items: []gen.ResourceReleaseBinding{
			bindingForEnv("analytics-db-dev", "dev"),
		},
		Pagination: gen.Pagination{},
	}, nil)
	mc.EXPECT().UpdateResourceReleaseBinding(
		mock.Anything,
		"my-org",
		"analytics-db-dev",
		mock.MatchedBy(func(b gen.ResourceReleaseBinding) bool {
			return b.Spec != nil && b.Spec.ResourceRelease != nil && *b.Spec.ResourceRelease == "analytics-db-abc123"
		}),
	).Return(&gen.ResourceReleaseBinding{Metadata: gen.ObjectMeta{Name: "analytics-db-dev"}}, nil)

	r := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, r.Promote(PromoteParams{Namespace: "my-org", ResourceName: "analytics-db", Environment: "dev"}))
	})

	assert.Contains(t, out, "analytics-db-dev")
	assert.Contains(t, out, "analytics-db-abc123")
}

func TestPromote_UpdateError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResource(mock.Anything, "my-org", "analytics-db").Return(
		resourceWithLatestRelease(), nil)
	mc.EXPECT().ListResourceReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(&gen.ResourceReleaseBindingList{
		Items: []gen.ResourceReleaseBinding{
			bindingForEnv("analytics-db-dev", "dev"),
		},
		Pagination: gen.Pagination{},
	}, nil)
	mc.EXPECT().UpdateResourceReleaseBinding(mock.Anything, "my-org", "analytics-db-dev", mock.Anything).
		Return(nil, fmt.Errorf("conflict"))

	r := New(mc)
	err := r.Promote(PromoteParams{Namespace: "my-org", ResourceName: "analytics-db", Environment: "dev"})
	assert.EqualError(t, err, "conflict")
}
