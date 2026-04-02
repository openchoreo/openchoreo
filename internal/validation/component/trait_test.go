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

func TestValidatePatchTarget_WhereClause_ResourceVariable(t *testing.T) {
	// Create a basic validator for trait context
	validator, err := NewCELValidator(TraitResource, SchemaOptions{})
	require.NoError(t, err)

	env := validator.GetBaseEnv()
	basePath := field.NewPath("spec", "patches").Index(0).Child("target")

	tests := []struct {
		name      string
		target    v1alpha1.PatchTarget
		wantError bool
		errMsg    string
	}{
		{
			// resource is typed as dyn, so any field access is allowed at validation time.
			name: "resource variable is available in where clause",
			target: v1alpha1.PatchTarget{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
				Where:   "${resource.metadata.name.endsWith('-http')}",
			},
			wantError: false,
		},
		{
			name: "undeclared variable in where clause fails",
			target: v1alpha1.PatchTarget{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
				Where:   "${unknownVar.metadata.name == 'test'}",
			},
			wantError: true,
			errMsg:    "undeclared reference to 'unknownVar'",
		},
		{
			name: "target without where clause",
			target: v1alpha1.PatchTarget{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			},
			wantError: false,
		},
		{
			name: "missing required field - version",
			target: v1alpha1.PatchTarget{
				Group: "apps",
				Kind:  "Deployment",
			},
			wantError: true,
			errMsg:    "version",
		},
		{
			name: "missing required field - kind",
			target: v1alpha1.PatchTarget{
				Group:   "apps",
				Version: "v1",
			},
			wantError: true,
			errMsg:    "kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validatePatchTarget(tt.target, validator, env, basePath)

			if tt.wantError {
				assert.NotEmpty(t, errs, "expected validation error")
				if tt.errMsg != "" {
					found := false
					for _, err := range errs {
						if assert.Contains(t, err.Error(), tt.errMsg) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected error containing %q, got %v", tt.errMsg, errs)
					}
				}
			} else {
				assert.Empty(t, errs, "unexpected validation errors: %v", errs)
			}
		})
	}
}

func TestValidatePatchTarget_WhereClause_WithSchema(t *testing.T) {
	// Create validator with a schema for parameters
	parametersSchema := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"endpointName": {Generic: apiextschema.Generic{Type: "string"}},
			"tier":         {Generic: apiextschema.Generic{Type: "string"}},
		},
	}

	validator, err := NewCELValidator(TraitResource, SchemaOptions{
		ParametersSchema: parametersSchema,
	})
	require.NoError(t, err)

	env := validator.GetBaseEnv()
	basePath := field.NewPath("spec", "patches").Index(0).Child("target")

	tests := []struct {
		name      string
		target    v1alpha1.PatchTarget
		wantError bool
		errMsg    string
	}{
		{
			name: "valid where with resource and typed parameter",
			target: v1alpha1.PatchTarget{
				Group:   "gateway.networking.k8s.io",
				Version: "v1",
				Kind:    "HTTPRoute",
				Where:   "${resource.metadata.name.endsWith('-' + parameters.endpointName)}",
			},
			wantError: false,
		},
		{
			name: "invalid where - undefined parameter field",
			target: v1alpha1.PatchTarget{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
				Where:   "${resource.metadata.name == parameters.nonExistentField}",
			},
			wantError: true,
			errMsg:    "undefined field 'nonExistentField'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validatePatchTarget(tt.target, validator, env, basePath)

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

func TestValidateTraitPatch(t *testing.T) {
	parametersSchema := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"endpoints": {
				Generic: apiextschema.Generic{Type: "array"},
				Items: &apiextschema.Structural{
					Generic: apiextschema.Generic{Type: "object"},
					Properties: map[string]apiextschema.Structural{
						"name": {Generic: apiextschema.Generic{Type: "string"}},
						"port": {Generic: apiextschema.Generic{Type: "integer"}},
					},
				},
			},
		},
	}

	validator, err := NewCELValidator(TraitResource, SchemaOptions{
		ParametersSchema: parametersSchema,
	})
	require.NoError(t, err)

	basePath := field.NewPath("spec", "patches").Index(0)

	// Helper to create a raw extension value
	makeValue := func(s string) *runtime.RawExtension {
		return &runtime.RawExtension{Raw: []byte(`"` + s + `"`)}
	}

	tests := []struct {
		name      string
		patch     v1alpha1.TraitPatch
		wantError bool
		errMsg    string
	}{
		{
			name: "valid forEach with where using resource",
			patch: v1alpha1.TraitPatch{
				ForEach: "${parameters.endpoints}",
				Var:     "ep",
				Target: v1alpha1.PatchTarget{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
					Where:   "${resource.metadata.labels.endpoint == ep.name}",
				},
				Operations: []v1alpha1.JSONPatchOperation{
					{Op: "add", Path: "/metadata/annotations/port", Value: makeValue("true")},
				},
			},
			wantError: false,
		},
		{
			name: "valid patch with where only (no forEach)",
			patch: v1alpha1.TraitPatch{
				Target: v1alpha1.PatchTarget{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
					Where:   "${resource.spec.replicas > 1}",
				},
				Operations: []v1alpha1.JSONPatchOperation{
					{Op: "add", Path: "/metadata/annotations/ha", Value: makeValue("true")},
				},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateTraitPatch(tt.patch, validator, basePath)

			if tt.wantError {
				assert.NotEmpty(t, errs, "expected validation error")
				if tt.errMsg != "" {
					found := false
					for _, err := range errs {
						if assert.Contains(t, err.Error(), tt.errMsg) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected error containing %q, got %v", tt.errMsg, errs)
					}
				}
			} else {
				assert.Empty(t, errs, "unexpected validation errors: %v", errs)
			}
		})
	}
}

func TestValidateValidationRule(t *testing.T) {
	parametersSchema := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"replicas": {Generic: apiextschema.Generic{Type: "integer"}},
			"name":     {Generic: apiextschema.Generic{Type: "string"}},
		},
	}

	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{
		ParametersSchema: parametersSchema,
	})
	require.NoError(t, err)

	basePath := field.NewPath("spec", "validations").Index(0)

	tests := []struct {
		name      string
		rule      v1alpha1.ValidationRule
		wantError bool
		errMsg    string
	}{
		{
			name: "valid boolean rule",
			rule: v1alpha1.ValidationRule{
				Rule:    "${parameters.replicas > 0}",
				Message: "replicas must be positive",
			},
			wantError: false,
		},
		{
			name: "rule not wrapped in ${...}",
			rule: v1alpha1.ValidationRule{
				Rule:    "parameters.replicas > 0",
				Message: "replicas must be positive",
			},
			wantError: true,
			errMsg:    "rule must be wrapped in ${...}",
		},
		{
			name: "rule returning string instead of boolean",
			rule: v1alpha1.ValidationRule{
				Rule:    "${parameters.name}",
				Message: "should fail",
			},
			wantError: true,
			errMsg:    "rule must return boolean",
		},
		{
			name: "rule with parse error",
			rule: v1alpha1.ValidationRule{
				Rule:    "${invalid syntax !!!}",
				Message: "should fail",
			},
			wantError: true,
			errMsg:    "rule must return boolean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateValidationRule(tt.rule, validator, basePath)

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

func TestValidateComponentTypeValidationRules(t *testing.T) {
	parametersSchema := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"replicas": {Generic: apiextschema.Generic{Type: "integer"}},
			"expose":   {Generic: apiextschema.Generic{Type: "boolean"}},
		},
	}

	t.Run("valid rules pass", func(t *testing.T) {
		ct := &v1alpha1.ComponentType{
			Spec: v1alpha1.ComponentTypeSpec{
				Validations: []v1alpha1.ValidationRule{
					{Rule: "${parameters.replicas > 0}", Message: "replicas must be positive"},
					{Rule: "${parameters.expose == true}", Message: "must expose"},
				},
				Resources: []v1alpha1.ResourceTemplate{
					{
						ID:       "deployment",
						Template: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"x"}}`)},
					},
				},
			},
		}

		errs := ValidateComponentTypeResourcesWithSchema(ct, parametersSchema, nil)
		assert.Empty(t, errs, "unexpected validation errors: %v", errs)
	})

	t.Run("non-boolean rule rejected", func(t *testing.T) {
		ct := &v1alpha1.ComponentType{
			Spec: v1alpha1.ComponentTypeSpec{
				Validations: []v1alpha1.ValidationRule{
					{Rule: "${parameters.replicas}", Message: "returns int not bool"},
				},
				Resources: []v1alpha1.ResourceTemplate{
					{
						ID:       "deployment",
						Template: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"x"}}`)},
					},
				},
			},
		}

		errs := ValidateComponentTypeResourcesWithSchema(ct, parametersSchema, nil)
		assert.NotEmpty(t, errs, "expected validation error")
		assert.Contains(t, errs.ToAggregate().Error(), "rule must return boolean")
	})
}

func TestValidateTraitValidationRules(t *testing.T) {
	parametersSchema := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"size": {Generic: apiextschema.Generic{Type: "integer"}},
		},
	}

	t.Run("valid rules pass", func(t *testing.T) {
		trait := &v1alpha1.Trait{
			Spec: v1alpha1.TraitSpec{
				Validations: []v1alpha1.ValidationRule{
					{Rule: "${parameters.size > 0}", Message: "size must be positive"},
				},
			},
		}

		errs := ValidateTraitCreatesAndPatchesWithSchema(trait, parametersSchema, nil)
		assert.Empty(t, errs, "unexpected validation errors: %v", errs)
	})

	t.Run("non-boolean rule rejected", func(t *testing.T) {
		trait := &v1alpha1.Trait{
			Spec: v1alpha1.TraitSpec{
				Validations: []v1alpha1.ValidationRule{
					{Rule: "${parameters.size}", Message: "returns int not bool"},
				},
			},
		}

		errs := ValidateTraitCreatesAndPatchesWithSchema(trait, parametersSchema, nil)
		assert.NotEmpty(t, errs, "expected validation error")
		assert.Contains(t, errs.ToAggregate().Error(), "rule must return boolean")
	})
}

func TestValidateClusterTraitCreatesAndPatchesWithSchema(t *testing.T) {
	parametersSchema := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"volumeName": {Generic: apiextschema.Generic{Type: "string"}},
			"mountPath":  {Generic: apiextschema.Generic{Type: "string"}},
		},
	}

	environmentConfigsSchema := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"size": {Generic: apiextschema.Generic{Type: "string"}},
		},
	}

	t.Run("valid creates pass", func(t *testing.T) {
		ct := &v1alpha1.ClusterTrait{
			Spec: v1alpha1.ClusterTraitSpec{
				Creates: []v1alpha1.TraitCreate{
					{
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": {"name": "${metadata.name}-pvc"}, "spec": {"resources": {"requests": {"storage": "${environmentConfigs.size}"}}}}`),
						},
					},
				},
			},
		}

		errs := ValidateClusterTraitCreatesAndPatchesWithSchema(ct, parametersSchema, environmentConfigsSchema)
		assert.Empty(t, errs, "unexpected validation errors: %v", errs)
	})

	t.Run("valid patches pass", func(t *testing.T) {
		ct := &v1alpha1.ClusterTrait{
			Spec: v1alpha1.ClusterTraitSpec{
				Patches: []v1alpha1.TraitPatch{
					{
						Target: v1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Operations: []v1alpha1.JSONPatchOperation{
							{
								Op:    "add",
								Path:  "/spec/template/spec/volumes/-",
								Value: &runtime.RawExtension{Raw: []byte(`{"name": "${parameters.volumeName}"}`)},
							},
						},
					},
				},
			},
		}

		errs := ValidateClusterTraitCreatesAndPatchesWithSchema(ct, parametersSchema, nil)
		assert.Empty(t, errs, "unexpected validation errors: %v", errs)
	})

	t.Run("invalid CEL in creates rejected", func(t *testing.T) {
		ct := &v1alpha1.ClusterTrait{
			Spec: v1alpha1.ClusterTraitSpec{
				Creates: []v1alpha1.TraitCreate{
					{
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "test"}, "data": {"key": "${parameters.volumeName +}"}}`),
						},
					},
				},
			},
		}

		errs := ValidateClusterTraitCreatesAndPatchesWithSchema(ct, parametersSchema, nil)
		assert.NotEmpty(t, errs, "expected validation error")
		assert.Contains(t, errs.ToAggregate().Error(), "invalid CEL expression")
	})

	t.Run("invalid CEL in patches rejected", func(t *testing.T) {
		ct := &v1alpha1.ClusterTrait{
			Spec: v1alpha1.ClusterTraitSpec{
				Patches: []v1alpha1.TraitPatch{
					{
						Target: v1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Operations: []v1alpha1.JSONPatchOperation{
							{
								Op:    "add",
								Path:  "/metadata/annotations/key",
								Value: &runtime.RawExtension{Raw: []byte(`"${parameters.volumeName +}"`)},
							},
						},
					},
				},
			},
		}

		errs := ValidateClusterTraitCreatesAndPatchesWithSchema(ct, parametersSchema, nil)
		assert.NotEmpty(t, errs, "expected validation error")
		assert.Contains(t, errs.ToAggregate().Error(), "invalid CEL expression")
	})

	t.Run("empty spec is valid", func(t *testing.T) {
		ct := &v1alpha1.ClusterTrait{
			Spec: v1alpha1.ClusterTraitSpec{},
		}

		errs := ValidateClusterTraitCreatesAndPatchesWithSchema(ct, nil, nil)
		assert.Empty(t, errs, "unexpected validation errors: %v", errs)
	})
}

func TestValidateTraitSpec_CustomBasePath(t *testing.T) {
	t.Run("errors use custom basePath", func(t *testing.T) {
		customBase := field.NewPath("spec", "traits").Index(0).Child("spec")
		spec := v1alpha1.TraitSpec{
			Creates: []v1alpha1.TraitCreate{
				{Template: nil},
			},
		}
		errs := ValidateTraitSpec(spec, nil, nil, customBase)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Field, "spec.traits[0].spec.creates")
	})

	t.Run("empty spec no errors", func(t *testing.T) {
		errs := ValidateTraitSpec(v1alpha1.TraitSpec{}, nil, nil, field.NewPath("spec"))
		assert.Empty(t, errs)
	})

	t.Run("basePath propagates to validations", func(t *testing.T) {
		customBase := field.NewPath("spec", "traits").Index(0).Child("spec")
		spec := v1alpha1.TraitSpec{
			Validations: []v1alpha1.ValidationRule{
				{Rule: "not-wrapped", Message: "test"},
			},
		}
		errs := ValidateTraitSpec(spec, nil, nil, customBase)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Field, "spec.traits[0].spec.validations")
	})
}

func TestValidateTraitCreate_ErrorPaths(t *testing.T) {
	validator, err := NewCELValidator(TraitResource, SchemaOptions{})
	require.NoError(t, err)
	basePath := field.NewPath("spec", "creates").Index(0)

	t.Run("includeWhen not wrapped", func(t *testing.T) {
		create := v1alpha1.TraitCreate{
			IncludeWhen: "true",
			Template: &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
			},
		}
		errs := validateTraitCreate(create, validator, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "includeWhen must be a template expression wrapped with ${...}")
	})

	t.Run("includeWhen non-boolean", func(t *testing.T) {
		parametersSchema := &apiextschema.Structural{
			Generic: apiextschema.Generic{Type: "object"},
			Properties: map[string]apiextschema.Structural{
				"name": {Generic: apiextschema.Generic{Type: "string"}},
			},
		}
		v, err := NewCELValidator(TraitResource, SchemaOptions{ParametersSchema: parametersSchema})
		require.NoError(t, err)

		create := v1alpha1.TraitCreate{
			IncludeWhen: "${parameters.name}",
			Template: &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
			},
		}
		errs := validateTraitCreate(create, v, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "includeWhen must return boolean")
	})

	t.Run("forEach not wrapped", func(t *testing.T) {
		create := v1alpha1.TraitCreate{
			ForEach: "parameters.items",
			Template: &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
			},
		}
		errs := validateTraitCreate(create, validator, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "forEach must be a template expression wrapped with ${...}")
	})

	t.Run("nil template", func(t *testing.T) {
		create := v1alpha1.TraitCreate{Template: nil}
		errs := validateTraitCreate(create, validator, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "template is required")
	})

	t.Run("valid create with all fields", func(t *testing.T) {
		create := v1alpha1.TraitCreate{
			IncludeWhen: "${true}",
			Template: &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"${metadata.name}"}}`),
			},
		}
		errs := validateTraitCreate(create, validator, basePath)
		assert.Empty(t, errs)
	})

	t.Run("valid forEach list with template using loop var", func(t *testing.T) {
		create := v1alpha1.TraitCreate{
			ForEach: `${["a","b","c"]}`,
			Var:     "item",
			Template: &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"${item}"}}`),
			},
		}
		errs := validateTraitCreate(create, validator, basePath)
		assert.Empty(t, errs)
	})

	t.Run("valid forEach map with template using loop var", func(t *testing.T) {
		create := v1alpha1.TraitCreate{
			ForEach: `${{"k1":"v1","k2":"v2"}}`,
			Var:     "entry",
			Template: &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"${entry.key}"}}`),
			},
		}
		errs := validateTraitCreate(create, validator, basePath)
		assert.Empty(t, errs)
	})

	t.Run("forEach with invalid iterable expression", func(t *testing.T) {
		parametersSchema := &apiextschema.Structural{
			Generic: apiextschema.Generic{Type: "object"},
			Properties: map[string]apiextschema.Structural{
				"count": {Generic: apiextschema.Generic{Type: "integer"}},
			},
		}
		v, err := NewCELValidator(TraitResource, SchemaOptions{ParametersSchema: parametersSchema})
		require.NoError(t, err)

		create := v1alpha1.TraitCreate{
			ForEach: "${parameters.count}",
			Template: &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
			},
		}
		errs := validateTraitCreate(create, v, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "forEach expression must return list or map")
	})

	t.Run("includeWhen must not reference forEach loop variable", func(t *testing.T) {
		parametersSchema := &apiextschema.Structural{
			Generic: apiextschema.Generic{Type: "object"},
			Properties: map[string]apiextschema.Structural{
				"volumes": {
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
		v, err := NewCELValidator(TraitResource, SchemaOptions{ParametersSchema: parametersSchema})
		require.NoError(t, err)

		create := v1alpha1.TraitCreate{
			ForEach:     `${parameters.volumes}`,
			Var:         "vol",
			IncludeWhen: `${vol.enabled}`,
			Template: &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"v1","kind":"PersistentVolumeClaim","metadata":{"name":"${vol.name}"}}`),
			},
		}
		errs := validateTraitCreate(create, v, basePath)
		require.NotEmpty(t, errs)
		errStr := errs.ToAggregate().Error()
		assert.Contains(t, errStr, "includeWhen")
		assert.Contains(t, errStr, "undeclared reference to 'vol'")
	})
}

func TestValidatePatchOperation(t *testing.T) {
	validator, err := NewCELValidator(TraitResource, SchemaOptions{})
	require.NoError(t, err)
	env := validator.GetBaseEnv()
	basePath := field.NewPath("spec", "patches").Index(0).Child("operations").Index(0)

	makeValue := func(s string) *runtime.RawExtension {
		return &runtime.RawExtension{Raw: []byte(s)}
	}

	t.Run("valid add with value", func(t *testing.T) {
		op := v1alpha1.JSONPatchOperation{
			Op: "add", Path: "/metadata/annotations/key", Value: makeValue(`"test"`),
		}
		errs := validatePatchOperation(op, validator, env, basePath)
		assert.Empty(t, errs)
	})

	t.Run("valid replace with value", func(t *testing.T) {
		op := v1alpha1.JSONPatchOperation{
			Op: "replace", Path: "/spec/replicas", Value: makeValue(`3`),
		}
		errs := validatePatchOperation(op, validator, env, basePath)
		assert.Empty(t, errs)
	})

	t.Run("valid remove without value", func(t *testing.T) {
		op := v1alpha1.JSONPatchOperation{
			Op: "remove", Path: "/metadata/annotations/key",
		}
		errs := validatePatchOperation(op, validator, env, basePath)
		assert.Empty(t, errs)
	})

	t.Run("invalid op", func(t *testing.T) {
		op := v1alpha1.JSONPatchOperation{
			Op: "merge", Path: "/spec/replicas", Value: makeValue(`3`),
		}
		errs := validatePatchOperation(op, validator, env, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "invalid patch operation")
	})

	t.Run("empty path", func(t *testing.T) {
		op := v1alpha1.JSONPatchOperation{
			Op: "add", Path: "", Value: makeValue(`"test"`),
		}
		errs := validatePatchOperation(op, validator, env, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "patch path is required")
	})

	t.Run("remove with value rejected", func(t *testing.T) {
		op := v1alpha1.JSONPatchOperation{
			Op: "remove", Path: "/metadata/annotations/key", Value: makeValue(`"test"`),
		}
		errs := validatePatchOperation(op, validator, env, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "value should not be specified")
	})

	t.Run("add without value rejected", func(t *testing.T) {
		op := v1alpha1.JSONPatchOperation{
			Op: "add", Path: "/metadata/annotations/key",
		}
		errs := validatePatchOperation(op, validator, env, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "value is required for 'add' operation")
	})

	t.Run("replace without value rejected", func(t *testing.T) {
		op := v1alpha1.JSONPatchOperation{
			Op: "replace", Path: "/spec/replicas",
		}
		errs := validatePatchOperation(op, validator, env, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "value is required for 'replace' operation")
	})

	t.Run("value with CEL expression validated", func(t *testing.T) {
		op := v1alpha1.JSONPatchOperation{
			Op: "add", Path: "/metadata/annotations/key", Value: makeValue(`"${metadata.name}"`),
		}
		errs := validatePatchOperation(op, validator, env, basePath)
		assert.Empty(t, errs)
	})

	t.Run("value with invalid CEL expression", func(t *testing.T) {
		op := v1alpha1.JSONPatchOperation{
			Op: "add", Path: "/metadata/annotations/key", Value: makeValue(`"${bad syntax !!!}"`),
		}
		errs := validatePatchOperation(op, validator, env, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "invalid CEL expression")
	})
}

func TestValidateTraitPatch_ForEachErrors(t *testing.T) {
	validator, err := NewCELValidator(TraitResource, SchemaOptions{})
	require.NoError(t, err)
	basePath := field.NewPath("spec", "patches").Index(0)

	makeValue := func(s string) *runtime.RawExtension {
		return &runtime.RawExtension{Raw: []byte(s)}
	}

	t.Run("forEach not wrapped", func(t *testing.T) {
		patch := v1alpha1.TraitPatch{
			ForEach: "parameters.items",
			Target: v1alpha1.PatchTarget{
				Group: "apps", Version: "v1", Kind: "Deployment",
			},
			Operations: []v1alpha1.JSONPatchOperation{
				{Op: "add", Path: "/metadata/annotations/key", Value: makeValue(`"test"`)},
			},
		}
		errs := validateTraitPatch(patch, validator, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "forEach must be a template expression wrapped with ${...}")
	})

	t.Run("valid patch without forEach", func(t *testing.T) {
		patch := v1alpha1.TraitPatch{
			Target: v1alpha1.PatchTarget{
				Group: "apps", Version: "v1", Kind: "Deployment",
			},
			Operations: []v1alpha1.JSONPatchOperation{
				{Op: "add", Path: "/metadata/annotations/key", Value: makeValue(`"test"`)},
			},
		}
		errs := validateTraitPatch(patch, validator, basePath)
		assert.Empty(t, errs)
	})

	t.Run("valid forEach list with patch operations", func(t *testing.T) {
		patch := v1alpha1.TraitPatch{
			ForEach: `${["port1","port2"]}`,
			Var:     "portName",
			Target: v1alpha1.PatchTarget{
				Group: "apps", Version: "v1", Kind: "Deployment",
			},
			Operations: []v1alpha1.JSONPatchOperation{
				{Op: "add", Path: "/metadata/annotations/port", Value: makeValue(`"${portName}"`)},
			},
		}
		errs := validateTraitPatch(patch, validator, basePath)
		assert.Empty(t, errs)
	})

	t.Run("forEach with invalid iterable in patch", func(t *testing.T) {
		parametersSchema := &apiextschema.Structural{
			Generic: apiextschema.Generic{Type: "object"},
			Properties: map[string]apiextschema.Structural{
				"count": {Generic: apiextschema.Generic{Type: "integer"}},
			},
		}
		v, err := NewCELValidator(TraitResource, SchemaOptions{ParametersSchema: parametersSchema})
		require.NoError(t, err)

		patch := v1alpha1.TraitPatch{
			ForEach: "${parameters.count}",
			Target: v1alpha1.PatchTarget{
				Group: "apps", Version: "v1", Kind: "Deployment",
			},
			Operations: []v1alpha1.JSONPatchOperation{
				{Op: "add", Path: "/metadata/annotations/key", Value: makeValue(`"test"`)},
			},
		}
		errs := validateTraitPatch(patch, v, basePath)
		require.NotEmpty(t, errs)
		assert.Contains(t, errs.ToAggregate().Error(), "forEach expression must return list or map")
	})
}

func TestValidatePatchTarget_WhereClauseVariants(t *testing.T) {
	validator, err := NewCELValidator(TraitResource, SchemaOptions{})
	require.NoError(t, err)
	env := validator.GetBaseEnv()
	basePath := field.NewPath("spec", "patches").Index(0).Child("target")

	t.Run("where with raw CEL no template syntax", func(t *testing.T) {
		target := v1alpha1.PatchTarget{
			Group: "apps", Version: "v1", Kind: "Deployment",
			Where: "resource.metadata.name == 'test'",
		}
		errs := validatePatchTarget(target, validator, env, basePath)
		assert.Empty(t, errs)
	})

	t.Run("where returning non-boolean", func(t *testing.T) {
		target := v1alpha1.PatchTarget{
			Group: "apps", Version: "v1", Kind: "Deployment",
			Where: "${resource.metadata.name}",
		}
		errs := validatePatchTarget(target, validator, env, basePath)
		// resource is DynType, so field access returns DynType which is accepted as boolean
		// This is intentional permissiveness
		assert.Empty(t, errs)
	})
}

func TestExtractCELFromTemplate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantCEL string
		wantOK  bool
	}{
		{
			name:    "simple expression",
			input:   "${resource.metadata.name}",
			wantCEL: "resource.metadata.name",
			wantOK:  true,
		},
		{
			name:    "expression with spaces",
			input:   "${ resource.metadata.name == 'test' }",
			wantCEL: "resource.metadata.name == 'test'",
			wantOK:  true,
		},
		{
			name:    "expression with newlines",
			input:   "${resource.metadata.name.endsWith('-' + parameters.endpointName)}",
			wantCEL: "resource.metadata.name.endsWith('-' + parameters.endpointName)",
			wantOK:  true,
		},
		{
			name:    "not a template expression",
			input:   "plain string",
			wantCEL: "",
			wantOK:  false,
		},
		{
			name:    "missing closing brace",
			input:   "${resource.metadata.name",
			wantCEL: "",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCEL, gotOK := extractCELFromTemplate(tt.input)
			assert.Equal(t, tt.wantOK, gotOK)
			if tt.wantOK {
				assert.Equal(t, tt.wantCEL, gotCEL)
			}
		})
	}
}
