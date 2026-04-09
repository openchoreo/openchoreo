// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

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

// --- printWorkloadList tests ---

func TestPrintWorkloadList_Nil(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printWorkloadList(nil))
	})
	assert.Contains(t, out, "No workloads found")
}

func TestPrintWorkloadList_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printWorkloadList([]gen.Workload{}))
	})
	assert.Contains(t, out, "No workloads found")
}

func TestPrintWorkloadList_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.Workload{
		{Metadata: gen.ObjectMeta{Name: "workload-1", CreationTimestamp: &now}},
		{Metadata: gen.ObjectMeta{Name: "workload-2"}},
	}
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printWorkloadList(items))
	})
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "workload-1")
	assert.Contains(t, out, "workload-2")
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListWorkloads(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("server error"))

	w := New(mc)
	assert.EqualError(t, w.List(ListParams{Namespace: "org-a"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListWorkloads(mock.Anything, "org-a", mock.Anything).Return(&gen.WorkloadList{
		Items:      []gen.Workload{{Metadata: gen.ObjectMeta{Name: "workload-1"}}},
		Pagination: gen.Pagination{},
	}, nil)

	w := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, w.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "workload-1")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListWorkloads(mock.Anything, "org-a", mock.Anything).Return(&gen.WorkloadList{
		Items: []gen.Workload{
			{Metadata: gen.ObjectMeta{Name: "workload-1", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "workload-2", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	w := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, w.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "workload-1")
	assert.Contains(t, out, "workload-2")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListWorkloads(mock.Anything, "org-a", mock.Anything).Return(&gen.WorkloadList{
		Items:      []gen.Workload{},
		Pagination: gen.Pagination{},
	}, nil)

	w := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, w.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "No workloads found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkload(mock.Anything, "org-a", "missing").Return(nil, fmt.Errorf("not found: missing"))

	w := New(mc)
	assert.EqualError(t, w.Get(GetParams{Namespace: "org-a", WorkloadName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkload(mock.Anything, "org-a", "workload-1").Return(&gen.Workload{
		Metadata: gen.ObjectMeta{Name: "workload-1"},
	}, nil)

	w := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, w.Get(GetParams{Namespace: "org-a", WorkloadName: "workload-1"}))
	})
	assert.Contains(t, out, "name: workload-1")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteWorkload(mock.Anything, "org-a", "workload-1").Return(fmt.Errorf("forbidden"))

	w := New(mc)
	assert.EqualError(t, w.Delete(DeleteParams{Namespace: "org-a", WorkloadName: "workload-1"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteWorkload(mock.Anything, "org-a", "workload-1").Return(nil)

	w := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, w.Delete(DeleteParams{Namespace: "org-a", WorkloadName: "workload-1"}))
	})
	assert.Contains(t, out, "Workload 'workload-1' deleted")
}

// --- Validation error tests ---

func TestList_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)
	err := w.List(ListParams{Namespace: ""})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestGet_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)
	err := w.Get(GetParams{Namespace: "", WorkloadName: "wl-1"})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestDelete_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)
	err := w.Delete(DeleteParams{Namespace: "", WorkloadName: "wl-1"})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestDelete_ValidationError_MissingName(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)
	err := w.Delete(DeleteParams{Namespace: "ns", WorkloadName: ""})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestCreate_ValidationError_MissingFields(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)
	err := w.Create(CreateParams{
		NamespaceName: "ns",
		ProjectName:   "proj",
		ComponentName: "comp",
		// ImageURL missing
	})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestCreate_UnsupportedMode(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)
	err := w.Create(CreateParams{
		NamespaceName: "ns",
		ProjectName:   "proj",
		ComponentName: "comp",
		ImageURL:      "img:latest",
		Mode:          "invalid-mode",
	})
	assert.ErrorContains(t, err, "unsupported mode")
}

// --- Constructor test ---

func TestNew(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)
	assert.NotNil(t, w)
	assert.Equal(t, mc, w.client)
}

// --- printWorkloadList edge cases ---

func TestPrintWorkloadList_NilTimestamp(t *testing.T) {
	items := []gen.Workload{
		{Metadata: gen.ObjectMeta{Name: "wl-no-ts", CreationTimestamp: nil}},
	}
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printWorkloadList(items))
	})
	assert.Contains(t, out, "wl-no-ts")
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
}

// --- Pagination ---

func TestList_Pagination(t *testing.T) {
	next := "cursor-2"
	mc := mocks.NewMockInterface(t)

	// First page — no cursor
	mc.EXPECT().ListWorkloads(mock.Anything, "org-a", mock.MatchedBy(func(p *gen.ListWorkloadsParams) bool {
		return p != nil && p.Cursor == nil
	})).Return(&gen.WorkloadList{
		Items:      []gen.Workload{{Metadata: gen.ObjectMeta{Name: "wl-page1"}}},
		Pagination: gen.Pagination{NextCursor: &next},
	}, nil).Once()

	// Second page — with cursor
	mc.EXPECT().ListWorkloads(mock.Anything, "org-a", mock.MatchedBy(func(p *gen.ListWorkloadsParams) bool {
		return p != nil && p.Cursor != nil && *p.Cursor == "cursor-2"
	})).Return(&gen.WorkloadList{
		Items:      []gen.Workload{{Metadata: gen.ObjectMeta{Name: "wl-page2"}}},
		Pagination: gen.Pagination{},
	}, nil).Once()

	w := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, w.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "wl-page1")
	assert.Contains(t, out, "wl-page2")
}

func TestList_NilTimestamp(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListWorkloads(mock.Anything, "org-a", mock.Anything).Return(&gen.WorkloadList{
		Items: []gen.Workload{
			{Metadata: gen.ObjectMeta{Name: "wl-no-ts", CreationTimestamp: nil}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	w := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, w.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "wl-no-ts")
}

// --- Get with spec ---

func TestGet_SuccessWithSpec(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkload(mock.Anything, "org-a", "wl-1").Return(&gen.Workload{
		Metadata: gen.ObjectMeta{Name: "wl-1"},
		Spec: &gen.WorkloadSpec{
			Container: &gen.WorkloadContainer{Image: "registry/my-svc:v1"},
			Owner: &struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{ComponentName: "comp-a", ProjectName: "proj-1"},
		},
	}, nil)

	w := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, w.Get(GetParams{Namespace: "org-a", WorkloadName: "wl-1"}))
	})
	assert.Contains(t, out, "name: wl-1")
	assert.Contains(t, out, "componentName: comp-a")
	assert.Contains(t, out, "projectName: proj-1")
}

// --- toSynthParams ---

func TestToAPIParams(t *testing.T) {
	p := CreateParams{
		FilePath:      "/path/to/descriptor.yaml",
		NamespaceName: "ns",
		ProjectName:   "proj",
		ComponentName: "comp",
		ImageURL:      "img:latest",
		OutputPath:    "/out",
		DryRun:        true,
		Mode:          "file-system",
		RootDir:       "/repo",
	}
	ap := toSynthParams(p)
	assert.Equal(t, p.FilePath, ap.FilePath)
	assert.Equal(t, p.NamespaceName, ap.NamespaceName)
	assert.Equal(t, p.ProjectName, ap.ProjectName)
	assert.Equal(t, p.ComponentName, ap.ComponentName)
	assert.Equal(t, p.ImageURL, ap.ImageURL)
	assert.Equal(t, p.OutputPath, ap.OutputPath)
	assert.Equal(t, p.DryRun, ap.DryRun)
	assert.Equal(t, p.Mode, ap.Mode)
	assert.Equal(t, p.RootDir, ap.RootDir)
}
