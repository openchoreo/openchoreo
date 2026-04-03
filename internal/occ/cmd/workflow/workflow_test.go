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
