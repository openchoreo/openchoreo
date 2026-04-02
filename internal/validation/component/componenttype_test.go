// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

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
			name: "valid list forEach with typed elements from toConfigFileList",
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
		{
			name: "valid forEach with toSecretEnvsByContainer accessing valid fields",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "secret",
							ForEach: `${configurations.toSecretEnvsByContainer()}`,
							Var:     "secretEnv",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "Secret", "metadata": {"name": "${secretEnv.resourceName}"}, "data": "${secretEnv.envs}"}`),
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "invalid forEach with toSecretEnvsByContainer accessing invalid field",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "secret",
							ForEach: `${configurations.toSecretEnvsByContainer()}`,
							Var:     "secretEnv",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "Secret", "metadata": {"name": "${secretEnv.nonExistent}"}}`),
							},
						},
					},
				},
			},
			wantError: true,
			errMsg:    "undefined field 'nonExistent'",
		},
		{
			name: "invalid forEach with toConfigFileList accessing invalid field",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "config",
							ForEach: `${configurations.toConfigFileList()}`,
							Var:     "config",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "${config.nonExistent}"}}`),
							},
						},
					},
				},
			},
			wantError: true,
			errMsg:    "undefined field 'nonExistent'",
		},
		{
			name: "invalid forEach with toSecretEnvsByContainer accessing nested field on envs element",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "secret",
							ForEach: `${configurations.toSecretEnvsByContainer()}`,
							Var:     "secretEnv",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "Secret", "metadata": {"name": "${secretEnv.envs[0].nonExistent}"}}`),
							},
						},
					},
				},
			},
			wantError: true,
			errMsg:    "undefined field 'nonExistent'",
		},
		{
			name: "invalid forEach with toServicePorts accessing invalid field",
			cct: &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{
							ID:      "service-port",
							ForEach: `${workload.toServicePorts()}`,
							Var:     "sp",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "Service", "metadata": {"name": "${sp.nonExistent}"}}`),
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

func TestValidateResourcesWithSchema_CustomBasePath(t *testing.T) {
	t.Run("errors use custom basePath for resources", func(t *testing.T) {
		customBase := field.NewPath("spec", "componentType", "spec")
		resources := []v1alpha1.ResourceTemplate{
			{
				ID:       "deployment",
				Template: nil,
			},
		}
		errs := ValidateResourcesWithSchema(resources, nil, nil, nil, customBase)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Field, "spec.componentType.spec.resources")
	})

	t.Run("errors use custom basePath for validations", func(t *testing.T) {
		customBase := field.NewPath("spec", "componentType", "spec")
		validations := []v1alpha1.ValidationRule{
			{Rule: "not-wrapped", Message: "test"},
		}
		errs := ValidateResourcesWithSchema(nil, validations, nil, nil, customBase)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Field, "spec.componentType.spec.validations")
	})

	t.Run("empty resources and validations", func(t *testing.T) {
		errs := ValidateResourcesWithSchema(nil, nil, nil, nil, field.NewPath("spec"))
		assert.Empty(t, errs)
	})
}

func TestValidateResourceTemplate_ForEachErrors(t *testing.T) {
	t.Run("forEach not wrapped in template syntax", func(t *testing.T) {
		ct := &v1alpha1.ComponentType{
			Spec: v1alpha1.ComponentTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{
					{
						ID:      "config",
						ForEach: `parameters.items`,
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
						},
					},
				},
			},
		}
		errs := ValidateComponentTypeResourcesWithSchema(ct, nil, nil)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "forEach must be wrapped in ${...}")
	})

	t.Run("includeWhen not wrapped in template syntax", func(t *testing.T) {
		ct := &v1alpha1.ComponentType{
			Spec: v1alpha1.ComponentTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{
					{
						ID:          "config",
						IncludeWhen: `true`,
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
						},
					},
				},
			},
		}
		errs := ValidateComponentTypeResourcesWithSchema(ct, nil, nil)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "includeWhen must be wrapped in ${...}")
	})

	t.Run("includeWhen returns non-boolean", func(t *testing.T) {
		parametersSchema := &apiextschema.Structural{
			Generic: apiextschema.Generic{Type: "object"},
			Properties: map[string]apiextschema.Structural{
				"name": {Generic: apiextschema.Generic{Type: "string"}},
			},
		}
		ct := &v1alpha1.ComponentType{
			Spec: v1alpha1.ComponentTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{
					{
						ID:          "config",
						IncludeWhen: `${parameters.name}`,
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
						},
					},
				},
			},
		}
		errs := ValidateComponentTypeResourcesWithSchema(ct, parametersSchema, nil)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "includeWhen must return boolean")
	})

	t.Run("nil template in resource", func(t *testing.T) {
		ct := &v1alpha1.ComponentType{
			Spec: v1alpha1.ComponentTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{
					{
						ID:       "config",
						Template: nil,
					},
				},
			},
		}
		errs := ValidateComponentTypeResourcesWithSchema(ct, nil, nil)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "template is required")
	})

	t.Run("includeWhen must not reference forEach loop variable", func(t *testing.T) {
		// At runtime, includeWhen is evaluated before forEach — the loop variable
		// is not in scope. Validation must reject this to prevent runtime errors.
		parametersSchema := &apiextschema.Structural{
			Generic: apiextschema.Generic{Type: "object"},
			Properties: map[string]apiextschema.Structural{
				"items": {
					Generic: apiextschema.Generic{Type: "array"},
					Items: &apiextschema.Structural{
						Generic: apiextschema.Generic{Type: "object"},
						Properties: map[string]apiextschema.Structural{
							"enabled": {Generic: apiextschema.Generic{Type: "boolean"}},
							"name":    {Generic: apiextschema.Generic{Type: "string"}},
						},
					},
				},
			},
		}
		ct := &v1alpha1.ComponentType{
			Spec: v1alpha1.ComponentTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{
					{
						ID:          "config",
						ForEach:     `${parameters.items}`,
						Var:         "item",
						IncludeWhen: `${item.enabled}`,
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"${item.name}"}}`),
						},
					},
				},
			},
		}
		errs := ValidateComponentTypeResourcesWithSchema(ct, parametersSchema, nil)
		require.NotEmpty(t, errs)
		errStr := errs.ToAggregate().Error()
		assert.Contains(t, errStr, "includeWhen")
		assert.Contains(t, errStr, "undeclared reference to 'item'")
	})
}

func TestValidateTemplateBody(t *testing.T) {
	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{})
	require.NoError(t, err)
	env := validator.GetBaseEnv()
	basePath := field.NewPath("spec", "resources").Index(0).Child("template")

	t.Run("empty body returns nil", func(t *testing.T) {
		errs := validateTemplateBody(runtime.RawExtension{Raw: []byte{}}, validator, env, basePath)
		assert.Nil(t, errs)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		errs := validateTemplateBody(runtime.RawExtension{Raw: []byte(`{invalid`)}, validator, env, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "invalid JSON")
	})

	t.Run("valid template with CEL", func(t *testing.T) {
		errs := validateTemplateBody(
			runtime.RawExtension{Raw: []byte(`{"name":"${metadata.name}"}`)},
			validator, env, basePath)
		assert.Empty(t, errs)
	})

	t.Run("invalid CEL in template", func(t *testing.T) {
		errs := validateTemplateBody(
			runtime.RawExtension{Raw: []byte(`{"name":"${invalid syntax !!!}"}`)},
			validator, env, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "invalid CEL expression")
	})
}

func TestWalkAndValidateCEL(t *testing.T) {
	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{})
	require.NoError(t, err)
	env := validator.GetBaseEnv()
	basePath := field.NewPath("template")

	t.Run("string with valid CEL", func(t *testing.T) {
		errs := walkAndValidateCEL("${metadata.name}", basePath, validator, env)
		assert.Empty(t, errs)
	})

	t.Run("string with invalid CEL", func(t *testing.T) {
		errs := walkAndValidateCEL("${bad syntax !!!}", basePath, validator, env)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "invalid CEL expression")
	})

	t.Run("map with CEL value", func(t *testing.T) {
		data := map[string]any{
			"name": "${metadata.name}",
		}
		errs := walkAndValidateCEL(data, basePath, validator, env)
		assert.Empty(t, errs)
	})

	t.Run("map with CEL key valid", func(t *testing.T) {
		data := map[string]any{
			"${metadata.name}": "value",
		}
		errs := walkAndValidateCEL(data, basePath, validator, env)
		assert.Empty(t, errs)
	})

	t.Run("map with invalid CEL key", func(t *testing.T) {
		data := map[string]any{
			"${bad syntax !!!}": "value",
		}
		errs := walkAndValidateCEL(data, basePath, validator, env)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "invalid CEL in map key")
	})

	t.Run("array of CEL values", func(t *testing.T) {
		data := []any{"${metadata.name}", "${metadata.namespace}"}
		errs := walkAndValidateCEL(data, basePath, validator, env)
		assert.Empty(t, errs)
	})

	t.Run("primitive types are no-op", func(t *testing.T) {
		// Numbers, bools, nil should not produce errors
		errs := walkAndValidateCEL(42.0, basePath, validator, env)
		assert.Empty(t, errs)
		errs = walkAndValidateCEL(true, basePath, validator, env)
		assert.Empty(t, errs)
		errs = walkAndValidateCEL(nil, basePath, validator, env)
		assert.Empty(t, errs)
	})

	t.Run("nested structure with mixed content", func(t *testing.T) {
		data := map[string]any{
			"spec": map[string]any{
				"replicas": "${metadata.name}",
				"ports":    []any{"${metadata.namespace}"},
			},
		}
		errs := walkAndValidateCEL(data, basePath, validator, env)
		assert.Empty(t, errs)
	})
}
