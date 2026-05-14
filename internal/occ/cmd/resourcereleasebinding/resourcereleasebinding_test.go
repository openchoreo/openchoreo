// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

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

func bindingFor(name, env string) gen.ResourceReleaseBinding {
	b := gen.ResourceReleaseBinding{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.ResourceReleaseBindingSpec{
			Environment: env,
		},
	}
	b.Spec.Owner.ResourceName = "analytics-db"
	return b
}

func ptrString(s string) *string { return &s }

func bindingWithReadyCondition(name, env, releaseName, readyReason string) gen.ResourceReleaseBinding {
	b := bindingFor(name, env)
	b.Spec.ResourceRelease = ptrString(releaseName)
	b.Status = &gen.ResourceReleaseBindingStatus{
		Conditions: &[]gen.Condition{
			{Type: "Ready", Reason: readyReason},
		},
	}
	return b
}

// --- printList tests ---

func TestPrint_Nil(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No resource release bindings found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ResourceReleaseBinding{}))
	})
	assert.Contains(t, out, "No resource release bindings found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ResourceReleaseBinding{
		bindingFor("analytics-db-dev", "dev"),
		bindingFor("analytics-db-prod", "prod"),
	}
	items[0].Metadata.CreationTimestamp = &now

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "RESOURCE")
	assert.Contains(t, out, "ENVIRONMENT")
	assert.Contains(t, out, "RELEASE")
	assert.Contains(t, out, "STATUS")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "analytics-db-dev")
	assert.Contains(t, out, "dev")
	assert.Contains(t, out, "prod")
	assert.Contains(t, out, "analytics-db")
}

func TestPrint_ShowsReleaseAndReadyReason(t *testing.T) {
	items := []gen.ResourceReleaseBinding{
		bindingWithReadyCondition("analytics-db-dev", "dev", "analytics-db-abc123", "Ready"),
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "analytics-db-abc123")
	assert.Contains(t, out, "Ready")
}

func TestPrint_HandlesMissingReadyCondition(t *testing.T) {
	// Binding with conditions but no Ready type — STATUS column stays empty
	b := bindingFor("analytics-db-dev", "dev")
	b.Spec.ResourceRelease = ptrString("analytics-db-abc123")
	b.Status = &gen.ResourceReleaseBindingStatus{
		Conditions: &[]gen.Condition{
			{Type: "Synced", Reason: "ReleaseSynced"},
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ResourceReleaseBinding{b}))
	})

	assert.Contains(t, out, "analytics-db-abc123")
	// "Synced" is not surfaced — only the Ready condition's Reason makes it to STATUS.
	assert.NotContains(t, out, "ReleaseSynced")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.ResourceReleaseBinding{
		{
			Metadata: gen.ObjectMeta{Name: "no-spec", CreationTimestamp: &now},
			Spec:     nil,
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-spec")
}

// --- Validation tests ---

func TestList_ValidationError(t *testing.T) {
	rrb := New(mocks.NewMockInterface(t))
	assert.Error(t, rrb.List(ListParams{Namespace: ""}))
}

func TestGet_ValidationError(t *testing.T) {
	rrb := New(mocks.NewMockInterface(t))
	assert.Error(t, rrb.Get(GetParams{Namespace: ""}))
}

func TestDelete_ValidationError(t *testing.T) {
	rrb := New(mocks.NewMockInterface(t))
	assert.Error(t, rrb.Delete(DeleteParams{Namespace: "my-org", ResourceReleaseBindingName: ""}))
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	rrb := New(mc)
	assert.EqualError(t, rrb.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success_NoResourceFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceReleaseBindings(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListResourceReleaseBindingsParams) bool {
		return p.Resource == nil
	})).Return(&gen.ResourceReleaseBindingList{
		Items:      []gen.ResourceReleaseBinding{bindingFor("analytics-db-dev", "dev")},
		Pagination: gen.Pagination{},
	}, nil)

	rrb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rrb.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "analytics-db-dev")
}

func TestList_Success_WithResourceFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceReleaseBindings(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListResourceReleaseBindingsParams) bool {
		return p.Resource != nil && *p.Resource == "analytics-db"
	})).Return(&gen.ResourceReleaseBindingList{
		Items:      []gen.ResourceReleaseBinding{bindingFor("analytics-db-dev", "dev")},
		Pagination: gen.Pagination{},
	}, nil)

	rrb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rrb.List(ListParams{Namespace: "my-org", Resource: "analytics-db"}))
	})

	assert.Contains(t, out, "analytics-db-dev")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListResourceReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(&gen.ResourceReleaseBindingList{
		Items:      []gen.ResourceReleaseBinding{},
		Pagination: gen.Pagination{},
	}, nil)

	rrb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rrb.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "No resource release bindings found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResourceReleaseBinding(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found"))

	rrb := New(mc)
	assert.EqualError(t, rrb.Get(GetParams{Namespace: "my-org", ResourceReleaseBindingName: "missing"}), "not found")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResourceReleaseBinding(mock.Anything, "my-org", "analytics-db-dev").Return(&gen.ResourceReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "analytics-db-dev"},
	}, nil)

	rrb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rrb.Get(GetParams{Namespace: "my-org", ResourceReleaseBindingName: "analytics-db-dev"}))
	})

	assert.Contains(t, out, "name: analytics-db-dev")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteResourceReleaseBinding(mock.Anything, "my-org", "analytics-db-dev").Return(fmt.Errorf("forbidden"))

	rrb := New(mc)
	assert.EqualError(t, rrb.Delete(DeleteParams{Namespace: "my-org", ResourceReleaseBindingName: "analytics-db-dev"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteResourceReleaseBinding(mock.Anything, "my-org", "analytics-db-dev").Return(nil)

	rrb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rrb.Delete(DeleteParams{Namespace: "my-org", ResourceReleaseBindingName: "analytics-db-dev"}))
	})

	assert.Contains(t, out, "ResourceReleaseBinding 'analytics-db-dev' deleted")
}
