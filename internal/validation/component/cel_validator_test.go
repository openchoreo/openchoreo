// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
)

func TestCELValidator_SchemaAwareValidation_InvalidFieldAccess(t *testing.T) {
	structural := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"replicas": {Generic: apiextschema.Generic{Type: "integer"}},
		},
	}

	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{
		ParametersSchema: structural,
	})
	require.NoError(t, err)

	// This should fail - nonExistentField doesn't exist
	err = validator.ValidateExpression("parameters.nonExistentField")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "undefined field")

	// This should pass
	err = validator.ValidateExpression("parameters.replicas")
	assert.NoError(t, err)
}

func TestCELValidator_SchemaAwareValidation_NestedFields(t *testing.T) {
	structural := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"resources": {
				Generic: apiextschema.Generic{Type: "object"},
				Properties: map[string]apiextschema.Structural{
					"limits": {
						Generic: apiextschema.Generic{Type: "object"},
						Properties: map[string]apiextschema.Structural{
							"cpu":    {Generic: apiextschema.Generic{Type: "string"}},
							"memory": {Generic: apiextschema.Generic{Type: "string"}},
						},
					},
					"requests": {
						Generic: apiextschema.Generic{Type: "object"},
						Properties: map[string]apiextschema.Structural{
							"cpu":    {Generic: apiextschema.Generic{Type: "string"}},
							"memory": {Generic: apiextschema.Generic{Type: "string"}},
						},
					},
				},
			},
		},
	}

	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{
		EnvOverridesSchema: structural,
	})
	require.NoError(t, err)

	// Valid nested access
	err = validator.ValidateExpression("envOverrides.resources.limits.cpu")
	assert.NoError(t, err)

	err = validator.ValidateExpression("envOverrides.resources.requests.memory")
	assert.NoError(t, err)

	// Invalid nested field
	err = validator.ValidateExpression("envOverrides.resources.limits.invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "undefined field")

	// Invalid intermediate field
	err = validator.ValidateExpression("envOverrides.resources.invalid.cpu")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "undefined field")
}

func TestCELValidator_BackwardCompatibility_NilSchema(t *testing.T) {
	// Without schemas, should use empty objects
	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{})
	require.NoError(t, err)

	// With empty objects, any field access should fail
	err = validator.ValidateExpression("parameters.anyField")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "undefined field")

	err = validator.ValidateExpression("envOverrides.anyField")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "undefined field")
}

func TestCELValidator_BooleanExpression_Valid(t *testing.T) {
	structural := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"enabled": {Generic: apiextschema.Generic{Type: "boolean"}},
			"count":   {Generic: apiextschema.Generic{Type: "integer"}},
		},
	}

	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{
		ParametersSchema: structural,
	})
	require.NoError(t, err)

	env := validator.GetBaseEnv()

	// Valid boolean expressions
	testCases := []string{
		"parameters.enabled",
		"parameters.count > 0",
		"parameters.enabled && parameters.count > 5",
		"!parameters.enabled",
	}

	for _, expr := range testCases {
		t.Run(expr, func(t *testing.T) {
			err := validator.ValidateBooleanExpression(expr, env)
			assert.NoError(t, err, "Expression '%s' should be valid boolean", expr)
		})
	}
}

func TestCELValidator_BooleanExpression_Invalid(t *testing.T) {
	structural := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"count": {Generic: apiextschema.Generic{Type: "integer"}},
			"name":  {Generic: apiextschema.Generic{Type: "string"}},
		},
	}

	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{
		ParametersSchema: structural,
	})
	require.NoError(t, err)

	env := validator.GetBaseEnv()

	// Invalid boolean expressions (return non-boolean types)
	testCases := []struct {
		expr        string
		description string
	}{
		{"parameters.count", "integer is not boolean"},
		{"parameters.name", "string is not boolean"},
		{"parameters.count + 5", "arithmetic result is not boolean"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := validator.ValidateBooleanExpression(tc.expr, env)
			assert.Error(t, err, "Expression '%s' should fail: %s", tc.expr, tc.description)
		})
	}
}

func TestCELValidator_IterableExpression_Valid(t *testing.T) {
	structural := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"items": {
				Generic: apiextschema.Generic{Type: "array"},
				Items: &apiextschema.Structural{
					Generic: apiextschema.Generic{Type: "string"},
				},
			},
			"mappings": {
				Generic: apiextschema.Generic{Type: "object"},
				AdditionalProperties: &apiextschema.StructuralOrBool{
					Structural: &apiextschema.Structural{
						Generic: apiextschema.Generic{Type: "string"},
					},
				},
			},
		},
	}

	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{
		ParametersSchema: structural,
	})
	require.NoError(t, err)

	env := validator.GetBaseEnv()

	// Valid iterable expressions
	testCases := []string{
		"parameters.items",
		"parameters.mappings",
		`["a", "b", "c"]`,
		`{"key": "value"}`,
	}

	for _, expr := range testCases {
		t.Run(expr, func(t *testing.T) {
			err := validator.ValidateIterableExpression(expr, env)
			assert.NoError(t, err, "Expression '%s' should be iterable", expr)
		})
	}
}

func TestCELValidator_IterableExpression_Invalid(t *testing.T) {
	structural := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"count": {Generic: apiextschema.Generic{Type: "integer"}},
			"name":  {Generic: apiextschema.Generic{Type: "string"}},
		},
	}

	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{
		ParametersSchema: structural,
	})
	require.NoError(t, err)

	env := validator.GetBaseEnv()

	// Invalid iterable expressions (not list or map)
	testCases := []struct {
		expr        string
		description string
	}{
		{"parameters.count", "integer is not iterable"},
		{"parameters.name", "string is not iterable"},
		{"true", "boolean is not iterable"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := validator.ValidateIterableExpression(tc.expr, env)
			assert.Error(t, err, "Expression '%s' should fail: %s", tc.expr, tc.description)
		})
	}
}

func TestCELValidator_TraitResource_AllVariables(t *testing.T) {
	// Trait validator should allow access to all variables including workload and configurations
	validator, err := NewCELValidator(TraitResource, SchemaOptions{})
	require.NoError(t, err)

	// All trait context variables should be accessible
	validExprs := []string{
		"trait.instanceName",
		"metadata.name",
		"dataplane.secretStore",
		"workload.container",
		"workload.endpoints",
		"configurations.configs",
	}

	for _, expr := range validExprs {
		t.Run(expr, func(t *testing.T) {
			err := validator.ValidateExpression(expr)
			assert.NoError(t, err, "Trait should have access to '%s'", expr)
		})
	}
}

func TestCELValidator_ComponentTypeResource_AllVariables(t *testing.T) {
	// ComponentType validator should allow access to all variables
	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{})
	require.NoError(t, err)

	// All component context variables should be accessible
	validExprs := []string{
		"metadata.name",
		"metadata.namespace",
		"workload.container",
		"workload.endpoints",
		"configurations.configs",
		"dataplane.secretStore",
	}

	for _, expr := range validExprs {
		t.Run(expr, func(t *testing.T) {
			err := validator.ValidateExpression(expr)
			assert.NoError(t, err, "ComponentType should have access to '%s'", expr)
		})
	}
}

func TestCELValidator_WithoutSchema(t *testing.T) {
	// Without schemas, parameters and envOverrides are empty objects
	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{})
	require.NoError(t, err)

	// Without schemas, parameters and envOverrides are empty objects
	err = validator.ValidateExpression("parameters.anyField")
	assert.Error(t, err)

	// Other variables should work
	err = validator.ValidateExpression("metadata.name")
	assert.NoError(t, err)
}
