// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

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

// --- printList tests ---

func TestPrintList_Nil(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No workflow planes found")
}

func TestPrintList_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.WorkflowPlane{}))
	})
	assert.Contains(t, out, "No workflow planes found")
}

func TestPrintList_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.WorkflowPlane{
		{Metadata: gen.ObjectMeta{Name: "wf-plane-1", CreationTimestamp: &now}},
		{Metadata: gen.ObjectMeta{Name: "wf-plane-2"}},
	}
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "wf-plane-1")
	assert.Contains(t, out, "wf-plane-2")
}

// --- List tests ---

func TestList_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	wp := New(mc)
	err := wp.List(ListParams{Namespace: ""})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListWorkflowPlanes(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("server error"))

	wp := New(mc)
	assert.EqualError(t, wp.List(ListParams{Namespace: "org-a"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListWorkflowPlanes(mock.Anything, "org-a", mock.Anything).Return(&gen.WorkflowPlaneList{
		Items:      []gen.WorkflowPlane{{Metadata: gen.ObjectMeta{Name: "wf-plane-1"}}},
		Pagination: gen.Pagination{},
	}, nil)

	wp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, wp.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "wf-plane-1")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListWorkflowPlanes(mock.Anything, "org-a", mock.Anything).Return(&gen.WorkflowPlaneList{
		Items: []gen.WorkflowPlane{
			{Metadata: gen.ObjectMeta{Name: "wf-plane-1", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "wf-plane-2", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	wp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, wp.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "wf-plane-1")
	assert.Contains(t, out, "wf-plane-2")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListWorkflowPlanes(mock.Anything, "org-a", mock.Anything).Return(&gen.WorkflowPlaneList{
		Items:      []gen.WorkflowPlane{},
		Pagination: gen.Pagination{},
	}, nil)

	wp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, wp.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "No workflow planes found")
}

// --- Get tests ---

func TestGet_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	wp := New(mc)
	err := wp.Get(GetParams{Namespace: "", WorkflowPlaneName: "wf-plane-1"})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflowPlane(mock.Anything, "org-a", "missing").Return(nil, fmt.Errorf("not found: missing"))

	wp := New(mc)
	assert.EqualError(t, wp.Get(GetParams{Namespace: "org-a", WorkflowPlaneName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflowPlane(mock.Anything, "org-a", "wf-plane-1").Return(&gen.WorkflowPlane{
		Metadata: gen.ObjectMeta{Name: "wf-plane-1"},
	}, nil)

	wp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, wp.Get(GetParams{Namespace: "org-a", WorkflowPlaneName: "wf-plane-1"}))
	})
	assert.Contains(t, out, "name: wf-plane-1")
}

// --- Delete tests ---

func TestDelete_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	wp := New(mc)
	err := wp.Delete(DeleteParams{Namespace: "", WorkflowPlaneName: "wf-plane-1"})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteWorkflowPlane(mock.Anything, "org-a", "wf-plane-1").Return(fmt.Errorf("forbidden"))

	wp := New(mc)
	assert.EqualError(t, wp.Delete(DeleteParams{Namespace: "org-a", WorkflowPlaneName: "wf-plane-1"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteWorkflowPlane(mock.Anything, "org-a", "wf-plane-1").Return(nil)

	wp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, wp.Delete(DeleteParams{Namespace: "org-a", WorkflowPlaneName: "wf-plane-1"}))
	})
	assert.Contains(t, out, "WorkflowPlane 'wf-plane-1' deleted")
}
