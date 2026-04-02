// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterworkflowplane/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
		w.Close()
		r.Close()
	}()

	fn()

	os.Stdout = origStdout
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	return buf.String()
}

func TestPrint_Nil(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No cluster workflow planes found")
}

func TestPrint_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList([]gen.ClusterWorkflowPlane{}))
	})
	assert.Contains(t, out, "No cluster workflow planes found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ClusterWorkflowPlane{
		{Metadata: gen.ObjectMeta{Name: "argo-prod", CreationTimestamp: &now}},
		{Metadata: gen.ObjectMeta{Name: "argo-dev"}},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "argo-prod")
	assert.Contains(t, out, "argo-dev")
}

func TestPrint_NilTimestamp(t *testing.T) {
	items := []gen.ClusterWorkflowPlane{
		{Metadata: gen.ObjectMeta{Name: "no-timestamp", CreationTimestamp: nil}},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-timestamp")
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterWorkflowPlanes(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("server error"))

	cwp := New(mc)
	assert.EqualError(t, cwp.List(), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterWorkflowPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterWorkflowPlaneList{
		Items:      []gen.ClusterWorkflowPlane{{Metadata: gen.ObjectMeta{Name: "argo-prod"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cwp := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cwp.List())
	})
	assert.Contains(t, out, "argo-prod")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterWorkflowPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterWorkflowPlaneList{
		Items: []gen.ClusterWorkflowPlane{
			{Metadata: gen.ObjectMeta{Name: "argo-prod", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "argo-dev", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	cwp := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cwp.List())
	})
	assert.Contains(t, out, "argo-prod")
	assert.Contains(t, out, "argo-dev")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterWorkflowPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterWorkflowPlaneList{
		Items:      []gen.ClusterWorkflowPlane{},
		Pagination: gen.Pagination{},
	}, nil)

	cwp := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cwp.List())
	})
	assert.Contains(t, out, "No cluster workflow planes found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetClusterWorkflowPlane(mock.Anything, "missing").Return(nil, fmt.Errorf("not found: missing"))

	cwp := New(mc)
	assert.EqualError(t, cwp.Get(GetParams{ClusterWorkflowPlaneName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetClusterWorkflowPlane(mock.Anything, "argo-prod").Return(&gen.ClusterWorkflowPlane{
		Metadata: gen.ObjectMeta{Name: "argo-prod"},
	}, nil)

	cwp := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cwp.Get(GetParams{ClusterWorkflowPlaneName: "argo-prod"}))
	})
	assert.Contains(t, out, "name: argo-prod")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteClusterWorkflowPlane(mock.Anything, "argo-prod").Return(fmt.Errorf("forbidden"))

	cwp := New(mc)
	assert.EqualError(t, cwp.Delete(DeleteParams{ClusterWorkflowPlaneName: "argo-prod"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteClusterWorkflowPlane(mock.Anything, "argo-prod").Return(nil)

	cwp := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cwp.Delete(DeleteParams{ClusterWorkflowPlaneName: "argo-prod"}))
	})
	assert.Contains(t, out, "ClusterWorkflowPlane 'argo-prod' deleted")
}
