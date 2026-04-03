// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

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

	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflowrun/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func makeRun(name string, labels map[string]string) gen.WorkflowRun {
	var lbls *map[string]string
	if labels != nil {
		lbls = &labels
	}
	return gen.WorkflowRun{
		Metadata: gen.ObjectMeta{
			Name:   name,
			Labels: lbls,
		},
	}
}

func TestFilterByComponent(t *testing.T) {
	runs := []gen.WorkflowRun{
		makeRun("run-1", map[string]string{componentLabel: "my-comp"}),
		makeRun("run-2", map[string]string{componentLabel: "other-comp"}),
		makeRun("run-3", nil),
		makeRun("run-4", map[string]string{"unrelated": "value"}),
	}

	tests := []struct {
		name      string
		component string
		wantCount int
		wantNames []string
	}{
		{name: "matches one", component: "my-comp", wantCount: 1, wantNames: []string{"run-1"}},
		{name: "no match", component: "nonexistent", wantCount: 0},
		{name: "matches other", component: "other-comp", wantCount: 1, wantNames: []string{"run-2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterByComponent(runs, tt.component)
			require.Len(t, got, tt.wantCount)
			for i, name := range tt.wantNames {
				assert.Equal(t, name, got[i].Metadata.Name)
			}
		})
	}

	t.Run("empty list", func(t *testing.T) {
		got := FilterByComponent(nil, "comp")
		assert.Empty(t, got)
	})
}

func TestExcludeComponentRuns(t *testing.T) {
	runs := []gen.WorkflowRun{
		makeRun("run-1", map[string]string{componentLabel: "comp"}),
		makeRun("run-2", nil),
		makeRun("run-3", map[string]string{"other": "val"}),
		makeRun("run-4", map[string]string{componentLabel: "comp2"}),
	}

	tests := []struct {
		name      string
		input     []gen.WorkflowRun
		wantCount int
		wantNames []string
	}{
		{name: "mix", input: runs, wantCount: 2, wantNames: []string{"run-2", "run-3"}},
		{name: "all labeled", input: []gen.WorkflowRun{
			makeRun("r1", map[string]string{componentLabel: "c"}),
		}, wantCount: 0},
		{name: "none labeled", input: []gen.WorkflowRun{
			makeRun("r1", nil), makeRun("r2", map[string]string{"x": "y"}),
		}, wantCount: 2},
		{name: "empty", input: nil, wantCount: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExcludeComponentRuns(tt.input)
			require.Len(t, got, tt.wantCount)
			for i, name := range tt.wantNames {
				assert.Equal(t, name, got[i].Metadata.Name)
			}
		})
	}
}

func TestGetComponentLabel(t *testing.T) {
	tests := []struct {
		name string
		run  gen.WorkflowRun
		want string
	}{
		{name: "present", run: makeRun("r", map[string]string{componentLabel: "comp"}), want: "comp"},
		{name: "nil labels", run: makeRun("r", nil), want: ""},
		{name: "missing key", run: makeRun("r", map[string]string{"other": "val"}), want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, getComponentLabel(tt.run))
		})
	}
}

func TestDeriveStatus(t *testing.T) {
	cond := func(typ, status, reason string) gen.Condition {
		return gen.Condition{Type: typ, Status: gen.ConditionStatus(status), Reason: reason}
	}

	tests := []struct {
		name       string
		conditions []gen.Condition
		want       string
	}{
		{name: "succeeded", conditions: []gen.Condition{cond("WorkflowSucceeded", "True", "Done")}, want: "Succeeded"},
		{name: "failed", conditions: []gen.Condition{cond("WorkflowFailed", "True", "Error")}, want: "Failed"},
		{name: "running", conditions: []gen.Condition{cond("WorkflowRunning", "True", "InProgress")}, want: "Running"},
		{name: "completed with reason", conditions: []gen.Condition{cond("WorkflowCompleted", "True", "Finished")}, want: "Finished"},
		{name: "empty returns pending", conditions: []gen.Condition{}, want: "Pending"},
		{name: "succeeded takes priority over running", conditions: []gen.Condition{
			cond("WorkflowRunning", "True", "InProgress"),
			cond("WorkflowSucceeded", "True", "Done"),
		}, want: "Succeeded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, deriveStatus(tt.conditions))
		})
	}
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
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("server error"))

	wr := New(mc)
	assert.EqualError(t, wr.List(ListParams{Namespace: "ns"}), "server error")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRunList{
		Items: []gen.WorkflowRun{{
			Metadata: gen.ObjectMeta{Name: "run-1"},
			Spec:     &gen.WorkflowRunSpec{Workflow: gen.WorkflowRunConfig{Name: "my-wf"}},
		}},
		Pagination: gen.Pagination{},
	}, nil)

	wr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wr.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "run-1")
	assert.Contains(t, out, "my-wf")
}

func TestList_MultipleItems(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRunList{
		Items: []gen.WorkflowRun{
			{Metadata: gen.ObjectMeta{Name: "run-1", CreationTimestamp: &now}, Spec: &gen.WorkflowRunSpec{Workflow: gen.WorkflowRunConfig{Name: "wf-a"}}},
			{Metadata: gen.ObjectMeta{Name: "run-2", CreationTimestamp: &now}, Spec: &gen.WorkflowRunSpec{Workflow: gen.WorkflowRunConfig{Name: "wf-b"}}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	wr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wr.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "run-1")
	assert.Contains(t, out, "run-2")
}

func TestList_Empty(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRunList{
		Items:      []gen.WorkflowRun{},
		Pagination: gen.Pagination{},
	}, nil)

	wr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wr.List(ListParams{Namespace: "ns"}))
	})

	assert.Contains(t, out, "No workflow runs found")
}

// --- Get tests ---

func TestGet_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetWorkflowRun(mock.Anything, "ns", "missing").Return(nil, fmt.Errorf("not found: missing"))

	wr := New(mc)
	assert.EqualError(t, wr.Get(GetParams{Namespace: "ns", WorkflowRunName: "missing"}), "not found: missing")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().GetWorkflowRun(mock.Anything, "ns", "run-1").Return(&gen.WorkflowRun{
		Metadata: gen.ObjectMeta{Name: "run-1"},
	}, nil)

	wr := New(mc)
	out := captureStdout(t, func() {
		require.NoError(t, wr.Get(GetParams{Namespace: "ns", WorkflowRunName: "run-1"}))
	})

	assert.Contains(t, out, "name: run-1")
}

// --- FetchAll tests ---

func TestFetchAll_APIError(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("server error"))

	wr := New(mc)
	_, err := wr.FetchAll("ns", "")
	assert.EqualError(t, err, "server error")
}

func TestFetchAll_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.Anything).Return(&gen.WorkflowRunList{
		Items: []gen.WorkflowRun{
			{Metadata: gen.ObjectMeta{Name: "run-1"}},
			{Metadata: gen.ObjectMeta{Name: "run-2"}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	wr := New(mc)
	items, err := wr.FetchAll("ns", "")
	require.NoError(t, err)
	assert.Len(t, items, 2)
	assert.Equal(t, "run-1", items[0].Metadata.Name)
}

func TestFetchAll_Pagination(t *testing.T) {
	cursor := "next-page-token"
	mc := mocks.NewMockClient(t)
	// First page returns a cursor
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.MatchedBy(func(p *gen.ListWorkflowRunsParams) bool {
		return p.Cursor == nil
	})).Return(&gen.WorkflowRunList{
		Items:      []gen.WorkflowRun{{Metadata: gen.ObjectMeta{Name: "run-1"}}},
		Pagination: gen.Pagination{NextCursor: &cursor},
	}, nil).Once()
	// Second page returns no cursor
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.MatchedBy(func(p *gen.ListWorkflowRunsParams) bool {
		return p.Cursor != nil && *p.Cursor == cursor
	})).Return(&gen.WorkflowRunList{
		Items:      []gen.WorkflowRun{{Metadata: gen.ObjectMeta{Name: "run-2"}}},
		Pagination: gen.Pagination{},
	}, nil).Once()

	wr := New(mc)
	items, err := wr.FetchAll("ns", "")
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestFetchAll_WithWorkflowFilter(t *testing.T) {
	mc := mocks.NewMockClient(t)
	mc.EXPECT().ListWorkflowRuns(mock.Anything, "ns", mock.MatchedBy(func(p *gen.ListWorkflowRunsParams) bool {
		return p.Workflow != nil && *p.Workflow == "my-wf"
	})).Return(&gen.WorkflowRunList{
		Items:      []gen.WorkflowRun{{Metadata: gen.ObjectMeta{Name: "run-1"}}},
		Pagination: gen.Pagination{},
	}, nil)

	wr := New(mc)
	items, err := wr.FetchAll("ns", "my-wf")
	require.NoError(t, err)
	assert.Len(t, items, 1)
}

// --- List: validation ---

func TestList_ValidationError(t *testing.T) {
	wr := New(nil)
	err := wr.List(ListParams{})
	require.Error(t, err)
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

// --- Get: validation ---

func TestGet_ValidationError(t *testing.T) {
	wr := New(nil)
	err := wr.Get(GetParams{WorkflowRunName: "run-1"})
	require.Error(t, err)
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

// --- PrintList: nil conditions ---

func TestPrintList_NilSpec(t *testing.T) {
	items := []gen.WorkflowRun{
		{Metadata: gen.ObjectMeta{Name: "run-no-spec"}},
	}
	out := captureStdout(t, func() {
		require.NoError(t, PrintList(items))
	})
	assert.Contains(t, out, "run-no-spec")
	assert.Contains(t, out, "Pending")
}
