// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

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

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponentReleases(mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("server error"))

	cr := New(mc)
	assert.EqualError(t, cr.List(ListParams{Namespace: "ns"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponentReleases(mock.Anything, "ns", mock.Anything).Return(&gen.ComponentReleaseList{
		Items:      []gen.ComponentRelease{{Metadata: gen.ObjectMeta{Name: "rel-1"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cr.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "rel-1")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponentReleases(mock.Anything, "ns", mock.Anything).Return(&gen.ComponentReleaseList{
		Items: []gen.ComponentRelease{
			{Metadata: gen.ObjectMeta{Name: "rel-1", CreationTimestamp: &now}, Spec: &gen.ComponentReleaseSpec{Owner: struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{ComponentName: "comp-a"}}},
			{Metadata: gen.ObjectMeta{Name: "rel-2", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	cr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cr.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "rel-1")
	assert.Contains(t, out, "rel-2")
	assert.Contains(t, out, "comp-a")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponentReleases(mock.Anything, "ns", mock.Anything).Return(&gen.ComponentReleaseList{
		Items:      []gen.ComponentRelease{},
		Pagination: gen.Pagination{},
	}, nil)

	cr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cr.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "No component releases found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "ns", "missing").Return(nil, fmt.Errorf("not found: missing"))

	cr := New(mc)
	assert.EqualError(t, cr.Get(GetParams{Namespace: "ns", ComponentReleaseName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "ns", "rel-1").Return(&gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: "rel-1"},
	}, nil)

	cr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cr.Get(GetParams{Namespace: "ns", ComponentReleaseName: "rel-1"}))
	})

	assert.Contains(t, out, "name: rel-1")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteComponentRelease(mock.Anything, "ns", "rel-1").Return(fmt.Errorf("forbidden: rel-1"))

	cr := New(mc)
	assert.EqualError(t, cr.Delete(DeleteParams{Namespace: "ns", ComponentReleaseName: "rel-1"}), "forbidden: rel-1")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteComponentRelease(mock.Anything, "ns", "rel-1").Return(nil)

	cr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cr.Delete(DeleteParams{Namespace: "ns", ComponentReleaseName: "rel-1"}))
	})

	assert.Contains(t, out, "ComponentRelease 'rel-1' deleted")
}

// --- Validation error tests ---

func TestList_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	cr := New(mc)
	err := cr.List(ListParams{Namespace: ""})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestGet_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	cr := New(mc)
	err := cr.Get(GetParams{Namespace: "", ComponentReleaseName: "rel-1"})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestDelete_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	cr := New(mc)
	err := cr.Delete(DeleteParams{Namespace: "", ComponentReleaseName: "rel-1"})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestDelete_ValidationError_MissingName(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	cr := New(mc)
	err := cr.Delete(DeleteParams{Namespace: "ns", ComponentReleaseName: ""})
	assert.ErrorContains(t, err, "Missing required parameter")
}

// --- Constructor test ---

func TestNew(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	cr := New(mc)
	assert.NotNil(t, cr)
	assert.Equal(t, mc, cr.client)
}

// --- printComponentReleases pure function tests ---

func TestPrintComponentReleases_Nil(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printComponentReleases(nil))
	})
	assert.Contains(t, out, "No component releases found")
}

func TestPrintComponentReleases_NilTimestamp(t *testing.T) {
	items := []gen.ComponentRelease{
		{Metadata: gen.ObjectMeta{Name: "rel-no-ts", CreationTimestamp: nil}},
	}
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printComponentReleases(items))
	})
	assert.Contains(t, out, "rel-no-ts")
}

func TestPrintComponentReleases_NilSpec(t *testing.T) {
	items := []gen.ComponentRelease{
		{Metadata: gen.ObjectMeta{Name: "rel-nil-spec"}, Spec: nil},
	}
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printComponentReleases(items))
	})
	assert.Contains(t, out, "rel-nil-spec")
}

func TestPrintComponentReleases_WithSpec(t *testing.T) {
	now := time.Now()
	items := []gen.ComponentRelease{
		{
			Metadata: gen.ObjectMeta{Name: "rel-with-spec", CreationTimestamp: &now},
			Spec: &gen.ComponentReleaseSpec{
				Owner: struct {
					ComponentName string `json:"componentName"`
					ProjectName   string `json:"projectName"`
				}{ComponentName: "comp-a", ProjectName: "proj-1"},
			},
		},
	}
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printComponentReleases(items))
	})
	assert.Contains(t, out, "rel-with-spec")
	assert.Contains(t, out, "comp-a")
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "COMPONENT")
	assert.Contains(t, out, "AGE")
}

// --- List with component filter ---

func TestList_WithComponentFilter(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponentReleases(mock.Anything, "ns", mock.MatchedBy(func(p *gen.ListComponentReleasesParams) bool {
		return p.Component != nil && *p.Component == "my-comp"
	})).Return(&gen.ComponentReleaseList{
		Items:      []gen.ComponentRelease{{Metadata: gen.ObjectMeta{Name: "rel-1"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cr.List(ListParams{Namespace: "ns", Component: "my-comp"}))
	})
	assert.Contains(t, out, "rel-1")
}

// --- Pagination ---

func TestList_Pagination(t *testing.T) {
	next := "cursor-2"
	mc := mocks.NewMockInterface(t)

	// First page — no cursor
	mc.EXPECT().ListComponentReleases(mock.Anything, "ns", mock.MatchedBy(func(p *gen.ListComponentReleasesParams) bool {
		return p.Cursor == nil
	})).Return(&gen.ComponentReleaseList{
		Items:      []gen.ComponentRelease{{Metadata: gen.ObjectMeta{Name: "rel-page1"}}},
		Pagination: gen.Pagination{NextCursor: &next},
	}, nil).Once()

	// Second page — with cursor
	mc.EXPECT().ListComponentReleases(mock.Anything, "ns", mock.MatchedBy(func(p *gen.ListComponentReleasesParams) bool {
		return p.Cursor != nil && *p.Cursor == "cursor-2"
	})).Return(&gen.ComponentReleaseList{
		Items:      []gen.ComponentRelease{{Metadata: gen.ObjectMeta{Name: "rel-page2"}}},
		Pagination: gen.Pagination{},
	}, nil).Once()

	cr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cr.List(ListParams{Namespace: "ns"}))
	})
	assert.Contains(t, out, "rel-page1")
	assert.Contains(t, out, "rel-page2")
}

func TestList_NilTimestamp(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponentReleases(mock.Anything, "ns", mock.Anything).Return(&gen.ComponentReleaseList{
		Items: []gen.ComponentRelease{
			{Metadata: gen.ObjectMeta{Name: "rel-no-ts", CreationTimestamp: nil}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	cr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cr.List(ListParams{Namespace: "ns"}))
	})
	assert.Contains(t, out, "rel-no-ts")
}

func TestGet_SuccessWithSpec(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "ns", "rel-1").Return(&gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: "rel-1"},
		Spec: &gen.ComponentReleaseSpec{
			Owner: struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{ComponentName: "comp-a", ProjectName: "proj-1"},
		},
	}, nil)

	cr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cr.Get(GetParams{Namespace: "ns", ComponentReleaseName: "rel-1"}))
	})
	assert.Contains(t, out, "name: rel-1")
	assert.Contains(t, out, "componentName: comp-a")
	assert.Contains(t, out, "projectName: proj-1")
}
