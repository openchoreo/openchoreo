// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
)

const testBuildNS = "build-ns"

// ---------------------------------------------------------------------------
// extractServiceAccountName
// ---------------------------------------------------------------------------

func TestExtractServiceAccountName(t *testing.T) {
	tests := []struct {
		name      string
		resource  map[string]any
		want      string
		wantError bool
	}{
		{
			name: "should extract service account name from resource",
			resource: map[string]any{
				"spec": map[string]any{
					"serviceAccountName": "my-service-account",
				},
			},
			want: "my-service-account",
		},
		{
			name: "should return error when spec not found",
			resource: map[string]any{
				"metadata": map[string]any{},
			},
			wantError: true,
		},
		{
			name: "should return error when serviceAccountName not found",
			resource: map[string]any{
				"spec": map[string]any{
					"otherField": "value",
				},
			},
			wantError: true,
		},
		{
			name: "should return error when serviceAccountName is empty",
			resource: map[string]any{
				"spec": map[string]any{
					"serviceAccountName": "",
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractServiceAccountName(tt.resource)
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.want {
					t.Errorf("expected %s, got %s", tt.want, result)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractRunResourceNamespace
// ---------------------------------------------------------------------------

func TestExtractRunResourceNamespace(t *testing.T) {
	tests := []struct {
		name      string
		resource  map[string]any
		want      string
		wantError bool
	}{
		{
			name: "should extract namespace from resource",
			resource: map[string]any{
				"metadata": map[string]any{
					"namespace": "my-namespace",
					"name":      "my-resource",
				},
			},
			want: "my-namespace",
		},
		{
			name: "should return error when metadata not found",
			resource: map[string]any{
				"spec": map[string]any{},
			},
			wantError: true,
		},
		{
			name: "should return error when namespace not found",
			resource: map[string]any{
				"metadata": map[string]any{
					"name": "my-resource",
				},
			},
			wantError: true,
		},
		{
			name: "should return error when namespace is empty",
			resource: map[string]any{
				"metadata": map[string]any{
					"namespace": "",
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractRunResourceNamespace(tt.resource)
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.want {
					t.Errorf("expected %s, got %s", tt.want, result)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// convertToString
// ---------------------------------------------------------------------------

func TestConvertToString(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		want     string
		contains string // use when exact match isn't practical (floats)
	}{
		{name: "string", input: "hello", want: "hello"},
		{name: "int", input: 42, want: "42"},
		{name: "int32", input: int32(42), want: "42"},
		{name: "int64", input: int64(42), want: "42"},
		{name: "float32", input: float32(3.14), contains: "3.14"},
		{name: "float64", input: 3.14159, contains: "3.14"},
		{name: "bool true", input: true, want: "true"},
		{name: "bool false", input: false, want: "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToString(tt.input)
			if tt.want != "" && result != tt.want {
				t.Errorf("expected %s, got %s", tt.want, result)
			}
			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("expected result to contain %q, got %q", tt.contains, result)
			}
		})
	}

	t.Run("map to JSON string", func(t *testing.T) {
		input := map[string]any{"key1": "value1", "key2": 42}
		result := convertToString(input)
		var decoded map[string]any
		if err := json.Unmarshal([]byte(result), &decoded); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}
		if decoded["key1"] != "value1" {
			t.Errorf("expected key1=value1, got %v", decoded["key1"])
		}
	})

	t.Run("slice to JSON string", func(t *testing.T) {
		input := []any{"item1", "item2", 3}
		result := convertToString(input)
		var decoded []any
		if err := json.Unmarshal([]byte(result), &decoded); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}
		if len(decoded) != 3 {
			t.Errorf("expected length 3, got %d", len(decoded))
		}
	})
}

// ---------------------------------------------------------------------------
// convertParameterValuesToStrings
// ---------------------------------------------------------------------------

func TestConvertParameterValuesToStrings(t *testing.T) {
	t.Run("should convert parameter values in workflow resource", func(t *testing.T) {
		resource := map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Workflow",
			"spec": map[string]any{
				"arguments": map[string]any{
					"parameters": []any{
						map[string]any{"name": "param1", "value": 42},
						map[string]any{"name": "param2", "value": true},
					},
				},
			},
		}

		result := convertParameterValuesToStrings(resource)
		spec := result["spec"].(map[string]any)
		args := spec["arguments"].(map[string]any)
		params := args["parameters"].([]any)

		if params[0].(map[string]any)["value"] != "42" {
			t.Errorf("expected param1 value '42', got %v", params[0].(map[string]any)["value"])
		}
		if params[1].(map[string]any)["value"] != "true" {
			t.Errorf("expected param2 value 'true', got %v", params[1].(map[string]any)["value"])
		}
	})

	t.Run("should preserve non-parameter fields", func(t *testing.T) {
		resource := map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Workflow",
			"metadata":   map[string]any{"name": "test"},
		}

		result := convertParameterValuesToStrings(resource)
		if result["apiVersion"] != "argoproj.io/v1alpha1" {
			t.Error("expected apiVersion to be preserved")
		}
		if result["kind"] != "Workflow" {
			t.Error("expected kind to be preserved")
		}
	})

	t.Run("should handle resource without spec", func(t *testing.T) {
		resource := map[string]any{"metadata": map[string]any{"name": "test"}}
		result := convertParameterValuesToStrings(resource)
		if result["metadata"] == nil {
			t.Error("expected metadata to be preserved")
		}
	})

	t.Run("should handle spec without arguments", func(t *testing.T) {
		resource := map[string]any{
			"spec": map[string]any{"entrypoint": "main"},
		}
		result := convertParameterValuesToStrings(resource)
		spec := result["spec"].(map[string]any)
		if spec["entrypoint"] != "main" {
			t.Error("expected entrypoint to be preserved")
		}
	})
}

// ---------------------------------------------------------------------------
// Condition functions
// ---------------------------------------------------------------------------

func TestConditionFunctions(t *testing.T) {
	newWFRun := func() *openchoreodevv1alpha1.WorkflowRun {
		return &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Generation: 1},
		}
	}

	t.Run("setWorkflowPendingCondition", func(t *testing.T) {
		wfr := newWFRun()
		setWorkflowPendingCondition(wfr)
		assertConditionCount(t, wfr, 1)
		assertCondition(t, wfr, string(ConditionWorkflowCompleted), metav1.ConditionFalse, string(ReasonWorkflowPending))
	})

	t.Run("setWorkflowRunningCondition", func(t *testing.T) {
		wfr := newWFRun()
		setWorkflowRunningCondition(wfr)
		assertConditionCount(t, wfr, 1)
		assertCondition(t, wfr, string(ConditionWorkflowRunning), metav1.ConditionTrue, string(ReasonWorkflowRunning))
	})

	t.Run("setWorkflowSucceededCondition", func(t *testing.T) {
		wfr := newWFRun()
		setWorkflowSucceededCondition(wfr)
		assertConditionCount(t, wfr, 3)
		assertCondition(t, wfr, string(ConditionWorkflowRunning), metav1.ConditionFalse, "")
		assertCondition(t, wfr, string(ConditionWorkflowSucceeded), metav1.ConditionTrue, string(ReasonWorkflowSucceeded))
		assertCondition(t, wfr, string(ConditionWorkflowCompleted), metav1.ConditionTrue, string(ReasonWorkflowSucceeded))
	})

	t.Run("setWorkflowFailedCondition", func(t *testing.T) {
		wfr := newWFRun()
		setWorkflowFailedCondition(wfr)
		assertConditionCount(t, wfr, 3)
		assertCondition(t, wfr, string(ConditionWorkflowRunning), metav1.ConditionFalse, "")
		assertCondition(t, wfr, string(ConditionWorkflowFailed), metav1.ConditionTrue, string(ReasonWorkflowFailed))
		assertCondition(t, wfr, string(ConditionWorkflowCompleted), metav1.ConditionTrue, string(ReasonWorkflowFailed))
	})

	t.Run("setWorkflowNotFoundCondition", func(t *testing.T) {
		wfr := newWFRun()
		setWorkflowNotFoundCondition(wfr)
		assertConditionCount(t, wfr, 2)
		assertCondition(t, wfr, string(ConditionWorkflowRunning), metav1.ConditionFalse, "")
		assertCondition(t, wfr, string(ConditionWorkflowCompleted), metav1.ConditionTrue, string(ReasonWorkflowFailed))
	})

	t.Run("setWorkflowPlaneNotFoundCondition", func(t *testing.T) {
		wfr := newWFRun()
		setWorkflowPlaneNotFoundCondition(wfr)
		assertConditionCount(t, wfr, 1)
		assertCondition(t, wfr, string(ConditionWorkflowCompleted), metav1.ConditionFalse, string(ReasonWorkflowPlaneNotFound))
	})

	t.Run("setWorkflowPlaneResolutionFailedCondition", func(t *testing.T) {
		wfr := newWFRun()
		setWorkflowPlaneResolutionFailedCondition(wfr, errors.New("test error"))
		assertConditionCount(t, wfr, 1)
		assertCondition(t, wfr, string(ConditionWorkflowCompleted), metav1.ConditionFalse, string(ReasonWorkflowPlaneResolutionFailed))
		cond := findConditionByType(wfr.Status.Conditions, string(ConditionWorkflowCompleted))
		if !strings.Contains(cond.Message, "test error") {
			t.Errorf("expected message to contain 'test error', got %q", cond.Message)
		}
	})
}

func TestIsWorkflowInitiated(t *testing.T) {
	t.Run("false when no conditions", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{}
		if isWorkflowInitiated(wfr) {
			t.Error("expected false")
		}
	})
	t.Run("true after pending condition set", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{}
		setWorkflowPendingCondition(wfr)
		if !isWorkflowInitiated(wfr) {
			t.Error("expected true")
		}
	})
}

func TestIsWorkflowCompleted(t *testing.T) {
	t.Run("false for pending workflow", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
		setWorkflowPendingCondition(wfr)
		if isWorkflowCompleted(wfr) {
			t.Error("expected false")
		}
	})
	t.Run("true for succeeded workflow", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
		setWorkflowSucceededCondition(wfr)
		if !isWorkflowCompleted(wfr) {
			t.Error("expected true")
		}
	})
	t.Run("true for failed workflow", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
		setWorkflowFailedCondition(wfr)
		if !isWorkflowCompleted(wfr) {
			t.Error("expected true")
		}
	})
}

func TestIsWorkflowSucceeded(t *testing.T) {
	t.Run("false for running workflow", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
		setWorkflowRunningCondition(wfr)
		if isWorkflowSucceeded(wfr) {
			t.Error("expected false")
		}
	})
	t.Run("true for succeeded workflow", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
		setWorkflowSucceededCondition(wfr)
		if !isWorkflowSucceeded(wfr) {
			t.Error("expected true")
		}
	})
	t.Run("false for failed workflow", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
		setWorkflowFailedCondition(wfr)
		if isWorkflowSucceeded(wfr) {
			t.Error("expected false")
		}
	})
}

// ---------------------------------------------------------------------------
// setStartedAtIfNeeded / setCompletedAtIfNeeded
// ---------------------------------------------------------------------------

func TestSetStartedAtIfNeeded(t *testing.T) {
	t.Run("sets StartedAt when nil", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{}
		setStartedAtIfNeeded(wfr)
		if wfr.Status.StartedAt == nil {
			t.Error("expected StartedAt to be set")
		}
	})
	t.Run("does not overwrite existing StartedAt", func(t *testing.T) {
		existing := metav1.NewTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			Status: openchoreodevv1alpha1.WorkflowRunStatus{StartedAt: &existing},
		}
		setStartedAtIfNeeded(wfr)
		if !wfr.Status.StartedAt.Equal(&existing) {
			t.Error("expected StartedAt to remain unchanged")
		}
	})
}

func TestSetCompletedAtIfNeeded(t *testing.T) {
	t.Run("sets CompletedAt when nil", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{}
		setCompletedAtIfNeeded(wfr)
		if wfr.Status.CompletedAt == nil {
			t.Error("expected CompletedAt to be set")
		}
	})
	t.Run("does not overwrite existing CompletedAt", func(t *testing.T) {
		existing := metav1.NewTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			Status: openchoreodevv1alpha1.WorkflowRunStatus{CompletedAt: &existing},
		}
		setCompletedAtIfNeeded(wfr)
		if !wfr.Status.CompletedAt.Equal(&existing) {
			t.Error("expected CompletedAt to remain unchanged")
		}
	})
}

// ---------------------------------------------------------------------------
// hasResourcesInStatus
// ---------------------------------------------------------------------------

func TestHasResourcesInStatus(t *testing.T) {
	tests := []struct {
		name string
		wfr  *openchoreodevv1alpha1.WorkflowRun
		want bool
	}{
		{
			name: "nil RunReference and nil Resources",
			wfr:  &openchoreodevv1alpha1.WorkflowRun{},
			want: false,
		},
		{
			name: "RunReference with empty name",
			wfr: &openchoreodevv1alpha1.WorkflowRun{
				Status: openchoreodevv1alpha1.WorkflowRunStatus{
					RunReference: &openchoreodevv1alpha1.ResourceReference{Name: ""},
				},
			},
			want: false,
		},
		{
			name: "RunReference with name",
			wfr: &openchoreodevv1alpha1.WorkflowRun{
				Status: openchoreodevv1alpha1.WorkflowRunStatus{
					RunReference: &openchoreodevv1alpha1.ResourceReference{Name: "wf-run"},
				},
			},
			want: true,
		},
		{
			name: "nil RunReference but Resources with entries",
			wfr: &openchoreodevv1alpha1.WorkflowRun{
				Status: openchoreodevv1alpha1.WorkflowRunStatus{
					Resources: &[]openchoreodevv1alpha1.ResourceReference{
						{Name: "secret1", Kind: "Secret"},
					},
				},
			},
			want: true,
		},
		{
			name: "nil RunReference and empty Resources slice",
			wfr: &openchoreodevv1alpha1.WorkflowRun{
				Status: openchoreodevv1alpha1.WorkflowRunStatus{
					Resources: &[]openchoreodevv1alpha1.ResourceReference{},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasResourcesInStatus(tt.wfr); got != tt.want {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractArgoStepOrderFromNodeName
// ---------------------------------------------------------------------------

func TestExtractArgoStepOrderFromNodeName(t *testing.T) {
	tests := []struct {
		name     string
		nodeName string
		want     int
	}{
		{"first step", "workflow-name[0].step-name", 0},
		{"second step", "workflow-name[1].step-name", 1},
		{"tenth step", "workflow-name[10].step-name", 10},
		{"no brackets", "no-brackets", -1},
		{"non-numeric in brackets", "brackets[abc].step", -1},
		{"empty string", "", -1},
		{"nested brackets extracts last pair", "name[0][1].step", 1},
		{"missing closing bracket", "name[0.step", -1},
		{"empty brackets", "name[].step", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractArgoStepOrderFromNodeName(tt.nodeName)
			if got != tt.want {
				t.Errorf("expected %d, got %d", tt.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractTaskNameFromArgoNodeName
// ---------------------------------------------------------------------------

func TestExtractTaskNameFromArgoNodeName(t *testing.T) {
	tests := []struct {
		name     string
		nodeName string
		want     string
	}{
		{"standard pattern", "workflow-name[0].step-name", "step-name"},
		{"workload task name", "workflow-name[0].generate-workload-cr", "generate-workload-cr"},
		{"no dot", "no-dot-in-name", ""},
		{"trailing dot", "trailing-dot.", ""},
		{"empty string", "", ""},
		{"multiple dots", "a.b.c", "c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTaskNameFromArgoNodeName(tt.nodeName)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractArgoTasksFromWorkflowNodes
// ---------------------------------------------------------------------------

func TestExtractArgoTasksFromWorkflowNodes(t *testing.T) {
	startTime := metav1.NewTime(time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC))
	finishTime := metav1.NewTime(time.Date(2025, 6, 1, 10, 5, 0, 0, time.UTC))

	t.Run("nil nodes returns nil", func(t *testing.T) {
		tasks := extractArgoTasksFromWorkflowNodes(nil)
		if tasks != nil {
			t.Errorf("expected nil, got %v", tasks)
		}
	})

	t.Run("empty nodes returns empty slice", func(t *testing.T) {
		tasks := extractArgoTasksFromWorkflowNodes(argoproj.Nodes{})
		if len(tasks) != 0 {
			t.Errorf("expected 0 tasks, got %d", len(tasks))
		}
	})

	t.Run("only Pod nodes are extracted", func(t *testing.T) {
		nodes := argoproj.Nodes{
			"pod-node": {
				Name:        "wf[0].build",
				DisplayName: "build",
				Type:        argoproj.NodeTypePod,
				Phase:       argoproj.NodeRunning,
			},
			"steps-node": {
				Name:        "wf[0]",
				DisplayName: "steps",
				Type:        argoproj.NodeTypeSteps,
				Phase:       argoproj.NodeRunning,
			},
			"group-node": {
				Name:        "wf[0]",
				DisplayName: "group",
				Type:        argoproj.NodeTypeStepGroup,
				Phase:       argoproj.NodeRunning,
			},
		}
		tasks := extractArgoTasksFromWorkflowNodes(nodes)
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}
		if tasks[0].Name != "build" {
			t.Errorf("expected task name 'build', got %q", tasks[0].Name)
		}
	})

	t.Run("tasks sorted by step order", func(t *testing.T) {
		nodes := argoproj.Nodes{
			"node-b": {
				Name:        "wf[1].deploy",
				DisplayName: "deploy",
				Type:        argoproj.NodeTypePod,
				Phase:       argoproj.NodeSucceeded,
			},
			"node-a": {
				Name:        "wf[0].build",
				DisplayName: "build",
				Type:        argoproj.NodeTypePod,
				Phase:       argoproj.NodeSucceeded,
			},
			"node-c": {
				Name:        "wf[2].verify",
				DisplayName: "verify",
				Type:        argoproj.NodeTypePod,
				Phase:       argoproj.NodeRunning,
			},
		}
		tasks := extractArgoTasksFromWorkflowNodes(nodes)
		if len(tasks) != 3 {
			t.Fatalf("expected 3 tasks, got %d", len(tasks))
		}
		expectedOrder := []string{"build", "deploy", "verify"}
		for i, name := range expectedOrder {
			if tasks[i].Name != name {
				t.Errorf("tasks[%d]: expected %q, got %q", i, name, tasks[i].Name)
			}
		}
	})

	t.Run("timestamps are set when available", func(t *testing.T) {
		nodes := argoproj.Nodes{
			"node1": {
				Name:        "wf[0].build",
				DisplayName: "build",
				Type:        argoproj.NodeTypePod,
				Phase:       argoproj.NodeSucceeded,
				StartedAt:   startTime,
				FinishedAt:  finishTime,
			},
		}
		tasks := extractArgoTasksFromWorkflowNodes(nodes)
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}
		if tasks[0].StartedAt == nil || !tasks[0].StartedAt.Equal(&startTime) {
			t.Errorf("expected StartedAt=%v, got %v", startTime, tasks[0].StartedAt)
		}
		if tasks[0].CompletedAt == nil || !tasks[0].CompletedAt.Equal(&finishTime) {
			t.Errorf("expected CompletedAt=%v, got %v", finishTime, tasks[0].CompletedAt)
		}
	})

	t.Run("falls back to parsed node name when no DisplayName", func(t *testing.T) {
		nodes := argoproj.Nodes{
			"node1": {
				Name:  "wf[0].checkout-code",
				Type:  argoproj.NodeTypePod,
				Phase: argoproj.NodeSucceeded,
			},
		}
		tasks := extractArgoTasksFromWorkflowNodes(nodes)
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}
		if tasks[0].Name != "checkout-code" {
			t.Errorf("expected task name 'checkout-code', got %q", tasks[0].Name)
		}
	})

	t.Run("phase and message are set", func(t *testing.T) {
		nodes := argoproj.Nodes{
			"node1": {
				Name:        "wf[0].build",
				DisplayName: "build",
				Type:        argoproj.NodeTypePod,
				Phase:       argoproj.NodeFailed,
				Message:     "OOMKilled",
			},
		}
		tasks := extractArgoTasksFromWorkflowNodes(nodes)
		if tasks[0].Phase != string(argoproj.NodeFailed) {
			t.Errorf("expected phase %q, got %q", argoproj.NodeFailed, tasks[0].Phase)
		}
		if tasks[0].Message != "OOMKilled" {
			t.Errorf("expected message 'OOMKilled', got %q", tasks[0].Message)
		}
	})
}

// ---------------------------------------------------------------------------
// syncWorkflowRunStatus
// ---------------------------------------------------------------------------

func TestSyncWorkflowRunStatus(t *testing.T) {
	r := &Reconciler{}

	newWFRun := func() *openchoreodevv1alpha1.WorkflowRun {
		return &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Generation: 1},
		}
	}

	t.Run("Running phase", func(t *testing.T) {
		wfr := newWFRun()
		runResource := &argoproj.Workflow{}
		runResource.Status.Phase = argoproj.WorkflowRunning

		result := r.syncWorkflowRunStatus(wfr, runResource)
		if result.RequeueAfter != 20*time.Second {
			t.Errorf("expected RequeueAfter=20s, got %v", result.RequeueAfter)
		}
		assertCondition(t, wfr, string(ConditionWorkflowRunning), metav1.ConditionTrue, "")
	})

	t.Run("Succeeded phase", func(t *testing.T) {
		wfr := newWFRun()
		runResource := &argoproj.Workflow{}
		runResource.Status.Phase = argoproj.WorkflowSucceeded

		result := r.syncWorkflowRunStatus(wfr, runResource)
		if !result.Requeue {
			t.Error("expected Requeue=true")
		}
		assertCondition(t, wfr, string(ConditionWorkflowSucceeded), metav1.ConditionTrue, "")
		assertCondition(t, wfr, string(ConditionWorkflowCompleted), metav1.ConditionTrue, "")
	})

	t.Run("Failed phase", func(t *testing.T) {
		wfr := newWFRun()
		runResource := &argoproj.Workflow{}
		runResource.Status.Phase = argoproj.WorkflowFailed

		result := r.syncWorkflowRunStatus(wfr, runResource)
		if result.Requeue || result.RequeueAfter > 0 {
			t.Error("expected no requeue for failed workflow")
		}
		assertCondition(t, wfr, string(ConditionWorkflowFailed), metav1.ConditionTrue, "")
	})

	t.Run("Error phase", func(t *testing.T) {
		wfr := newWFRun()
		runResource := &argoproj.Workflow{}
		runResource.Status.Phase = argoproj.WorkflowError

		result := r.syncWorkflowRunStatus(wfr, runResource)
		if result.Requeue || result.RequeueAfter > 0 {
			t.Error("expected no requeue for error workflow")
		}
		assertCondition(t, wfr, string(ConditionWorkflowFailed), metav1.ConditionTrue, "")
	})

	t.Run("unknown phase requeues", func(t *testing.T) {
		wfr := newWFRun()
		runResource := &argoproj.Workflow{}
		runResource.Status.Phase = "" // unknown

		result := r.syncWorkflowRunStatus(wfr, runResource)
		if !result.Requeue {
			t.Error("expected Requeue=true for unknown phase")
		}
	})

	t.Run("tasks are extracted from nodes", func(t *testing.T) {
		wfr := newWFRun()
		runResource := &argoproj.Workflow{}
		runResource.Status.Phase = argoproj.WorkflowRunning
		runResource.Status.Nodes = argoproj.Nodes{
			"node1": {
				Name:        "wf[0].build",
				DisplayName: "build",
				Type:        argoproj.NodeTypePod,
				Phase:       argoproj.NodeSucceeded,
			},
		}

		r.syncWorkflowRunStatus(wfr, runResource)
		if len(wfr.Status.Tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(wfr.Status.Tasks))
		}
		if wfr.Status.Tasks[0].Name != "build" {
			t.Errorf("expected task name 'build', got %q", wfr.Status.Tasks[0].Name)
		}
	})
}

// ---------------------------------------------------------------------------
// checkTTLExpiration (non-delete paths)
// ---------------------------------------------------------------------------

func TestCheckTTLExpiration(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{Client: fakeClient, Scheme: scheme}

	t.Run("returns false when CompletedAt is nil", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			Spec:   openchoreodevv1alpha1.WorkflowRunSpec{TTLAfterCompletion: "1h"},
			Status: openchoreodevv1alpha1.WorkflowRunStatus{CompletedAt: nil},
		}
		shouldReturn, result, err := r.checkTTLExpiration(context.Background(), wfr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if shouldReturn {
			t.Error("expected shouldReturn=false")
		}
		if result != (ctrl.Result{}) {
			t.Errorf("expected empty result, got %v", result)
		}
	})

	t.Run("returns false when TTLAfterCompletion is empty", func(t *testing.T) {
		now := metav1.Now()
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			Spec:   openchoreodevv1alpha1.WorkflowRunSpec{TTLAfterCompletion: ""},
			Status: openchoreodevv1alpha1.WorkflowRunStatus{CompletedAt: &now},
		}
		shouldReturn, result, err := r.checkTTLExpiration(context.Background(), wfr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if shouldReturn {
			t.Error("expected shouldReturn=false")
		}
		if result != (ctrl.Result{}) {
			t.Errorf("expected empty result, got %v", result)
		}
	})

	t.Run("returns false for invalid TTL format", func(t *testing.T) {
		now := metav1.Now()
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			Spec:   openchoreodevv1alpha1.WorkflowRunSpec{TTLAfterCompletion: "invalid"},
			Status: openchoreodevv1alpha1.WorkflowRunStatus{CompletedAt: &now},
		}
		shouldReturn, _, err := r.checkTTLExpiration(context.Background(), wfr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if shouldReturn {
			t.Error("expected shouldReturn=false for invalid TTL")
		}
	})

	t.Run("requeues when TTL has not expired", func(t *testing.T) {
		notExpiredTime := metav1.NewTime(time.Now().Add(-30 * time.Minute))
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			Spec:   openchoreodevv1alpha1.WorkflowRunSpec{TTLAfterCompletion: "2h"},
			Status: openchoreodevv1alpha1.WorkflowRunStatus{CompletedAt: &notExpiredTime},
		}
		shouldReturn, result, err := r.checkTTLExpiration(context.Background(), wfr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !shouldReturn {
			t.Error("expected shouldReturn=true")
		}
		if result.RequeueAfter <= 0 {
			t.Error("expected positive RequeueAfter")
		}
		if result.RequeueAfter > 2*time.Hour {
			t.Errorf("RequeueAfter too large: %v", result.RequeueAfter)
		}
	})
}

// ---------------------------------------------------------------------------
// make* helpers (run_engine.go)
// ---------------------------------------------------------------------------

func TestMakeNamespace(t *testing.T) {
	ns := makeNamespace(testBuildNS)
	if ns.Name != testBuildNS {
		t.Errorf("expected name 'build-ns', got %q", ns.Name)
	}
}

func TestMakeServiceAccount(t *testing.T) {
	sa := makeServiceAccount(testBuildNS, "workflow-sa")
	if sa.Name != "workflow-sa" {
		t.Errorf("expected name 'workflow-sa', got %q", sa.Name)
	}
	if sa.Namespace != testBuildNS {
		t.Errorf("expected namespace 'build-ns', got %q", sa.Namespace)
	}
}

func TestMakeRole(t *testing.T) {
	role := makeRole(testBuildNS, "workflow-role")
	if role.Name != "workflow-role" {
		t.Errorf("expected name 'workflow-role', got %q", role.Name)
	}
	if role.Namespace != testBuildNS {
		t.Errorf("expected namespace 'build-ns', got %q", role.Namespace)
	}
	if len(role.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(role.Rules))
	}
	rule := role.Rules[0]
	if len(rule.APIGroups) != 1 || rule.APIGroups[0] != "argoproj.io" {
		t.Errorf("expected APIGroup 'argoproj.io', got %v", rule.APIGroups)
	}
	if len(rule.Resources) != 1 || rule.Resources[0] != "workflowtaskresults" {
		t.Errorf("expected resource 'workflowtaskresults', got %v", rule.Resources)
	}
}

func TestMakeRoleBinding(t *testing.T) {
	rb := makeRoleBinding(testBuildNS, "workflow-sa", "workflow-role", "workflow-rb")
	if rb.Name != "workflow-rb" {
		t.Errorf("expected name 'workflow-rb', got %q", rb.Name)
	}
	if rb.Namespace != testBuildNS {
		t.Errorf("expected namespace 'build-ns', got %q", rb.Namespace)
	}
	if len(rb.Subjects) != 1 {
		t.Fatalf("expected 1 subject, got %d", len(rb.Subjects))
	}
	if rb.Subjects[0].Name != "workflow-sa" {
		t.Errorf("expected subject name 'workflow-sa', got %q", rb.Subjects[0].Name)
	}
	if rb.RoleRef.Name != "workflow-role" {
		t.Errorf("expected roleRef name 'workflow-role', got %q", rb.RoleRef.Name)
	}
	if rb.RoleRef.Kind != "Role" {
		t.Errorf("expected roleRef kind 'Role', got %q", rb.RoleRef.Kind)
	}
}

// ---------------------------------------------------------------------------
// validateComponentWorkflowRun
// ---------------------------------------------------------------------------

func TestValidateComponentWorkflowRun(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(scheme)

	t.Run("standalone workflow run (no labels) skips validation", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wfr", Namespace: "default"},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "my-wf"},
			},
		}

		result := r.validateComponentWorkflowRun(context.Background(), wfr)
		if result.shouldReturn {
			t.Error("expected shouldReturn=false for standalone workflow run")
		}
	})

	t.Run("only project label present fails", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				Labels:    map[string]string{"openchoreo.dev/project": "my-proj"},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "my-wf"},
			},
		}

		result := r.validateComponentWorkflowRun(context.Background(), wfr)
		if !result.shouldReturn {
			t.Error("expected shouldReturn=true")
		}
		assertCondition(t, wfr, string(ConditionWorkflowCompleted), metav1.ConditionTrue, string(ReasonComponentValidationFailed))
	})

	t.Run("only component label present fails", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				Labels:    map[string]string{"openchoreo.dev/component": "my-comp"},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "my-wf"},
			},
		}

		result := r.validateComponentWorkflowRun(context.Background(), wfr)
		if !result.shouldReturn {
			t.Error("expected shouldReturn=true")
		}
		assertCondition(t, wfr, string(ConditionWorkflowCompleted), metav1.ConditionTrue, string(ReasonComponentValidationFailed))
	})

	t.Run("component not found fails permanently", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				Labels: map[string]string{
					"openchoreo.dev/project":   "my-proj",
					"openchoreo.dev/component": "missing-comp",
				},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "my-wf"},
			},
		}

		result := r.validateComponentWorkflowRun(context.Background(), wfr)
		if !result.shouldReturn {
			t.Error("expected shouldReturn=true")
		}
		if result.result.Requeue {
			t.Error("expected no requeue for permanent failure")
		}
		cond := findConditionByType(wfr.Status.Conditions, string(ConditionWorkflowCompleted))
		if cond == nil || !strings.Contains(cond.Message, "not found") {
			t.Error("expected condition message to mention 'not found'")
		}
	})

	t.Run("project label mismatch fails permanently", func(t *testing.T) {
		comp := &openchoreodevv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentSpec{
				Owner:         openchoreodevv1alpha1.ComponentOwner{ProjectName: "actual-proj"},
				ComponentType: openchoreodevv1alpha1.ComponentTypeRef{Name: "deployment/my-ct"},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(comp).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				Labels: map[string]string{
					"openchoreo.dev/project":   "wrong-proj",
					"openchoreo.dev/component": "my-comp",
				},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "my-wf"},
			},
		}

		result := r.validateComponentWorkflowRun(context.Background(), wfr)
		if !result.shouldReturn {
			t.Error("expected shouldReturn=true")
		}
		if result.result.Requeue {
			t.Error("expected no requeue for permanent failure")
		}
		assertCondition(t, wfr, string(ConditionWorkflowCompleted), metav1.ConditionTrue, string(ReasonComponentValidationFailed))
	})

	t.Run("workflow not in allowedWorkflows fails permanently", func(t *testing.T) {
		ct := &openchoreodevv1alpha1.ComponentType{
			ObjectMeta: metav1.ObjectMeta{Name: "my-ct", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				AllowedWorkflows: []openchoreodevv1alpha1.WorkflowRef{
					{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "allowed-wf"},
				},
				Resources: []openchoreodevv1alpha1.ResourceTemplate{
					{ID: "deployment", Template: &runtime.RawExtension{Raw: []byte("{}")}},
				},
			},
		}
		comp := &openchoreodevv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentSpec{
				Owner:         openchoreodevv1alpha1.ComponentOwner{ProjectName: "my-proj"},
				ComponentType: openchoreodevv1alpha1.ComponentTypeRef{Name: "deployment/my-ct"},
				Workflow: &openchoreodevv1alpha1.ComponentWorkflowConfig{
					Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow,
					Name: "not-allowed-wf",
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(ct, comp).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				Labels: map[string]string{
					"openchoreo.dev/project":   "my-proj",
					"openchoreo.dev/component": "my-comp",
				},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "not-allowed-wf"},
			},
		}

		result := r.validateComponentWorkflowRun(context.Background(), wfr)
		if !result.shouldReturn {
			t.Error("expected shouldReturn=true")
		}
		cond := findConditionByType(wfr.Status.Conditions, string(ConditionWorkflowCompleted))
		if cond == nil || !strings.Contains(cond.Message, "not allowed") {
			t.Error("expected condition message to mention 'not allowed'")
		}
	})

	t.Run("workflow does not match component configured workflow fails", func(t *testing.T) {
		ct := &openchoreodevv1alpha1.ComponentType{
			ObjectMeta: metav1.ObjectMeta{Name: "my-ct", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				AllowedWorkflows: []openchoreodevv1alpha1.WorkflowRef{
					{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "wf-a"},
					{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "wf-b"},
				},
				Resources: []openchoreodevv1alpha1.ResourceTemplate{
					{ID: "deployment", Template: &runtime.RawExtension{Raw: []byte("{}")}},
				},
			},
		}
		comp := &openchoreodevv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentSpec{
				Owner:         openchoreodevv1alpha1.ComponentOwner{ProjectName: "my-proj"},
				ComponentType: openchoreodevv1alpha1.ComponentTypeRef{Name: "deployment/my-ct"},
				Workflow: &openchoreodevv1alpha1.ComponentWorkflowConfig{
					Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow,
					Name: "wf-a",
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(ct, comp).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				Labels: map[string]string{
					"openchoreo.dev/project":   "my-proj",
					"openchoreo.dev/component": "my-comp",
				},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "wf-b"},
			},
		}

		result := r.validateComponentWorkflowRun(context.Background(), wfr)
		if !result.shouldReturn {
			t.Error("expected shouldReturn=true")
		}
		cond := findConditionByType(wfr.Status.Conditions, string(ConditionWorkflowCompleted))
		if cond == nil || !strings.Contains(cond.Message, "configured with workflow") {
			t.Error("expected condition message to mention workflow mismatch")
		}
	})

	t.Run("valid component workflow run passes", func(t *testing.T) {
		ct := &openchoreodevv1alpha1.ComponentType{
			ObjectMeta: metav1.ObjectMeta{Name: "my-ct", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				AllowedWorkflows: []openchoreodevv1alpha1.WorkflowRef{
					{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "my-wf"},
				},
				Resources: []openchoreodevv1alpha1.ResourceTemplate{
					{ID: "deployment", Template: &runtime.RawExtension{Raw: []byte("{}")}},
				},
			},
		}
		comp := &openchoreodevv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentSpec{
				Owner:         openchoreodevv1alpha1.ComponentOwner{ProjectName: "my-proj"},
				ComponentType: openchoreodevv1alpha1.ComponentTypeRef{Name: "deployment/my-ct"},
				Workflow: &openchoreodevv1alpha1.ComponentWorkflowConfig{
					Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow,
					Name: "my-wf",
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(ct, comp).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				Labels: map[string]string{
					"openchoreo.dev/project":   "my-proj",
					"openchoreo.dev/component": "my-comp",
				},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "my-wf"},
			},
		}

		result := r.validateComponentWorkflowRun(context.Background(), wfr)
		if result.shouldReturn {
			t.Error("expected shouldReturn=false for valid workflow run")
		}
	})

	t.Run("empty allowedWorkflows rejects any workflow", func(t *testing.T) {
		ct := &openchoreodevv1alpha1.ComponentType{
			ObjectMeta: metav1.ObjectMeta{Name: "my-ct", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				Resources: []openchoreodevv1alpha1.ResourceTemplate{
					{ID: "deployment", Template: &runtime.RawExtension{Raw: []byte("{}")}},
				},
			},
		}
		comp := &openchoreodevv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentSpec{
				Owner:         openchoreodevv1alpha1.ComponentOwner{ProjectName: "my-proj"},
				ComponentType: openchoreodevv1alpha1.ComponentTypeRef{Name: "deployment/my-ct"},
				Workflow: &openchoreodevv1alpha1.ComponentWorkflowConfig{
					Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow,
					Name: "my-wf",
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(ct, comp).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				Labels: map[string]string{
					"openchoreo.dev/project":   "my-proj",
					"openchoreo.dev/component": "my-comp",
				},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "my-wf"},
			},
		}

		result := r.validateComponentWorkflowRun(context.Background(), wfr)
		if !result.shouldReturn {
			t.Error("expected shouldReturn=true")
		}
		cond := findConditionByType(wfr.Status.Conditions, string(ConditionWorkflowCompleted))
		if cond == nil || !strings.Contains(cond.Message, "no workflows are allowed") {
			t.Error("expected condition message about no workflows allowed")
		}
	})

	t.Run("component with nil workflow skips workflow match check", func(t *testing.T) {
		ct := &openchoreodevv1alpha1.ComponentType{
			ObjectMeta: metav1.ObjectMeta{Name: "my-ct", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				AllowedWorkflows: []openchoreodevv1alpha1.WorkflowRef{
					{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "my-wf"},
				},
				Resources: []openchoreodevv1alpha1.ResourceTemplate{
					{ID: "deployment", Template: &runtime.RawExtension{Raw: []byte("{}")}},
				},
			},
		}
		comp := &openchoreodevv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentSpec{
				Owner:         openchoreodevv1alpha1.ComponentOwner{ProjectName: "my-proj"},
				ComponentType: openchoreodevv1alpha1.ComponentTypeRef{Name: "deployment/my-ct"},
				// Workflow is nil
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(ct, comp).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				Labels: map[string]string{
					"openchoreo.dev/project":   "my-proj",
					"openchoreo.dev/component": "my-comp",
				},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "my-wf"},
			},
		}

		result := r.validateComponentWorkflowRun(context.Background(), wfr)
		if result.shouldReturn {
			t.Error("expected shouldReturn=false when component has no workflow set")
		}
	})
}

func TestFormatAllowedWorkflows(t *testing.T) {
	t.Run("formats workflow refs", func(t *testing.T) {
		refs := []openchoreodevv1alpha1.WorkflowRef{
			{Kind: "Workflow", Name: "wf-a"},
			{Kind: "ClusterWorkflow", Name: "cwf-b"},
			{Kind: "ClusterWorkflow", Name: "wf-c"},
		}
		result := formatAllowedWorkflows(refs)
		if result != "[Workflow/wf-a, ClusterWorkflow/cwf-b, ClusterWorkflow/wf-c]" {
			t.Errorf("unexpected format: %s", result)
		}
	})
}

func TestIsWorkflowRunning(t *testing.T) {
	t.Run("false when no running condition", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
		setWorkflowPendingCondition(wfr)
		if isWorkflowRunning(wfr) {
			t.Error("expected false")
		}
	})
	t.Run("true when running condition is set", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
		setWorkflowRunningCondition(wfr)
		if !isWorkflowRunning(wfr) {
			t.Error("expected true")
		}
	})
}

func TestValidationSkippedWhenRunReferenceSet(t *testing.T) {
	// The Reconcile guard skips validation when RunReference is set.
	// Verify that a WorkflowRun with RunReference set and component labels
	// pointing to a non-existent component would fail validation if called,
	// confirming the guard is load-bearing.
	scheme := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{Client: fakeClient, Scheme: scheme}

	wfr := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-wfr",
			Namespace: "default",
			Labels: map[string]string{
				"openchoreo.dev/project":   "my-proj",
				"openchoreo.dev/component": "missing-comp",
			},
		},
		Spec: openchoreodevv1alpha1.WorkflowRunSpec{
			Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "my-wf"},
		},
		Status: openchoreodevv1alpha1.WorkflowRunStatus{
			RunReference: &openchoreodevv1alpha1.ResourceReference{
				Name:      "submitted-run",
				Namespace: "build-ns",
			},
		},
	}

	// The guard condition: validation should be skipped when RunReference is set
	shouldSkipValidation := isWorkflowRunning(wfr) || wfr.Status.RunReference != nil
	if !shouldSkipValidation {
		t.Fatal("expected validation to be skipped when RunReference is set")
	}

	// Confirm validation would have returned shouldReturn=true (failure) if called
	result := r.validateComponentWorkflowRun(context.Background(), wfr)
	if !result.shouldReturn {
		t.Error("expected validation to fail for missing component, confirming the guard is needed")
	}
}

func TestSetComponentValidationFailedCondition(t *testing.T) {
	wfr := &openchoreodevv1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
	setComponentValidationFailedCondition(wfr, "test message")
	assertConditionCount(t, wfr, 1)
	assertCondition(t, wfr, string(ConditionWorkflowCompleted), metav1.ConditionTrue, string(ReasonComponentValidationFailed))
	cond := findConditionByType(wfr.Status.Conditions, string(ConditionWorkflowCompleted))
	if cond.Message != "test message" {
		t.Errorf("expected message 'test message', got %q", cond.Message)
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func findConditionByType(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

func assertConditionCount(t *testing.T, wfr *openchoreodevv1alpha1.WorkflowRun, expected int) {
	t.Helper()
	if len(wfr.Status.Conditions) != expected {
		t.Errorf("expected %d conditions, got %d", expected, len(wfr.Status.Conditions))
	}
}

func assertCondition(t *testing.T, wfr *openchoreodevv1alpha1.WorkflowRun, condType string, status metav1.ConditionStatus, reason string) {
	t.Helper()
	cond := findConditionByType(wfr.Status.Conditions, condType)
	if cond == nil {
		t.Errorf("condition %q not found", condType)
		return
	}
	if cond.Status != status {
		t.Errorf("condition %q: expected status %s, got %s", condType, status, cond.Status)
	}
	if reason != "" && cond.Reason != reason {
		t.Errorf("condition %q: expected reason %q, got %q", condType, reason, cond.Reason)
	}
}
