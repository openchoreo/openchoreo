// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestValidateClusterComponentTypeResourcesWithSchema(t *testing.T) {
	tests := []struct {
		name      string
		cct       *v1alpha1.ClusterComponentType
		wantError bool
		errMsg    string
	}{
		{
			name: "valid resources",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID: "deployment",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test"}}`),
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "empty resources is valid",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{},
			},
			wantError: false,
		},
		{
			name: "invalid CEL in resource template",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID: "deployment",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "${parameters.name +}"}}`),
							},
						},
					},
				},
			},
			wantError: true,
			errMsg:    "invalid CEL expression",
		},
		{
			name: "nil validations passed for ClusterComponentType",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID: "deployment",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test"}}`),
							},
						},
					},
				},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateClusterComponentTypeResourcesWithSchema(tt.cct, nil, nil)
			if tt.wantError {
				assert.NotEmpty(t, errs, "expected validation error")
				if tt.errMsg != "" {
					errStr := errs.ToAggregate().Error()
					assert.Contains(t, errStr, tt.errMsg)
				}
			} else {
				assert.Empty(t, errs, "unexpected validation errors: %v", errs)
			}
		})
	}
}

func TestValidateClusterComponentTypeResourcesWithSchema_WithTypedSchema(t *testing.T) {
	parametersSchema := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"replicas": {Generic: apiextschema.Generic{Type: "integer"}},
			"name":     {Generic: apiextschema.Generic{Type: "string"}},
		},
	}

	tests := []struct {
		name      string
		cct       *v1alpha1.ClusterComponentType
		wantError bool
		errMsg    string
	}{
		{
			name: "valid CEL with typed parameters",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID: "deployment",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "${parameters.name}"}, "spec": {"replicas": "${parameters.replicas}"}}`),
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "invalid CEL referencing undefined parameter field",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID: "deployment",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "${parameters.nonExistent}"}}`),
							},
						},
					},
				},
			},
			wantError: true,
			errMsg:    "undefined field 'nonExistent'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateClusterComponentTypeResourcesWithSchema(tt.cct, parametersSchema, nil)
			if tt.wantError {
				assert.NotEmpty(t, errs, "expected validation error")
				if tt.errMsg != "" {
					errStr := errs.ToAggregate().Error()
					assert.Contains(t, errStr, tt.errMsg)
				}
			} else {
				assert.Empty(t, errs, "unexpected validation errors: %v", errs)
			}
		})
	}
}

func TestValidateResourceTemplate_ForEachMapFieldAccess(t *testing.T) {
	tests := []struct {
		name      string
		cct       *v1alpha1.ClusterComponentType
		wantError bool
		errMsg    string
	}{
		{
			name: "valid map forEach with .key and .value access",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "route",
							ForEach: `${{"http": 80, "grpc": 9090}}`,
							Var:     "endpoint",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "Service", "metadata": {"name": "${endpoint.key}"}, "spec": {"port": "${endpoint.value}"}}`),
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "invalid map forEach accessing undefined field on loop variable",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "route",
							ForEach: `${{"http": 80, "grpc": 9090}}`,
							Var:     "endpoint",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "Service", "metadata": {"name": "${endpoint.key}"}, "spec": {"port": "${endpoint.value2}"}}`),
							},
						},
					},
				},
			},
			wantError: true,
			errMsg:    "undefined field 'value2'",
		},
		{
			name: "valid map forEach with optional field access on .value",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "route",
							ForEach: `${{"http": 80, "grpc": 9090}}`,
							Var:     "item",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "Service", "metadata": {"name": "${item.key}"}, "spec": {"port": "${item.value}"}}`),
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "valid deep field access on workload.endpoints map value",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "route",
							ForEach: `${workload.endpoints}`,
							Var:     "ep",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "Service", "metadata": {"name": "${ep.key}"}, "spec": {"port": "${ep.value.port}"}}`),
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "invalid deep field access on workload.endpoints map value",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "route",
							ForEach: `${workload.endpoints}`,
							Var:     "ep",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "Service", "metadata": {"name": "${ep.key}"}, "spec": {"path": "${ep.value.basePath2}"}}`),
							},
						},
					},
				},
			},
			wantError: true,
			errMsg:    "undefined field 'basePath2'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateClusterComponentTypeResourcesWithSchema(tt.cct, nil, nil)
			if tt.wantError {
				assert.NotEmpty(t, errs, "expected validation error")
				if tt.errMsg != "" {
					errStr := errs.ToAggregate().Error()
					assert.Contains(t, errStr, tt.errMsg)
				}
			} else {
				assert.Empty(t, errs, "unexpected validation errors: %v", errs)
			}
		})
	}
}

func TestValidateResourceTemplate_ForEachListFieldAccess(t *testing.T) {
	tests := []struct {
		name      string
		cct       *v1alpha1.ClusterComponentType
		wantError bool
		errMsg    string
	}{
		{
			name: "valid list forEach with element access",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "config",
							ForEach: `${["a", "b", "c"]}`,
							Var:     "item",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "${item}"}}`),
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "valid list forEach with dyn-typed elements",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "config",
							ForEach: `${configurations.toConfigFileList()}`,
							Var:     "config",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "${config.resourceName}"}}`),
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "valid list forEach without explicit var uses default item",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "config",
							ForEach: `${["x", "y"]}`,
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "${item}"}}`),
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "valid map forEach with transformMap preserves type checking",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "route",
							ForEach: `${workload.endpoints.transformMap(name, ep, ep.type == "HTTP", ep)}`,
							Var:     "endpoint",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "Service", "metadata": {"name": "${endpoint.key}"}, "spec": {"port": "${endpoint.value.port}"}}`),
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "invalid map forEach with transformMap catches bad field on value",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "route",
							ForEach: `${workload.endpoints.transformMap(name, ep, ep.type == "HTTP", ep)}`,
							Var:     "endpoint",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "Service", "metadata": {"name": "${endpoint.key}"}, "spec": {"port": "${endpoint.value.nonExistent}"}}`),
							},
						},
					},
				},
			},
			wantError: true,
			errMsg:    "undefined field 'nonExistent'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateClusterComponentTypeResourcesWithSchema(tt.cct, nil, nil)
			if tt.wantError {
				assert.NotEmpty(t, errs, "expected validation error")
				if tt.errMsg != "" {
					errStr := errs.ToAggregate().Error()
					assert.Contains(t, errStr, tt.errMsg)
				}
			} else {
				assert.Empty(t, errs, "unexpected validation errors: %v", errs)
			}
		})
	}
}

func TestValidateValidationRuleForClusterComponentType(t *testing.T) {
	// ClusterComponentType passes nil for validations, so the validation rules loop is skipped.
	// This test verifies that ValidateClusterComponentTypeResourcesWithSchema works correctly
	// when no validation rules are present (which is always the case for ClusterComponentType).
	cct := &v1alpha1.ClusterComponentType{
		Spec: v1alpha1.ClusterComponentTypeSpec{
			Resources: []v1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test"}}`),
					},
				},
			},
		},
	}

	errs := ValidateClusterComponentTypeResourcesWithSchema(cct, nil, nil)
	assert.Empty(t, errs, "unexpected validation errors: %v", errs)
}
