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
		name     string
		input    *RenderInput
		wantErr  bool
		validate func(t *testing.T, output *RenderOutput)
	}{
		{
			name: "basic workflow rendering with context variables",
			input: &RenderInput{
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						Owner: v1alpha1.WorkflowOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						WorkflowDefinitionRef: v1alpha1.WorkflowDefinitionReference{
							Name: "test-workflow-def",
						},
						Parameters: &runtime.RawExtension{
							Raw: []byte(`{"version": 1, "testMode": "unit"}`),
						},
					},
				},
				WorkflowDefinition: &v1alpha1.WorkflowDefinition{
					Spec: v1alpha1.WorkflowDefinitionSpec{
						FixedParameters: []v1alpha1.WorkflowParameter{
							{Name: "builder-image", Value: "gcr.io/buildpacks/builder:v1"},
						},
						Resource: v1alpha1.WorkflowResource{
							Template: &runtime.RawExtension{
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
												map[string]any{
													"name":  "builder-image",
													"value": "${fixedParameters[\"builder-image\"]}",
												},
											},
										},
									},
								}),
							},
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
				if metadata["name"] != "test-component-abc123de" {
					t.Errorf("unexpected metadata.name: %v", metadata["name"])
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

				// Validate builder-image parameter from fixedParameters
				builderParam := params[2].(map[string]any)
				if builderParam["value"] != "gcr.io/buildpacks/builder:v1" {
					t.Errorf("unexpected builder-image value: %v", builderParam["value"])
				}
			},
		},
		{
			name: "ComponentTypeDefinition overrides fixed parameters",
			input: &RenderInput{
				Workflow: &v1alpha1.Workflow{
					Spec: v1alpha1.WorkflowSpec{
						Owner: v1alpha1.WorkflowOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						WorkflowDefinitionRef: v1alpha1.WorkflowDefinitionReference{
							Name: "google-cloud-buildpacks",
						},
					},
				},
				WorkflowDefinition: &v1alpha1.WorkflowDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "google-cloud-buildpacks",
					},
					Spec: v1alpha1.WorkflowDefinitionSpec{
						FixedParameters: []v1alpha1.WorkflowParameter{
							{Name: "security-scan-enabled", Value: "true"},
							{Name: "build-timeout", Value: "30m"},
						},
						Resource: v1alpha1.WorkflowResource{
							Template: &runtime.RawExtension{
								Raw: mustMarshalJSON(map[string]any{
									"apiVersion": "argoproj.io/v1alpha1",
									"kind":       "Workflow",
									"metadata": map[string]any{
										"name": "${ctx.componentName}",
									},
									"spec": map[string]any{
										"arguments": map[string]any{
											"parameters": []any{
												map[string]any{
													"name":  "security-scan",
													"value": "${fixedParameters[\"security-scan-enabled\"]}",
												},
												map[string]any{
													"name":  "timeout",
													"value": "${fixedParameters[\"build-timeout\"]}",
												},
											},
										},
									},
								}),
							},
						},
					},
				},
				ComponentTypeDefinition: &v1alpha1.ComponentTypeDefinition{
					Spec: v1alpha1.ComponentTypeDefinitionSpec{
						Build: &v1alpha1.ComponentTypeBuildConfig{
							AllowedTemplates: []v1alpha1.AllowedWorkflowTemplate{
								{
									Name: "google-cloud-buildpacks",
									FixedParameters: []v1alpha1.WorkflowParameter{
										{Name: "security-scan-enabled", Value: "false"}, // Override
										{Name: "build-timeout", Value: "45m"},           // Override
									},
								},
							},
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

				// Security scan should be overridden to false
				securityParam := params[0].(map[string]any)
				if securityParam["value"] != "false" {
					t.Errorf("expected security-scan to be 'false', got: %v", securityParam["value"])
				}

				// Timeout should be overridden to 45m
				timeoutParam := params[1].(map[string]any)
				if timeoutParam["value"] != "45m" {
					t.Errorf("expected timeout to be '45m', got: %v", timeoutParam["value"])
				}
			},
		},
		{
			name: "missing workflow should error",
			input: &RenderInput{
				Workflow: nil,
				WorkflowDefinition: &v1alpha1.WorkflowDefinition{
					Spec: v1alpha1.WorkflowDefinitionSpec{
						Resource: v1alpha1.WorkflowResource{
							Template: &runtime.RawExtension{
								Raw: []byte(`{}`),
							},
						},
					},
				},
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

func mustMarshalJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
