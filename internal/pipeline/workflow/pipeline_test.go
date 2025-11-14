// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowpipeline

import (
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// These tests validate the Workflow pipeline rendering functionality.
// The Workflow API uses a simplified design where Workflows define resource templates
// and WorkflowRuns provide schema-based parameters for rendering.
// The old WorkflowDefinition type with fixed parameters has been removed.

func TestPipeline_Render(t *testing.T) {
	tests := []struct {
		name     string
		input    *RenderInput
		wantErr  bool
		validate func(t *testing.T, output *RenderOutput)
	}{
		{
			name: "basic workflow rendering with context variables",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{
					Spec: v1alpha1.WorkflowRunSpec{
						Owner: v1alpha1.WorkflowOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						Workflow: v1alpha1.WorkflowConfig{
							Name: "test-workflow",
							Schema: &runtime.RawExtension{
								Raw: []byte(`{"version": 1, "testMode": "unit"}`),
							},
						},
					},
				},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						Resource: &runtime.RawExtension{
							Raw: mustMarshalJSON(map[string]any{
								"apiVersion": "argoproj.io/v1alpha1",
								"kind":       "Workflow",
								"metadata": map[string]any{
									"name":      "${ctx.componentName}-${ctx.uuid}",
									"namespace": "build-plane-${ctx.orgName}",
								},
								"spec": map[string]any{
									"arguments": map[string]any{
										"parameters": []any{
											map[string]any{
												"name":  "version",
												"value": "${schema.version}",
											},
											map[string]any{
												"name":  "test-mode",
												"value": "${schema.testMode}",
											},
										},
									},
								},
							}),
						},
					},
				},
				Context: WorkflowContext{
					OrgName:       "test-org",
					ProjectName:   "test-project",
					ComponentName: "test-component",
					Timestamp:     1234567890,
					UUID:          "abc123de",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, output *RenderOutput) {
				if output.Resource["apiVersion"] != "argoproj.io/v1alpha1" {
					t.Errorf("unexpected apiVersion: %v", output.Resource["apiVersion"])
				}
				if output.Resource["kind"] != "Workflow" {
					t.Errorf("unexpected kind: %v", output.Resource["kind"])
				}

				metadata := output.Resource["metadata"].(map[string]any)
				name := metadata["name"].(string)
				// UUID is auto-generated, so just check prefix
				if !strings.HasPrefix(name, "test-component-") {
					t.Errorf("unexpected metadata.name: %v, expected prefix 'test-component-'", name)
				}
				if metadata["namespace"] != "build-plane-test-org" {
					t.Errorf("unexpected metadata.namespace: %v", metadata["namespace"])
				}

				spec := output.Resource["spec"].(map[string]any)
				args := spec["arguments"].(map[string]any)
				params := args["parameters"].([]any)

				// Validate version parameter
				versionParam := params[0].(map[string]any)
				if versionParam["name"] != "version" {
					t.Errorf("unexpected parameter name: %v", versionParam["name"])
				}
				// version should be rendered as a number (JSON unmarshals to float64)
				versionValue := versionParam["value"]
				var versionNum float64
				switch v := versionValue.(type) {
				case int64:
					versionNum = float64(v)
				case float64:
					versionNum = v
				default:
					t.Errorf("unexpected version type: %T", versionValue)
				}
				if versionNum != 1 {
					t.Errorf("unexpected version value: %v", versionNum)
				}

				// Validate test-mode parameter
				testModeParam := params[1].(map[string]any)
				if testModeParam["value"] != "unit" {
					t.Errorf("unexpected test-mode value: %v", testModeParam["value"])
				}
			},
		},
		{
			name: "missing workflow should error",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow:    nil,
				Context: WorkflowContext{
					OrgName:       "test-org",
					ProjectName:   "test-project",
					ComponentName: "test-component",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline()
			output, err := p.Render(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, output)
			}
		})
	}
}

func TestPipeline_Render_ArrayAndObjectParameters(t *testing.T) {
	tests := []struct {
		name     string
		input    *RenderInput
		wantErr  bool
		validate func(t *testing.T, output *RenderOutput)
	}{
		{
			name: "array parameters are converted to FlowStyleArray",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{
					Spec: v1alpha1.WorkflowRunSpec{
						Owner: v1alpha1.WorkflowOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						Workflow: v1alpha1.WorkflowConfig{
							Name: "test-workflow",
							Schema: &runtime.RawExtension{
								Raw: []byte(`{"command": ["npm", "run", "build"], "flags": ["--verbose", "--production"]}`),
							},
						},
					},
				},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						Resource: &runtime.RawExtension{
							Raw: mustMarshalJSON(map[string]any{
								"apiVersion": "argoproj.io/v1alpha1",
								"kind":       "Workflow",
								"metadata":   map[string]any{"name": "test"},
								"spec": map[string]any{
									"arguments": map[string]any{
										"parameters": []any{
											map[string]any{
												"name":  "command",
												"value": "${schema.command}",
											},
											map[string]any{
												"name":  "flags",
												"value": "${schema.flags}",
											},
										},
									},
								},
							}),
						},
					},
				},
				Context: WorkflowContext{
					OrgName:       "test-org",
					ProjectName:   "test-project",
					ComponentName: "test-component",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, output *RenderOutput) {
				spec := output.Resource["spec"].(map[string]any)
				args := spec["arguments"].(map[string]any)
				params := args["parameters"].([]any)

				commandParam := params[0].(map[string]any)
				commandValue := commandParam["value"].([]any)
				if len(commandValue) != 3 {
					t.Errorf("expected command array length 3, got: %d", len(commandValue))
				}
				if commandValue[0] != "npm" || commandValue[1] != "run" || commandValue[2] != "build" {
					t.Errorf("unexpected command array: %v", commandValue)
				}

				flagsParam := params[1].(map[string]any)
				flagsValue := flagsParam["value"].([]any)
				if len(flagsValue) != 2 {
					t.Errorf("expected flags array length 2, got: %d", len(flagsValue))
				}
			},
		},
		{
			name: "object parameters are converted to JSON strings",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{
					Spec: v1alpha1.WorkflowRunSpec{
						Owner: v1alpha1.WorkflowOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						Workflow: v1alpha1.WorkflowConfig{
							Name: "test-workflow",
							Schema: &runtime.RawExtension{
								Raw: []byte(`{"config": {"key1": "value1", "key2": "value2"}}`),
							},
						},
					},
				},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						Resource: &runtime.RawExtension{
							Raw: mustMarshalJSON(map[string]any{
								"apiVersion": "argoproj.io/v1alpha1",
								"kind":       "Workflow",
								"metadata":   map[string]any{"name": "test"},
								"spec": map[string]any{
									"arguments": map[string]any{
										"parameters": []any{
											map[string]any{
												"name":  "config",
												"value": "${schema.config}",
											},
										},
									},
								},
							}),
						},
					},
				},
				Context: WorkflowContext{
					OrgName:       "test-org",
					ProjectName:   "test-project",
					ComponentName: "test-component",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, output *RenderOutput) {
				spec := output.Resource["spec"].(map[string]any)
				args := spec["arguments"].(map[string]any)
				params := args["parameters"].([]any)

				configParam := params[0].(map[string]any)
				configValue := configParam["value"].(string)

				// Should be JSON string
				if !strings.Contains(configValue, "key1") || !strings.Contains(configValue, "value1") {
					t.Errorf("expected JSON string with key1/value1, got: %v", configValue)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline()
			output, err := p.Render(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, output)
			}
		})
	}
}

func TestPipeline_Render_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		input       *RenderInput
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil input",
			input:       nil,
			wantErr:     true,
			errContains: "input is nil",
		},
		{
			name: "missing workflow",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow:    nil,
				Context: WorkflowContext{
					OrgName:       "test-org",
					ProjectName:   "test-project",
					ComponentName: "test-component",
				},
			},
			wantErr:     true,
			errContains: "workflow",
		},
		{
			name: "missing resource template",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{},
				},
				Context: WorkflowContext{
					OrgName:       "test-org",
					ProjectName:   "test-project",
					ComponentName: "test-component",
				},
			},
			wantErr:     true,
			errContains: "resource",
		},
		{
			name: "missing org name",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						Resource: &runtime.RawExtension{Raw: []byte(`{}`)},
					},
				},
				Context: WorkflowContext{
					ProjectName:   "test-project",
					ComponentName: "test-component",
				},
			},
			wantErr:     true,
			errContains: "orgName",
		},
		{
			name: "missing project name",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						Resource: &runtime.RawExtension{Raw: []byte(`{}`)},
					},
				},
				Context: WorkflowContext{
					OrgName:       "test-org",
					ComponentName: "test-component",
				},
			},
			wantErr:     true,
			errContains: "projectName",
		},
		{
			name: "missing component name",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						Resource: &runtime.RawExtension{Raw: []byte(`{}`)},
					},
				},
				Context: WorkflowContext{
					OrgName:     "test-org",
					ProjectName: "test-project",
				},
			},
			wantErr:     true,
			errContains: "componentName",
		},
		{
			name: "rendered resource missing apiVersion",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						Resource: &runtime.RawExtension{
							Raw: mustMarshalJSON(map[string]any{
								"kind":     "Workflow",
								"metadata": map[string]any{"name": "test"},
							}),
						},
					},
				},
				Context: WorkflowContext{
					OrgName:       "test-org",
					ProjectName:   "test-project",
					ComponentName: "test-component",
				},
			},
			wantErr:     true,
			errContains: "apiVersion",
		},
		{
			name: "rendered resource missing kind",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						Resource: &runtime.RawExtension{
							Raw: mustMarshalJSON(map[string]any{
								"apiVersion": "v1",
								"metadata":   map[string]any{"name": "test"},
							}),
						},
					},
				},
				Context: WorkflowContext{
					OrgName:       "test-org",
					ProjectName:   "test-project",
					ComponentName: "test-component",
				},
			},
			wantErr:     true,
			errContains: "kind",
		},
		{
			name: "rendered resource missing metadata",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						Resource: &runtime.RawExtension{
							Raw: mustMarshalJSON(map[string]any{
								"apiVersion": "v1",
								"kind":       "Workflow",
							}),
						},
					},
				},
				Context: WorkflowContext{
					OrgName:       "test-org",
					ProjectName:   "test-project",
					ComponentName: "test-component",
				},
			},
			wantErr:     true,
			errContains: "metadata",
		},
		{
			name: "rendered resource missing metadata.name",
			input: &RenderInput{
				WorkflowRun: &v1alpha1.WorkflowRun{},
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						Resource: &runtime.RawExtension{
							Raw: mustMarshalJSON(map[string]any{
								"apiVersion": "v1",
								"kind":       "Workflow",
								"metadata":   map[string]any{},
							}),
						},
					},
				},
				Context: WorkflowContext{
					OrgName:       "test-org",
					ProjectName:   "test-project",
					ComponentName: "test-component",
				},
			},
			wantErr:     true,
			errContains: "metadata.name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline()
			_, err := p.Render(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got: %v", tt.errContains, err)
				}
			}
		})
	}
}

func TestGenerateShortUUID(t *testing.T) {
	uuid1, err := generateShortUUID()
	if err != nil {
		t.Fatalf("generateShortUUID() error = %v", err)
	}

	if len(uuid1) != 8 {
		t.Errorf("expected UUID length 8, got: %d", len(uuid1))
	}

	// Generate another UUID and ensure it's different
	uuid2, err := generateShortUUID()
	if err != nil {
		t.Fatalf("generateShortUUID() error = %v", err)
	}

	if uuid1 == uuid2 {
		t.Errorf("expected different UUIDs, got same: %s", uuid1)
	}
}

func TestExtractParameters(t *testing.T) {
	tests := []struct {
		name    string
		raw     *runtime.RawExtension
		want    map[string]any
		wantErr bool
	}{
		{
			name: "valid parameters",
			raw: &runtime.RawExtension{
				Raw: []byte(`{"key1": "value1", "key2": 123}`),
			},
			want: map[string]any{
				"key1": "value1",
				"key2": float64(123),
			},
			wantErr: false,
		},
		{
			name:    "nil raw extension",
			raw:     nil,
			want:    map[string]any{},
			wantErr: false,
		},
		{
			name: "empty raw bytes",
			raw: &runtime.RawExtension{
				Raw: nil,
			},
			want:    map[string]any{},
			wantErr: false,
		},
		{
			name: "invalid JSON",
			raw: &runtime.RawExtension{
				Raw: []byte(`{invalid json}`),
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractParameters(tt.raw)

			if (err != nil) != tt.wantErr {
				t.Errorf("extractParameters() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("extractParameters() got length %d, want %d", len(got), len(tt.want))
				}
				for k, v := range tt.want {
					if got[k] != v {
						t.Errorf("extractParameters()[%q] = %v, want %v", k, got[k], v)
					}
				}
			}
		})
	}
}

func TestRawExtensionToMap(t *testing.T) {
	tests := []struct {
		name    string
		raw     *runtime.RawExtension
		want    map[string]any
		wantErr bool
	}{
		{
			name: "valid map",
			raw: &runtime.RawExtension{
				Raw: []byte(`{"key": "value"}`),
			},
			want: map[string]any{
				"key": "value",
			},
			wantErr: false,
		},
		{
			name:    "nil raw extension",
			raw:     nil,
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid JSON",
			raw: &runtime.RawExtension{
				Raw: []byte(`not json`),
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rawExtensionToMap(tt.raw)

			if (err != nil) != tt.wantErr {
				t.Errorf("rawExtensionToMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(got) != len(tt.want) {
				t.Errorf("rawExtensionToMap() got length %d, want %d", len(got), len(tt.want))
			}
		})
	}
}

func TestFlowStyleArray_String(t *testing.T) {
	tests := []struct {
		name string
		arr  FlowStyleArray
		want string
	}{
		{
			name: "string array",
			arr:  FlowStyleArray{"a", "b", "c"},
			want: `["a", "b", "c"]`,
		},
		{
			name: "number array",
			arr:  FlowStyleArray{1, 2, 3},
			want: "[1, 2, 3]",
		},
		{
			name: "mixed array",
			arr:  FlowStyleArray{"test", 42, true},
			want: `["test", 42, true]`,
		},
		{
			name: "empty array",
			arr:  FlowStyleArray{},
			want: "[]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.arr.String()
			if got != tt.want {
				t.Errorf("FlowStyleArray.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConvertValueToString(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want any
	}{
		{
			name: "string value",
			val:  "test",
			want: "test",
		},
		{
			name: "int value",
			val:  42,
			want: 42,
		},
		{
			name: "bool value",
			val:  true,
			want: true,
		},
		{
			name: "array value",
			val:  []any{"a", "b", "c"},
			want: FlowStyleArray{"a", "b", "c"},
		},
		{
			name: "map value",
			val:  map[string]any{"key": "value"},
			want: `{"key":"value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertValueToString(tt.val)

			switch wantVal := tt.want.(type) {
			case FlowStyleArray:
				gotArr, ok := got.(FlowStyleArray)
				if !ok {
					t.Errorf("convertValueToString() type = %T, want FlowStyleArray", got)
					return
				}
				if len(gotArr) != len(wantVal) {
					t.Errorf("convertValueToString() array length = %d, want %d", len(gotArr), len(wantVal))
				}
			case string:
				if got != wantVal {
					t.Errorf("convertValueToString() = %v, want %v", got, wantVal)
				}
			default:
				if got != tt.want {
					t.Errorf("convertValueToString() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestConvertFlowStyleArraysToSlices(t *testing.T) {
	tests := []struct {
		name string
		data any
		want any
	}{
		{
			name: "FlowStyleArray to slice",
			data: FlowStyleArray{"a", "b", "c"},
			want: []any{"a", "b", "c"},
		},
		{
			name: "nested FlowStyleArray",
			data: map[string]any{
				"arr": FlowStyleArray{1, 2, 3},
			},
			want: map[string]any{
				"arr": []any{1, 2, 3},
			},
		},
		{
			name: "regular slice unchanged",
			data: []any{1, 2, 3},
			want: []any{1, 2, 3},
		},
		{
			name: "primitive unchanged",
			data: "test",
			want: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertFlowStyleArraysToSlices(tt.data)

			switch wantVal := tt.want.(type) {
			case []any:
				gotSlice, ok := got.([]any)
				if !ok {
					t.Errorf("convertFlowStyleArraysToSlices() type = %T, want []any", got)
					return
				}
				if len(gotSlice) != len(wantVal) {
					t.Errorf("convertFlowStyleArraysToSlices() length = %d, want %d", len(gotSlice), len(wantVal))
				}
			case map[string]any:
				gotMap, ok := got.(map[string]any)
				if !ok {
					t.Errorf("convertFlowStyleArraysToSlices() type = %T, want map[string]any", got)
					return
				}
				for k := range wantVal {
					if _, exists := gotMap[k]; !exists {
						t.Errorf("convertFlowStyleArraysToSlices() missing key: %s", k)
					}
				}
			default:
				if got != tt.want {
					t.Errorf("convertFlowStyleArraysToSlices() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func mustMarshalJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
