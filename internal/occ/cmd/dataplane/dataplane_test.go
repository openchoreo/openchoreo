// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

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

func TestPrint_Nil(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No data planes found")
}

func TestPrint_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList([]gen.DataPlane{}))
	})
	assert.Contains(t, out, "No data planes found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.DataPlane{
		{Metadata: gen.ObjectMeta{Name: "dp-prod", CreationTimestamp: &now}},
		{Metadata: gen.ObjectMeta{Name: "dp-dev"}},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "dp-prod")
	assert.Contains(t, out, "dp-dev")
}

func TestPrint_NilTimestamp(t *testing.T) {
	items := []gen.DataPlane{
		{Metadata: gen.ObjectMeta{Name: "no-timestamp", CreationTimestamp: nil}},
	}

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-timestamp")
}

// --- Params tests ---

func TestListParams_GetNamespace(t *testing.T) {
	assert.Equal(t, "my-ns", ListParams{Namespace: "my-ns"}.GetNamespace())
}

func TestGetParams_GetNamespace(t *testing.T) {
	assert.Equal(t, "my-ns", GetParams{Namespace: "my-ns"}.GetNamespace())
}

func TestDeleteParams_Getters(t *testing.T) {
	p := DeleteParams{Namespace: "my-ns", DataPlaneName: "dp-prod"}
	assert.Equal(t, "my-ns", p.GetNamespace())
	assert.Equal(t, "dp-prod", p.GetDataPlaneName())
}

// --- List tests ---

func TestList_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	dp := New(mc)
	err := dp.List(ListParams{Namespace: ""})
	assert.Error(t, err)
}

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListDataPlanes(mock.Anything, "my-org", mock.Anything).Return(nil, fmt.Errorf("server error"))

	dp := New(mc)
	assert.EqualError(t, dp.List(ListParams{Namespace: "my-org"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListDataPlanes(mock.Anything, "my-org", mock.Anything).Return(&gen.DataPlaneList{
		Items:      []gen.DataPlane{{Metadata: gen.ObjectMeta{Name: "dp-prod"}}},
		Pagination: gen.Pagination{},
	}, nil)

	dp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, dp.List(ListParams{Namespace: "my-org"}))
	})
	assert.Contains(t, out, "dp-prod")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListDataPlanes(mock.Anything, "my-org", mock.Anything).Return(&gen.DataPlaneList{
		Items: []gen.DataPlane{
			{Metadata: gen.ObjectMeta{Name: "dp-prod", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "dp-dev", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	dp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, dp.List(ListParams{Namespace: "my-org"}))
	})
	assert.Contains(t, out, "dp-prod")
	assert.Contains(t, out, "dp-dev")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListDataPlanes(mock.Anything, "my-org", mock.Anything).Return(&gen.DataPlaneList{
		Items:      []gen.DataPlane{},
		Pagination: gen.Pagination{},
	}, nil)

	dp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, dp.List(ListParams{Namespace: "my-org"}))
	})
	assert.Contains(t, out, "No data planes found")
}

// --- Get tests ---

func TestGet_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	dp := New(mc)
	err := dp.Get(GetParams{Namespace: "", DataPlaneName: "dp-prod"})
	assert.Error(t, err)
}

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetDataPlane(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("not found: missing"))

	dp := New(mc)
	assert.EqualError(t, dp.Get(GetParams{Namespace: "my-org", DataPlaneName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetDataPlane(mock.Anything, "my-org", "dp-prod").Return(&gen.DataPlane{
		Metadata: gen.ObjectMeta{Name: "dp-prod"},
	}, nil)

	dp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, dp.Get(GetParams{Namespace: "my-org", DataPlaneName: "dp-prod"}))
	})
	assert.Contains(t, out, "name: dp-prod")
}

// --- Delete tests ---

func TestDelete_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	dp := New(mc)
	err := dp.Delete(DeleteParams{Namespace: "", DataPlaneName: "dp-prod"})
	assert.Error(t, err)
}

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteDataPlane(mock.Anything, "my-org", "dp-prod").Return(fmt.Errorf("forbidden"))

	dp := New(mc)
	assert.EqualError(t, dp.Delete(DeleteParams{Namespace: "my-org", DataPlaneName: "dp-prod"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteDataPlane(mock.Anything, "my-org", "dp-prod").Return(nil)

	dp := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, dp.Delete(DeleteParams{Namespace: "my-org", DataPlaneName: "dp-prod"}))
	})
	assert.Contains(t, out, "DataPlane 'dp-prod' deleted")
}
