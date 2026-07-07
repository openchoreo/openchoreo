// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"encoding/json"
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
	templateJSON := []byte(`{"apiVersion":"v1","kind":"ConfigMap"}`)

	tests := []struct {
		name       string
		trait      *Trait
		wantParams bool
		wantEnv    bool
		validate   func(t *testing.T, spec map[string]interface{})
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
		{
			name: "with creates",
			trait: &Trait{
				Trait: &v1alpha1.Trait{
					Spec: v1alpha1.TraitSpec{
						Creates: []v1alpha1.TraitCreate{
							{
								TargetPlane: "dataplane",
								IncludeWhen: "${parameters.enabled}",
								ForEach:     "${parameters.items}",
								Var:         "item",
								Template:    &runtime.RawExtension{Raw: templateJSON},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, spec map[string]interface{}) {
				creates := spec["creates"].([]interface{})
				require.Len(t, creates, 1)
				c := creates[0].(map[string]interface{})
				assert.Equal(t, "dataplane", c["targetPlane"])
				assert.Equal(t, "${parameters.enabled}", c["includeWhen"])
				assert.Equal(t, "${parameters.items}", c["forEach"])
				assert.Equal(t, "item", c["var"])
				require.Contains(t, c, "template")
				tmpl := c["template"].(map[string]interface{})
				assert.Equal(t, "ConfigMap", tmpl["kind"])
			},
		},
		{
			name: "with patches including where and operations with value",
			trait: &Trait{
				Trait: &v1alpha1.Trait{
					Spec: v1alpha1.TraitSpec{
						Patches: []v1alpha1.TraitPatch{
							{
								ForEach:     "${parameters.mounts}",
								Var:         "mount",
								TargetPlane: "dataplane",
								Target: v1alpha1.PatchTarget{
									Group:   "apps",
									Version: "v1",
									Kind:    "Deployment",
									Where:   "${item.name == 'main'}",
								},
								Operations: []v1alpha1.JSONPatchOperation{
									{
										Op:    "add",
										Path:  "/spec/replicas",
										Value: &runtime.RawExtension{Raw: []byte(`3`)},
									},
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, spec map[string]interface{}) {
				patches := spec["patches"].([]interface{})
				require.Len(t, patches, 1)
				p := patches[0].(map[string]interface{})
				assert.Equal(t, "${parameters.mounts}", p["forEach"])
				assert.Equal(t, "mount", p["var"])
				assert.Equal(t, "dataplane", p["targetPlane"])

				target := p["target"].(map[string]interface{})
				assert.Equal(t, "apps", target["group"])
				assert.Equal(t, "v1", target["version"])
				assert.Equal(t, "Deployment", target["kind"])
				assert.Equal(t, "${item.name == 'main'}", target["where"])

				ops := p["operations"].([]interface{})
				require.Len(t, ops, 1)
				op := ops[0].(map[string]interface{})
				assert.Equal(t, "add", op["op"])
				assert.Equal(t, "/spec/replicas", op["path"])
				assert.Contains(t, op, "value")
			},
		},
		{
			name: "patch without optional fields",
			trait: &Trait{
				Trait: &v1alpha1.Trait{
					Spec: v1alpha1.TraitSpec{
						Patches: []v1alpha1.TraitPatch{
							{
								Target: v1alpha1.PatchTarget{
									Group:   "",
									Version: "v1",
									Kind:    "Service",
								},
								Operations: []v1alpha1.JSONPatchOperation{
									{
										Op:   "remove",
										Path: "/spec/ports/0",
									},
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, spec map[string]interface{}) {
				patches := spec["patches"].([]interface{})
				p := patches[0].(map[string]interface{})
				assert.NotContains(t, p, "forEach")
				assert.NotContains(t, p, "var")
				assert.NotContains(t, p, "targetPlane")

				target := p["target"].(map[string]interface{})
				assert.NotContains(t, target, "where")

				ops := p["operations"].([]interface{})
				op := ops[0].(map[string]interface{})
				assert.NotContains(t, op, "value")
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
			if tt.validate != nil {
				tt.validate(t, spec)
			}
		})
	}
}

// TestTraitGetSpecValidationsAndRemoves verifies that GetSpec carries the trait's
// validation rules (deprecated validations, preRenderValidations, postRenderValidations)
// and removes into the emitted spec. These flow into the embedded trait spec of the
// generated ComponentRelease, where the rendering pipeline enforces them; dropping any of
// them here silently disables the corresponding trait behavior.
func TestTraitGetSpecValidationsAndRemoves(t *testing.T) {
	mustMatch := false
	trait := &Trait{
		Trait: &v1alpha1.Trait{
			Spec: v1alpha1.TraitSpec{
				Validations: []v1alpha1.ValidationRule{
					{Rule: "${parameters.a > 0}", Message: "a must be positive"},
				},
				PreRenderValidations: []v1alpha1.ValidationRule{
					{Rule: "${parameters.b != ''}", Message: "b is required"},
				},
				PostRenderValidations: []v1alpha1.PostRenderValidation{
					{
						When:    "${parameters.enabled}",
						ForEach: "${parameters.items}",
						Var:     "item",
						Target: v1alpha1.PostRenderTarget{
							PatchTarget: v1alpha1.PatchTarget{
								Group:   "apps",
								Version: "v1",
								Kind:    "Deployment",
								Where:   "${resource.metadata.name == item}",
							},
							MustMatch: &mustMatch,
						},
						TargetPlane: "dataplane",
						Rule:        "${resource.spec.replicas > 0}",
						Message:     "replicas must be positive",
					},
				},
				Removes: []v1alpha1.TraitRemove{
					{
						ForEach:     "${parameters.routesToDrop}",
						Var:         "route",
						TargetPlane: "dataplane",
						Target: v1alpha1.PatchTarget{
							Group:   "",
							Version: "v1",
							Kind:    "Service",
							Where:   "${resource.metadata.name == route}",
						},
					},
				},
			},
		},
	}

	spec := trait.GetSpec()
	require.NotNil(t, spec)

	// Round-trip the emitted map back into a typed TraitSpec, mirroring how the controller
	// decodes the embedded trait spec from a ComponentRelease. This guarantees the emitted
	// keys match the CRD JSON tags exactly.
	raw, err := json.Marshal(spec)
	require.NoError(t, err)
	var decoded v1alpha1.TraitSpec
	require.NoError(t, json.Unmarshal(raw, &decoded))

	require.Len(t, decoded.Validations, 1)
	assert.Equal(t, "${parameters.a > 0}", decoded.Validations[0].Rule)
	assert.Equal(t, "a must be positive", decoded.Validations[0].Message)

	require.Len(t, decoded.PreRenderValidations, 1)
	assert.Equal(t, "${parameters.b != ''}", decoded.PreRenderValidations[0].Rule)
	assert.Equal(t, "b is required", decoded.PreRenderValidations[0].Message)

	require.Len(t, decoded.PostRenderValidations, 1)
	prv := decoded.PostRenderValidations[0]
	assert.Equal(t, "${parameters.enabled}", prv.When)
	assert.Equal(t, "${parameters.items}", prv.ForEach)
	assert.Equal(t, "item", prv.Var)
	assert.Equal(t, "apps", prv.Target.Group)
	assert.Equal(t, "v1", prv.Target.Version)
	assert.Equal(t, "Deployment", prv.Target.Kind)
	assert.Equal(t, "${resource.metadata.name == item}", prv.Target.Where)
	require.NotNil(t, prv.Target.MustMatch)
	assert.False(t, *prv.Target.MustMatch)
	assert.Equal(t, "dataplane", prv.TargetPlane)
	assert.Equal(t, "${resource.spec.replicas > 0}", prv.Rule)
	assert.Equal(t, "replicas must be positive", prv.Message)

	require.Len(t, decoded.Removes, 1)
	rm := decoded.Removes[0]
	assert.Equal(t, "${parameters.routesToDrop}", rm.ForEach)
	assert.Equal(t, "route", rm.Var)
	assert.Equal(t, "dataplane", rm.TargetPlane)
	assert.Equal(t, "v1", rm.Target.Version)
	assert.Equal(t, "Service", rm.Target.Kind)
	assert.Equal(t, "${resource.metadata.name == route}", rm.Target.Where)
}

// TestTraitGetSpecOmitsEmptyValidationsAndRemoves verifies that validation and removes
// keys are absent when the trait defines none, so empty slices are not written into the
// generated ComponentRelease.
func TestTraitGetSpecOmitsEmptyValidationsAndRemoves(t *testing.T) {
	trait := &Trait{Trait: &v1alpha1.Trait{Spec: v1alpha1.TraitSpec{}}}
	spec := trait.GetSpec()
	require.NotNil(t, spec)
	assert.NotContains(t, spec, "validations")
	assert.NotContains(t, spec, "preRenderValidations")
	assert.NotContains(t, spec, "postRenderValidations")
	assert.NotContains(t, spec, "removes")
}
