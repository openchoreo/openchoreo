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
			errs := ValidatePatchTarget(tt.target, validator, env, basePath)

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
			errs := ValidatePatchTarget(tt.target, validator, env, basePath)

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
			errs := ValidateTraitPatch(tt.patch, validator, basePath)

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
			errs := ValidateValidationRule(tt.rule, validator, basePath)

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
