// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowpipeline

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
				WorkflowRun: &v1alpha1.WorkflowRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-run",
						Namespace: "default",
					},
					Spec: v1alpha1.WorkflowRunSpec{
						Workflow: v1alpha1.WorkflowRunConfig{
							Name: "test-workflow",
							Parameters: mustRawExtension(t, map[string]interface{}{
								"version":  1,
								"testMode": "unit",
								"gitRepo":  "https://github.com/test/repo",
							}),
						},
					},
				},
				Workflow: &v1alpha1.Workflow{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workflow",
						Namespace: "default",
					},
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "argoproj.io/v1alpha1",
							"kind":       "Workflow",
							"metadata": map[string]interface{}{
								"name":      "${metadata.workflowRunName}",
								"namespace": "ci-${metadata.namespaceName}",
							},
							"spec": map[string]interface{}{
								"arguments": map[string]interface{}{
									"parameters": []interface{}{
										map[string]interface{}{
											"name":  "git-repo",
											"value": "${parameters.gitRepo}",
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
				Context: WorkflowContext{
					NamespaceName:   "default",
					WorkflowRunName: "test-run",
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
			name: "nil WorkflowRun",
			input: &RenderInput{
				WorkflowRun: nil,
				Workflow:    &v1alpha1.Workflow{},
				Context: WorkflowContext{
					NamespaceName: "test",
				},
			},
			wantErr: true,
			errMsg:  "workflow run is nil",
		},
		{
			name: "nil Workflow",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow:    nil,
				Context: WorkflowContext{
					NamespaceName: "test",
				},
			},
			wantErr: true,
			errMsg:  "workflow is nil",
		},
		{
			name: "missing runTemplate",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: nil,
					},
				},
				Context: WorkflowContext{
					NamespaceName: "test",
				},
			},
			wantErr: true,
			errMsg:  "workflow has no runTemplate",
		},
		{
			name: "missing context namespaceName",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Pod",
						}),
					},
				},
				Context: WorkflowContext{
					NamespaceName: "",
				},
			},
			wantErr: true,
			errMsg:  "context.namespaceName is required",
		},
		{
			name: "missing context workflowRunName",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Pod",
							"metadata": map[string]interface{}{
								"name": "test",
							},
						}),
					},
				},
				Context: WorkflowContext{
					NamespaceName:   "test",
					WorkflowRunName: "",
				},
			},
			wantErr: true,
			errMsg:  "context.workflowRunName is required",
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
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: &runtime.RawExtension{},
					},
				},
				Context: WorkflowContext{
					NamespaceName:   "test-namespace",
					WorkflowRunName: "test-run",
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

func TestPipeline_Render_SchemaWithDefaults(t *testing.T) {
	tests := []struct {
		name    string
		input   *RenderInput
		wantErr bool
		check   func(*testing.T, *RenderOutput)
	}{
		{
			name: "parameters without schema",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-run",
						Namespace: "default",
					},
					Spec: v1alpha1.WorkflowRunSpec{
						Workflow: v1alpha1.WorkflowRunConfig{
							Name: "test-workflow",
							Parameters: mustRawExtension(t, map[string]interface{}{
								"buildType":   "production",
								"enableCache": true,
								"timeout":     3600,
							}),
						},
					},
				},
				Workflow: &v1alpha1.Workflow{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workflow",
						Namespace: "default",
					},
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "argoproj.io/v1alpha1",
							"kind":       "Workflow",
							"metadata": map[string]interface{}{
								"name":      "${metadata.workflowRunName}",
								"namespace": "default",
							},
							"spec": map[string]interface{}{
								"arguments": map[string]interface{}{
									"parameters": []interface{}{
										map[string]interface{}{
											"name":  "build-type",
											"value": "${parameters.buildType}",
										},
										map[string]interface{}{
											"name":  "enable-cache",
											"value": "${parameters.enableCache}",
										},
										map[string]interface{}{
											"name":  "timeout",
											"value": "${parameters.timeout}",
										},
									},
								},
							},
						}),
					},
				},
				Context: WorkflowContext{
					NamespaceName:   "default",
					WorkflowRunName: "test-run",
				},
			},
			wantErr: false,
			check: func(t *testing.T, output *RenderOutput) {
				spec := output.Resource["spec"].(map[string]interface{})
				args := spec["arguments"].(map[string]interface{})
				params := args["parameters"].([]interface{})

				buildTypeParam := params[0].(map[string]interface{})
				if buildTypeParam["value"] != "production" {
					t.Errorf("expected buildType 'production', got %v", buildTypeParam["value"])
				}

				enableCacheParam := params[1].(map[string]interface{})
				if enableCacheParam["value"] != true {
					t.Errorf("expected enableCache true, got %v", enableCacheParam["value"])
				}

				timeoutParam := params[2].(map[string]interface{})
				if timeoutParam["value"] != float64(3600) {
					t.Errorf("expected timeout 3600, got %v", timeoutParam["value"])
				}
			},
		},
		{
			name: "empty parameters when no schema provided",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{
					Spec: v1alpha1.WorkflowRunSpec{
						Workflow: v1alpha1.WorkflowRunConfig{
							Name:       "test-workflow",
							Parameters: nil, // No parameters provided
						},
					},
				},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Pod",
							"metadata": map[string]interface{}{
								"name": "test",
							},
						}),
					},
				},
				Context: WorkflowContext{
					NamespaceName:   "test-namespace",
					WorkflowRunName: "test-run",
				},
			},
			wantErr: false,
			check: func(t *testing.T, output *RenderOutput) {
				if output.Resource["kind"] != "Pod" {
					t.Errorf("expected kind Pod, got %v", output.Resource["kind"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline()
			output, err := p.Render(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
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

func TestPipeline_Render_ComplexParameters(t *testing.T) {
	tests := []struct {
		name    string
		input   *RenderInput
		wantErr bool
		check   func(*testing.T, *RenderOutput)
	}{
		{
			name: "array parameters rendered as arrays",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{
					Spec: v1alpha1.WorkflowRunSpec{
						Workflow: v1alpha1.WorkflowRunConfig{
							Name: "test-workflow",
							Parameters: mustRawExtension(t, map[string]interface{}{
								"buildSteps": []interface{}{"compile", "test", "package"},
								"flags":      []interface{}{"--verbose", "--production"},
							}),
						},
					},
				},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "argoproj.io/v1alpha1",
							"kind":       "Workflow",
							"metadata": map[string]interface{}{
								"name": "test",
							},
							"spec": map[string]interface{}{
								"arguments": map[string]interface{}{
									"parameters": []interface{}{
										map[string]interface{}{
											"name":  "build-steps",
											"value": "${parameters.buildSteps}",
										},
										map[string]interface{}{
											"name":  "flags",
											"value": "${parameters.flags}",
										},
									},
								},
							},
						}),
					},
				},
				Context: WorkflowContext{
					NamespaceName:   "test-namespace",
					WorkflowRunName: "test-run",
				},
			},
			wantErr: false,
			check: func(t *testing.T, output *RenderOutput) {
				spec := output.Resource["spec"].(map[string]interface{})
				args := spec["arguments"].(map[string]interface{})
				params := args["parameters"].([]interface{})

				// Array parameters remain as arrays in the output
				buildStepsParam := params[0].(map[string]interface{})
				buildStepsValue, ok := buildStepsParam["value"].([]interface{})
				if !ok {
					t.Errorf("expected build-steps value to be []interface{}, got %T", buildStepsParam["value"])
					return
				}
				if len(buildStepsValue) != 3 {
					t.Errorf("expected build-steps to have 3 elements, got %d", len(buildStepsValue))
				}
				if buildStepsValue[0] != "compile" {
					t.Errorf("expected first step to be 'compile', got %v", buildStepsValue[0])
				}
			},
		},
		{
			name: "object parameters converted to JSON strings",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{
					Spec: v1alpha1.WorkflowRunSpec{
						Workflow: v1alpha1.WorkflowRunConfig{
							Name: "test-workflow",
							Parameters: mustRawExtension(t, map[string]interface{}{
								"config": map[string]interface{}{
									"database": "postgres",
									"port":     5432,
									"ssl":      true,
								},
							}),
						},
					},
				},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "argoproj.io/v1alpha1",
							"kind":       "Workflow",
							"metadata": map[string]interface{}{
								"name": "test",
							},
							"spec": map[string]interface{}{
								"arguments": map[string]interface{}{
									"parameters": []interface{}{
										map[string]interface{}{
											"name":  "config",
											"value": "${parameters.config}",
										},
									},
								},
							},
						}),
					},
				},
				Context: WorkflowContext{
					NamespaceName:   "test-namespace",
					WorkflowRunName: "test-run",
				},
			},
			wantErr: false,
			check: func(t *testing.T, output *RenderOutput) {
				spec := output.Resource["spec"].(map[string]interface{})
				args := spec["arguments"].(map[string]interface{})
				params := args["parameters"].([]interface{})

				configParam := params[0].(map[string]interface{})
				configValue, ok := configParam["value"].(string)
				if !ok {
					t.Errorf("expected config value to be string, got %T", configParam["value"])
				}

				// Should be valid JSON containing expected keys
				var configMap map[string]interface{}
				if err := json.Unmarshal([]byte(configValue), &configMap); err != nil {
					t.Errorf("config value is not valid JSON: %v", err)
				}

				if configMap["database"] != "postgres" {
					t.Errorf("expected database 'postgres', got %v", configMap["database"])
				}
			},
		},
		{
			name: "nested parameter access",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{
					Spec: v1alpha1.WorkflowRunSpec{
						Workflow: v1alpha1.WorkflowRunConfig{
							Name: "test-workflow",
							Parameters: mustRawExtension(t, map[string]interface{}{
								"database": map[string]interface{}{
									"host": "localhost",
									"port": 5432,
								},
							}),
						},
					},
				},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name": "test",
							},
							"data": map[string]interface{}{
								"value": "${parameters.database}",
							},
						}),
					},
				},
				Context: WorkflowContext{
					NamespaceName:   "test-namespace",
					WorkflowRunName: "test-run",
				},
			},
			wantErr: false,
			check: func(t *testing.T, output *RenderOutput) {
				data := output.Resource["data"].(map[string]interface{})
				value, ok := data["value"].(string)
				if !ok {
					t.Errorf("expected value to be string, got %T", data["value"])
				}

				var dbConfig map[string]interface{}
				if err := json.Unmarshal([]byte(value), &dbConfig); err != nil {
					t.Errorf("value is not valid JSON: %v", err)
				}

				if dbConfig["host"] != "localhost" {
					t.Errorf("expected host 'localhost', got %v", dbConfig["host"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline()
			output, err := p.Render(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
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

func TestPipeline_Render_CELContextVariables(t *testing.T) {
	tests := []struct {
		name    string
		input   *RenderInput
		wantErr bool
		check   func(*testing.T, *RenderOutput)
	}{
		{
			name: "metadata context variables rendered correctly",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{
					Spec: v1alpha1.WorkflowRunSpec{
						Workflow: v1alpha1.WorkflowRunConfig{
							Name: "my-workflow",
						},
					},
				},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name":      "${metadata.workflowRunName}",
								"namespace": "ci-${metadata.namespaceName}",
								"labels": map[string]interface{}{
									"workflow-run": "${metadata.workflowRunName}",
									"namespace":    "${metadata.namespaceName}",
								},
							},
						}),
					},
				},
				Context: WorkflowContext{
					NamespaceName:   "my-namespace",
					WorkflowRunName: "run-12345",
				},
			},
			wantErr: false,
			check: func(t *testing.T, output *RenderOutput) {
				metadata := output.Resource["metadata"].(map[string]interface{})

				if metadata["name"] != "run-12345" {
					t.Errorf("expected name 'run-12345', got %v", metadata["name"])
				}

				if metadata["namespace"] != "ci-my-namespace" {
					t.Errorf("expected namespace 'ci-my-namespace', got %v", metadata["namespace"])
				}

				labels := metadata["labels"].(map[string]interface{})
				if labels["workflow-run"] != "run-12345" {
					t.Errorf("expected workflow-run 'run-12345', got %v", labels["workflow-run"])
				}

				if labels["namespace"] != "my-namespace" {
					t.Errorf("expected namespace 'my-namespace', got %v", labels["namespace"])
				}
			},
		},
		{
			name: "parameter variables rendered correctly",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{
					Spec: v1alpha1.WorkflowRunSpec{
						Workflow: v1alpha1.WorkflowRunConfig{
							Name: "test-workflow",
							Parameters: mustRawExtension(t, map[string]interface{}{
								"repository": map[string]interface{}{
									"url": "https://github.com/org/repo",
									"revision": map[string]interface{}{
										"branch": "feature-branch",
										"commit": "abc123def456",
									},
									"appPath": "/services/api",
								},
							}),
						},
					},
				},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name": "test",
							},
							"data": map[string]interface{}{
								"repo_url": "${parameters.repository.url}",
								"branch":   "${parameters.repository.revision.branch}",
								"commit":   "${parameters.repository.revision.commit}",
								"app_path": "${parameters.repository.appPath}",
							},
						}),
					},
				},
				Context: WorkflowContext{
					NamespaceName:   "test-namespace",
					WorkflowRunName: "test-run",
				},
			},
			wantErr: false,
			check: func(t *testing.T, output *RenderOutput) {
				data := output.Resource["data"].(map[string]interface{})

				if data["repo_url"] != "https://github.com/org/repo" {
					t.Errorf("expected repo_url, got %v", data["repo_url"])
				}

				if data["branch"] != "feature-branch" {
					t.Errorf("expected branch 'feature-branch', got %v", data["branch"])
				}

				if data["commit"] != "abc123def456" {
					t.Errorf("expected commit 'abc123def456', got %v", data["commit"])
				}

				if data["app_path"] != "/services/api" {
					t.Errorf("expected app_path '/services/api', got %v", data["app_path"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline()
			output, err := p.Render(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
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

func TestPipeline_Render_SecretRefContextVariables(t *testing.T) {
	t.Run("secretRef context variables rendered correctly", func(t *testing.T) {
		input := &RenderInput{
			WorkflowRun: &v1alpha1.WorkflowRun{
				Spec: v1alpha1.WorkflowRunSpec{
					Workflow: v1alpha1.WorkflowRunConfig{
						Name: "test-workflow",
					},
				},
			},
			Workflow: &v1alpha1.Workflow{
				Spec: v1alpha1.WorkflowSpec{
					RunTemplate: mustRawExtension(t, map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name": "secret-context-test",
						},
						"data": map[string]interface{}{
							"secret_name":       "${secretRef.name}",
							"secret_type":       "${secretRef.type}",
							"first_secret_key":  "${secretRef.data[0].secretKey}",
							"first_remote_key":  "${secretRef.data[0].remoteRef.key}",
							"first_remote_prop": "${secretRef.data[0].remoteRef.property}",
						},
					}),
				},
			},
			Context: WorkflowContext{
				NamespaceName:   "test-namespace",
				WorkflowRunName: "test-run",
				SecretRef: &SecretRefInfo{
					Name: "repo-git-secret",
					Type: "kubernetes.io/basic-auth",
					Data: []SecretDataInfo{
						{
							SecretKey: "username",
							RemoteRef: RemoteRefInfo{
								Key:      "secret/data/repo-creds",
								Property: "username",
							},
						},
					},
				},
			},
		}

		output, err := NewPipeline().Render(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := output.Resource["data"].(map[string]interface{})
		if data["secret_name"] != "repo-git-secret" {
			t.Errorf("expected secret_name 'repo-git-secret', got %v", data["secret_name"])
		}
		if data["secret_type"] != "kubernetes.io/basic-auth" {
			t.Errorf("expected secret_type 'kubernetes.io/basic-auth', got %v", data["secret_type"])
		}
		if data["first_secret_key"] != "username" {
			t.Errorf("expected first_secret_key 'username', got %v", data["first_secret_key"])
		}
		if data["first_remote_key"] != "secret/data/repo-creds" {
			t.Errorf("expected first_remote_key 'secret/data/repo-creds', got %v", data["first_remote_key"])
		}
		if data["first_remote_prop"] != "username" {
			t.Errorf("expected first_remote_prop 'username', got %v", data["first_remote_prop"])
		}
	})

	t.Run("secretRef context defaults to empty values when absent", func(t *testing.T) {
		input := &RenderInput{
			WorkflowRun: &v1alpha1.WorkflowRun{
				Spec: v1alpha1.WorkflowRunSpec{
					Workflow: v1alpha1.WorkflowRunConfig{
						Name: "test-workflow",
					},
				},
			},
			Workflow: &v1alpha1.Workflow{
				Spec: v1alpha1.WorkflowSpec{
					RunTemplate: mustRawExtension(t, map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name": "secret-context-defaults-test",
						},
						"data": map[string]interface{}{
							"secret_name": "${secretRef.name}",
							"secret_type": "${secretRef.type}",
						},
					}),
				},
			},
			Context: WorkflowContext{
				NamespaceName:   "test-namespace",
				WorkflowRunName: "test-run",
			},
		}

		output, err := NewPipeline().Render(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := output.Resource["data"].(map[string]interface{})
		if data["secret_name"] != "" {
			t.Errorf("expected empty secret_name, got %v", data["secret_name"])
		}
		if data["secret_type"] != "" {
			t.Errorf("expected empty secret_type, got %v", data["secret_type"])
		}
	})
}

func TestPipeline_Render_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   *RenderInput
		wantErr bool
		errMsg  string
	}{
		{
			name: "missing resource metadata",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Pod",
						}),
					},
				},
				Context: WorkflowContext{
					NamespaceName:   "test-namespace",
					WorkflowRunName: "test-run",
				},
			},
			wantErr: true,
			errMsg:  "missing metadata",
		},
		{
			name: "invalid runTemplate JSON",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: &runtime.RawExtension{
							Raw: []byte("invalid json content"),
						},
					},
				},
				Context: WorkflowContext{
					NamespaceName:   "test-namespace",
					WorkflowRunName: "test-run",
				},
			},
			wantErr: true,
			errMsg:  "failed to unmarshal",
		},
		{
			name: "invalid parameters JSON",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{
					Spec: v1alpha1.WorkflowRunSpec{
						Workflow: v1alpha1.WorkflowRunConfig{
							Name: "test-workflow",
							Parameters: &runtime.RawExtension{
								Raw: []byte("not json"),
							},
						},
					},
				},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: mustRawExtension(t, map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Pod",
							"metadata": map[string]interface{}{
								"name": "test",
							},
						}),
					},
				},
				Context: WorkflowContext{
					NamespaceName:   "test-namespace",
					WorkflowRunName: "test-run",
				},
			},
			wantErr: true,
			errMsg:  "failed to unmarshal parameters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline()
			_, err := p.Render(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestPipeline_Render_DifferentResourceTypes(t *testing.T) {
	tests := []struct {
		name         string
		resourceKind string
		template     map[string]interface{}
		wantErr      bool
	}{
		{
			name:         "ConfigMap resource",
			resourceKind: "ConfigMap",
			template: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "test-config",
				},
				"data": map[string]interface{}{
					"key": "value",
				},
			},
			wantErr: false,
		},
		{
			name:         "Secret resource",
			resourceKind: "Secret",
			template: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name": "test-secret",
				},
				"type": "Opaque",
				"data": map[string]interface{}{
					"password": "cGFzc3dvcmQ=",
				},
			},
			wantErr: false,
		},
		{
			name:         "CronJob resource",
			resourceKind: "CronJob",
			template: map[string]interface{}{
				"apiVersion": "batch/v1",
				"kind":       "CronJob",
				"metadata": map[string]interface{}{
					"name": "test-cron",
				},
				"spec": map[string]interface{}{
					"schedule": "*/5 * * * *",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{
					Spec: v1alpha1.WorkflowRunSpec{
						Workflow: v1alpha1.WorkflowRunConfig{
							Name: "test-workflow",
						},
					},
				},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						RunTemplate: mustRawExtension(t, tt.template),
					},
				},
				Context: WorkflowContext{
					NamespaceName:   "test-namespace",
					WorkflowRunName: "test-run",
				},
			}

			p := NewPipeline()
			output, err := p.Render(input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if output.Resource["kind"] != tt.resourceKind {
				t.Errorf("expected kind %s, got %v", tt.resourceKind, output.Resource["kind"])
			}
		})
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
