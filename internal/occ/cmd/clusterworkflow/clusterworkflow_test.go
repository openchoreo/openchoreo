// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

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

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterworkflow/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// captureStdout captures stdout output from a function call.
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
	assert.Contains(t, out, "No cluster workflows found")
}

func TestPrint_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList([]gen.ClusterWorkflow{}))
	})
	assert.Contains(t, out, "No cluster workflows found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.ClusterWorkflow{
		{
			Metadata: gen.ObjectMeta{
				Name:              "build-go",
				CreationTimestamp: &now,
			},
		},
		{
			Metadata: gen.ObjectMeta{
				Name: "build-docker",
			},
		},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "build-go")
	assert.Contains(t, out, "build-docker")
}

func TestPrint_NilTimestamp(t *testing.T) {
	items := []gen.ClusterWorkflow{
		{
			Metadata: gen.ObjectMeta{
				Name:              "no-timestamp",
				CreationTimestamp: nil,
			},
		},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-timestamp")
}

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterWorkflows(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("server error"))

	cw := New(mc)
	assert.EqualError(t, cw.List(), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterWorkflows(mock.Anything, mock.Anything).Return(&gen.ClusterWorkflowList{
		Items:      []gen.ClusterWorkflow{{Metadata: gen.ObjectMeta{Name: "build-go"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cw := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cw.List())
	})
	assert.Contains(t, out, "build-go")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterWorkflows(mock.Anything, mock.Anything).Return(&gen.ClusterWorkflowList{
		Items: []gen.ClusterWorkflow{
			{Metadata: gen.ObjectMeta{Name: "build-go", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "build-docker", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	cw := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cw.List())
	})
	assert.Contains(t, out, "build-go")
	assert.Contains(t, out, "build-docker")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterWorkflows(mock.Anything, mock.Anything).Return(&gen.ClusterWorkflowList{
		Items:      []gen.ClusterWorkflow{},
		Pagination: gen.Pagination{},
	}, nil)

	cw := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cw.List())
	})
	assert.Contains(t, out, "No cluster workflows found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetClusterWorkflow(mock.Anything, "missing").Return(nil, fmt.Errorf("not found: missing"))

	cw := New(mc)
	assert.EqualError(t, cw.Get(GetParams{ClusterWorkflowName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetClusterWorkflow(mock.Anything, "build-go").Return(&gen.ClusterWorkflow{
		Metadata: gen.ObjectMeta{Name: "build-go"},
	}, nil)

	cw := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cw.Get(GetParams{ClusterWorkflowName: "build-go"}))
	})
	assert.Contains(t, out, "name: build-go")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteClusterWorkflow(mock.Anything, "build-go").Return(fmt.Errorf("forbidden"))

	cw := New(mc)
	assert.EqualError(t, cw.Delete(DeleteParams{ClusterWorkflowName: "build-go"}), "forbidden")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteClusterWorkflow(mock.Anything, "build-go").Return(nil)

	cw := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cw.Delete(DeleteParams{ClusterWorkflowName: "build-go"}))
	})
	assert.Contains(t, out, "ClusterWorkflow 'build-go' deleted")
}

// --- Validation tests ---

func TestStartRun_ValidationError(t *testing.T) {
	cw := New(nil) // client is never called when validation fails

	t.Run("missing namespace", func(t *testing.T) {
		err := cw.StartRun(StartRunParams{})
		require.Error(t, err)
		assert.EqualError(t, err, "namespace is required")
	})

	t.Run("missing cluster workflow name", func(t *testing.T) {
		err := cw.StartRun(StartRunParams{Namespace: "ns"})
		require.Error(t, err)
		assert.EqualError(t, err, "cluster workflow name is required")
	})
}

func TestLogs_ValidationError(t *testing.T) {
	cw := New(nil) // client is never called when validation fails

	t.Run("missing namespace", func(t *testing.T) {
		err := cw.Logs(LogsParams{})
		require.Error(t, err)
		assert.EqualError(t, err, "namespace is required")
	})

	t.Run("missing cluster workflow name", func(t *testing.T) {
		err := cw.Logs(LogsParams{Namespace: "ns"})
		require.Error(t, err)
		assert.EqualError(t, err, "cluster workflow name is required")
	})
}
