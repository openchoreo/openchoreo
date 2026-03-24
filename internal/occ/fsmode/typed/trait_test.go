// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func makeTraitEntry(t *testing.T, trait *v1alpha1.Trait) *index.ResourceEntry {
	t.Helper()
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(trait)
	require.NoError(t, err)
	obj := &unstructured.Unstructured{Object: raw}
	obj.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("Trait"))
	return &index.ResourceEntry{Resource: obj}
}

func TestNewTrait(t *testing.T) {
	tests := []struct {
		name    string
		entry   *index.ResourceEntry
		wantErr bool
	}{
		{
			name:  "valid entry",
			entry: makeTraitEntry(t, &v1alpha1.Trait{}),
		},
		{
			name:    "nil entry",
			entry:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trait, err := NewTrait(tt.entry)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, trait)
		})
	}
}

func TestTraitGetSpec(t *testing.T) {
	schemaJSON := []byte(`{"type":"object"}`)

	tests := []struct {
		name       string
		trait      *Trait
		wantParams bool
		wantEnv    bool
	}{
		{
			name: "parameters present",
			trait: &Trait{
				Trait: &v1alpha1.Trait{
					Spec: v1alpha1.TraitSpec{
						Parameters: &v1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{Raw: schemaJSON}},
					},
				},
			},
			wantParams: true,
		},
		{
			name: "environmentConfigs present",
			trait: &Trait{
				Trait: &v1alpha1.Trait{
					Spec: v1alpha1.TraitSpec{
						EnvironmentConfigs: &v1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{Raw: schemaJSON}},
					},
				},
			},
			wantEnv: true,
		},
		{
			name: "no schemas",
			trait: &Trait{
				Trait: &v1alpha1.Trait{
					Spec: v1alpha1.TraitSpec{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.trait.GetSpec()
			require.NotNil(t, spec)
			if tt.wantParams {
				require.Contains(t, spec, "parameters", "spec should contain 'parameters' key")
				paramsMap, ok := spec["parameters"].(map[string]interface{})
				require.True(t, ok, "spec[\"parameters\"] should be a map[string]interface{}")
				require.Contains(t, paramsMap, "openAPIV3Schema", "spec[\"parameters\"] should contain 'openAPIV3Schema' key")
				schema, ok := paramsMap["openAPIV3Schema"].(map[string]interface{})
				require.True(t, ok, "spec[\"parameters\"][\"openAPIV3Schema\"] should be a map[string]interface{}")
				assert.Equal(t, "object", schema["type"], "spec[\"parameters\"][\"openAPIV3Schema\"][\"type\"] should match input schema")
			} else {
				assert.NotContains(t, spec, "parameters")
			}
			if tt.wantEnv {
				require.Contains(t, spec, "environmentConfigs", "spec should contain 'environmentConfigs' key")
				envMap, ok := spec["environmentConfigs"].(map[string]interface{})
				require.True(t, ok, "spec[\"environmentConfigs\"] should be a map[string]interface{}")
				require.Contains(t, envMap, "openAPIV3Schema", "spec[\"environmentConfigs\"] should contain 'openAPIV3Schema' key")
				schema, ok := envMap["openAPIV3Schema"].(map[string]interface{})
				require.True(t, ok, "spec[\"environmentConfigs\"][\"openAPIV3Schema\"] should be a map[string]interface{}")
				assert.Equal(t, "object", schema["type"], "spec[\"environmentConfigs\"][\"openAPIV3Schema\"][\"type\"] should match input schema")
			} else {
				assert.NotContains(t, spec, "environmentConfigs")
			}
		})
	}
}
