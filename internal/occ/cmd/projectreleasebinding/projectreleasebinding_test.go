// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

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

func bindingFor(name, env string) gen.ProjectReleaseBinding {
	b := gen.ProjectReleaseBinding{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.ProjectReleaseBindingSpec{
			Environment: env,
		},
	}
	b.Spec.Owner.ProjectName = "online-store"
	return b
}

func ptrString(s string) *string { return &s }

func bindingWithReadyCondition(name, env, releaseName, readyReason string) gen.ProjectReleaseBinding {
	b := bindingFor(name, env)
	b.Spec.ProjectRelease = ptrString(releaseName)
	b.Status = &gen.ProjectReleaseBindingStatus{
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
	assert.Contains(t, out, "No project release bindings found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ProjectReleaseBinding{}))
	})
	assert.Contains(t, out, "No project release bindings found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ProjectReleaseBinding{
		bindingFor("online-store-dev", "dev"),
		bindingFor("online-store-prod", "prod"),
	}
	items[0].Metadata.CreationTimestamp = &now

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "PROJECT")
	assert.Contains(t, out, "ENVIRONMENT")
	assert.Contains(t, out, "RELEASE")
	assert.Contains(t, out, "STATUS")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "online-store-dev")
	assert.Contains(t, out, "dev")
	assert.Contains(t, out, "prod")
	assert.Contains(t, out, "online-store")
}

func TestPrint_ShowsReleaseAndReadyReason(t *testing.T) {
	items := []gen.ProjectReleaseBinding{
		bindingWithReadyCondition("online-store-dev", "dev", "online-store-abc123", "Ready"),
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "online-store-abc123")
	assert.Contains(t, out, "Ready")
}

func TestPrint_HandlesMissingReadyCondition(t *testing.T) {
	// Binding with conditions but no Ready type — STATUS column stays empty
	b := bindingFor("online-store-dev", "dev")
	b.Spec.ProjectRelease = ptrString("online-store-abc123")
	b.Status = &gen.ProjectReleaseBindingStatus{
		Conditions: &[]gen.Condition{
			{Type: "Synced", Reason: "ReleaseSynced"},
		},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ProjectReleaseBinding{b}))
	})

	assert.Contains(t, out, "online-store-abc123")
	// "Synced" is not surfaced — only the Ready condition's Reason makes it to STATUS.
	assert.NotContains(t, out, "ReleaseSynced")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.ProjectReleaseBinding{
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
	prb := New(mocks.NewMockInterface(t))
	assert.Error(t, prb.List(ListParams{Namespace: ""}))
}

func TestGet_ValidationError(t *testing.T) {
	prb := New(mocks.NewMockInterface(t))
	assert.Error(t, prb.Get(GetParams{Namespace: ""}))
}

func TestDelete_ValidationError(t *testing.T) {
	prb := New(mocks.NewMockInterface(t))
	assert.Error(t, prb.Delete(DeleteParams{Namespace: "my-org", ProjectReleaseBindingName: ""}))
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	prb := New(mc)
	assert.EqualError(t, prb.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success_NoProjectFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListProjectReleaseBindingsParams) bool {
		return p.Project == nil
	})).Return(&gen.ProjectReleaseBindingList{
		Items:      []gen.ProjectReleaseBinding{bindingFor("online-store-dev", "dev")},
		Pagination: gen.Pagination{},
	}, nil)

	prb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, prb.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "online-store-dev")
}

func TestList_Success_WithProjectFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListProjectReleaseBindingsParams) bool {
		return p.Project != nil && *p.Project == "online-store"
	})).Return(&gen.ProjectReleaseBindingList{
		Items:      []gen.ProjectReleaseBinding{bindingFor("online-store-dev", "dev")},
		Pagination: gen.Pagination{},
	}, nil)

	prb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, prb.List(ListParams{Namespace: "my-org", Project: "online-store"}))
	})

	assert.Contains(t, out, "online-store-dev")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(&gen.ProjectReleaseBindingList{
		Items:      []gen.ProjectReleaseBinding{},
		Pagination: gen.Pagination{},
	}, nil)

	prb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, prb.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "No project release bindings found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectReleaseBinding(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found"))

	prb := New(mc)
	assert.EqualError(t, prb.Get(GetParams{Namespace: "my-org", ProjectReleaseBindingName: "missing"}), "not found")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectReleaseBinding(mock.Anything, "my-org", "online-store-dev").Return(&gen.ProjectReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "online-store-dev"},
	}, nil)

	prb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, prb.Get(GetParams{Namespace: "my-org", ProjectReleaseBindingName: "online-store-dev"}))
	})

	assert.Contains(t, out, "name: online-store-dev")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteProjectReleaseBinding(mock.Anything, "my-org", "online-store-dev").Return(fmt.Errorf("forbidden"))

	prb := New(mc)
	assert.EqualError(t, prb.Delete(DeleteParams{Namespace: "my-org", ProjectReleaseBindingName: "online-store-dev"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteProjectReleaseBinding(mock.Anything, "my-org", "online-store-dev").Return(nil)

	prb := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, prb.Delete(DeleteParams{Namespace: "my-org", ProjectReleaseBindingName: "online-store-dev"}))
	})

	assert.Contains(t, out, "ProjectReleaseBinding 'online-store-dev' deleted")
}
