// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

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

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterdataplane/mocks"
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
	assert.Contains(t, out, "No cluster data planes found")
}

func TestPrint_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList([]gen.ClusterDataPlane{}))
	})
	assert.Contains(t, out, "No cluster data planes found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ClusterDataPlane{
		{Metadata: gen.ObjectMeta{Name: "dp-prod", CreationTimestamp: &now}},
		{Metadata: gen.ObjectMeta{Name: "dp-dev"}},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "dp-prod")
	assert.Contains(t, out, "dp-dev")
}

func TestPrint_NilTimestamp(t *testing.T) {
	items := []gen.ClusterDataPlane{
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
	mc.EXPECT().ListClusterDataPlanes(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("server error"))

	cdp := New(mc)
	assert.EqualError(t, cdp.List(), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterDataPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterDataPlaneList{
		Items:      []gen.ClusterDataPlane{{Metadata: gen.ObjectMeta{Name: "dp-prod"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cdp := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cdp.List())
	})
	assert.Contains(t, out, "dp-prod")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterDataPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterDataPlaneList{
		Items: []gen.ClusterDataPlane{
			{Metadata: gen.ObjectMeta{Name: "dp-prod", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "dp-dev", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	cdp := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cdp.List())
	})
	assert.Contains(t, out, "dp-prod")
	assert.Contains(t, out, "dp-dev")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterDataPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterDataPlaneList{
		Items:      []gen.ClusterDataPlane{},
		Pagination: gen.Pagination{},
	}, nil)

	cdp := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cdp.List())
	})
	assert.Contains(t, out, "No cluster data planes found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetClusterDataPlane(mock.Anything, "missing").Return(nil, fmt.Errorf("not found: missing"))

	cdp := New(mc)
	assert.EqualError(t, cdp.Get(GetParams{ClusterDataPlaneName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetClusterDataPlane(mock.Anything, "dp-prod").Return(&gen.ClusterDataPlane{
		Metadata: gen.ObjectMeta{Name: "dp-prod"},
	}, nil)

	cdp := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cdp.Get(GetParams{ClusterDataPlaneName: "dp-prod"}))
	})
	assert.Contains(t, out, "name: dp-prod")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteClusterDataPlane(mock.Anything, "dp-prod").Return(fmt.Errorf("forbidden"))

	cdp := New(mc)
	assert.EqualError(t, cdp.Delete(DeleteParams{ClusterDataPlaneName: "dp-prod"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteClusterDataPlane(mock.Anything, "dp-prod").Return(nil)

	cdp := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cdp.Delete(DeleteParams{ClusterDataPlaneName: "dp-prod"}))
	})
	assert.Contains(t, out, "ClusterDataPlane 'dp-prod' deleted")
}
