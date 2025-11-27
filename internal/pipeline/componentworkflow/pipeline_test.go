// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentworkflowpipeline

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestPipeline_Render(t *testing.T) {
	tests := []struct {
		name    string
		input   *RenderInput
		wantErr bool
		errMsg  string
		check   func(*testing.T, *RenderOutput)
	}{
		{
			name: "successful render with all fields",
			input: &RenderInput{
				ComponentWorkflowRun: &v1alpha1.ComponentWorkflowRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-run",
						Namespace: "default",
					},
					Spec: v1alpha1.ComponentWorkflowRunSpec{
						Owner: v1alpha1.ComponentWorkflowOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						Workflow: v1alpha1.ComponentWorkflowRunConfig{
							Name: "test-workflow",
							SystemParameters: v1alpha1.SystemParametersValues{
								Repository: v1alpha1.RepositoryValues{
									URL: "https://github.com/test/repo",
									Revision: v1alpha1.RepositoryRevisionValues{
										Branch: "main",
										Commit: "abc123",
									},
									AppPath: "/app",
								},
							},
							Parameters: mustRawExtension(t, map[string]interface{}{
								"version":  1,
								"testMode": "unit",
							}),
						},
					},
				},
				ComponentWorkflow: &v1alpha1.ComponentWorkflow{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workflow",
						Namespace: "default",
					},
					Spec: v1alpha1.ComponentWorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "argoproj.io/v1alpha1",
							"kind":       "Workflow",
							"metadata": map[string]interface{}{
								"name":      "${ctx.componentWorkflowRunName}",
								"namespace": "ci-${ctx.orgName}",
							},
							"spec": map[string]interface{}{
								"arguments": map[string]interface{}{
									"parameters": []interface{}{
										map[string]interface{}{
											"name":  "git-repo",
											"value": "${systemParameters.repository.url}",
										},
										map[string]interface{}{
											"name":  "version",
											"value": "${parameters.version}",
										},
									},
								},
							},
						}),
					},
				},
				Context: ComponentWorkflowContext{
					OrgName:                  "default",
					ProjectName:              "test-project",
					ComponentName:            "test-component",
					ComponentWorkflowRunName: "test-run",
				},
			},
			wantErr: false,
			check: func(t *testing.T, output *RenderOutput) {
				if output == nil {
					t.Fatal("expected output to be non-nil")
				}
				if output.Resource == nil {
					t.Fatal("expected resource to be non-nil")
				}

				// Check rendered values
				metadata := output.Resource["metadata"].(map[string]interface{})
				if metadata["name"] != "test-run" {
					t.Errorf("expected name to be 'test-run', got %v", metadata["name"])
				}
				if metadata["namespace"] != "ci-default" {
					t.Errorf("expected namespace to be 'ci-default', got %v", metadata["namespace"])
				}

				// Check parameters
				spec := output.Resource["spec"].(map[string]interface{})
				args := spec["arguments"].(map[string]interface{})
				params := args["parameters"].([]interface{})

				gitRepoParam := params[0].(map[string]interface{})
				if gitRepoParam["value"] != "https://github.com/test/repo" {
					t.Errorf("expected git-repo value to be rendered, got %v", gitRepoParam["value"])
				}

				versionParam := params[1].(map[string]interface{})
				// Version remains as integer (scalar values are not converted)
				if versionParam["value"] != float64(1) { // JSON unmarshals numbers as float64
					t.Errorf("expected version value to be 1, got %v", versionParam["value"])
				}
			},
		},
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
			errMsg:  "input is nil",
		},
		{
			name: "nil ComponentWorkflowRun",
			input: &RenderInput{
				ComponentWorkflowRun: nil,
				ComponentWorkflow:    &v1alpha1.ComponentWorkflow{},
				Context: ComponentWorkflowContext{
					OrgName: "test",
				},
			},
			wantErr: true,
			errMsg:  "component workflow run is nil",
		},
		{
			name: "nil ComponentWorkflow",
			input: &RenderInput{
				ComponentWorkflowRun: &v1alpha1.ComponentWorkflowRun{},
				ComponentWorkflow:    nil,
				Context: ComponentWorkflowContext{
					OrgName: "test",
				},
			},
			wantErr: true,
			errMsg:  "component workflow is nil",
		},
		{
			name: "missing runTemplate",
			input: &RenderInput{
				ComponentWorkflowRun: &v1alpha1.ComponentWorkflowRun{},
				ComponentWorkflow: &v1alpha1.ComponentWorkflow{
					Spec: v1alpha1.ComponentWorkflowSpec{
						RunTemplate: nil,
					},
				},
				Context: ComponentWorkflowContext{
					OrgName: "test",
				},
			},
			wantErr: true,
			errMsg:  "component workflow has no runTemplate",
		},
		{
			name: "missing context orgName",
			input: &RenderInput{
				ComponentWorkflowRun: &v1alpha1.ComponentWorkflowRun{},
				ComponentWorkflow: &v1alpha1.ComponentWorkflow{
					Spec: v1alpha1.ComponentWorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Pod",
						}),
					},
				},
				Context: ComponentWorkflowContext{
					OrgName: "",
				},
			},
			wantErr: true,
			errMsg:  "context.orgName is required",
		},
		{
			name: "missing context projectName",
			input: &RenderInput{
				ComponentWorkflowRun: &v1alpha1.ComponentWorkflowRun{},
				ComponentWorkflow: &v1alpha1.ComponentWorkflow{
					Spec: v1alpha1.ComponentWorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Pod",
						}),
					},
				},
				Context: ComponentWorkflowContext{
					OrgName:     "test",
					ProjectName: "",
				},
			},
			wantErr: true,
			errMsg:  "context.projectName is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline()
			output, err := p.Render(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.check != nil {
				tt.check(t, output)
			}
		})
	}
}

func TestPipeline_validateInput(t *testing.T) {
	tests := []struct {
		name    string
		input   *RenderInput
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
			errMsg:  "input is nil",
		},
		{
			name: "valid input",
			input: &RenderInput{
				ComponentWorkflowRun: &v1alpha1.ComponentWorkflowRun{},
				ComponentWorkflow: &v1alpha1.ComponentWorkflow{
					Spec: v1alpha1.ComponentWorkflowSpec{
						RunTemplate: &runtime.RawExtension{},
					},
				},
				Context: ComponentWorkflowContext{
					OrgName:                  "test-org",
					ProjectName:              "test-project",
					ComponentName:            "test-component",
					ComponentWorkflowRunName: "test-run",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline()
			err := p.validateInput(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuildSystemParameters(t *testing.T) {
	tests := []struct {
		name     string
		input    v1alpha1.SystemParametersValues
		expected map[string]interface{}
	}{
		{
			name: "full system parameters",
			input: v1alpha1.SystemParametersValues{
				Repository: v1alpha1.RepositoryValues{
					URL: "https://github.com/test/repo",
					Revision: v1alpha1.RepositoryRevisionValues{
						Branch: "main",
						Commit: "abc123",
					},
					AppPath: "/app",
				},
			},
			expected: map[string]interface{}{
				"repository": map[string]interface{}{
					"url": "https://github.com/test/repo",
					"revision": map[string]interface{}{
						"branch": "main",
						"commit": "abc123",
					},
					"appPath": "/app",
				},
			},
		},
		{
			name:  "empty system parameters",
			input: v1alpha1.SystemParametersValues{},
			expected: map[string]interface{}{
				"repository": map[string]interface{}{
					"url": "",
					"revision": map[string]interface{}{
						"branch": "",
						"commit": "",
					},
					"appPath": "",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildSystemParameters(tt.input)

			if !deepEqual(result, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestExtractParameters(t *testing.T) {
	tests := []struct {
		name     string
		input    *runtime.RawExtension
		expected map[string]interface{}
		wantErr  bool
	}{
		{
			name:     "nil RawExtension",
			input:    nil,
			expected: map[string]interface{}{},
			wantErr:  false,
		},
		{
			name: "valid parameters",
			input: mustRawExtension(t, map[string]interface{}{
				"version":  1,
				"testMode": "unit",
			}),
			expected: map[string]interface{}{
				"version":  float64(1), // JSON unmarshals numbers as float64
				"testMode": "unit",
			},
			wantErr: false,
		},
		{
			name:     "empty parameters",
			input:    mustRawExtension(t, map[string]interface{}{}),
			expected: map[string]interface{}{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractParameters(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if !deepEqual(result, tt.expected) {
					t.Errorf("expected %+v, got %+v", tt.expected, result)
				}
			}
		})
	}
}

func TestValidateRenderedResource(t *testing.T) {
	tests := []struct {
		name     string
		resource map[string]interface{}
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid resource",
			resource: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name": "test-pod",
				},
			},
			wantErr: false,
		},
		{
			name: "missing apiVersion",
			resource: map[string]interface{}{
				"kind": "Pod",
				"metadata": map[string]interface{}{
					"name": "test-pod",
				},
			},
			wantErr: true,
			errMsg:  "missing apiVersion",
		},
		{
			name: "missing kind",
			resource: map[string]interface{}{
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name": "test-pod",
				},
			},
			wantErr: true,
			errMsg:  "missing kind",
		},
		{
			name: "missing metadata",
			resource: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
			},
			wantErr: true,
			errMsg:  "missing metadata",
		},
		{
			name: "missing metadata.name",
			resource: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata":   map[string]interface{}{},
			},
			wantErr: true,
			errMsg:  "missing metadata.name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline()
			err := p.validateRenderedResource(tt.resource)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestConvertComplexValuesToJSONStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name: "convert array value to JSON string",
			input: map[string]interface{}{
				"parameters": []interface{}{
					map[string]interface{}{
						"name":  "test",
						"value": []interface{}{"a", "b", "c"},
					},
				},
			},
			expected: map[string]interface{}{
				"parameters": []interface{}{
					map[string]interface{}{
						"name":  "test",
						"value": `["a","b","c"]`,
					},
				},
			},
		},
		{
			name: "convert object value to JSON string",
			input: map[string]interface{}{
				"parameters": []interface{}{
					map[string]interface{}{
						"name": "test",
						"value": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
			},
			expected: map[string]interface{}{
				"parameters": []interface{}{
					map[string]interface{}{
						"name":  "test",
						"value": `{"foo":"bar"}`,
					},
				},
			},
		},
		{
			name: "scalar value unchanged",
			input: map[string]interface{}{
				"parameters": []interface{}{
					map[string]interface{}{
						"name":  "test",
						"value": "scalar",
					},
				},
			},
			expected: map[string]interface{}{
				"parameters": []interface{}{
					map[string]interface{}{
						"name":  "test",
						"value": "scalar",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertComplexValuesToJSONStrings(tt.input)

			if !deepEqual(result, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestConvertFlowStyleArraysToSlices(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name:     "convert flow-style array string",
			input:    `["a","b","c"]`,
			expected: []interface{}{"a", "b", "c"},
		},
		{
			name:     "non-array string unchanged",
			input:    "regular string",
			expected: "regular string",
		},
		{
			name: "nested flow-style arrays",
			input: map[string]interface{}{
				"items": `["x","y","z"]`,
			},
			expected: map[string]interface{}{
				"items": []interface{}{"x", "y", "z"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertFlowStyleArraysToSlices(tt.input)

			if !deepEqual(result, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestRawExtensionToMap(t *testing.T) {
	tests := []struct {
		name    string
		input   *runtime.RawExtension
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil RawExtension",
			input:   nil,
			wantErr: true,
			errMsg:  "raw extension is nil",
		},
		{
			name: "valid RawExtension",
			input: mustRawExtension(t, map[string]interface{}{
				"key": "value",
			}),
			wantErr: false,
		},
		{
			name: "invalid JSON",
			input: &runtime.RawExtension{
				Raw: []byte("invalid json"),
			},
			wantErr: true,
			errMsg:  "failed to unmarshal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rawExtensionToMap(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == nil {
					t.Error("expected non-nil result")
				}
			}
		})
	}
}

func TestGenerateShortUUID(t *testing.T) {
	// Test that it generates valid 8-character hex strings
	for i := 0; i < 10; i++ {
		uuid, err := generateShortUUID()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(uuid) != 8 {
			t.Errorf("expected UUID length 8, got %d", len(uuid))
		}
		// Verify it's valid hex
		for _, c := range uuid {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("invalid hex character: %c", c)
			}
		}
	}

	// Test uniqueness (should be very unlikely to collide)
	uuids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		uuid, _ := generateShortUUID()
		if uuids[uuid] {
			t.Error("generated duplicate UUID")
		}
		uuids[uuid] = true
	}
}

// Helper functions

func mustRawExtension(t *testing.T, data interface{}) *runtime.RawExtension {
	t.Helper()
	bytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal data: %v", err)
	}
	return &runtime.RawExtension{Raw: bytes}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsInner(s, substr))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func deepEqual(a, b interface{}) bool {
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}
