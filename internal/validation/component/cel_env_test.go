// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
)

func TestBuildComponentCELEnv_WithParametersSchema(t *testing.T) {
	// Build a structural schema for parameters
	structural := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"replicas": {Generic: apiextschema.Generic{Type: "integer"}},
			"port":     {Generic: apiextschema.Generic{Type: "integer"}},
		},
	}

	env, err := BuildComponentCELEnv(SchemaOptions{
		ParametersSchema: structural,
	})
	require.NoError(t, err)
	require.NotNil(t, env)

	// Valid expression should compile
	ast, issues := env.Compile("parameters.replicas")
	assert.Nil(t, issues.Err())
	assert.NotNil(t, ast)

	// Invalid field should fail
	_, issues = env.Compile("parameters.invalidField")
	assert.NotNil(t, issues.Err())
	assert.Contains(t, issues.Err().Error(), "undefined field")
}

func TestBuildComponentCELEnv_WithEnvOverridesSchema(t *testing.T) {
	// Build a structural schema for envOverrides with nested structure
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
				},
			},
		},
	}

	env, err := BuildComponentCELEnv(SchemaOptions{
		EnvOverridesSchema: structural,
	})
	require.NoError(t, err)
	require.NotNil(t, env)

	// Valid nested field access should work
	ast, issues := env.Compile("envOverrides.resources.limits.cpu")
	assert.Nil(t, issues.Err())
	assert.NotNil(t, ast)

	// Invalid nested field should fail
	_, issues = env.Compile("envOverrides.resources.limits.invalidField")
	assert.NotNil(t, issues.Err())
	assert.Contains(t, issues.Err().Error(), "undefined field")
}

func TestBuildComponentCELEnv_WithoutSchema(t *testing.T) {
	// Without schema, parameters and envOverrides should be empty objects
	env, err := BuildComponentCELEnv(SchemaOptions{})
	require.NoError(t, err)
	require.NotNil(t, env)

	// With empty object, any field access should fail
	_, issues := env.Compile("parameters.anyField")
	assert.NotNil(t, issues.Err())
	assert.Contains(t, issues.Err().Error(), "undefined field")

	_, issues = env.Compile("envOverrides.anyField")
	assert.NotNil(t, issues.Err())
	assert.Contains(t, issues.Err().Error(), "undefined field")
}

func TestBuildComponentCELEnv_OtherVariables(t *testing.T) {
	// Other variables (metadata, workload, etc.) should be accessible
	env, err := BuildComponentCELEnv(SchemaOptions{})
	require.NoError(t, err)
	require.NotNil(t, env)

	// These variables use MapType(StringType, DynType) so any field access is allowed
	testCases := []string{
		"metadata.name",
		"metadata.namespace",
		"workload.container",
		"configurations.configs",
		"dataplane.secretStore",
	}

	for _, expr := range testCases {
		t.Run(expr, func(t *testing.T) {
			ast, issues := env.Compile(expr)
			assert.Nil(t, issues.Err(), "Expression '%s' should compile: %v", expr, issues)
			assert.NotNil(t, ast)
		})
	}
}

func TestBuildComponentCELEnv_CustomFunctions(t *testing.T) {
	// Verify custom functions are available
	env, err := BuildComponentCELEnv(SchemaOptions{})
	require.NoError(t, err)
	require.NotNil(t, env)

	// Test OpenChoreo custom functions
	testCases := []string{
		`oc_merge({}, {})`,
		`oc_omit()`,
		`oc_generate_name("prefix")`,
	}

	for _, expr := range testCases {
		t.Run(expr, func(t *testing.T) {
			ast, issues := env.Compile(expr)
			assert.Nil(t, issues.Err(), "Expression '%s' should compile: %v", expr, issues)
			assert.NotNil(t, ast)
		})
	}
}

func TestBuildTraitCELEnv_WithParametersSchema(t *testing.T) {
	structural := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"volumeName":    {Generic: apiextschema.Generic{Type: "string"}},
			"mountPath":     {Generic: apiextschema.Generic{Type: "string"}},
			"containerName": {Generic: apiextschema.Generic{Type: "string"}},
		},
	}

	env, err := BuildTraitCELEnv(SchemaOptions{
		ParametersSchema: structural,
	})
	require.NoError(t, err)
	require.NotNil(t, env)

	// Valid field should work
	ast, issues := env.Compile("parameters.volumeName")
	assert.Nil(t, issues.Err())
	assert.NotNil(t, ast)

	// Invalid field should fail
	_, issues = env.Compile("parameters.badField")
	assert.NotNil(t, issues.Err())
	assert.Contains(t, issues.Err().Error(), "undefined field")
}

func TestBuildTraitCELEnv_AllVariables(t *testing.T) {
	// Traits should have access to all variables including workload and configurations
	env, err := BuildTraitCELEnv(SchemaOptions{})
	require.NoError(t, err)
	require.NotNil(t, env)

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
			ast, issues := env.Compile(expr)
			assert.Nil(t, issues.Err(), "Expression '%s' should compile: %v", expr, issues)
			assert.NotNil(t, ast)
		})
	}
}

func TestBuildTraitCELEnv_WithoutSchema(t *testing.T) {
	// Without schema, parameters and envOverrides should be empty objects
	env, err := BuildTraitCELEnv(SchemaOptions{})
	require.NoError(t, err)
	require.NotNil(t, env)

	// With empty object, any field access should fail
	_, issues := env.Compile("parameters.anyField")
	assert.NotNil(t, issues.Err())
	assert.Contains(t, issues.Err().Error(), "undefined field")

	_, issues = env.Compile("envOverrides.anyField")
	assert.NotNil(t, issues.Err())
	assert.Contains(t, issues.Err().Error(), "undefined field")
}

func TestBuildComponentCELEnv_ReflectionBasedTypes(t *testing.T) {
	env, err := BuildComponentCELEnv(SchemaOptions{})
	require.NoError(t, err)

	tests := []struct {
		name       string
		expression string
		wantErr    bool
		errMsg     string
	}{
		// Valid metadata field access
		{"valid metadata.name", "metadata.name", false, ""},
		{"valid metadata.componentName", "metadata.componentName", false, ""},
		{"valid metadata.namespace", "metadata.namespace", false, ""},
		{"valid metadata.componentNamespace", "metadata.componentNamespace", false, ""},
		{"valid metadata.labels", "metadata.labels", false, ""},
		{"valid metadata.podSelectors", "metadata.podSelectors", false, ""},

		// Invalid metadata field access
		{"invalid metadata.invalidField", "metadata.invalidField", true, "undefined field"},
		{"invalid metadata.notAField", "metadata.notAField", true, "undefined field"},

		// Valid dataplane field access
		{"valid dataplane.secretStore", "dataplane.secretStore", false, ""},
		{"valid dataplane.publicVirtualHost", "dataplane.publicVirtualHost", false, ""},

		// Invalid dataplane field access
		{"invalid dataplane.badField", "dataplane.badField", true, "undefined field"},
		{"invalid dataplane.notExists", "dataplane.notExists", true, "undefined field"},

		// Valid workload field access
		{"valid workload.container", "workload.container", false, ""},

		// Invalid workload field access
		{"invalid workload.badField", "workload.badField", true, "undefined field"},

		// Valid configurations access (struct type with fixed fields)
		{"valid configurations.configs access", `configurations.configs`, false, ""},
		{"valid configurations.secrets access", `configurations.secrets`, false, ""},

		// Nested access through struct
		{"valid nested workload", `workload.container.image`, false, ""},
		{"invalid nested workload", `workload.container.invalidField`, true, "undefined field"},
		{"invalid configurations field", `configurations.invalidField`, true, "undefined field"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, issues := env.Compile(tt.expression)
			if tt.wantErr {
				assert.NotNil(t, issues.Err(), "expected error for %s", tt.expression)
				if tt.errMsg != "" {
					assert.Contains(t, issues.Err().Error(), tt.errMsg, "error message should contain: %s", tt.errMsg)
				}
			} else {
				assert.Nil(t, issues.Err(), "unexpected error for %s: %v", tt.expression, issues.Err())
			}
		})
	}
}

func TestBuildTraitCELEnv_ReflectionBasedTypes(t *testing.T) {
	env, err := BuildTraitCELEnv(SchemaOptions{})
	require.NoError(t, err)

	tests := []struct {
		name       string
		expression string
		wantErr    bool
		errMsg     string
	}{
		// Valid trait field access
		{"valid trait.name", "trait.name", false, ""},
		{"valid trait.instanceName", "trait.instanceName", false, ""},

		// Invalid trait field access
		{"invalid trait.badField", "trait.badField", true, "undefined field"},

		// Valid metadata access
		{"valid metadata.namespace", "metadata.namespace", false, ""},
		{"valid metadata.componentName", "metadata.componentName", false, ""},

		// Invalid metadata access
		{"invalid metadata.badField", "metadata.badField", true, "undefined field"},

		// Valid dataplane access
		{"valid dataplane.secretStore", "dataplane.secretStore", false, ""},

		// Invalid dataplane access
		{"invalid dataplane.badField", "dataplane.badField", true, "undefined field"},

		// Trait has access to workload
		{"valid workload.container", "workload.container", false, ""},
		{"valid workload.endpoints", "workload.endpoints", false, ""},

		// Trait has access to configurations
		{"valid configurations", "configurations", false, ""},
		{"valid configurations.configs", "configurations.configs", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, issues := env.Compile(tt.expression)
			if tt.wantErr {
				assert.NotNil(t, issues.Err(), "expected error for %s", tt.expression)
				if tt.errMsg != "" {
					assert.Contains(t, issues.Err().Error(), tt.errMsg, "error message should contain: %s", tt.errMsg)
				}
			} else {
				assert.Nil(t, issues.Err(), "unexpected error for %s: %v", tt.expression, issues.Err())
			}
		})
	}
}
