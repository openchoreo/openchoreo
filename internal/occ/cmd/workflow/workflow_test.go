// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

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

	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflow/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestIsComponentWorkflow(t *testing.T) {
	tests := []struct {
		name string
		wf   gen.Workflow
		want bool
	}{
		{
			name: "has component-scope label",
			wf: gen.Workflow{
				Metadata: gen.ObjectMeta{
					Name:   "wf-1",
					Labels: &map[string]string{labels.LabelKeyWorkflowType: labels.LabelValueWorkflowTypeComponent},
				},
			},
			want: true,
		},
		{
			name: "wrong value",
			wf: gen.Workflow{
				Metadata: gen.ObjectMeta{
					Name:   "wf-2",
					Labels: &map[string]string{labels.LabelKeyWorkflowType: "other"},
				},
			},
			want: false,
		},
		{
			name: "no labels",
			wf: gen.Workflow{
				Metadata: gen.ObjectMeta{Name: "wf-3"},
			},
			want: false,
		},
		{
			name: "different key",
			wf: gen.Workflow{
				Metadata: gen.ObjectMeta{
					Name:   "wf-4",
					Labels: &map[string]string{"unrelated": "value"},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isComponentWorkflow(tt.wf))
		})
	}
}

func TestApplySetOverrides(t *testing.T) {
	baseRun := func(name, workflowName string) gen.WorkflowRun {
		ns := "test-ns"
		return gen.WorkflowRun{
			Metadata: gen.ObjectMeta{
				Name:      name,
				Namespace: &ns,
			},
			Spec: &gen.WorkflowRunSpec{
				Workflow: gen.WorkflowRunConfig{
					Name: workflowName,
				},
			},
		}
	}

	t.Run("empty set values returns unchanged", func(t *testing.T) {
		req := baseRun("noop-run", "build-wf")
		got, err := applySetOverrides(req, "build-wf", nil)
		require.NoError(t, err)
		assert.Equal(t, "noop-run", got.Metadata.Name)
		assert.Equal(t, "build-wf", got.Spec.Workflow.Name)
	})

	t.Run("override metadata name", func(t *testing.T) {
		req := baseRun("original-run", "deploy-wf")
		got, err := applySetOverrides(req, "deploy-wf", []string{"metadata.name=renamed-run"})
		require.NoError(t, err)
		assert.Equal(t, "renamed-run", got.Metadata.Name)
	})

	t.Run("workflow name override is enforced back", func(t *testing.T) {
		req := baseRun("enforce-run", "protected-wf")
		got, err := applySetOverrides(req, "protected-wf", []string{"spec.workflow.name=hijacked"})
		require.NoError(t, err)
		assert.Equal(t, "protected-wf", got.Spec.Workflow.Name, "workflow name should be enforced")
	})

	t.Run("invalid set value returns error", func(t *testing.T) {
		req := baseRun("bad-input-run", "test-wf")
		_, err := applySetOverrides(req, "test-wf", []string{"no-equals-sign"})
		require.Error(t, err)
	})

	t.Run("multiple overrides applied", func(t *testing.T) {
		req := baseRun("multi-override-run", "ci-wf")
		got, err := applySetOverrides(req, "ci-wf", []string{
			"metadata.name=custom-run",
		})
		require.NoError(t, err)
		assert.Equal(t, "custom-run", got.Metadata.Name)
		assert.Equal(t, "ci-wf", got.Spec.Workflow.Name, "workflow name should be enforced")
	})
}

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

// --- List tests ---

func TestList_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflows(mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("server error"))

	wf := New(mc)
	assert.EqualError(t, wf.List(ListParams{Namespace: "ns"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflows(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowList{
		Items:      []gen.Workflow{{Metadata: gen.ObjectMeta{Name: "my-workflow"}}},
		Pagination: gen.Pagination{},
	}, nil)

	wf := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wf.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "my-workflow")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflows(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowList{
		Items: []gen.Workflow{
			{Metadata: gen.ObjectMeta{Name: "wf-build", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "wf-deploy", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	wf := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wf.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "wf-build")
	assert.Contains(t, out, "wf-deploy")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflows(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowList{
		Items:      []gen.Workflow{},
		Pagination: gen.Pagination{},
	}, nil)

	wf := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wf.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "No workflows found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetWorkflow(mock.Anything, "ns", "missing").Return(nil, fmt.Errorf("not found: missing"))

	wf := New(mc)
	assert.EqualError(t, wf.Get(GetParams{Namespace: "ns", WorkflowName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetWorkflow(mock.Anything, "ns", "my-workflow").Return(&gen.Workflow{
		Metadata: gen.ObjectMeta{Name: "my-workflow"},
	}, nil)

	wf := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wf.Get(GetParams{Namespace: "ns", WorkflowName: "my-workflow"}))
	})

	assert.Contains(t, out, "name: my-workflow")
}

// --- Delete tests ---

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteWorkflow(mock.Anything, "ns", "my-workflow").Return(fmt.Errorf("forbidden: my-workflow"))

	wf := New(mc)
	assert.EqualError(t, wf.Delete(DeleteParams{Namespace: "ns", WorkflowName: "my-workflow"}), "forbidden: my-workflow")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().DeleteWorkflow(mock.Anything, "ns", "my-workflow").Return(nil)

	wf := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wf.Delete(DeleteParams{Namespace: "ns", WorkflowName: "my-workflow"}))
	})

	assert.Contains(t, out, "Workflow 'my-workflow' deleted")
}

// --- StartRun tests ---

func TestStartRun_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().CreateWorkflowRun(mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("server error"))

	wf := New(mc)
	assert.EqualError(t, wf.StartRun(StartRunParams{
		Namespace:    "ns",
		WorkflowName: "my-wf",
		RunName:      "run-1",
	}), "server error")
}

func TestStartRun_Success(t *testing.T) {
	ns := "ns"
	mc := mocks.NewMockClient(t)
	mc.EXPECT().CreateWorkflowRun(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRun{
		Metadata: gen.ObjectMeta{Name: "run-1", Namespace: &ns},
		Spec:     &gen.WorkflowRunSpec{Workflow: gen.WorkflowRunConfig{Name: "my-wf"}},
	}, nil)

	wf := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wf.StartRun(StartRunParams{
			Namespace:    "ns",
			WorkflowName: "my-wf",
			RunName:      "run-1",
		}))
	})

	assert.Contains(t, out, "Successfully started workflow run: run-1")
	assert.Contains(t, out, "Workflow: my-wf")
}

// --- Validation tests ---

func TestGet_ValidationError(t *testing.T) {
	wf := New(nil) // client is never called when validation fails

	t.Run("missing namespace", func(t *testing.T) {
		err := wf.Get(GetParams{})
		require.Error(t, err)
		assert.EqualError(t, err, "namespace is required")
	})

	t.Run("missing workflow name", func(t *testing.T) {
		err := wf.Get(GetParams{Namespace: "ns"})
		require.Error(t, err)
		assert.EqualError(t, err, "workflow name is required")
	})
}

func TestStartRun_ValidationError(t *testing.T) {
	wf := New(nil) // client is never called when validation fails

	t.Run("missing namespace", func(t *testing.T) {
		err := wf.StartRun(StartRunParams{})
		require.Error(t, err)
		assert.EqualError(t, err, "namespace is required")
	})

	t.Run("missing workflow name", func(t *testing.T) {
		err := wf.StartRun(StartRunParams{Namespace: "ns"})
		require.Error(t, err)
		assert.EqualError(t, err, "workflow name is required")
	})
}

// --- Logs validation tests ---

func TestLogs_ValidationError(t *testing.T) {
	wf := New(nil) // client is never called when validation fails

	t.Run("missing namespace", func(t *testing.T) {
		err := wf.Logs(LogsParams{WorkflowName: "my-wf"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "Missing required parameter: --namespace")
	})

	t.Run("missing workflow name", func(t *testing.T) {
		err := wf.Logs(LogsParams{Namespace: "ns"})
		require.Error(t, err)
		assert.EqualError(t, err, "workflow name is required")
	})
}

// --- Logs: RunName provided — bypasses ResolveLatestRun ---

func TestLogs_WithRunName_LiveLogs(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(
		&gen.WorkflowRunStatusResponse{HasLiveObservability: true}, nil)
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).Return(
		[]gen.WorkflowRunLogEntry{{Timestamp: &now, Log: "build step"}}, nil)

	wf := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wf.Logs(LogsParams{Namespace: "ns", WorkflowName: "my-wf", RunName: "run-1"}))
	})
	assert.Contains(t, out, "build step")
}

func TestLogs_WithRunName_StatusError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(nil, fmt.Errorf("unavailable"))

	wf := New(mc)
	err := wf.Logs(LogsParams{Namespace: "ns", WorkflowName: "my-wf", RunName: "run-1"})
	assert.ErrorContains(t, err, "failed to get workflow run status")
}

// --- ResolveLatestRun ---

func TestResolveLatestRun_Success(t *testing.T) {
	now := time.Now()
	older := now.Add(-time.Hour)
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRunList{
		Items: []gen.WorkflowRun{
			{Metadata: gen.ObjectMeta{Name: "run-old", CreationTimestamp: &older}},
			{Metadata: gen.ObjectMeta{Name: "run-new", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	wf := New(mc)
	got, err := wf.ResolveLatestRun("ns", "my-wf", nil)
	require.NoError(t, err)
	assert.Equal(t, "run-new", got)
}

func TestResolveLatestRun_NoRuns(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRunList{
		Items:      []gen.WorkflowRun{},
		Pagination: gen.Pagination{},
	}, nil)

	wf := New(mc)
	_, err := wf.ResolveLatestRun("ns", "my-wf", nil)
	assert.ErrorContains(t, err, "no workflow runs found")
}

func TestResolveLatestRun_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("server error"))

	wf := New(mc)
	_, err := wf.ResolveLatestRun("ns", "my-wf", nil)
	assert.ErrorContains(t, err, "failed to list workflow runs")
}

// --- Logs: RunName NOT provided — resolves from latest run ---

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
		[]gen.WorkflowRunLogEntry{{Timestamp: &now, Log: "auto-resolved log"}}, nil)

	wf := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wf.Logs(LogsParams{Namespace: "ns", WorkflowName: "my-wf"}))
	})
	assert.Contains(t, out, "auto-resolved log")
}

func TestLogs_WithoutRunName_NoRuns(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRunList{
		Items:      []gen.WorkflowRun{},
		Pagination: gen.Pagination{},
	}, nil)

	wf := New(mc)
	err := wf.Logs(LogsParams{Namespace: "ns", WorkflowName: "my-wf"})
	assert.ErrorContains(t, err, "no workflow runs found")
}

// --- ResolveLatestRun: pagination cursor branch ---

func TestResolveLatestRun_Pagination(t *testing.T) {
	cursor := "page2"
	now := time.Now()
	later := now.Add(time.Minute)
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.MatchedBy(func(p *gen.ListWorkflowRunsParams) bool {
		return p.Cursor == nil
	})).Return(&gen.WorkflowRunList{
		Items:      []gen.WorkflowRun{{Metadata: gen.ObjectMeta{Name: "run-old", CreationTimestamp: &now}}},
		Pagination: gen.Pagination{NextCursor: &cursor},
	}, nil).Once()
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.MatchedBy(func(p *gen.ListWorkflowRunsParams) bool {
		return p.Cursor != nil && *p.Cursor == cursor
	})).Return(&gen.WorkflowRunList{
		Items:      []gen.WorkflowRun{{Metadata: gen.ObjectMeta{Name: "run-new", CreationTimestamp: &later}}},
		Pagination: gen.Pagination{},
	}, nil).Once()

	wf := New(mc)
	got, err := wf.ResolveLatestRun("ns", "my-wf", nil)
	require.NoError(t, err)
	assert.Equal(t, "run-new", got)
}

// --- List: pagination cursor branch ---

func TestList_Pagination(t *testing.T) {
	cursor := "page2"
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflows(mock.Anything, "ns", mock.MatchedBy(func(p *gen.ListWorkflowsParams) bool {
		return p.Cursor == nil
	})).Return(&gen.WorkflowList{
		Items:      []gen.Workflow{{Metadata: gen.ObjectMeta{Name: "wf-1"}}},
		Pagination: gen.Pagination{NextCursor: &cursor},
	}, nil).Once()
	mc.EXPECT().ListWorkflows(mock.Anything, "ns", mock.MatchedBy(func(p *gen.ListWorkflowsParams) bool {
		return p.Cursor != nil && *p.Cursor == cursor
	})).Return(&gen.WorkflowList{
		Items:      []gen.Workflow{{Metadata: gen.ObjectMeta{Name: "wf-2"}}},
		Pagination: gen.Pagination{},
	}, nil).Once()

	wf := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wf.List(ListParams{Namespace: "ns"}))
	})
	assert.Contains(t, out, "wf-1")
	assert.Contains(t, out, "wf-2")
}

// --- Params getters ---

func TestListParams_GetNamespace(t *testing.T) {
	p := ListParams{Namespace: "my-ns"}
	assert.Equal(t, "my-ns", p.GetNamespace())
}

// --- StartRun: with Parameters and Labels ---

func TestStartRun_WithParametersAndLabels(t *testing.T) {
	ns := "ns"
	mc := mocks.NewMockClient(t)
	mc.EXPECT().CreateWorkflowRun(mock.Anything, "ns", mock.MatchedBy(func(r gen.WorkflowRun) bool {
		return r.Spec != nil && r.Spec.Workflow.Parameters != nil && r.Metadata.Labels != nil
	})).Return(&gen.WorkflowRun{
		Metadata: gen.ObjectMeta{Name: "run-1", Namespace: &ns},
		Spec:     &gen.WorkflowRunSpec{Workflow: gen.WorkflowRunConfig{Name: "my-wf"}},
	}, nil)

	wf := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wf.StartRun(StartRunParams{
			Namespace:    "ns",
			WorkflowName: "my-wf",
			RunName:      "run-1",
			Parameters:   map[string]interface{}{"key": "value"},
			Labels:       map[string]string{"env": "test"},
		}))
	})
	assert.Contains(t, out, "run-1")
}

// --- Delete: validation ---

func TestDelete_ValidationError(t *testing.T) {
	wf := New(nil)
	err := wf.Delete(DeleteParams{})
	require.Error(t, err)
	assert.ErrorContains(t, err, "Missing required parameters:")
}

func TestResolveLatestRun_WithFilter(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRunList{
		Items: []gen.WorkflowRun{
			{Metadata: gen.ObjectMeta{Name: "keep", CreationTimestamp: &now}},
			{Metadata: gen.ObjectMeta{Name: "discard", CreationTimestamp: &now}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	wf := New(mc)
	got, err := wf.ResolveLatestRun("ns", "my-wf", func(items []gen.WorkflowRun) []gen.WorkflowRun {
		return []gen.WorkflowRun{items[0]} // keep only first
	})
	require.NoError(t, err)
	assert.Equal(t, "keep", got)
}
