// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"encoding/json"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
)

// Unit tests for helper functions that don't require k8s test environment

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
			want:      "my-service-account",
			wantError: false,
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
			want:      "my-namespace",
			wantError: false,
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

func TestConvertToString(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		want     string
		contains string
	}{
		{name: "string to string", input: "hello", want: "hello"},
		{name: "int to string", input: 42, want: "42"},
		{name: "int32 to string", input: int32(42), want: "42"},
		{name: "int64 to string", input: int64(42), want: "42"},
		{name: "float32 to string", input: float32(3.14), contains: "3.14"},
		{name: "float64 to string", input: 3.14159, contains: "3.14"},
		{name: "bool true to string", input: true, want: "true"},
		{name: "bool false to string", input: false, want: "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToString(tt.input)
			if tt.want != "" && result != tt.want {
				t.Errorf("expected %s, got %s", tt.want, result)
			}
			if tt.contains != "" && !contains(result, tt.contains) {
				t.Errorf("expected result to contain %s, got %s", tt.contains, result)
			}
		})
	}

	t.Run("map to JSON string", func(t *testing.T) {
		input := map[string]any{
			"key1": "value1",
			"key2": 42,
		}
		result := convertToString(input)
		var decoded map[string]any
		if err := json.Unmarshal([]byte(result), &decoded); err != nil {
			t.Errorf("failed to unmarshal JSON: %v", err)
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
			t.Errorf("failed to unmarshal JSON: %v", err)
		}
		if len(decoded) != 3 {
			t.Errorf("expected length 3, got %d", len(decoded))
		}
	})
}

func TestConvertParameterValuesToStrings(t *testing.T) {
	t.Run("should convert parameter values in workflow resource", func(t *testing.T) {
		resource := map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Workflow",
			"spec": map[string]any{
				"arguments": map[string]any{
					"parameters": []any{
						map[string]any{
							"name":  "param1",
							"value": 42,
						},
						map[string]any{
							"name":  "param2",
							"value": true,
						},
					},
				},
			},
		}

		result := convertParameterValuesToStrings(resource)
		spec := result["spec"].(map[string]any)
		args := spec["arguments"].(map[string]any)
		params := args["parameters"].([]any)

		param1 := params[0].(map[string]any)
		if param1["value"] != "42" {
			t.Errorf("expected param1 value to be '42', got %v", param1["value"])
		}

		param2 := params[1].(map[string]any)
		if param2["value"] != "true" {
			t.Errorf("expected param2 value to be 'true', got %v", param2["value"])
		}
	})

	t.Run("should preserve non-parameter fields", func(t *testing.T) {
		resource := map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Workflow",
			"metadata": map[string]any{
				"name": "test",
			},
		}

		result := convertParameterValuesToStrings(resource)
		if result["apiVersion"] != "argoproj.io/v1alpha1" {
			t.Errorf("expected apiVersion to be preserved")
		}
		if result["kind"] != "Workflow" {
			t.Errorf("expected kind to be preserved")
		}
	})
}

// ResourceReference tests
func TestResourceReference(t *testing.T) {
	t.Run("should correctly store RunReference in status", func(t *testing.T) {
		cwf := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-run",
				Namespace: "default",
			},
		}

		cwf.Status.RunReference = &openchoreodevv1alpha1.ResourceReference{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "Workflow",
			Name:       "test-workflow-run",
			Namespace:  "build-namespace",
		}

		if cwf.Status.RunReference == nil {
			t.Error("expected RunReference to be set")
		}
		if cwf.Status.RunReference.APIVersion != "argoproj.io/v1alpha1" {
			t.Errorf("expected APIVersion argoproj.io/v1alpha1, got %s", cwf.Status.RunReference.APIVersion)
		}
		if cwf.Status.RunReference.Kind != "Workflow" {
			t.Errorf("expected Kind Workflow, got %s", cwf.Status.RunReference.Kind)
		}
		if cwf.Status.RunReference.Name != "test-workflow-run" {
			t.Errorf("expected Name test-workflow-run, got %s", cwf.Status.RunReference.Name)
		}
		if cwf.Status.RunReference.Namespace != "build-namespace" {
			t.Errorf("expected Namespace build-namespace, got %s", cwf.Status.RunReference.Namespace)
		}
	})

	t.Run("should correctly store Resources in status", func(t *testing.T) {
		cwf := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-run",
				Namespace: "default",
			},
		}

		resources := []openchoreodevv1alpha1.ResourceReference{
			{
				APIVersion: "v1",
				Kind:       "Secret",
				Name:       "registry-credentials",
				Namespace:  "build-namespace",
			},
			{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "build-config",
				Namespace:  "build-namespace",
			},
		}
		cwf.Status.Resources = &resources

		if cwf.Status.Resources == nil {
			t.Error("expected Resources to be set")
		}
		if len(*cwf.Status.Resources) != 2 {
			t.Errorf("expected 2 resources, got %d", len(*cwf.Status.Resources))
		}
		if (*cwf.Status.Resources)[0].Kind != "Secret" {
			t.Errorf("expected first resource kind Secret, got %s", (*cwf.Status.Resources)[0].Kind)
		}
		if (*cwf.Status.Resources)[1].Kind != "ConfigMap" {
			t.Errorf("expected second resource kind ConfigMap, got %s", (*cwf.Status.Resources)[1].Kind)
		}
	})

	t.Run("should handle nil RunReference", func(t *testing.T) {
		cwf := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-run",
				Namespace: "default",
			},
		}

		if cwf.Status.RunReference != nil {
			t.Error("expected RunReference to be nil")
		}
	})

	t.Run("should handle nil Resources", func(t *testing.T) {
		cwf := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-run",
				Namespace: "default",
			},
		}

		if cwf.Status.Resources != nil {
			t.Error("expected Resources to be nil")
		}
	})

	t.Run("should handle empty Resources slice", func(t *testing.T) {
		cwf := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-run",
				Namespace: "default",
			},
		}

		emptyResources := []openchoreodevv1alpha1.ResourceReference{}
		cwf.Status.Resources = &emptyResources

		if cwf.Status.Resources == nil {
			t.Error("expected Resources to not be nil")
		}
		if len(*cwf.Status.Resources) != 0 {
			t.Errorf("expected 0 resources, got %d", len(*cwf.Status.Resources))
		}
	})
}

// Finalizer constant test
func TestWorkflowRunCleanupFinalizer(t *testing.T) {
	t.Run("should have correct finalizer value", func(t *testing.T) {
		expected := "openchoreo.dev/workflowrun-cleanup"
		if WorkflowRunCleanupFinalizer != expected {
			t.Errorf("expected finalizer %s, got %s", expected, WorkflowRunCleanupFinalizer)
		}
	})
}

// Condition function tests
func TestConditionFunctions(t *testing.T) {
	t.Run("setWorkflowPendingCondition", func(t *testing.T) {
		cwf := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 1,
			},
		}
		setWorkflowPendingCondition(cwf)

		if len(cwf.Status.Conditions) != 1 {
			t.Errorf("expected 1 condition, got %d", len(cwf.Status.Conditions))
		}
		cond := cwf.Status.Conditions[0]
		if cond.Type != string(ConditionWorkflowCompleted) {
			t.Errorf("expected type %s, got %s", ConditionWorkflowCompleted, cond.Type)
		}
		if cond.Status != metav1.ConditionFalse {
			t.Errorf("expected status False, got %s", cond.Status)
		}
	})

	t.Run("setWorkflowRunningCondition", func(t *testing.T) {
		cwf := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 1,
			},
		}
		setWorkflowRunningCondition(cwf)

		if len(cwf.Status.Conditions) != 1 {
			t.Errorf("expected 1 condition, got %d", len(cwf.Status.Conditions))
		}
		cond := cwf.Status.Conditions[0]
		if cond.Type != string(ConditionWorkflowRunning) {
			t.Errorf("expected type %s, got %s", ConditionWorkflowRunning, cond.Type)
		}
		if cond.Status != metav1.ConditionTrue {
			t.Errorf("expected status True, got %s", cond.Status)
		}
	})

	t.Run("setWorkflowSucceededCondition", func(t *testing.T) {
		cwf := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 1,
			},
		}
		setWorkflowSucceededCondition(cwf)

		if len(cwf.Status.Conditions) != 3 {
			t.Errorf("expected 3 conditions, got %d", len(cwf.Status.Conditions))
		}

		var succeededCond *metav1.Condition
		for i := range cwf.Status.Conditions {
			if cwf.Status.Conditions[i].Type == string(ConditionWorkflowSucceeded) {
				succeededCond = &cwf.Status.Conditions[i]
				break
			}
		}
		if succeededCond == nil {
			t.Error("expected WorkflowSucceeded condition")
		} else if succeededCond.Status != metav1.ConditionTrue {
			t.Errorf("expected status True, got %s", succeededCond.Status)
		}
	})

	t.Run("isWorkflowInitiated", func(t *testing.T) {
		cwf := &openchoreodevv1alpha1.WorkflowRun{}
		if isWorkflowInitiated(cwf) {
			t.Error("expected false for uninitialized workflow")
		}

		setWorkflowPendingCondition(cwf)
		if !isWorkflowInitiated(cwf) {
			t.Error("expected true after setting pending condition")
		}
	})

	t.Run("isWorkflowCompleted", func(t *testing.T) {
		cwf := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Generation: 1},
		}
		setWorkflowPendingCondition(cwf)
		if isWorkflowCompleted(cwf) {
			t.Error("expected false for pending workflow")
		}

		cwf2 := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Generation: 1},
		}
		setWorkflowSucceededCondition(cwf2)
		if !isWorkflowCompleted(cwf2) {
			t.Error("expected true for succeeded workflow")
		}
	})

	t.Run("isWorkflowSucceeded", func(t *testing.T) {
		cwf := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Generation: 1},
		}
		setWorkflowRunningCondition(cwf)
		if isWorkflowSucceeded(cwf) {
			t.Error("expected false for running workflow")
		}

		cwf2 := &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Generation: 1},
		}
		setWorkflowSucceededCondition(cwf2)
		if !isWorkflowSucceeded(cwf2) {
			t.Error("expected true for succeeded workflow")
		}
	})
}

// ---- extractArgoStepOrderFromNodeName ----

func TestExtractArgoStepOrderFromNodeName(t *testing.T) {
	tests := []struct {
		name     string
		nodeName string
		want     int
	}{
		{"standard pattern index 0", "workflow-name[0].step-name", 0},
		{"standard pattern index 3", "my-wf[3].build-step", 3},
		{"no brackets", "simple-name", -1},
		{"empty string", "", -1},
		{"only open bracket", "name[0", -1},
		{"only close bracket", "name]", -1},
		{"non-numeric index", "name[abc].step", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractArgoStepOrderFromNodeName(tt.nodeName)
			if got != tt.want {
				t.Errorf("extractArgoStepOrderFromNodeName(%q) = %d, want %d", tt.nodeName, got, tt.want)
			}
		})
	}
}

// ---- extractArgoTasksFromWorkflowNodes ----

func TestExtractArgoTasksFromWorkflowNodes_Nil(t *testing.T) {
	got := extractArgoTasksFromWorkflowNodes(nil)
	if got != nil {
		t.Errorf("extractArgoTasksFromWorkflowNodes(nil) = %v, want nil", got)
	}
}

func TestExtractArgoTasksFromWorkflowNodes_OnlyNonPod(t *testing.T) {
	nodes := argoproj.Nodes{
		"dag-node": {
			Name: "wf[0].step",
			Type: argoproj.NodeTypeDAG,
		},
	}
	got := extractArgoTasksFromWorkflowNodes(nodes)
	if len(got) != 0 {
		t.Errorf("expected 0 tasks for non-Pod nodes, got %d", len(got))
	}
}

func TestExtractArgoTasksFromWorkflowNodes_SinglePod(t *testing.T) {
	nodes := argoproj.Nodes{
		"pod-node": {
			Name:         "wf[0].build",
			Type:         argoproj.NodeTypePod,
			TemplateName: "build-step",
			Phase:        argoproj.NodeSucceeded,
			Message:      "done",
		},
	}
	got := extractArgoTasksFromWorkflowNodes(nodes)
	if len(got) != 1 {
		t.Fatalf("expected 1 task, got %d", len(got))
	}
	if got[0].Name != "build-step" {
		t.Errorf("Task.Name = %q, want build-step", got[0].Name)
	}
	if got[0].Phase != string(argoproj.NodeSucceeded) {
		t.Errorf("Task.Phase = %q, want Succeeded", got[0].Phase)
	}
}

func TestExtractArgoTasksFromWorkflowNodes_MultiplePodsSorted(t *testing.T) {
	now := metav1.NewTime(time.Now())
	nodes := argoproj.Nodes{
		"b": {Name: "wf[1].step-b", Type: argoproj.NodeTypePod, TemplateName: "step-b", StartedAt: now, FinishedAt: now},
		"a": {Name: "wf[0].step-a", Type: argoproj.NodeTypePod, TemplateName: "step-a", StartedAt: now, FinishedAt: now},
	}
	got := extractArgoTasksFromWorkflowNodes(nodes)
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got))
	}
	if got[0].Name != "step-a" {
		t.Errorf("tasks not sorted: first task = %q, want step-a", got[0].Name)
	}
	if got[1].Name != "step-b" {
		t.Errorf("tasks not sorted: second task = %q, want step-b", got[1].Name)
	}
	if got[0].StartedAt == nil {
		t.Error("expected StartedAt to be set for pod with StartedAt")
	}
	if got[0].CompletedAt == nil {
		t.Error("expected CompletedAt to be set for pod with FinishedAt")
	}
}

// ---- setStartedAtIfNeeded ----

func TestSetStartedAtIfNeeded_SetsWhenNil(t *testing.T) {
	wfRun := &openchoreodevv1alpha1.WorkflowRun{}
	if wfRun.Status.StartedAt != nil {
		t.Fatal("precondition: StartedAt should be nil")
	}
	setStartedAtIfNeeded(wfRun)
	if wfRun.Status.StartedAt == nil {
		t.Error("setStartedAtIfNeeded should set StartedAt when nil")
	}
}

func TestSetStartedAtIfNeeded_DoesNotOverwrite(t *testing.T) {
	original := metav1.NewTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	wfRun := &openchoreodevv1alpha1.WorkflowRun{}
	wfRun.Status.StartedAt = &original
	setStartedAtIfNeeded(wfRun)
	if !wfRun.Status.StartedAt.Equal(&original) {
		t.Error("setStartedAtIfNeeded should not overwrite existing StartedAt")
	}
}

// ---- setCompletedAtIfNeeded ----

func TestSetCompletedAtIfNeeded_SetsWhenNil(t *testing.T) {
	wfRun := &openchoreodevv1alpha1.WorkflowRun{}
	if wfRun.Status.CompletedAt != nil {
		t.Fatal("precondition: CompletedAt should be nil")
	}
	setCompletedAtIfNeeded(wfRun)
	if wfRun.Status.CompletedAt == nil {
		t.Error("setCompletedAtIfNeeded should set CompletedAt when nil")
	}
}

func TestSetCompletedAtIfNeeded_DoesNotOverwrite(t *testing.T) {
	original := metav1.NewTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	wfRun := &openchoreodevv1alpha1.WorkflowRun{}
	wfRun.Status.CompletedAt = &original
	setCompletedAtIfNeeded(wfRun)
	if !wfRun.Status.CompletedAt.Equal(&original) {
		t.Error("setCompletedAtIfNeeded should not overwrite existing CompletedAt")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && len(substr) > 0 && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
