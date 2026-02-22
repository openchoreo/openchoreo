// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"encoding/json"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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

func TestParseComponentWorkflowAnnotation(t *testing.T) {
	annotation := "repoUrl: parameters.repository.url, secretRef: parameters.repository.secretRef, invalid, commit: parameters.repository.revision.commit"
	result := parseComponentWorkflowAnnotation(annotation)

	if result["repoUrl"] != "parameters.repository.url" {
		t.Errorf("expected repoUrl path, got %q", result["repoUrl"])
	}
	if result["secretRef"] != "parameters.repository.secretRef" {
		t.Errorf("expected secretRef path, got %q", result["secretRef"])
	}
	if result["commit"] != "parameters.repository.revision.commit" {
		t.Errorf("expected commit path, got %q", result["commit"])
	}
	if _, ok := result["invalid"]; ok {
		t.Errorf("expected invalid entry to be ignored")
	}
}

func TestGetNestedStringFromRawExtension(t *testing.T) {
	makeRaw := func(t *testing.T, data map[string]any) *runtime.RawExtension {
		t.Helper()
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("failed to marshal test data: %v", err)
		}
		return &runtime.RawExtension{Raw: raw}
	}

	tests := []struct {
		name       string
		raw        *runtime.RawExtension
		path       string
		wantValue  string
		wantFound  bool
		wantErr    bool
		errContain string
	}{
		{
			name:      "nil parameters",
			raw:       nil,
			path:      "parameters.repository.secretRef",
			wantFound: false,
		},
		{
			name: "existing path",
			raw: makeRaw(t, map[string]any{
				"repository": map[string]any{
					"secretRef": "repo-git-secret",
				},
			}),
			path:      "parameters.repository.secretRef",
			wantValue: "repo-git-secret",
			wantFound: true,
		},
		{
			name: "missing path",
			raw: makeRaw(t, map[string]any{
				"repository": map[string]any{},
			}),
			path:      "parameters.repository.secretRef",
			wantFound: false,
		},
		{
			name: "non-string value",
			raw: makeRaw(t, map[string]any{
				"repository": map[string]any{
					"secretRef": 123,
				},
			}),
			path:       "parameters.repository.secretRef",
			wantErr:    true,
			errContain: "value is not a string",
		},
		{
			name: "invalid json",
			raw: &runtime.RawExtension{
				Raw: []byte("{invalid-json"),
			},
			path:       "parameters.repository.secretRef",
			wantErr:    true,
			errContain: "failed to unmarshal parameters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found, err := getNestedStringFromRawExtension(tt.raw, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContain != "" && !contains(err.Error(), tt.errContain) {
					t.Fatalf("expected error containing %q, got %q", tt.errContain, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if found != tt.wantFound {
				t.Fatalf("expected found=%v, got %v", tt.wantFound, found)
			}
			if value != tt.wantValue {
				t.Fatalf("expected value=%q, got %q", tt.wantValue, value)
			}
		})
	}
}

func TestResolveSecretRefInfo(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := openchoreodevv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	t.Run("resolves secret reference info", func(t *testing.T) {
		secretRef := &openchoreodevv1alpha1.SecretReference{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "repo-git-secret",
				Namespace: "default",
			},
			Spec: openchoreodevv1alpha1.SecretReferenceSpec{
				Template: openchoreodevv1alpha1.SecretTemplate{
					Type: "kubernetes.io/basic-auth",
				},
				Data: []openchoreodevv1alpha1.SecretDataSource{
					{
						SecretKey: "username",
						RemoteRef: openchoreodevv1alpha1.RemoteReference{
							Key:      "secret/data/repo-creds",
							Property: "username",
						},
					},
				},
			},
		}

		reconciler := &Reconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secretRef).Build(),
		}

		info, err := reconciler.resolveSecretRefInfo(t.Context(), "default", "repo-git-secret")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if info.Name != "repo-git-secret" {
			t.Fatalf("expected name repo-git-secret, got %s", info.Name)
		}
		if info.Type != "kubernetes.io/basic-auth" {
			t.Fatalf("expected type kubernetes.io/basic-auth, got %s", info.Type)
		}
		if len(info.Data) != 1 {
			t.Fatalf("expected 1 data entry, got %d", len(info.Data))
		}
		if info.Data[0].SecretKey != "username" {
			t.Fatalf("expected secretKey username, got %s", info.Data[0].SecretKey)
		}
		if info.Data[0].RemoteRef.Key != "secret/data/repo-creds" {
			t.Fatalf("expected remote key secret/data/repo-creds, got %s", info.Data[0].RemoteRef.Key)
		}
		if info.Data[0].RemoteRef.Property != "username" {
			t.Fatalf("expected remote property username, got %s", info.Data[0].RemoteRef.Property)
		}
	})

	t.Run("returns error when secret reference does not exist", func(t *testing.T) {
		reconciler := &Reconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		}

		_, err := reconciler.resolveSecretRefInfo(t.Context(), "default", "missing-secret")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !contains(err.Error(), "failed to get SecretReference") {
			t.Fatalf("expected get SecretReference error, got %v", err)
		}
	})

	t.Run("returns error when secret reference has no data", func(t *testing.T) {
		secretRef := &openchoreodevv1alpha1.SecretReference{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "empty-secret",
				Namespace: "default",
			},
			Spec: openchoreodevv1alpha1.SecretReferenceSpec{
				Template: openchoreodevv1alpha1.SecretTemplate{
					Type: "kubernetes.io/basic-auth",
				},
				Data: []openchoreodevv1alpha1.SecretDataSource{},
			},
		}

		reconciler := &Reconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secretRef).Build(),
		}

		_, err := reconciler.resolveSecretRefInfo(t.Context(), "default", "empty-secret")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !contains(err.Error(), fmt.Sprintf("SecretReference %q has no data sources", "empty-secret")) {
			t.Fatalf("unexpected error: %v", err)
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
