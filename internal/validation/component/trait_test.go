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

func TestValidateClusterTraitCreatesAndPatchesWithSchema(t *testing.T) {
	tests := []struct {
		name      string
		ct        *v1alpha1.ClusterTrait
		wantError bool
		errMsg    string
	}{
		{
			name: "valid creates and patches",
			ct: &v1alpha1.ClusterTrait{
				Spec: v1alpha1.ClusterTraitSpec{
					Creates: []v1alpha1.TraitCreate{
						{
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "test"}}`),
							},
						},
					},
					Patches: []v1alpha1.TraitPatch{
						{
							Target: v1alpha1.PatchTarget{
								Group:   "apps",
								Version: "v1",
								Kind:    "Deployment",
							},
							Operations: []v1alpha1.JSONPatchOperation{
								{Op: "add", Path: "/metadata/labels/foo", Value: &runtime.RawExtension{Raw: []byte(`"bar"`)}},
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "empty creates and patches is valid",
			ct: &v1alpha1.ClusterTrait{
				Spec: v1alpha1.ClusterTraitSpec{},
			},
			wantError: false,
		},
		{
			name: "invalid CEL in creates template",
			ct: &v1alpha1.ClusterTrait{
				Spec: v1alpha1.ClusterTraitSpec{
					Creates: []v1alpha1.TraitCreate{
						{
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "${parameters.name +}"}}`),
							},
						},
					},
				},
			},
			wantError: true,
			errMsg:    "invalid CEL expression",
		},
		{
			name: "invalid CEL in patch value",
			ct: &v1alpha1.ClusterTrait{
				Spec: v1alpha1.ClusterTraitSpec{
					Patches: []v1alpha1.TraitPatch{
						{
							Target: v1alpha1.PatchTarget{
								Group:   "apps",
								Version: "v1",
								Kind:    "Deployment",
							},
							Operations: []v1alpha1.JSONPatchOperation{
								{Op: "add", Path: "/metadata/labels/foo", Value: &runtime.RawExtension{Raw: []byte(`"${parameters.name +}"`)}},
							},
						},
					},
				},
			},
			wantError: true,
			errMsg:    "invalid CEL expression",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateClusterTraitCreatesAndPatchesWithSchema(tt.ct, nil, nil)
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
