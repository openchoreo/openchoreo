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

// --- Logs: RunName provided — bypasses ResolveLatestRun ---

func TestLogs_WithRunName_LiveLogs(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(
		&gen.WorkflowRunStatusResponse{HasLiveObservability: true}, nil)
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).Return(
		[]gen.WorkflowRunLogEntry{{Timestamp: &now, Log: "deploy step"}}, nil)

	cw := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cw.Logs(LogsParams{Namespace: "ns", WorkflowName: "my-cwf", RunName: "run-1"}))
	})
	assert.Contains(t, out, "deploy step")
}

func TestLogs_WithRunName_StatusError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(nil, fmt.Errorf("unavailable"))

	cw := New(mc)
	err := cw.Logs(LogsParams{Namespace: "ns", WorkflowName: "my-cwf", RunName: "run-1"})
	assert.ErrorContains(t, err, "failed to get workflow run status")
}

// --- StartRun ---

func TestStartRun_Success(t *testing.T) {
	ns := "ns"
	mc := mocks.NewMockClient(t)
	mc.EXPECT().CreateWorkflowRun(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRun{
		Metadata: gen.ObjectMeta{Name: "run-1", Namespace: &ns},
		Spec:     &gen.WorkflowRunSpec{Workflow: gen.WorkflowRunConfig{Name: "my-cwf"}},
	}, nil)

	cw := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cw.StartRun(StartRunParams{Namespace: "ns", WorkflowName: "my-cwf"}))
	})
	assert.Contains(t, out, "Successfully started workflow run: run-1")
}

func TestStartRun_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().CreateWorkflowRun(mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("forbidden"))

	cw := New(mc)
	err := cw.StartRun(StartRunParams{Namespace: "ns", WorkflowName: "my-cwf"})
	assert.EqualError(t, err, "forbidden")
}

// --- Logs: RunName NOT provided — resolves latest run ---

func TestLogs_WithoutRunName_ResolvesLatest(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRunList{
		Items:      []gen.WorkflowRun{{Metadata: gen.ObjectMeta{Name: "latest-run", CreationTimestamp: &now}}},
		Pagination: gen.Pagination{},
	}, nil)
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "latest-run").Return(
		&gen.WorkflowRunStatusResponse{HasLiveObservability: true}, nil)
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "latest-run", mock.Anything).Return(
		[]gen.WorkflowRunLogEntry{{Timestamp: &now, Log: "resolved log"}}, nil)

	cw := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cw.Logs(LogsParams{Namespace: "ns", WorkflowName: "my-cwf"}))
	})
	assert.Contains(t, out, "resolved log")
}

func TestLogs_WithoutRunName_NoRuns(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRunList{
		Items:      []gen.WorkflowRun{},
		Pagination: gen.Pagination{},
	}, nil)

	cw := New(mc)
	err := cw.Logs(LogsParams{Namespace: "ns", WorkflowName: "my-cwf"})
	assert.ErrorContains(t, err, "no workflow runs found")
}

// --- List: pagination cursor branch ---

func TestList_Pagination(t *testing.T) {
	cursor := "page2"
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListClusterWorkflows(mock.Anything, mock.MatchedBy(func(p *gen.ListClusterWorkflowsParams) bool {
		return p.Cursor == nil
	})).Return(&gen.ClusterWorkflowList{
		Items:      []gen.ClusterWorkflow{{Metadata: gen.ObjectMeta{Name: "wf-1"}}},
		Pagination: gen.Pagination{NextCursor: &cursor},
	}, nil).Once()
	mc.EXPECT().ListClusterWorkflows(mock.Anything, mock.MatchedBy(func(p *gen.ListClusterWorkflowsParams) bool {
		return p.Cursor != nil && *p.Cursor == cursor
	})).Return(&gen.ClusterWorkflowList{
		Items:      []gen.ClusterWorkflow{{Metadata: gen.ObjectMeta{Name: "wf-2"}}},
		Pagination: gen.Pagination{},
	}, nil).Once()

	cw := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, cw.List())
	})
	assert.Contains(t, out, "wf-1")
	assert.Contains(t, out, "wf-2")
}

// --- Params getters ---

func TestLogsParams_Getters(t *testing.T) {
	p := LogsParams{Namespace: "my-ns", WorkflowName: "my-wf"}
	assert.Equal(t, "my-ns", p.GetNamespace())
	assert.Equal(t, "my-wf", p.GetWorkflowName())
}
