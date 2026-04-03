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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	workflowpipeline "github.com/openchoreo/openchoreo/internal/pipeline/workflow"
)

const (
	testBuildNS             = "build-ns"
	testRunResourceName     = "wf-run-1"
	testManagedByLabelValue = "workflowrun-controller"
)

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

	t.Run("component type not found fails permanently", func(t *testing.T) {
		// Component exists but references a ComponentType that doesn't exist
		comp := &openchoreodevv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentSpec{
				Owner:         openchoreodevv1alpha1.ComponentOwner{ProjectName: "my-proj"},
				ComponentType: openchoreodevv1alpha1.ComponentTypeRef{Name: "deployment/missing-ct"},
				Workflow: &openchoreodevv1alpha1.ComponentWorkflowConfig{
					Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow,
					Name: "my-wf",
				},
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
		if result.result.Requeue {
			t.Error("expected no requeue for permanent failure")
		}
		cond := findConditionByType(wfr.Status.Conditions, string(ConditionWorkflowCompleted))
		if cond == nil || !strings.Contains(cond.Message, "not found") {
			t.Error("expected condition message to mention 'not found'")
		}
	})

	t.Run("resolveComponentType error requeues", func(t *testing.T) {
		// Component with invalid ComponentType name format causes resolveComponentType error
		comp := &openchoreodevv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentSpec{
				Owner:         openchoreodevv1alpha1.ComponentOwner{ProjectName: "my-proj"},
				ComponentType: openchoreodevv1alpha1.ComponentTypeRef{Name: "invalid-no-slash"},
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
		if !result.result.Requeue {
			t.Error("expected Requeue=true for transient resolveComponentType error")
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

// ---------------------------------------------------------------------------
// finalize
// ---------------------------------------------------------------------------

func TestFinalize(t *testing.T) {
	s := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(s)

	t.Run("no-op when finalizer not present", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wfr", Namespace: "default"},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
			},
		}
		fc := fake.NewClientBuilder().WithScheme(s).WithObjects(wfr).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		result, err := r.finalize(context.Background(), wfr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != (ctrl.Result{}) {
			t.Errorf("expected empty result, got %v", result)
		}
	})

	t.Run("removes finalizer when workflow not found", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-wfr",
				Namespace:  "default",
				Finalizers: []string{WorkflowRunCleanupFinalizer},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "missing-wf"},
			},
		}
		fc := fake.NewClientBuilder().WithScheme(s).WithObjects(wfr).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		result, err := r.finalize(context.Background(), wfr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != (ctrl.Result{}) {
			t.Errorf("expected empty result, got %v", result)
		}
		if controllerutil.ContainsFinalizer(wfr, WorkflowRunCleanupFinalizer) {
			t.Error("expected finalizer to be removed")
		}
	})

	t.Run("removes finalizer when workflow exists but no workflow plane", func(t *testing.T) {
		cwf := &openchoreodevv1alpha1.ClusterWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
			Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
				RunTemplate: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow"}`)},
			},
		}
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-wfr",
				Namespace:  "default",
				Finalizers: []string{WorkflowRunCleanupFinalizer},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-wf"},
			},
		}
		fc := fake.NewClientBuilder().WithScheme(s).WithObjects(cwf, wfr).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		result, err := r.finalize(context.Background(), wfr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != (ctrl.Result{}) {
			t.Errorf("expected empty result, got %v", result)
		}
		if controllerutil.ContainsFinalizer(wfr, WorkflowRunCleanupFinalizer) {
			t.Error("expected finalizer to be removed")
		}
	})

	t.Run("removes finalizer when namespace-scoped workflow exists but no workflow plane", func(t *testing.T) {
		wf := &openchoreodevv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wf", Namespace: "default"},
			Spec: openchoreodevv1alpha1.WorkflowSpec{
				RunTemplate: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow"}`)},
				WorkflowPlaneRef: &openchoreodevv1alpha1.WorkflowPlaneRef{
					Kind: "ClusterWorkflowPlane",
					Name: "default",
				},
			},
		}
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-wfr",
				Namespace:  "default",
				Finalizers: []string{WorkflowRunCleanupFinalizer},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{
					Kind: openchoreodevv1alpha1.WorkflowRefKindWorkflow,
					Name: "test-wf",
				},
			},
		}
		fc := fake.NewClientBuilder().WithScheme(s).WithObjects(wf, wfr).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		result, err := r.finalize(context.Background(), wfr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != (ctrl.Result{}) {
			t.Errorf("expected empty result, got %v", result)
		}
		if controllerutil.ContainsFinalizer(wfr, WorkflowRunCleanupFinalizer) {
			t.Error("expected finalizer to be removed")
		}
	})
}

// ---------------------------------------------------------------------------
// Reconcile — unit-level paths using fake client
// ---------------------------------------------------------------------------

func TestReconcileNotFound(t *testing.T) {
	s := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(s)

	fc := fake.NewClientBuilder().WithScheme(s).Build()
	r := &Reconciler{Client: fc, Scheme: s}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != (ctrl.Result{}) {
		t.Errorf("expected empty result for not found, got %v", result)
	}
}

func TestReconcileCompletedWorkflowSetsCompletedAt(t *testing.T) {
	s := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(s)

	wfr := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "completed-wfr",
			Namespace:  "default",
			Finalizers: []string{WorkflowRunCleanupFinalizer},
			Generation: 1,
		},
		Spec: openchoreodevv1alpha1.WorkflowRunSpec{
			Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).
		WithObjects(wfr).
		WithStatusSubresource(wfr).
		Build()

	// Set the completed condition via status update
	setWorkflowSucceededCondition(wfr)
	if err := fc.Status().Update(context.Background(), wfr); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	r := &Reconciler{Client: fc, Scheme: s}
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "completed-wfr", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != (ctrl.Result{}) {
		t.Errorf("expected empty result for completed workflow, got %v", result)
	}

	// Verify CompletedAt was set
	got := &openchoreodevv1alpha1.WorkflowRun{}
	if err := fc.Get(context.Background(), types.NamespacedName{Name: "completed-wfr", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get WorkflowRun: %v", err)
	}
	if got.Status.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestReconcileUninitiated(t *testing.T) {
	s := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(s)

	wfr := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "uninit-wfr",
			Namespace:  "default",
			Finalizers: []string{WorkflowRunCleanupFinalizer},
		},
		Spec: openchoreodevv1alpha1.WorkflowRunSpec{
			Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).
		WithObjects(wfr).
		WithStatusSubresource(wfr).
		Build()
	r := &Reconciler{Client: fc, Scheme: s}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "uninit-wfr", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Requeue {
		t.Error("expected Requeue=true for uninitiated workflow")
	}

	got := &openchoreodevv1alpha1.WorkflowRun{}
	if err := fc.Get(context.Background(), types.NamespacedName{Name: "uninit-wfr", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get WorkflowRun: %v", err)
	}
	if got.Status.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}
	cond := findConditionByType(got.Status.Conditions, string(ConditionWorkflowCompleted))
	if cond == nil {
		t.Fatal("expected WorkflowCompleted condition")
	}
	if cond.Reason != string(ReasonWorkflowPending) {
		t.Errorf("expected reason WorkflowPending, got %s", cond.Reason)
	}
}

func TestReconcileWorkflowNotFound(t *testing.T) {
	s := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(s)

	wfr := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "wf-notfound-wfr",
			Namespace:  "default",
			Finalizers: []string{WorkflowRunCleanupFinalizer},
			Generation: 1,
		},
		Spec: openchoreodevv1alpha1.WorkflowRunSpec{
			Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "missing-wf"},
		},
	}
	// Pre-set the pending condition so we get past the initiation check
	setWorkflowPendingCondition(wfr)

	fc := fake.NewClientBuilder().WithScheme(s).
		WithObjects(wfr).
		WithStatusSubresource(wfr).
		Build()
	r := &Reconciler{Client: fc, Scheme: s}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "wf-notfound-wfr", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Requeue || result.RequeueAfter > 0 {
		t.Error("expected no requeue for workflow not found")
	}

	got := &openchoreodevv1alpha1.WorkflowRun{}
	if err := fc.Get(context.Background(), types.NamespacedName{Name: "wf-notfound-wfr", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get WorkflowRun: %v", err)
	}
	cond := findConditionByType(got.Status.Conditions, string(ConditionWorkflowCompleted))
	if cond == nil {
		t.Fatal("expected WorkflowCompleted condition")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected ConditionTrue, got %s", cond.Status)
	}
	if cond.Reason != string(ReasonWorkflowFailed) {
		t.Errorf("expected reason WorkflowFailed, got %s", cond.Reason)
	}
}

func TestReconcileWorkflowPlaneNotFound(t *testing.T) {
	s := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(s)

	cwf := &openchoreodevv1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
			RunTemplate: &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"test","namespace":"default"},"spec":{"entrypoint":"main"}}`),
			},
		},
	}
	wfr := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "wp-notfound-wfr",
			Namespace:  "default",
			Finalizers: []string{WorkflowRunCleanupFinalizer},
			Generation: 1,
		},
		Spec: openchoreodevv1alpha1.WorkflowRunSpec{
			Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-wf"},
		},
	}
	setWorkflowPendingCondition(wfr)

	fc := fake.NewClientBuilder().WithScheme(s).
		WithObjects(cwf, wfr).
		WithStatusSubresource(wfr).
		Build()
	r := &Reconciler{Client: fc, Scheme: s}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "wp-notfound-wfr", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != 1*time.Minute {
		t.Errorf("expected RequeueAfter=1m, got %v", result.RequeueAfter)
	}

	got := &openchoreodevv1alpha1.WorkflowRun{}
	if err := fc.Get(context.Background(), types.NamespacedName{Name: "wp-notfound-wfr", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get WorkflowRun: %v", err)
	}
	cond := findConditionByType(got.Status.Conditions, string(ConditionWorkflowCompleted))
	if cond == nil {
		t.Fatal("expected WorkflowCompleted condition")
	}
	if cond.Reason != string(ReasonWorkflowPlaneNotFound) {
		t.Errorf("expected reason WorkflowPlaneNotFound, got %s", cond.Reason)
	}
}

// ---------------------------------------------------------------------------
// Deep Reconcile paths using pre-seeded KubeMultiClientManager
// ---------------------------------------------------------------------------

// newReconcilerWithWorkflowPlane creates a Reconciler with a fake control-plane client containing
// a ClusterWorkflow and ClusterWorkflowPlane, and a KubeMultiClientManager pre-seeded with
// wpClient as the workflow plane client. This allows testing the full Reconcile path past
// ResolveWorkflowPlane without real cluster infrastructure.
func newReconcilerWithWorkflowPlane(
	t *testing.T,
	s *runtime.Scheme,
	cpObjects []runtime.Object,
	wpClient *fake.ClientBuilder,
) (*Reconciler, *openchoreodevv1alpha1.ClusterWorkflowPlane) {
	t.Helper()
	cwp := &openchoreodevv1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
	}

	cpBuilder := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(cpObjects...).WithObjects(cwp)
	fc := cpBuilder.Build()

	mgr := kubernetesClient.NewManager()
	// Pre-seed the client cache so getWorkflowPlaneClient returns our fake wpClient
	// Key format: v2/clusterworkflowplane/{planeID}/{name} — planeID defaults to name
	_, err := mgr.GetOrAddClient("v2/clusterworkflowplane/default/default", func() (client.Client, error) {
		return wpClient.Build(), nil
	})
	if err != nil {
		t.Fatalf("failed to seed client manager: %v", err)
	}

	return &Reconciler{
		Client:       fc,
		Scheme:       s,
		K8sClientMgr: mgr,
		Pipeline:     workflowpipeline.NewPipeline(),
		GatewayURL:   "https://gateway.test:443",
	}, cwp
}

func TestReconcileTTLCopyFromWorkflow(t *testing.T) {
	s := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(s)

	cwf := &openchoreodevv1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
			TTLAfterCompletion: "2h",
			RunTemplate: &runtime.RawExtension{Raw: []byte(`{
				"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow",
				"metadata":{"name":"${metadata.workflowRunName}","namespace":"${metadata.namespace}"},
				"spec":{"entrypoint":"main","serviceAccountName":"wf-sa"}
			}`)},
		},
	}

	wfr := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "ttl-copy-wfr",
			Namespace:  "default",
			Finalizers: []string{WorkflowRunCleanupFinalizer},
			Generation: 1,
		},
		Spec: openchoreodevv1alpha1.WorkflowRunSpec{
			Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "test-wf"},
		},
	}
	setWorkflowPendingCondition(wfr)

	wpFake := fake.NewClientBuilder().WithScheme(s)
	r, _ := newReconcilerWithWorkflowPlane(t, s, []runtime.Object{cwf, wfr}, wpFake)

	// Enable status subresource on the control-plane client
	cpClient := fake.NewClientBuilder().WithScheme(s).
		WithObjects(cwf, wfr).
		WithStatusSubresource(wfr).
		Build()
	r.Client = cpClient

	// Create ClusterWorkflowPlane in the updated client
	cwp := &openchoreodevv1alpha1.ClusterWorkflowPlane{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	if err := cpClient.Create(context.Background(), cwp); err != nil {
		t.Fatalf("failed to create ClusterWorkflowPlane: %v", err)
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "ttl-copy-wfr", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Requeue {
		t.Error("expected Requeue=true after TTL copy")
	}

	// Verify TTL was copied from workflow to workflowrun
	got := &openchoreodevv1alpha1.WorkflowRun{}
	if err := cpClient.Get(context.Background(), types.NamespacedName{Name: "ttl-copy-wfr", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get WorkflowRun: %v", err)
	}
	if got.Spec.TTLAfterCompletion != "2h" {
		t.Errorf("expected TTL=2h, got %s", got.Spec.TTLAfterCompletion)
	}
}

func TestReconcileFullRenderAndApply(t *testing.T) {
	s := newTestScheme()

	cwf := &openchoreodevv1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "build-wf"},
		Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
			RunTemplate: &runtime.RawExtension{Raw: []byte(`{
				"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow",
				"metadata":{"name":"${metadata.workflowRunName}","namespace":"${metadata.namespace}"},
				"spec":{"entrypoint":"main","serviceAccountName":"wf-sa",
					"templates":[{"name":"main","container":{"image":"alpine","command":["echo","hello"]}}]}
			}`)},
		},
	}

	wfr := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "render-wfr",
			Namespace:  "default",
			Finalizers: []string{WorkflowRunCleanupFinalizer},
			Generation: 1,
			UID:        "test-uid",
		},
		Spec: openchoreodevv1alpha1.WorkflowRunSpec{
			Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "build-wf"},
		},
	}
	setWorkflowPendingCondition(wfr)

	cwp := &openchoreodevv1alpha1.ClusterWorkflowPlane{ObjectMeta: metav1.ObjectMeta{Name: "default"}}

	cpClient := fake.NewClientBuilder().WithScheme(s).
		WithObjects(cwf, wfr, cwp).
		WithStatusSubresource(wfr).
		Build()

	wpClient := fake.NewClientBuilder().WithScheme(s).Build()

	mgr := kubernetesClient.NewManager()
	_, _ = mgr.GetOrAddClient("v2/clusterworkflowplane/default/default", func() (client.Client, error) {
		return wpClient, nil
	})

	r := &Reconciler{
		Client:       cpClient,
		Scheme:       s,
		K8sClientMgr: mgr,
		Pipeline:     workflowpipeline.NewPipeline(),
		GatewayURL:   "https://gateway.test:443",
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "render-wfr", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Requeue {
		t.Error("expected Requeue=true after successful render and apply")
	}

	// Verify RunReference was set in status
	got := &openchoreodevv1alpha1.WorkflowRun{}
	if err := cpClient.Get(context.Background(), types.NamespacedName{Name: "render-wfr", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get WorkflowRun: %v", err)
	}
	if got.Status.RunReference == nil {
		t.Fatal("expected RunReference to be set after render and apply")
	}
	if got.Status.RunReference.Kind != "Workflow" {
		t.Errorf("expected RunReference kind Workflow, got %s", got.Status.RunReference.Kind)
	}

	// Verify the rendered resource was created in the workflow plane
	rendered := &unstructured.Unstructured{}
	rendered.SetGroupVersionKind(schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Workflow"})
	if err := wpClient.Get(context.Background(), types.NamespacedName{
		Name:      "render-wfr",
		Namespace: "workflows-default",
	}, rendered); err != nil {
		t.Fatalf("expected rendered workflow to exist in workflow plane: %v", err)
	}

	// Verify prerequisites were created in workflow plane
	ns := &corev1.Namespace{}
	if err := wpClient.Get(context.Background(), types.NamespacedName{Name: "workflows-default"}, ns); err != nil {
		t.Errorf("expected namespace to be created in workflow plane: %v", err)
	}
	sa := &corev1.ServiceAccount{}
	if err := wpClient.Get(context.Background(), types.NamespacedName{Name: "wf-sa", Namespace: "workflows-default"}, sa); err != nil {
		t.Errorf("expected service account to be created in workflow plane: %v", err)
	}
}

func TestReconcileSyncsRunningWorkflow(t *testing.T) {
	s := newTestScheme()

	cwf := &openchoreodevv1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "build-wf"},
		Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
			RunTemplate: &runtime.RawExtension{Raw: []byte(`{
				"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow",
				"metadata":{"name":"${metadata.workflowRunName}","namespace":"${metadata.namespace}"},
				"spec":{"entrypoint":"main","serviceAccountName":"wf-sa"}
			}`)},
		},
	}

	wfr := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "syncing-wfr",
			Namespace:  "default",
			Finalizers: []string{WorkflowRunCleanupFinalizer},
			Generation: 1,
		},
		Spec: openchoreodevv1alpha1.WorkflowRunSpec{
			Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "build-wf"},
		},
		Status: openchoreodevv1alpha1.WorkflowRunStatus{
			RunReference: &openchoreodevv1alpha1.ResourceReference{
				APIVersion: "argoproj.io/v1alpha1",
				Kind:       "Workflow",
				Name:       "syncing-wfr",
				Namespace:  "workflows-default",
			},
		},
	}
	setWorkflowPendingCondition(wfr)

	cwp := &openchoreodevv1alpha1.ClusterWorkflowPlane{ObjectMeta: metav1.ObjectMeta{Name: "default"}}

	cpClient := fake.NewClientBuilder().WithScheme(s).
		WithObjects(cwf, wfr, cwp).
		WithStatusSubresource(wfr).
		Build()

	// Create a running Argo Workflow in the workflow plane.
	// The wpScheme must include the argoproj types so the fake client can
	// roundtrip the typed Workflow object (including status.phase).
	wpScheme := runtime.NewScheme()
	_ = argoproj.AddToScheme(wpScheme)
	_ = corev1.AddToScheme(wpScheme)
	_ = rbacv1.AddToScheme(wpScheme)

	argoWf := &argoproj.Workflow{}
	argoWf.SetName("syncing-wfr")
	argoWf.SetNamespace("workflows-default")
	argoWf.Status.Phase = argoproj.WorkflowRunning

	wpClient := fake.NewClientBuilder().WithScheme(wpScheme).
		WithObjects(argoWf).
		WithStatusSubresource(argoWf).
		Build()
	// Update status after creation since fake client may not persist status on Create
	argoWf.Status.Phase = argoproj.WorkflowRunning
	if err := wpClient.Status().Update(context.Background(), argoWf); err != nil {
		t.Fatalf("failed to update argo workflow status: %v", err)
	}

	mgr := kubernetesClient.NewManager()
	_, _ = mgr.GetOrAddClient("v2/clusterworkflowplane/default/default", func() (client.Client, error) {
		return wpClient, nil
	})

	r := &Reconciler{
		Client:       cpClient,
		Scheme:       s,
		K8sClientMgr: mgr,
		Pipeline:     workflowpipeline.NewPipeline(),
		GatewayURL:   "https://gateway.test:443",
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "syncing-wfr", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Running workflows requeue after 20 seconds
	if result.RequeueAfter != 20*time.Second {
		t.Errorf("expected RequeueAfter=20s for running workflow, got %v", result.RequeueAfter)
	}

	// Verify the running condition was set
	got := &openchoreodevv1alpha1.WorkflowRun{}
	if err := cpClient.Get(context.Background(), types.NamespacedName{Name: "syncing-wfr", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get WorkflowRun: %v", err)
	}
	cond := findConditionByType(got.Status.Conditions, string(ConditionWorkflowRunning))
	if cond == nil {
		t.Fatal("expected WorkflowRunning condition to be set")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected WorkflowRunning=True, got %s", cond.Status)
	}
}

// ---------------------------------------------------------------------------
// finalize — full cleanup path (workflow plane exists, resources in status)
// ---------------------------------------------------------------------------

func TestFinalizeWithResourceCleanup(t *testing.T) {
	s := newTestScheme()
	_ = argoproj.AddToScheme(s)

	cwf := &openchoreodevv1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "cleanup-wf"},
		Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
			RunTemplate: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow"}`)},
		},
	}
	cwp := &openchoreodevv1alpha1.ClusterWorkflowPlane{ObjectMeta: metav1.ObjectMeta{Name: "default"}}

	wfr := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cleanup-wfr",
			Namespace:  "default",
			Finalizers: []string{WorkflowRunCleanupFinalizer},
		},
		Spec: openchoreodevv1alpha1.WorkflowRunSpec{
			Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "cleanup-wf"},
		},
		Status: openchoreodevv1alpha1.WorkflowRunStatus{
			RunReference: &openchoreodevv1alpha1.ResourceReference{
				APIVersion: "argoproj.io/v1alpha1",
				Kind:       "Workflow",
				Name:       "run-to-delete",
				Namespace:  "workflows-default",
			},
			Resources: &[]openchoreodevv1alpha1.ResourceReference{
				{APIVersion: "v1", Kind: "Secret", Name: "secret-to-delete", Namespace: "workflows-default"},
			},
		},
	}

	cpClient := fake.NewClientBuilder().WithScheme(s).WithObjects(cwf, cwp, wfr).Build()

	// Create the resources that should be deleted in the workflow plane
	wpSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret-to-delete", Namespace: "workflows-default"}}
	wpArgoWf := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Workflow",
		"metadata":   map[string]any{"name": "run-to-delete", "namespace": "workflows-default"},
	}}
	wpClient := fake.NewClientBuilder().WithScheme(s).WithObjects(wpSecret, wpArgoWf).Build()

	mgr := kubernetesClient.NewManager()
	_, _ = mgr.GetOrAddClient("v2/clusterworkflowplane/default/default", func() (client.Client, error) {
		return wpClient, nil
	})

	r := &Reconciler{
		Client:       cpClient,
		Scheme:       s,
		K8sClientMgr: mgr,
		GatewayURL:   "https://gateway.test:443",
	}

	result, err := r.finalize(context.Background(), wfr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != (ctrl.Result{}) {
		t.Errorf("expected empty result, got %v", result)
	}

	// Verify finalizer was removed
	if controllerutil.ContainsFinalizer(wfr, WorkflowRunCleanupFinalizer) {
		t.Error("expected finalizer to be removed")
	}

	// Verify the Secret was deleted from workflow plane
	err = wpClient.Get(context.Background(), types.NamespacedName{Name: "secret-to-delete", Namespace: "workflows-default"}, &corev1.Secret{})
	if err == nil {
		t.Error("expected secret to be deleted from workflow plane")
	}

	// Verify the run resource was deleted from workflow plane
	runObj := &unstructured.Unstructured{}
	runObj.SetGroupVersionKind(schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Workflow"})
	err = wpClient.Get(context.Background(), types.NamespacedName{Name: "run-to-delete", Namespace: "workflows-default"}, runObj)
	if err == nil {
		t.Error("expected run resource to be deleted from workflow plane")
	}
}

// ---------------------------------------------------------------------------
// Reconcile — transient workflow resolution failure
// ---------------------------------------------------------------------------

func TestReconcileWorkflowResolutionTransientFailure(t *testing.T) {
	s := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(s)

	// Create a ClusterWorkflow but with a WorkflowPlaneRef pointing to a non-existent
	// plane kind, which causes ResolveWorkflowPlane to return a non-IsNotFound error
	cwf := &openchoreodevv1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "transient-wf"},
		Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
			RunTemplate: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"test","namespace":"default"},"spec":{"entrypoint":"main"}}`)},
			WorkflowPlaneRef: &openchoreodevv1alpha1.ClusterWorkflowPlaneRef{
				Kind: "InvalidKind",
				Name: "default",
			},
		},
	}

	wfr := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "transient-wfr",
			Namespace:  "default",
			Finalizers: []string{WorkflowRunCleanupFinalizer},
			Generation: 1,
		},
		Spec: openchoreodevv1alpha1.WorkflowRunSpec{
			Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "transient-wf"},
		},
	}
	setWorkflowPendingCondition(wfr)

	fc := fake.NewClientBuilder().WithScheme(s).
		WithObjects(cwf, wfr).
		WithStatusSubresource(wfr).
		Build()
	r := &Reconciler{Client: fc, Scheme: s}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "transient-wfr", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should requeue after 30s for transient failure
	if result.RequeueAfter != 30*time.Second {
		t.Errorf("expected RequeueAfter=30s, got %v", result.RequeueAfter)
	}

	got := &openchoreodevv1alpha1.WorkflowRun{}
	if err := fc.Get(context.Background(), types.NamespacedName{Name: "transient-wfr", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get WorkflowRun: %v", err)
	}
	cond := findConditionByType(got.Status.Conditions, string(ConditionWorkflowCompleted))
	if cond == nil {
		t.Fatal("expected WorkflowCompleted condition")
	}
	if cond.Reason != string(ReasonWorkflowPlaneResolutionFailed) {
		t.Errorf("expected reason WorkflowPlaneResolutionFailed, got %s", cond.Reason)
	}
}

// ---------------------------------------------------------------------------
// Reconcile — RunReference exists but workflow not found in workflow plane
// ---------------------------------------------------------------------------

func TestReconcileRunReferenceWorkflowNotFound(t *testing.T) {
	s := newTestScheme()

	cwf := &openchoreodevv1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "ref-wf"},
		Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
			RunTemplate: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"test","namespace":"default"},"spec":{"entrypoint":"main"}}`)},
		},
	}
	cwp := &openchoreodevv1alpha1.ClusterWorkflowPlane{ObjectMeta: metav1.ObjectMeta{Name: "default"}}

	wfr := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "ref-notfound-wfr",
			Namespace:  "default",
			Finalizers: []string{WorkflowRunCleanupFinalizer},
			Generation: 1,
		},
		Spec: openchoreodevv1alpha1.WorkflowRunSpec{
			Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "ref-wf"},
		},
		Status: openchoreodevv1alpha1.WorkflowRunStatus{
			RunReference: &openchoreodevv1alpha1.ResourceReference{
				APIVersion: "argoproj.io/v1alpha1",
				Kind:       "Workflow",
				Name:       "deleted-run",
				Namespace:  "workflows-default",
			},
		},
	}
	setWorkflowPendingCondition(wfr)

	cpClient := fake.NewClientBuilder().WithScheme(s).
		WithObjects(cwf, cwp, wfr).
		WithStatusSubresource(wfr).
		Build()

	// Empty workflow plane — the referenced run doesn't exist.
	// Must include argoproj types so the fake client recognizes the Workflow GVK.
	wpScheme := newTestScheme()
	_ = argoproj.AddToScheme(wpScheme)
	wpClient := fake.NewClientBuilder().WithScheme(wpScheme).Build()

	mgr := kubernetesClient.NewManager()
	_, _ = mgr.GetOrAddClient("v2/clusterworkflowplane/default/default", func() (client.Client, error) {
		return wpClient, nil
	})

	r := &Reconciler{
		Client:       cpClient,
		Scheme:       s,
		K8sClientMgr: mgr,
		Pipeline:     workflowpipeline.NewPipeline(),
		GatewayURL:   "https://gateway.test:443",
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "ref-notfound-wfr", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Requeue || result.RequeueAfter > 0 {
		t.Error("expected no requeue when run reference is not found")
	}

	got := &openchoreodevv1alpha1.WorkflowRun{}
	if err := cpClient.Get(context.Background(), types.NamespacedName{Name: "ref-notfound-wfr", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get WorkflowRun: %v", err)
	}
	cond := findConditionByType(got.Status.Conditions, string(ConditionWorkflowCompleted))
	if cond == nil {
		t.Fatal("expected WorkflowCompleted condition")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected ConditionTrue, got %s", cond.Status)
	}
	if cond.Reason != string(ReasonWorkflowFailed) {
		t.Errorf("expected reason WorkflowFailed, got %s", cond.Reason)
	}
}

// ---------------------------------------------------------------------------
// ensureRunResource — error paths
// ---------------------------------------------------------------------------

func TestEnsureRunResourcePrerequisiteFailure(t *testing.T) {
	s := newTestScheme()

	wfr := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wfr", Namespace: "default", UID: "test-uid"},
	}

	output := &workflowpipeline.RenderOutput{
		Resource: map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Workflow",
			"metadata":   map[string]any{"name": "wf-run", "namespace": "build-ns"},
			"spec":       map[string]any{"entrypoint": "main", "serviceAccountName": "wf-sa"},
		},
		// Include a resource that will fail to apply (invalid GVK with no scheme registration)
		Resources: []workflowpipeline.RenderedResource{
			{
				ID: "bad-resource",
				Resource: map[string]any{
					"apiVersion": "nonexistent.io/v1",
					"kind":       "DoesNotExist",
					"metadata":   map[string]any{"name": "bad", "namespace": "build-ns"},
				},
			},
		},
	}

	// Use a scheme that deliberately lacks the custom resource type to force an error
	wpClient := fake.NewClientBuilder().WithScheme(s).Build()
	r := &Reconciler{Client: wpClient, Scheme: s}

	result := r.ensureRunResource(context.Background(), wfr, output, "build-ns", wpClient)
	if !result.Requeue {
		t.Error("expected Requeue=true when applyRenderedResources fails")
	}
}

// newTestScheme creates a scheme with all types needed for unit tests.
func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	return s
}

// ---------------------------------------------------------------------------
// resolveComponentType
// ---------------------------------------------------------------------------

func TestResolveComponentType(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(scheme)

	t.Run("resolves namespace-scoped ComponentType", func(t *testing.T) {
		ct := &openchoreodevv1alpha1.ComponentType{
			ObjectMeta: metav1.ObjectMeta{Name: "my-ct", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				AllowedWorkflows: []openchoreodevv1alpha1.WorkflowRef{
					{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "wf-a"},
				},
				Resources: []openchoreodevv1alpha1.ResourceTemplate{
					{ID: "deployment", Template: &runtime.RawExtension{Raw: []byte("{}")}},
				},
			},
		}
		comp := &openchoreodevv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentSpec{
				ComponentType: openchoreodevv1alpha1.ComponentTypeRef{Name: "deployment/my-ct"},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ct).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		result, err := r.resolveComponentType(context.Background(), comp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.Name != "my-ct" {
			t.Errorf("expected name my-ct, got %s", result.Name)
		}
		if len(result.Spec.AllowedWorkflows) != 1 {
			t.Errorf("expected 1 allowed workflow, got %d", len(result.Spec.AllowedWorkflows))
		}
	})

	t.Run("resolves ClusterComponentType", func(t *testing.T) {
		cct := &openchoreodevv1alpha1.ClusterComponentType{
			ObjectMeta: metav1.ObjectMeta{Name: "my-cct"},
			Spec: openchoreodevv1alpha1.ClusterComponentTypeSpec{
				WorkloadType: "deployment",
				AllowedWorkflows: []openchoreodevv1alpha1.ClusterWorkflowRef{
					{Kind: openchoreodevv1alpha1.ClusterWorkflowRefKindClusterWorkflow, Name: "cwf-a"},
				},
			},
		}
		comp := &openchoreodevv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentSpec{
				ComponentType: openchoreodevv1alpha1.ComponentTypeRef{
					Kind: openchoreodevv1alpha1.ComponentTypeRefKindClusterComponentType,
					Name: "deployment/my-cct",
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cct).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		result, err := r.resolveComponentType(context.Background(), comp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.Spec.WorkloadType != "deployment" {
			t.Errorf("expected workloadType deployment, got %s", result.Spec.WorkloadType)
		}
		if len(result.Spec.AllowedWorkflows) != 1 {
			t.Errorf("expected 1 allowed workflow, got %d", len(result.Spec.AllowedWorkflows))
		}
		if result.Spec.AllowedWorkflows[0].Name != "cwf-a" {
			t.Errorf("expected workflow name cwf-a, got %s", result.Spec.AllowedWorkflows[0].Name)
		}
	})

	t.Run("invalid format returns error", func(t *testing.T) {
		comp := &openchoreodevv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentSpec{
				ComponentType: openchoreodevv1alpha1.ComponentTypeRef{Name: "no-slash"},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		_, err := r.resolveComponentType(context.Background(), comp)
		if err == nil {
			t.Fatal("expected error for invalid format")
		}
		if !strings.Contains(err.Error(), "invalid componentType name format") {
			t.Errorf("expected format error, got: %v", err)
		}
	})

	t.Run("namespace-scoped ComponentType not found returns nil", func(t *testing.T) {
		comp := &openchoreodevv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentSpec{
				ComponentType: openchoreodevv1alpha1.ComponentTypeRef{Name: "deployment/missing-ct"},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		result, err := r.resolveComponentType(context.Background(), comp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Error("expected nil result for not found")
		}
	})

	t.Run("ClusterComponentType not found returns nil", func(t *testing.T) {
		comp := &openchoreodevv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "default"},
			Spec: openchoreodevv1alpha1.ComponentSpec{
				ComponentType: openchoreodevv1alpha1.ComponentTypeRef{
					Kind: openchoreodevv1alpha1.ComponentTypeRefKindClusterComponentType,
					Name: "deployment/missing-cct",
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &Reconciler{Client: fakeClient, Scheme: scheme}

		result, err := r.resolveComponentType(context.Background(), comp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Error("expected nil result for not found")
		}
	})
}

// ---------------------------------------------------------------------------
// setWorkflowResolutionFailedCondition
// ---------------------------------------------------------------------------

func TestSetWorkflowResolutionFailedCondition(t *testing.T) {
	wfr := &openchoreodevv1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
	setWorkflowResolutionFailedCondition(wfr, errors.New("connection refused"))
	assertConditionCount(t, wfr, 1)
	assertCondition(t, wfr, string(ConditionWorkflowCompleted), metav1.ConditionFalse, string(ReasonWorkflowResolutionFailed))
	cond := findConditionByType(wfr.Status.Conditions, string(ConditionWorkflowCompleted))
	if !strings.Contains(cond.Message, "connection refused") {
		t.Errorf("expected message to contain 'connection refused', got %q", cond.Message)
	}
}

// ---------------------------------------------------------------------------
// ensureResource
// ---------------------------------------------------------------------------

func TestEnsureResource(t *testing.T) {
	s := newTestScheme()

	t.Run("creates resource successfully", func(t *testing.T) {
		fc := fake.NewClientBuilder().WithScheme(s).Build()
		ns := makeNamespace("test-ns")
		err := ensureResource(context.Background(), fc, ns, "Namespace", ctrl.Log)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify resource was created
		got := &corev1.Namespace{}
		if err := fc.Get(context.Background(), types.NamespacedName{Name: "test-ns"}, got); err != nil {
			t.Fatalf("expected namespace to exist: %v", err)
		}
	})

	t.Run("no error when resource already exists", func(t *testing.T) {
		existing := makeNamespace("existing-ns")
		fc := fake.NewClientBuilder().WithScheme(s).WithObjects(existing).Build()

		ns := makeNamespace("existing-ns")
		err := ensureResource(context.Background(), fc, ns, "Namespace", ctrl.Log)
		if err != nil {
			t.Fatalf("expected no error for already existing resource, got: %v", err)
		}
	})

	t.Run("returns error when create fails with non-AlreadyExists error", func(t *testing.T) {
		// Use a minimal scheme that lacks the Namespace type to provoke a create error
		emptyScheme := runtime.NewScheme()
		fc := fake.NewClientBuilder().WithScheme(emptyScheme).Build()

		ns := makeNamespace("test-ns")
		err := ensureResource(context.Background(), fc, ns, "Namespace", ctrl.Log)
		if err == nil {
			t.Fatal("expected error when create fails")
		}
	})
}

// ---------------------------------------------------------------------------
// ensurePrerequisites
// ---------------------------------------------------------------------------

func TestEnsurePrerequisites(t *testing.T) {
	s := newTestScheme()

	t.Run("creates all prerequisite resources", func(t *testing.T) {
		fc := fake.NewClientBuilder().WithScheme(s).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		err := r.ensurePrerequisites(context.Background(), "workflow-ns", "wf-sa", fc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify namespace
		ns := &corev1.Namespace{}
		if err := fc.Get(context.Background(), types.NamespacedName{Name: "workflow-ns"}, ns); err != nil {
			t.Errorf("expected namespace to exist: %v", err)
		}

		// Verify service account
		sa := &corev1.ServiceAccount{}
		if err := fc.Get(context.Background(), types.NamespacedName{Name: "wf-sa", Namespace: "workflow-ns"}, sa); err != nil {
			t.Errorf("expected service account to exist: %v", err)
		}

		// Verify role
		role := &rbacv1.Role{}
		if err := fc.Get(context.Background(), types.NamespacedName{Name: "wf-sa-role", Namespace: "workflow-ns"}, role); err != nil {
			t.Errorf("expected role to exist: %v", err)
		}

		// Verify role binding
		rb := &rbacv1.RoleBinding{}
		if err := fc.Get(context.Background(), types.NamespacedName{Name: "wf-sa-role-binding", Namespace: "workflow-ns"}, rb); err != nil {
			t.Errorf("expected role binding to exist: %v", err)
		}
	})

	t.Run("succeeds when resources already exist", func(t *testing.T) {
		fc := fake.NewClientBuilder().WithScheme(s).
			WithObjects(
				makeNamespace("workflow-ns"),
				makeServiceAccount("workflow-ns", "wf-sa"),
				makeRole("workflow-ns", "wf-sa-role"),
				makeRoleBinding("workflow-ns", "wf-sa", "wf-sa-role", "wf-sa-role-binding"),
			).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		err := r.ensurePrerequisites(context.Background(), "workflow-ns", "wf-sa", fc)
		if err != nil {
			t.Fatalf("expected no error when resources already exist, got: %v", err)
		}
	})

	t.Run("returns error when resource creation fails", func(t *testing.T) {
		// Use a scheme that lacks Namespace type so the first resource fails
		noNSScheme := runtime.NewScheme()
		_ = rbacv1.AddToScheme(noNSScheme)
		fc := fake.NewClientBuilder().WithScheme(noNSScheme).Build()
		r := &Reconciler{Client: fc, Scheme: noNSScheme}

		err := r.ensurePrerequisites(context.Background(), "workflow-ns", "wf-sa", fc)
		if err == nil {
			t.Fatal("expected error when resource creation fails")
		}
		if !strings.Contains(err.Error(), "failed to ensure") {
			t.Errorf("expected 'failed to ensure' error, got: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// deleteResource
// ---------------------------------------------------------------------------

func TestDeleteResource(t *testing.T) {
	s := newTestScheme()

	t.Run("deletes existing resource", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-secret", Namespace: "test-ns"},
		}
		fc := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		ref := openchoreodevv1alpha1.ResourceReference{
			APIVersion: "v1",
			Kind:       "Secret",
			Name:       "test-secret",
			Namespace:  "test-ns",
		}

		err := r.deleteResource(context.Background(), fc, ref)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify deleted
		got := &corev1.Secret{}
		err = fc.Get(context.Background(), types.NamespacedName{Name: "test-secret", Namespace: "test-ns"}, got)
		if err == nil {
			t.Error("expected resource to be deleted")
		}
	})

	t.Run("returns error for non-existent resource", func(t *testing.T) {
		fc := fake.NewClientBuilder().WithScheme(s).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		ref := openchoreodevv1alpha1.ResourceReference{
			APIVersion: "v1",
			Kind:       "Secret",
			Name:       "missing-secret",
			Namespace:  "test-ns",
		}

		err := r.deleteResource(context.Background(), fc, ref)
		if err == nil {
			t.Error("expected error for non-existent resource")
		}
	})

	t.Run("returns error for invalid API version", func(t *testing.T) {
		fc := fake.NewClientBuilder().WithScheme(s).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		ref := openchoreodevv1alpha1.ResourceReference{
			APIVersion: "///invalid",
			Kind:       "Secret",
			Name:       "test",
			Namespace:  "test-ns",
		}

		err := r.deleteResource(context.Background(), fc, ref)
		if err == nil {
			t.Error("expected error for invalid API version")
		}
	})
}

// ---------------------------------------------------------------------------
// ensureFinalizer
// ---------------------------------------------------------------------------

func TestEnsureFinalizer(t *testing.T) {
	s := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(s)

	t.Run("adds finalizer when not present", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wfr", Namespace: "default"},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
			},
		}
		fc := fake.NewClientBuilder().WithScheme(s).WithObjects(wfr).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		added, err := r.ensureFinalizer(context.Background(), wfr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !added {
			t.Error("expected finalizer to be added")
		}
		if !controllerutil.ContainsFinalizer(wfr, WorkflowRunCleanupFinalizer) {
			t.Error("expected finalizer to be present on the object")
		}
	})

	t.Run("no-op when finalizer already present", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-wfr",
				Namespace:  "default",
				Finalizers: []string{WorkflowRunCleanupFinalizer},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
			},
		}
		fc := fake.NewClientBuilder().WithScheme(s).WithObjects(wfr).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		added, err := r.ensureFinalizer(context.Background(), wfr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if added {
			t.Error("expected no finalizer addition")
		}
	})

	t.Run("no-op during deletion", func(t *testing.T) {
		now := metav1.Now()
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-wfr",
				Namespace:         "default",
				DeletionTimestamp: &now,
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
			},
		}
		fc := fake.NewClientBuilder().WithScheme(s).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		added, err := r.ensureFinalizer(context.Background(), wfr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if added {
			t.Error("expected no finalizer addition during deletion")
		}
	})
}

// ---------------------------------------------------------------------------
// removeFinalizer
// ---------------------------------------------------------------------------

func TestRemoveFinalizer(t *testing.T) {
	s := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(s)

	t.Run("removes existing finalizer", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-wfr",
				Namespace:  "default",
				Finalizers: []string{WorkflowRunCleanupFinalizer},
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
			},
		}
		fc := fake.NewClientBuilder().WithScheme(s).WithObjects(wfr).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		result, err := r.removeFinalizer(context.Background(), wfr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != (ctrl.Result{}) {
			t.Errorf("expected empty result, got %v", result)
		}
		if controllerutil.ContainsFinalizer(wfr, WorkflowRunCleanupFinalizer) {
			t.Error("expected finalizer to be removed")
		}
	})

	t.Run("no-op when finalizer not present", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wfr", Namespace: "default"},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
			},
		}
		fc := fake.NewClientBuilder().WithScheme(s).WithObjects(wfr).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		result, err := r.removeFinalizer(context.Background(), wfr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != (ctrl.Result{}) {
			t.Errorf("expected empty result, got %v", result)
		}
	})
}

// ---------------------------------------------------------------------------
// checkTTLExpiration — expired TTL deletes the resource
// ---------------------------------------------------------------------------

func TestCheckTTLExpirationDeletesExpired(t *testing.T) {
	s := runtime.NewScheme()
	_ = openchoreodevv1alpha1.AddToScheme(s)

	wfr := &openchoreodevv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{Name: "ttl-expired", Namespace: "default"},
		Spec: openchoreodevv1alpha1.WorkflowRunSpec{
			Workflow:           openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
			TTLAfterCompletion: "0s",
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(wfr).Build()
	r := &Reconciler{Client: fc, Scheme: s}

	// Set CompletedAt in the past
	past := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	wfr.Status.CompletedAt = &past

	shouldReturn, _, err := r.checkTTLExpiration(context.Background(), wfr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !shouldReturn {
		t.Error("expected shouldReturn=true for expired TTL")
	}

	// Verify the resource was deleted
	got := &openchoreodevv1alpha1.WorkflowRun{}
	err = fc.Get(context.Background(), types.NamespacedName{Name: "ttl-expired", Namespace: "default"}, got)
	if err == nil {
		t.Error("expected resource to be deleted")
	}
}

// ---------------------------------------------------------------------------
// applyRenderedRunResource
// ---------------------------------------------------------------------------

func TestApplyRenderedRunResource(t *testing.T) {
	s := newTestScheme()

	t.Run("creates new run resource in same namespace", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				UID:       "test-uid",
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
			},
		}

		resource := map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Workflow",
			"metadata": map[string]any{
				"name":      testRunResourceName,
				"namespace": "default",
			},
			"spec": map[string]any{
				"entrypoint":         "main",
				"serviceAccountName": "wf-sa",
			},
		}

		fc := fake.NewClientBuilder().WithScheme(s).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		err := r.applyRenderedRunResource(context.Background(), wfr, resource, fc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify status was updated
		if wfr.Status.RunReference == nil {
			t.Fatal("expected RunReference to be set")
		}
		if wfr.Status.RunReference.Name != testRunResourceName {
			t.Errorf("expected RunReference name wf-run-1, got %s", wfr.Status.RunReference.Name)
		}
		if wfr.Status.RunReference.Namespace != "default" {
			t.Errorf("expected RunReference namespace default, got %s", wfr.Status.RunReference.Namespace)
		}
	})

	t.Run("updates existing run resource", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				UID:       "test-uid",
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
			},
		}

		// Pre-create the resource
		existing := &unstructured.Unstructured{}
		existing.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "argoproj.io",
			Version: "v1alpha1",
			Kind:    "Workflow",
		})
		existing.SetName(testRunResourceName)
		existing.SetNamespace("default")

		fc := fake.NewClientBuilder().WithScheme(s).WithObjects(existing).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		resource := map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Workflow",
			"metadata": map[string]any{
				"name":      testRunResourceName,
				"namespace": "default",
			},
			"spec": map[string]any{
				"entrypoint":         "main",
				"serviceAccountName": "wf-sa",
			},
		}

		err := r.applyRenderedRunResource(context.Background(), wfr, resource, fc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if wfr.Status.RunReference == nil {
			t.Fatal("expected RunReference to be set")
		}
		if wfr.Status.RunReference.Name != testRunResourceName {
			t.Errorf("expected RunReference name wf-run-1, got %s", wfr.Status.RunReference.Name)
		}
	})

	t.Run("sets labels for cross-namespace resource", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				UID:       "test-uid",
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
			},
		}

		resource := map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Workflow",
			"metadata": map[string]any{
				"name":      testRunResourceName,
				"namespace": "build-ns",
			},
			"spec": map[string]any{
				"entrypoint":         "main",
				"serviceAccountName": "wf-sa",
			},
		}

		fc := fake.NewClientBuilder().WithScheme(s).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		err := r.applyRenderedRunResource(context.Background(), wfr, resource, fc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify labels were set on cross-namespace resource
		created := &unstructured.Unstructured{}
		created.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "argoproj.io",
			Version: "v1alpha1",
			Kind:    "Workflow",
		})
		if err := fc.Get(context.Background(), types.NamespacedName{Name: testRunResourceName, Namespace: "build-ns"}, created); err != nil {
			t.Fatalf("expected resource to exist: %v", err)
		}
		labels := created.GetLabels()
		if labels["openchoreo.dev/workflowrun"] != "test-wfr" {
			t.Errorf("expected workflowrun label, got %v", labels)
		}
		if labels["openchoreo.dev/managed-by"] != testManagedByLabelValue {
			t.Errorf("expected managed-by label, got %v", labels)
		}
	})
}

// ---------------------------------------------------------------------------
// applyRenderedResources
// ---------------------------------------------------------------------------

func TestApplyRenderedResources(t *testing.T) {
	s := newTestScheme()

	t.Run("returns nil for empty resources", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wfr", Namespace: "default"},
		}
		fc := fake.NewClientBuilder().WithScheme(s).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		result, err := r.applyRenderedResources(context.Background(), wfr, nil, fc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil for empty resources, got %v", result)
		}
	})

	t.Run("creates and tracks single resource", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wfr", Namespace: "default"},
		}
		fc := fake.NewClientBuilder().WithScheme(s).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		resources := []workflowpipeline.RenderedResource{
			{
				ID: "git-secret",
				Resource: map[string]any{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]any{
						"name":      "git-creds",
						"namespace": "build-ns",
					},
					"data": map[string]any{},
				},
			},
		}

		result, err := r.applyRenderedResources(context.Background(), wfr, resources, fc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || len(*result) != 1 {
			t.Fatalf("expected 1 applied resource, got %v", result)
		}
		if (*result)[0].Kind != "Secret" {
			t.Errorf("expected kind Secret, got %s", (*result)[0].Kind)
		}
		if (*result)[0].Name != "git-creds" {
			t.Errorf("expected name git-creds, got %s", (*result)[0].Name)
		}
	})

	t.Run("creates and tracks multiple resources", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wfr", Namespace: "default"},
		}
		fc := fake.NewClientBuilder().WithScheme(s).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		resources := []workflowpipeline.RenderedResource{
			{
				ID: "git-secret",
				Resource: map[string]any{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]any{
						"name":      "git-creds",
						"namespace": "build-ns",
					},
				},
			},
			{
				ID: "build-config",
				Resource: map[string]any{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]any{
						"name":      "build-cfg",
						"namespace": "build-ns",
					},
					"data": map[string]any{"key": "value"},
				},
			},
		}

		result, err := r.applyRenderedResources(context.Background(), wfr, resources, fc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || len(*result) != 2 {
			t.Fatalf("expected 2 applied resources, got %v", result)
		}
		if (*result)[0].Kind != "Secret" {
			t.Errorf("expected first resource kind Secret, got %s", (*result)[0].Kind)
		}
		if (*result)[1].Kind != "ConfigMap" {
			t.Errorf("expected second resource kind ConfigMap, got %s", (*result)[1].Kind)
		}
	})

	t.Run("updates existing resource", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wfr", Namespace: "default"},
		}

		// Pre-create the resource
		existing := &unstructured.Unstructured{}
		existing.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		})
		existing.SetName("git-creds")
		existing.SetNamespace("build-ns")

		fc := fake.NewClientBuilder().WithScheme(s).WithObjects(existing).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		resources := []workflowpipeline.RenderedResource{
			{
				ID: "git-secret",
				Resource: map[string]any{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]any{
						"name":      "git-creds",
						"namespace": "build-ns",
					},
				},
			},
		}

		result, err := r.applyRenderedResources(context.Background(), wfr, resources, fc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || len(*result) != 1 {
			t.Fatalf("expected 1 applied resource, got %v", result)
		}

		// Verify labels were set
		updated := &unstructured.Unstructured{}
		updated.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Secret"})
		if err := fc.Get(context.Background(), types.NamespacedName{Name: "git-creds", Namespace: "build-ns"}, updated); err != nil {
			t.Fatalf("expected resource to exist: %v", err)
		}
		if updated.GetLabels()["openchoreo.dev/workflowrun"] != "test-wfr" {
			t.Error("expected workflowrun label on updated resource")
		}
	})
}

// ---------------------------------------------------------------------------
// ensureRunResource
// ---------------------------------------------------------------------------

func TestEnsureRunResource(t *testing.T) {
	s := newTestScheme()

	t.Run("creates run resource and prerequisites", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				UID:       "test-uid",
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
			},
		}

		output := &workflowpipeline.RenderOutput{
			Resource: map[string]any{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Workflow",
				"metadata": map[string]any{
					"name":      testRunResourceName,
					"namespace": "build-ns",
				},
				"spec": map[string]any{
					"entrypoint":         "main",
					"serviceAccountName": "wf-sa",
				},
			},
		}

		fc := fake.NewClientBuilder().WithScheme(s).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		result := r.ensureRunResource(context.Background(), wfr, output, "build-ns", fc)
		if !result.Requeue {
			t.Error("expected Requeue=true after successful resource creation")
		}

		// Verify RunReference was set
		if wfr.Status.RunReference == nil {
			t.Fatal("expected RunReference to be set")
		}
		if wfr.Status.RunReference.Name != testRunResourceName {
			t.Errorf("expected RunReference name wf-run-1, got %s", wfr.Status.RunReference.Name)
		}

		// Verify prerequisites were created
		ns := &corev1.Namespace{}
		if err := fc.Get(context.Background(), types.NamespacedName{Name: "build-ns"}, ns); err != nil {
			t.Errorf("expected namespace to be created: %v", err)
		}
	})

	t.Run("creates additional resources from output", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				UID:       "test-uid",
			},
			Spec: openchoreodevv1alpha1.WorkflowRunSpec{
				Workflow: openchoreodevv1alpha1.WorkflowRunConfig{Name: "wf"},
			},
		}

		output := &workflowpipeline.RenderOutput{
			Resource: map[string]any{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Workflow",
				"metadata": map[string]any{
					"name":      testRunResourceName,
					"namespace": "build-ns",
				},
				"spec": map[string]any{
					"entrypoint":         "main",
					"serviceAccountName": "wf-sa",
				},
			},
			Resources: []workflowpipeline.RenderedResource{
				{
					ID: "secret",
					Resource: map[string]any{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]any{
							"name":      "build-secret",
							"namespace": "build-ns",
						},
					},
				},
			},
		}

		fc := fake.NewClientBuilder().WithScheme(s).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		result := r.ensureRunResource(context.Background(), wfr, output, "build-ns", fc)
		if !result.Requeue {
			t.Error("expected Requeue=true")
		}

		// Verify additional resources were tracked
		if wfr.Status.Resources == nil || len(*wfr.Status.Resources) != 1 {
			t.Fatalf("expected 1 tracked resource, got %v", wfr.Status.Resources)
		}
		if (*wfr.Status.Resources)[0].Name != "build-secret" {
			t.Errorf("expected tracked resource name build-secret, got %s", (*wfr.Status.Resources)[0].Name)
		}
	})

	t.Run("requeues when service account name missing", func(t *testing.T) {
		wfr := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfr",
				Namespace: "default",
				UID:       "test-uid",
			},
		}

		// Resource missing serviceAccountName
		output := &workflowpipeline.RenderOutput{
			Resource: map[string]any{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Workflow",
				"metadata": map[string]any{
					"name":      "wf-run",
					"namespace": "build-ns",
				},
				"spec": map[string]any{
					"entrypoint": "main",
				},
			},
		}

		fc := fake.NewClientBuilder().WithScheme(s).Build()
		r := &Reconciler{Client: fc, Scheme: s}

		result := r.ensureRunResource(context.Background(), wfr, output, "build-ns", fc)
		if !result.Requeue {
			t.Error("expected Requeue=true when service account name is missing")
		}
	})
}

// ---------------------------------------------------------------------------
// convertSpecParametersToStrings / convertArgumentsParametersToStrings / convertParameterListToStrings
// ---------------------------------------------------------------------------

func TestConvertSpecParametersToStrings(t *testing.T) {
	t.Run("passes through non-arguments fields", func(t *testing.T) {
		spec := map[string]any{
			"entrypoint": "main",
			"templates":  []any{"a", "b"},
		}
		result := convertSpecParametersToStrings(spec)
		if result["entrypoint"] != "main" {
			t.Errorf("expected entrypoint=main, got %v", result["entrypoint"])
		}
	})

	t.Run("converts arguments parameters", func(t *testing.T) {
		spec := map[string]any{
			"arguments": map[string]any{
				"parameters": []any{
					map[string]any{"name": "p1", "value": 42},
				},
			},
		}
		result := convertSpecParametersToStrings(spec)
		args := result["arguments"].(map[string]any)
		params := args["parameters"].([]any)
		if params[0].(map[string]any)["value"] != "42" {
			t.Errorf("expected converted value '42', got %v", params[0].(map[string]any)["value"])
		}
	})

	t.Run("handles non-map arguments gracefully", func(t *testing.T) {
		spec := map[string]any{
			"arguments": "not-a-map",
		}
		result := convertSpecParametersToStrings(spec)
		if result["arguments"] != "not-a-map" {
			t.Errorf("expected arguments passed through, got %v", result["arguments"])
		}
	})
}

func TestConvertArgumentsParametersToStrings(t *testing.T) {
	t.Run("passes through non-parameters fields", func(t *testing.T) {
		args := map[string]any{
			"artifacts": []any{"artifact1"},
		}
		result := convertArgumentsParametersToStrings(args)
		if result["artifacts"] == nil {
			t.Error("expected artifacts to be preserved")
		}
	})

	t.Run("handles non-slice parameters gracefully", func(t *testing.T) {
		args := map[string]any{
			"parameters": "not-a-slice",
		}
		result := convertArgumentsParametersToStrings(args)
		if result["parameters"] != "not-a-slice" {
			t.Errorf("expected parameters passed through, got %v", result["parameters"])
		}
	})
}

func TestConvertParameterListToStrings(t *testing.T) {
	t.Run("converts parameter values to strings", func(t *testing.T) {
		params := []any{
			map[string]any{"name": "p1", "value": 100},
			map[string]any{"name": "p2", "value": true},
		}
		result := convertParameterListToStrings(params)
		if result[0].(map[string]any)["value"] != "100" {
			t.Errorf("expected '100', got %v", result[0].(map[string]any)["value"])
		}
		if result[1].(map[string]any)["value"] != "true" {
			t.Errorf("expected 'true', got %v", result[1].(map[string]any)["value"])
		}
	})

	t.Run("preserves non-value keys", func(t *testing.T) {
		params := []any{
			map[string]any{"name": "p1", "value": 42, "description": "desc"},
		}
		result := convertParameterListToStrings(params)
		p := result[0].(map[string]any)
		if p["name"] != "p1" {
			t.Errorf("expected name=p1, got %v", p["name"])
		}
		if p["description"] != "desc" {
			t.Errorf("expected description=desc, got %v", p["description"])
		}
	})

	t.Run("handles non-map param items", func(t *testing.T) {
		params := []any{"not-a-map", 42}
		result := convertParameterListToStrings(params)
		if result[0] != "not-a-map" {
			t.Errorf("expected non-map item preserved, got %v", result[0])
		}
	})
}
