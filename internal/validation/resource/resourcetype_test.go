// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/localdevaddresses"
)

// rawTemplate marshals a Go map into a runtime.RawExtension for use as a
// ResourceTypeManifest template. Test fixtures use map literals for
// readability; this helper handles the marshal-or-fail boilerplate.
func rawTemplate(t *testing.T, body map[string]any) *runtime.RawExtension {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: data}
}

// schemaSection wraps a JSON-Schema fragment into a SchemaSection ready to
// drop on a ResourceTypeSpec. Tests build the smallest schema they need.
func schemaSection(t *testing.T, body map[string]any) *v1alpha1.SchemaSection {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	return &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: data},
	}
}

func TestValidateResourceTypeSpec_NilSpec(t *testing.T) {
	errs := ValidateResourceTypeSpec(nil, field.NewPath("spec"))
	assert.Empty(t, errs)
}

func TestValidateResourceTypeSpec_MinimalValid(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Resources: []v1alpha1.ResourceTypeManifest{
			{
				ID: "claim",
				Template: rawTemplate(t, map[string]any{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]any{"name": "smoke"},
				}),
			},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	assert.Empty(t, errs, "minimal valid spec should have no errors: %v", errs)
}

func TestValidateResourceTypeSpec_MalformedParametersSchema(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Parameters: &v1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte("{not json")},
		},
		Resources: []v1alpha1.ResourceTypeManifest{
			{ID: "claim", Template: rawTemplate(t, map[string]any{"kind": "X"})},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	require.NotEmpty(t, errs)
	assert.True(t, hasErrorAtPath(errs, "spec.parameters"), "expected error at spec.parameters: %v", errs)
}

func TestValidateResourceTypeSpec_MalformedEnvironmentConfigsSchema(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		EnvironmentConfigs: &v1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte("{not json")},
		},
		Resources: []v1alpha1.ResourceTypeManifest{
			{ID: "claim", Template: rawTemplate(t, map[string]any{"kind": "X"})},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	require.NotEmpty(t, errs)
	assert.True(t, hasErrorAtPath(errs, "spec.environmentConfigs"), "expected error at spec.environmentConfigs: %v", errs)
}

func TestValidateResourceTypeSpec_TemplateValidCEL(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Parameters: schemaSection(t, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"version": map[string]any{"type": "string"},
			},
		}),
		EnvironmentConfigs: schemaSection(t, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"replicas": map[string]any{"type": "integer"},
			},
		}),
		Resources: []v1alpha1.ResourceTypeManifest{
			{
				ID: "claim",
				Template: rawTemplate(t, map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "StatefulSet",
					"metadata":   map[string]any{"name": "${metadata.resourceName}"},
					"spec": map[string]any{
						"version":  "${parameters.version}",
						"replicas": "${environmentConfigs.replicas}",
						"secret":   "${dataplane.secretStore}",
					},
				}),
			},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	assert.Empty(t, errs, "valid template should pass: %v", errs)
}

func TestValidateResourceTypeSpec_TemplateRejectsApplied(t *testing.T) {
	// applied.* is only available during outputs / readyWhen, never during
	// template rendering. The validator must reject any applied reference
	// in resources[].template.
	spec := &v1alpha1.ResourceTypeSpec{
		Resources: []v1alpha1.ResourceTypeManifest{
			{
				ID: "claim",
				Template: rawTemplate(t, map[string]any{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"data": map[string]any{
						"host": "${applied.claim.status.host}",
					},
				}),
			},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	require.NotEmpty(t, errs, "applied.* in template should error")
	assert.True(t, hasErrorContaining(errs, "applied"), "expected error mentioning applied: %v", errs)
}

func TestValidateResourceTypeSpec_IncludeWhenBool(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		EnvironmentConfigs: schemaSection(t, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tlsEnabled": map[string]any{"type": "boolean"},
			},
		}),
		Resources: []v1alpha1.ResourceTypeManifest{
			{
				ID:          "claim",
				IncludeWhen: "${environmentConfigs.tlsEnabled}",
				Template:    rawTemplate(t, map[string]any{"kind": "X"}),
			},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	assert.Empty(t, errs, "bool includeWhen should pass: %v", errs)
}

func TestValidateResourceTypeSpec_IncludeWhenNonBool(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Parameters: schemaSection(t, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"size": map[string]any{"type": "integer"},
			},
		}),
		Resources: []v1alpha1.ResourceTypeManifest{
			{
				ID:          "claim",
				IncludeWhen: "${parameters.size}",
				Template:    rawTemplate(t, map[string]any{"kind": "X"}),
			},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	require.NotEmpty(t, errs, "non-bool includeWhen should error")
	assert.True(t, hasErrorAtPath(errs, "spec.resources[0].includeWhen"), "expected error at includeWhen path: %v", errs)
}

func TestValidateResourceTypeSpec_IncludeWhenRejectsApplied(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Resources: []v1alpha1.ResourceTypeManifest{
			{
				ID:          "claim",
				IncludeWhen: "${applied.claim.status.ready}",
				Template:    rawTemplate(t, map[string]any{"kind": "X"}),
			},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	require.NotEmpty(t, errs, "applied.* in includeWhen should error")
	assert.True(t, hasErrorAtPath(errs, "spec.resources[0].includeWhen"), "expected error at includeWhen path: %v", errs)
}

func TestValidateResourceTypeSpec_ReadyWhenDeclaredID(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Resources: []v1alpha1.ResourceTypeManifest{
			{
				ID:        "claim",
				ReadyWhen: "${applied.claim.status.ready}",
				Template:  rawTemplate(t, map[string]any{"kind": "X"}),
			},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	assert.Empty(t, errs, "applied.<declaredID> in readyWhen should pass: %v", errs)
}

func TestValidateResourceTypeSpec_ReadyWhenUndeclaredID(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Resources: []v1alpha1.ResourceTypeManifest{
			{
				ID:        "claim",
				ReadyWhen: "${applied.unknown.status.ready}",
				Template:  rawTemplate(t, map[string]any{"kind": "X"}),
			},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	require.NotEmpty(t, errs, "applied.<undeclaredID> in readyWhen should error")
	assert.True(t, hasErrorAtPath(errs, "spec.resources[0].readyWhen"), "expected error at readyWhen path: %v", errs)
	assert.True(t, hasErrorContaining(errs, "unknown"), "expected error to name the undeclared id: %v", errs)
}

// Bracket-form applied["<id>"] takes a different AST path (CallKind on the index
// operator) than the dot form (SelectKind). The validator recognizes both — these
// two tests lock the bracket-form path.

func TestValidateResourceTypeSpec_ReadyWhenBracketDeclaredID(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Resources: []v1alpha1.ResourceTypeManifest{
			{
				ID:        "claim",
				ReadyWhen: `${applied["claim"].status.ready}`,
				Template:  rawTemplate(t, map[string]any{"kind": "X"}),
			},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	assert.Empty(t, errs, `applied["<declaredID>"] in readyWhen should pass: %v`, errs)
}

func TestValidateResourceTypeSpec_ReadyWhenBracketUndeclaredID(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Resources: []v1alpha1.ResourceTypeManifest{
			{
				ID:        "claim",
				ReadyWhen: `${applied["unknown"].status.ready}`,
				Template:  rawTemplate(t, map[string]any{"kind": "X"}),
			},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	require.NotEmpty(t, errs, `applied["<undeclaredID>"] in readyWhen should error`)
	assert.True(t, hasErrorAtPath(errs, "spec.resources[0].readyWhen"), "expected error at readyWhen path: %v", errs)
	assert.True(t, hasErrorContaining(errs, "unknown"), "expected error to name the undeclared id: %v", errs)
}

func TestValidateResourceTypeSpec_ReadyWhenNonBool(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Resources: []v1alpha1.ResourceTypeManifest{
			{
				ID:        "claim",
				ReadyWhen: `${"not a bool"}`,
				Template:  rawTemplate(t, map[string]any{"kind": "X"}),
			},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	require.NotEmpty(t, errs, "non-bool readyWhen should error")
	assert.True(t, hasErrorAtPath(errs, "spec.resources[0].readyWhen"), "expected error at readyWhen path: %v", errs)
}

func TestValidateResourceTypeSpec_OutputValueDeclaredID(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Outputs: []v1alpha1.ResourceTypeOutput{
			{Name: "host", Value: "${applied.claim.status.host}"},
		},
		Resources: []v1alpha1.ResourceTypeManifest{
			{ID: "claim", Template: rawTemplate(t, map[string]any{"kind": "X"})},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	assert.Empty(t, errs, "output value referencing declared id should pass: %v", errs)
}

func TestValidateResourceTypeSpec_OutputValueUndeclaredID(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Outputs: []v1alpha1.ResourceTypeOutput{
			{Name: "host", Value: "${applied.bogus.status.host}"},
		},
		Resources: []v1alpha1.ResourceTypeManifest{
			{ID: "claim", Template: rawTemplate(t, map[string]any{"kind": "X"})},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	require.NotEmpty(t, errs, "output value referencing undeclared id should error")
	assert.True(t, hasErrorAtPath(errs, "spec.outputs[0].value"), "expected error at outputs[0].value: %v", errs)
}

func TestValidateResourceTypeSpec_OutputSecretKeyRef(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Outputs: []v1alpha1.ResourceTypeOutput{
			{
				Name: "password",
				SecretKeyRef: &v1alpha1.SecretKeyRef{
					Name: "${metadata.resourceName}-conn",
					Key:  "password",
				},
			},
		},
		Resources: []v1alpha1.ResourceTypeManifest{
			{ID: "claim", Template: rawTemplate(t, map[string]any{"kind": "X"})},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	assert.Empty(t, errs, "output secretKeyRef with valid CEL should pass: %v", errs)
}

func TestValidateResourceTypeSpec_OutputConfigMapKeyRef(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Outputs: []v1alpha1.ResourceTypeOutput{
			{
				Name: "caCert",
				ConfigMapKeyRef: &v1alpha1.ConfigMapKeyRef{
					Name: "${metadata.resourceName}-tls",
					Key:  "ca.crt",
				},
			},
		},
		Resources: []v1alpha1.ResourceTypeManifest{
			{ID: "claim", Template: rawTemplate(t, map[string]any{"kind": "X"})},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	assert.Empty(t, errs, "output configMapKeyRef with valid CEL should pass: %v", errs)
}

func TestValidateResourceTypeSpec_OutputSecretKeyRefUndeclaredID(t *testing.T) {
	spec := &v1alpha1.ResourceTypeSpec{
		Outputs: []v1alpha1.ResourceTypeOutput{
			{
				Name: "password",
				SecretKeyRef: &v1alpha1.SecretKeyRef{
					Name: "${applied.bogus.status.secretName}",
					Key:  "password",
				},
			},
		},
		Resources: []v1alpha1.ResourceTypeManifest{
			{ID: "claim", Template: rawTemplate(t, map[string]any{"kind": "X"})},
		},
	}

	errs := ValidateResourceTypeSpec(spec, field.NewPath("spec"))
	require.NotEmpty(t, errs, "secretKeyRef.name referencing undeclared id should error")
	assert.True(t, hasErrorAtPath(errs, "spec.outputs[0].secretKeyRef.name"), "expected error at secretKeyRef.name: %v", errs)
}

func TestValidateClusterResourceTypeSpec_DelegatesToResourceTypeSpec(t *testing.T) {
	// Spec validation logic is identical for cluster-scoped sibling. Locks
	// that delegation so a future divergence on the ClusterResourceTypeSpec
	// shape doesn't silently bypass validation.
	spec := &v1alpha1.ClusterResourceTypeSpec{
		Resources: []v1alpha1.ResourceTypeManifest{
			{
				ID:        "claim",
				ReadyWhen: "${applied.unknown.status.ready}",
				Template:  rawTemplate(t, map[string]any{"kind": "X"}),
			},
		},
	}

	errs := ValidateClusterResourceTypeSpec(spec, field.NewPath("spec"))
	require.NotEmpty(t, errs, "cluster spec with undeclared id should error")
	assert.True(t, hasErrorAtPath(errs, "spec.resources[0].readyWhen"), "expected error at readyWhen: %v", errs)
}

// hasErrorAtPath reports whether any field error in errs has the exact field
// path.
func hasErrorAtPath(errs field.ErrorList, path string) bool {
	for _, e := range errs {
		if e.Field == path {
			return true
		}
	}
	return false
}

// hasErrorContaining reports whether any field error in errs has a Detail
// that contains the substring.
func hasErrorContaining(errs field.ErrorList, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e.Detail, substr) || strings.Contains(e.Error(), substr) {
			return true
		}
	}
	return false
}

func TestValidateLocalDevAddressesAnnotation(t *testing.T) {
	outputs := []v1alpha1.ResourceTypeOutput{
		{Name: "host", Value: "h"},
		{Name: "port", Value: "6379"},
		{Name: "adminPort", Value: "8222"},
	}
	annotationsPath := field.NewPath("metadata", "annotations")
	validate := func(value string) field.ErrorList {
		return ValidateLocalDevAddressesAnnotation(
			map[string]string{localdevaddresses.AnnotationKey: value}, outputs, annotationsPath)
	}

	t.Run("accepts_distinct_port_outputs", func(t *testing.T) {
		require.Empty(t, validate("database=outputs.host:outputs.port,admin=outputs.host:outputs.adminPort"))
	})

	// An absent annotation is a resource type with nothing to dial, not an omission.
	t.Run("absent_annotation_is_valid", func(t *testing.T) {
		require.Empty(t, ValidateLocalDevAddressesAnnotation(nil, outputs, annotationsPath))
		require.Empty(t, ValidateLocalDevAddressesAnnotation(
			map[string]string{"openchoreo.dev/description": "unrelated"}, outputs, annotationsPath))
	})

	// A typo in an output name would otherwise surface as a dependency with no address,
	// long after the type was published.
	t.Run("rejects_undeclared_host_output", func(t *testing.T) {
		errs := validate("database=outputs.nope:outputs.port")
		require.Len(t, errs, 1)
		require.Equal(t, "metadata.annotations[openchoreo.dev/local-dev-addresses]", errs[0].Field)
		require.Contains(t, errs[0].Detail, `host output "nope" is not declared`)
	})

	t.Run("rejects_undeclared_port_output", func(t *testing.T) {
		errs := validate("database=outputs.host:outputs.nope")
		require.Len(t, errs, 1)
		require.Contains(t, errs[0].Detail, `port output "nope" is not declared`)
	})

	// One env var cannot carry a redirected address for two addresses.
	t.Run("rejects_shared_port_output", func(t *testing.T) {
		errs := validate("writer=outputs.host:outputs.port,reader=outputs.host:outputs.port")
		require.Len(t, errs, 1)
		require.Equal(t, "metadata.annotations[openchoreo.dev/local-dev-addresses]", errs[0].Field)
		require.Contains(t, errs[0].Detail, `already used by address "writer"`)
	})

	// A malformed value is reported as one error against the annotation: the parse
	// failed, so there are no per-endpoint findings to report under it.
	t.Run("rejects_malformed_value", func(t *testing.T) {
		for _, value := range []string{"database", "database=outputs.host", "database=host:port", "database=outputs.host:outputs.port,"} {
			errs := validate(value)
			require.Len(t, errs, 1, "value %q", value)
			require.Equal(t, "metadata.annotations[openchoreo.dev/local-dev-addresses]", errs[0].Field)
		}
	})

	// Both halves of an address may be missing from outputs; each is reported, so an
	// author fixes the declaration in one pass.
	t.Run("reports_both_halves", func(t *testing.T) {
		errs := validate("database=outputs.noHost:outputs.noPort")
		require.Len(t, errs, 2)
	})
}
