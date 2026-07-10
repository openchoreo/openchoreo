// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectrelease

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

func releaseFor(name string) gen.ProjectRelease {
	r := gen.ProjectRelease{
		Metadata: gen.ObjectMeta{Name: name},
		Spec:     &gen.ProjectReleaseSpec{},
	}
	r.Spec.Owner.ProjectName = "online-store"
	return r
}

// --- printList tests ---

func TestPrint_Nil(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No project releases found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.ProjectRelease{}))
	})
	assert.Contains(t, out, "No project releases found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ProjectRelease{
		releaseFor("online-store-abc123"),
		releaseFor("online-store-def456"),
	}
	items[0].Metadata.CreationTimestamp = &now

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "PROJECT")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "online-store-abc123")
	assert.Contains(t, out, "online-store")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.ProjectRelease{
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
	pr := New(mocks.NewMockInterface(t))
	assert.Error(t, pr.List(ListParams{Namespace: ""}))
}

func TestGet_ValidationError(t *testing.T) {
	pr := New(mocks.NewMockInterface(t))
	assert.Error(t, pr.Get(GetParams{Namespace: ""}))
}

func TestDelete_ValidationError(t *testing.T) {
	pr := New(mocks.NewMockInterface(t))
	assert.Error(t, pr.Delete(DeleteParams{Namespace: "my-org", ProjectReleaseName: ""}))
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListProjectReleases(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	pr := New(mc)
	assert.EqualError(t, pr.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success_NoProjectFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListProjectReleases(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListProjectReleasesParams) bool {
		return p.Project == nil
	})).Return(&gen.ProjectReleaseList{
		Items:      []gen.ProjectRelease{releaseFor("online-store-abc123")},
		Pagination: gen.Pagination{},
	}, nil)

	pr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, pr.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "online-store-abc123")
}

func TestList_Success_WithProjectFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListProjectReleases(mock.Anything, "my-org", mock.MatchedBy(func(p *gen.ListProjectReleasesParams) bool {
		return p.Project != nil && *p.Project == "online-store"
	})).Return(&gen.ProjectReleaseList{
		Items:      []gen.ProjectRelease{releaseFor("online-store-abc123")},
		Pagination: gen.Pagination{},
	}, nil)

	pr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, pr.List(ListParams{Namespace: "my-org", Project: "online-store"}))
	})

	assert.Contains(t, out, "online-store-abc123")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListProjectReleases(mock.Anything, "my-org", mock.Anything).Return(&gen.ProjectReleaseList{
		Items:      []gen.ProjectRelease{},
		Pagination: gen.Pagination{},
	}, nil)

	pr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, pr.List(ListParams{Namespace: "my-org"}))
	})

	assert.Contains(t, out, "No project releases found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectRelease(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found"))

	pr := New(mc)
	assert.EqualError(t, pr.Get(GetParams{Namespace: "my-org", ProjectReleaseName: "missing"}), "not found")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectRelease(mock.Anything, "my-org", "online-store-abc123").Return(&gen.ProjectRelease{
		Metadata: gen.ObjectMeta{Name: "online-store-abc123"},
	}, nil)

	pr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, pr.Get(GetParams{Namespace: "my-org", ProjectReleaseName: "online-store-abc123"}))
	})

	assert.Contains(t, out, "name: online-store-abc123")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteProjectRelease(mock.Anything, "my-org", "online-store-abc123").Return(fmt.Errorf("forbidden"))

	pr := New(mc)
	assert.EqualError(t, pr.Delete(DeleteParams{Namespace: "my-org", ProjectReleaseName: "online-store-abc123"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteProjectRelease(mock.Anything, "my-org", "online-store-abc123").Return(nil)

	pr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, pr.Delete(DeleteParams{Namespace: "my-org", ProjectReleaseName: "online-store-abc123"}))
	})

	assert.Contains(t, out, "ProjectRelease 'online-store-abc123' deleted")
}
