// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"testing"

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
			if len(got) != tt.wantCount {
				t.Errorf("FilterByComponent() returned %d, want %d", len(got), tt.wantCount)
			}
			for i, name := range tt.wantNames {
				if got[i].Metadata.Name != name {
					t.Errorf("FilterByComponent()[%d].Name = %q, want %q", i, got[i].Metadata.Name, name)
				}
			}
		})
	}

	t.Run("empty list", func(t *testing.T) {
		got := FilterByComponent(nil, "comp")
		if len(got) != 0 {
			t.Errorf("FilterByComponent(nil) returned %d, want 0", len(got))
		}
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
			if len(got) != tt.wantCount {
				t.Errorf("ExcludeComponentRuns() returned %d, want %d", len(got), tt.wantCount)
			}
			for i, name := range tt.wantNames {
				if got[i].Metadata.Name != name {
					t.Errorf("ExcludeComponentRuns()[%d].Name = %q, want %q", i, got[i].Metadata.Name, name)
				}
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
			if got := getComponentLabel(tt.run); got != tt.want {
				t.Errorf("getComponentLabel() = %q, want %q", got, tt.want)
			}
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
			if got := deriveStatus(tt.conditions); got != tt.want {
				t.Errorf("deriveStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
