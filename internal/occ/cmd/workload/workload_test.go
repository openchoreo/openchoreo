// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

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

	"github.com/openchoreo/openchoreo/internal/occ/cmd/workload/mocks"
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

// --- printWorkloadList tests ---

func TestPrintWorkloadList_Nil(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printWorkloadList(nil))
	})
	assert.Contains(t, out, "No workloads found")
}

func TestPrintWorkloadList_Empty(t *testing.T) {
	out := captureStdout(t, func() {
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
	out := captureStdout(t, func() {
		require.NoError(t, printWorkloadList(items))
	})
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "workload-1")
	assert.Contains(t, out, "workload-2")
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkloads(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("server error"))

	w := New(mc)
	assert.EqualError(t, w.List(ListParams{Namespace: "org-a"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkloads(mock.Anything, "org-a", mock.Anything).Return(&gen.WorkloadList{
		Items:      []gen.Workload{{Metadata: gen.ObjectMeta{Name: "workload-1"}}},
		Pagination: gen.Pagination{},
	}, nil)

	w := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, w.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "workload-1")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkloads(mock.Anything, "org-a", mock.Anything).Return(&gen.WorkloadList{
		Items: []gen.Workload{
			{Metadata: gen.ObjectMeta{Name: "workload-1", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "workload-2", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	w := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, w.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "workload-1")
	assert.Contains(t, out, "workload-2")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkloads(mock.Anything, "org-a", mock.Anything).Return(&gen.WorkloadList{
		Items:      []gen.Workload{},
		Pagination: gen.Pagination{},
	}, nil)

	w := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, w.List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "No workloads found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetWorkload(mock.Anything, "org-a", "missing").Return(nil, fmt.Errorf("not found: missing"))

	w := New(mc)
	assert.EqualError(t, w.Get(GetParams{Namespace: "org-a", WorkloadName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetWorkload(mock.Anything, "org-a", "workload-1").Return(&gen.Workload{
		Metadata: gen.ObjectMeta{Name: "workload-1"},
	}, nil)

	w := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, w.Get(GetParams{Namespace: "org-a", WorkloadName: "workload-1"}))
	})
	assert.Contains(t, out, "name: workload-1")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteWorkload(mock.Anything, "org-a", "workload-1").Return(fmt.Errorf("forbidden"))

	w := New(mc)
	assert.EqualError(t, w.Delete(DeleteParams{Namespace: "org-a", WorkloadName: "workload-1"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteWorkload(mock.Anything, "org-a", "workload-1").Return(nil)

	w := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, w.Delete(DeleteParams{Namespace: "org-a", WorkloadName: "workload-1"}))
	})
	assert.Contains(t, out, "Workload 'workload-1' deleted")
}
