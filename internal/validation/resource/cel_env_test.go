// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
)

func TestBuildResourceCELEnv_WithParametersSchema(t *testing.T) {
	structural := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"version":   {Generic: apiextschema.Generic{Type: "string"}},
			"storageGB": {Generic: apiextschema.Generic{Type: "integer"}},
		},
	}

	env, err := buildResourceCELEnv(SchemaOptions{ParametersSchema: structural})
	require.NoError(t, err)
	require.NotNil(t, env)

	_, issues := env.Compile("parameters.version")
	assert.Nil(t, issues.Err())
	_, issues = env.Compile("parameters.storageGB")
	assert.Nil(t, issues.Err())

	_, issues = env.Compile("parameters.invalidField")
	require.NotNil(t, issues)
	assert.NotNil(t, issues.Err())
	assert.Contains(t, issues.Err().Error(), "undefined field")
}

func TestBuildResourceCELEnv_WithEnvironmentConfigsSchema(t *testing.T) {
	structural := &apiextschema.Structural{
		Generic: apiextschema.Generic{Type: "object"},
		Properties: map[string]apiextschema.Structural{
			"tlsMode":   {Generic: apiextschema.Generic{Type: "string"}},
			"storageGB": {Generic: apiextschema.Generic{Type: "integer"}},
		},
	}

	env, err := buildResourceCELEnv(SchemaOptions{EnvironmentConfigsSchema: structural})
	require.NoError(t, err)

	_, issues := env.Compile("environmentConfigs.tlsMode")
	assert.Nil(t, issues.Err())

	_, issues = env.Compile("environmentConfigs.unknown")
	require.NotNil(t, issues)
	assert.NotNil(t, issues.Err())
}

func TestBuildResourceCELEnv_WithoutSchema(t *testing.T) {
	env, err := buildResourceCELEnv(SchemaOptions{})
	require.NoError(t, err)

	_, issues := env.Compile("parameters.anyField")
	require.NotNil(t, issues)
	assert.NotNil(t, issues.Err(), "unknown parameters field should error against empty schema")

	_, issues = env.Compile("environmentConfigs.anyField")
	require.NotNil(t, issues)
	assert.NotNil(t, issues.Err(), "unknown environmentConfigs field should error against empty schema")
}

func TestBuildResourceCELEnv_MetadataFields(t *testing.T) {
	env, err := buildResourceCELEnv(SchemaOptions{})
	require.NoError(t, err)

	okExprs := []string{
		"metadata.name",
		"metadata.namespace",
		"metadata.resourceNamespace",
		"metadata.resourceName",
		"metadata.resourceUID",
		"metadata.projectName",
		"metadata.projectUID",
		"metadata.environmentName",
		"metadata.environmentUID",
		"metadata.dataPlaneName",
		"metadata.dataPlaneUID",
		"metadata.labels",
		"metadata.annotations",
		`metadata.labels["app.kubernetes.io/name"]`,
	}
	for _, expr := range okExprs {
		t.Run(expr, func(t *testing.T) {
			_, issues := env.Compile(expr)
			assert.Nil(t, issues.Err(), "expr %q should compile", expr)
		})
	}
}

// componentName/componentUID/podSelectors are reserved for component-bound
// resources, which are not currently supported. The base surface must reject
// them so a PE template cannot quietly depend on a field the runtime won't
// provide.
func TestBuildResourceCELEnv_RejectsForwardCompatFields(t *testing.T) {
	env, err := buildResourceCELEnv(SchemaOptions{})
	require.NoError(t, err)

	rejected := []string{
		"metadata.componentName",
		"metadata.componentUID",
		"metadata.podSelectors",
	}
	for _, expr := range rejected {
		t.Run(expr, func(t *testing.T) {
			_, issues := env.Compile(expr)
			require.NotNil(t, issues)
			assert.NotNil(t, issues.Err(), "expr %q should fail", expr)
		})
	}
}

func TestBuildResourceCELEnv_DataPlaneFields(t *testing.T) {
	env, err := buildResourceCELEnv(SchemaOptions{})
	require.NoError(t, err)

	okExprs := []string{
		"dataplane.secretStore",
		"dataplane.observabilityPlaneRef.kind",
		"dataplane.observabilityPlaneRef.name",
	}
	for _, expr := range okExprs {
		t.Run(expr, func(t *testing.T) {
			_, issues := env.Compile(expr)
			assert.Nil(t, issues.Err(), "expr %q should compile", expr)
		})
	}

	rejected := []string{
		"dataplane.gateway",
		"dataplane.observabilityPlaneRef.unknown",
	}
	for _, expr := range rejected {
		t.Run(expr, func(t *testing.T) {
			_, issues := env.Compile(expr)
			require.NotNil(t, issues)
			assert.NotNil(t, issues.Err(), "expr %q should fail", expr)
		})
	}
}

// applied.<id> is only valid during outputs / readyWhen evaluation, never
// during template or includeWhen rendering. The base env must reject any
// applied reference.
func TestBuildResourceCELEnv_RejectsAppliedAtBase(t *testing.T) {
	env, err := buildResourceCELEnv(SchemaOptions{})
	require.NoError(t, err)

	_, issues := env.Compile("applied.foo")
	require.NotNil(t, issues)
	assert.NotNil(t, issues.Err(), "applied.* must not be in scope at base")
}

func TestExtendEnvWithApplied_AccessCompiles(t *testing.T) {
	base, err := buildResourceCELEnv(SchemaOptions{})
	require.NoError(t, err)

	extended, err := extendEnvWithApplied(base)
	require.NoError(t, err)
	require.NotNil(t, extended)

	okExprs := []string{
		"applied.claim",
		"applied.claim.status",
		"applied.claim.status.host",
		`applied["tls-cert"]`,
		`applied["tls-cert"].status.ready`,
	}
	for _, expr := range okExprs {
		t.Run(expr, func(t *testing.T) {
			_, issues := extended.Compile(expr)
			assert.Nil(t, issues.Err(), "expr %q should compile", expr)
		})
	}
}

// Locks the contract that extendEnvWithApplied returns a new env without
// mutating the base. The base env is reused across many template / includeWhen
// validations, so accidental mutation would leak applied.* into rendering
// scope where the runtime can't satisfy it.
func TestExtendEnvWithApplied_BaseEnvUnchanged(t *testing.T) {
	base, err := buildResourceCELEnv(SchemaOptions{})
	require.NoError(t, err)

	_, err = extendEnvWithApplied(base)
	require.NoError(t, err)

	_, issues := base.Compile("applied.claim")
	require.NotNil(t, issues)
	assert.NotNil(t, issues.Err(), "extending should not mutate base env")
}
