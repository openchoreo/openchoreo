// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcepipeline

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// rawExt marshals v to JSON and wraps the bytes in a RawExtension. Used to
// build inline ResourceTypeSpec.Resources[].Template values in tests.
func rawExt(t *testing.T, v any) *runtime.RawExtension {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: b}
}

// schemaSection builds a v1alpha1.SchemaSection from a Go map shaped like an
// OpenAPI v3 schema fragment.
func schemaSection(t *testing.T, openAPIV3 map[string]any) *v1alpha1.SchemaSection {
	t.Helper()
	return &v1alpha1.SchemaSection{
		OpenAPIV3Schema: rawExt(t, openAPIV3),
	}
}

// makeRenderInput builds a RenderInput with a ResourceType carrying the
// given spec, plus an empty Resource. Tests mutate fields via the returned
// input directly or through setParams / setEnvConfigs.
func makeRenderInput(spec v1alpha1.ResourceTypeSpec) *RenderInput {
	return &RenderInput{
		ResourceType: &v1alpha1.ResourceType{
			ObjectMeta: metav1.ObjectMeta{Name: "test-resource-type"},
			Spec:       spec,
		},
		Resource: &v1alpha1.Resource{
			ObjectMeta: metav1.ObjectMeta{Name: "test-resource"},
		},
		Metadata: fixtureMetadata(),
	}
}

// setParams marshals params into the Resource's Parameters RawExtension.
func setParams(t *testing.T, in *RenderInput, params map[string]any) {
	t.Helper()
	in.Resource.Spec.Parameters = rawExt(t, params)
}

// setEnvConfigs marshals envCfgs into the binding's
// ResourceTypeEnvironmentConfigs. Creates the binding if absent.
func setEnvConfigs(t *testing.T, in *RenderInput, envCfgs map[string]any) {
	t.Helper()
	if in.ResourceReleaseBinding == nil {
		in.ResourceReleaseBinding = &v1alpha1.ResourceReleaseBinding{}
	}
	in.ResourceReleaseBinding.Spec.ResourceTypeEnvironmentConfigs = rawExt(t, envCfgs)
}

// resolveInput is a single-output ResolveOutputs input shorthand.
func resolveInput(out v1alpha1.ResourceTypeOutput) *RenderInput {
	return makeRenderInput(v1alpha1.ResourceTypeSpec{
		Outputs: []v1alpha1.ResourceTypeOutput{out},
	})
}

// fixtureMetadata is a fully-populated MetadataContext used as the default
// for tests that don't care about specific field values.
func fixtureMetadata() MetadataContext {
	return MetadataContext{
		Name:              "analytics-shared-db-dev-a1b2c3d4",
		Namespace:         "dp-acme-payment-dev-x1y2z3w4",
		ResourceNamespace: "cp-acme",
		ResourceName:      "analytics-shared-db",
		ResourceUID:       "res-uid",
		ProjectName:       "analytics",
		ProjectUID:        "proj-uid",
		EnvironmentName:   "dev",
		EnvironmentUID:    "env-uid",
		DataPlaneName:     "kind-cluster",
		DataPlaneUID:      "dp-uid",
		Labels: map[string]string{
			"openchoreo.dev/resource":    "analytics-shared-db",
			"openchoreo.dev/project":     "analytics",
			"openchoreo.dev/environment": "dev",
		},
		Annotations: map[string]string{
			"openchoreo.dev/owner": "platform-team",
		},
	}
}

// renderSingle is a one-entry RenderInput shorthand for tests focused on a
// single template body. The entry is named "claim" by default.
func renderSingle(t *testing.T, template *runtime.RawExtension, opts ...func(*RenderInput)) *RenderInput {
	t.Helper()
	in := makeRenderInput(v1alpha1.ResourceTypeSpec{
		Resources: []v1alpha1.ResourceTypeManifest{
			{ID: "claim", Template: template},
		},
	})
	for _, opt := range opts {
		opt(in)
	}
	return in
}

func TestRenderManifests(t *testing.T) {
	t.Run("renders_bare_template", func(t *testing.T) {
		input := renderSingle(t, rawExt(t, map[string]any{
			"apiVersion": "example.org/v1alpha1",
			"kind":       "MySQL",
			"metadata": map[string]any{
				"name": "static-name",
			},
		}))

		got, err := NewPipeline().RenderManifests(input)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Len(t, got.Entries, 1)

		want := RenderedEntry{
			ID: "claim",
			Object: map[string]any{
				"apiVersion": "example.org/v1alpha1",
				"kind":       "MySQL",
				"metadata": map[string]any{
					"name": "static-name",
				},
			},
		}

		if diff := cmp.Diff(want, got.Entries[0]); diff != "" {
			t.Errorf("entry mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("substitutes_parameters_and_environment_configs", func(t *testing.T) {
		input := renderSingle(t, rawExt(t, map[string]any{
			"apiVersion": "example.org/v1alpha1",
			"kind":       "MySQL",
			"spec": map[string]any{
				"version":   "${parameters.version}",
				"storageGB": "${environmentConfigs.storageGB}",
			},
		}))
		setParams(t, input, map[string]any{"version": "8.0"})
		setEnvConfigs(t, input, map[string]any{"storageGB": 100})

		got, err := NewPipeline().RenderManifests(input)
		require.NoError(t, err)

		spec := got.Entries[0].Object["spec"].(map[string]any)
		require.Equal(t, "8.0", spec["version"])
		// The CEL engine returns the unmarshalled JSON number type; JSON has
		// no native integer kind, so numeric scalars come back as float64.
		// The contract is "standalone CEL ref preserves the native number
		// type, not stringified" — float64 satisfies that contract.
		require.Equal(t, float64(100), spec["storageGB"], "standalone CEL ref returns native number, not string")
	})

	t.Run("exposes_dataplane_secret_store", func(t *testing.T) {
		input := renderSingle(t,
			rawExt(t, map[string]any{
				"apiVersion": "v1",
				"kind":       "Probe",
				"value":      "${dataplane.secretStore}",
			}),
			func(in *RenderInput) {
				in.DataPlane = DataPlaneContext{SecretStore: "kind-cluster-store"}
			},
		)

		got, err := NewPipeline().RenderManifests(input)
		require.NoError(t, err)
		require.Equal(t, "kind-cluster-store", got.Entries[0].Object["value"])
	})

	t.Run("exposes_dataplane_observability_plane_ref", func(t *testing.T) {
		t.Run("populated", func(t *testing.T) {
			input := renderSingle(t,
				rawExt(t, map[string]any{
					"apiVersion": "v1",
					"kind":       "Probe",
					"kindRef":    "${dataplane.observabilityPlaneRef.kind}",
					"nameRef":    "${dataplane.observabilityPlaneRef.name}",
				}),
				func(in *RenderInput) {
					in.DataPlane = DataPlaneContext{
						ObservabilityPlaneRef: &ObservabilityPlaneRefContext{
							Kind: "ObservabilityPlane",
							Name: "primary-obs",
						},
					}
				},
			)

			got, err := NewPipeline().RenderManifests(input)
			require.NoError(t, err)
			require.Equal(t, "ObservabilityPlane", got.Entries[0].Object["kindRef"])
			require.Equal(t, "primary-obs", got.Entries[0].Object["nameRef"])
		})

		t.Run("absent_yields_empty_string", func(t *testing.T) {
			input := renderSingle(t, rawExt(t, map[string]any{
				"apiVersion": "v1",
				"kind":       "Probe",
				"value":      "${dataplane.observabilityPlaneRef.name}",
			}))

			got, err := NewPipeline().RenderManifests(input)
			require.NoError(t, err)
			require.Equal(t, "", got.Entries[0].Object["value"])
		})
	})

	t.Run("applies_parameter_defaults_from_schema", func(t *testing.T) {
		input := renderSingle(t, rawExt(t, map[string]any{
			"apiVersion": "v1",
			"kind":       "Probe",
			"value":      "${parameters.replicas}",
		}))
		input.ResourceType.Spec.Parameters = schemaSection(t, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"replicas": map[string]any{
					"type":    "integer",
					"default": 3,
				},
			},
		})
		// Parameters RawExtension is nil; the default fills replicas.

		got, err := NewPipeline().RenderManifests(input)
		require.NoError(t, err)
		require.EqualValues(t, 3, got.Entries[0].Object["value"], "schema default fills the value")
	})

	t.Run("applies_environment_configs_defaults_from_schema", func(t *testing.T) {
		input := renderSingle(t, rawExt(t, map[string]any{
			"apiVersion": "v1",
			"kind":       "Probe",
			"value":      "${environmentConfigs.tlsMode}",
		}))
		input.ResourceType.Spec.EnvironmentConfigs = schemaSection(t, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tlsMode": map[string]any{
					"type":    "string",
					"default": "disabled",
				},
			},
		})

		got, err := NewPipeline().RenderManifests(input)
		require.NoError(t, err)
		require.Equal(t, "disabled", got.Entries[0].Object["value"])
	})

	t.Run("input_parameter_overrides_schema_default", func(t *testing.T) {
		input := renderSingle(t, rawExt(t, map[string]any{
			"apiVersion": "v1",
			"kind":       "Probe",
			"value":      "${parameters.replicas}",
		}))
		input.ResourceType.Spec.Parameters = schemaSection(t, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"replicas": map[string]any{
					"type":    "integer",
					"default": 3,
				},
			},
		})
		setParams(t, input, map[string]any{"replicas": 7})

		got, err := NewPipeline().RenderManifests(input)
		require.NoError(t, err)
		require.EqualValues(t, 7, got.Entries[0].Object["value"], "explicit input overrides default")
	})

	t.Run("preserves_resource_id_verbatim", func(t *testing.T) {
		// IDs intentionally don't match the kind/name pattern — proves the
		// pipeline doesn't synthesize IDs from kind+name like
		// releasebinding.convertToReleaseResources.
		input := makeRenderInput(v1alpha1.ResourceTypeSpec{
			Resources: []v1alpha1.ResourceTypeManifest{
				{
					ID:       "claim",
					Template: rawExt(t, map[string]any{"apiVersion": "example.org/v1", "kind": "MySQL", "metadata": map[string]any{"name": "first"}}),
				},
				{
					ID:       "credentials-bootstrap",
					Template: rawExt(t, map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": "second"}}),
				},
				{
					ID:       "tls-trust-anchor",
					Template: rawExt(t, map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "third"}}),
				},
			},
		})

		got, err := NewPipeline().RenderManifests(input)
		require.NoError(t, err)
		require.Len(t, got.Entries, 3)
		require.Equal(t, "claim", got.Entries[0].ID)
		require.Equal(t, "credentials-bootstrap", got.Entries[1].ID)
		require.Equal(t, "tls-trust-anchor", got.Entries[2].ID)
		// Spec ordering is preserved.
		require.Equal(t, "first", got.Entries[0].Object["metadata"].(map[string]any)["name"])
		require.Equal(t, "second", got.Entries[1].Object["metadata"].(map[string]any)["name"])
		require.Equal(t, "third", got.Entries[2].Object["metadata"].(map[string]any)["name"])
	})

	t.Run("include_when", func(t *testing.T) {
		// Helper to build a 2-entry input where the first carries an
		// includeWhen expression and the second is unconditional.
		makeInput := func(includeWhen string) *RenderInput {
			return makeRenderInput(v1alpha1.ResourceTypeSpec{
				Resources: []v1alpha1.ResourceTypeManifest{
					{
						ID:          "optional",
						IncludeWhen: includeWhen,
						Template: rawExt(t, map[string]any{
							"apiVersion": "v1",
							"kind":       "Probe",
							"metadata":   map[string]any{"name": "optional"},
						}),
					},
					{
						ID: "always",
						Template: rawExt(t, map[string]any{
							"apiVersion": "v1",
							"kind":       "Probe",
							"metadata":   map[string]any{"name": "always"},
						}),
					},
				},
			})
		}

		t.Run("includes_entry_when_expression_true", func(t *testing.T) {
			input := makeInput("${parameters.tlsEnabled}")
			setParams(t, input, map[string]any{"tlsEnabled": true})

			got, err := NewPipeline().RenderManifests(input)
			require.NoError(t, err)
			require.Len(t, got.Entries, 2, "both entries rendered")
			require.Equal(t, "optional", got.Entries[0].ID)
			require.Equal(t, "always", got.Entries[1].ID)
		})

		t.Run("skips_entry_when_expression_false", func(t *testing.T) {
			input := makeInput("${parameters.tlsEnabled}")
			setParams(t, input, map[string]any{"tlsEnabled": false})

			got, err := NewPipeline().RenderManifests(input)
			require.NoError(t, err)
			require.Len(t, got.Entries, 1, "only the unconditional entry survives")
			require.Equal(t, "always", got.Entries[0].ID, "skipped entry leaves no gap; spec ordering of remaining entries preserved")
		})

		t.Run("includes_entry_when_unset", func(t *testing.T) {
			input := makeInput("")

			got, err := NewPipeline().RenderManifests(input)
			require.NoError(t, err)
			require.Len(t, got.Entries, 2, "empty includeWhen means always include")
		})

		t.Run("errors_on_non_bool_result", func(t *testing.T) {
			// String result instead of bool.
			input := makeInput("${parameters.label}")
			setParams(t, input, map[string]any{"label": "yes"})

			got, err := NewPipeline().RenderManifests(input)
			require.Error(t, err)
			require.Nil(t, got)
			require.Contains(t, err.Error(), "includeWhen must evaluate to bool")
		})

		t.Run("errors_on_cel_evaluation_failure", func(t *testing.T) {
			// References applied.* which is not in scope at render time.
			input := makeInput("${applied.foo.status.ready}")

			got, err := NewPipeline().RenderManifests(input)
			require.Error(t, err)
			require.Nil(t, got)
			require.Contains(t, err.Error(), "evaluate includeWhen for resource \"optional\"")
		})
	})

	t.Run("strips_omit_sentinel", func(t *testing.T) {
		input := renderSingle(t, rawExt(t, map[string]any{
			"apiVersion": "v1",
			"kind":       "Probe",
			"spec": map[string]any{
				"alwaysPresent": "yes",
				"conditional":   "${parameters.show ? 'visible' : oc_omit()}",
			},
		}))
		setParams(t, input, map[string]any{"show": false})

		got, err := NewPipeline().RenderManifests(input)
		require.NoError(t, err)

		spec := got.Entries[0].Object["spec"].(map[string]any)
		require.Equal(t, "yes", spec["alwaysPresent"])
		_, present := spec["conditional"]
		require.False(t, present, "oc_omit() should remove the key from the rendered output")
	})

	t.Run("does_not_force_set_namespace", func(t *testing.T) {
		t.Run("template_omits_namespace_then_output_omits_it", func(t *testing.T) {
			input := renderSingle(t, rawExt(t, map[string]any{
				"apiVersion": "v1",
				"kind":       "Probe",
				"metadata": map[string]any{
					"name": "${metadata.resourceName}",
				},
			}))

			got, err := NewPipeline().RenderManifests(input)
			require.NoError(t, err)

			meta := got.Entries[0].Object["metadata"].(map[string]any)
			_, present := meta["namespace"]
			require.False(t, present, "pipeline must not auto-inject metadata.namespace")
		})

		t.Run("template_hardcodes_namespace_then_output_keeps_it", func(t *testing.T) {
			input := renderSingle(t, rawExt(t, map[string]any{
				"apiVersion": "v1",
				"kind":       "Probe",
				"metadata": map[string]any{
					"name":      "${metadata.resourceName}",
					"namespace": "kube-system",
				},
			}))

			got, err := NewPipeline().RenderManifests(input)
			require.NoError(t, err)

			meta := got.Entries[0].Object["metadata"].(map[string]any)
			require.Equal(t, "kube-system", meta["namespace"], "pipeline must not override hardcoded namespaces")
		})
	})

	t.Run("rejects_applied_reference", func(t *testing.T) {
		// applied.<id>.* is only in scope during output resolution. A
		// render-time template reference must fail fast; the webhook
		// normally rejects these at admission, but the pipeline is the
		// runtime fence.
		input := renderSingle(t, rawExt(t, map[string]any{
			"apiVersion": "v1",
			"kind":       "Probe",
			"value":      "${applied.claim.status.host}",
		}))

		got, err := NewPipeline().RenderManifests(input)
		require.Error(t, err, "applied.* must not be available during manifest rendering")
		require.Nil(t, got)
	})

	t.Run("rejects_component_name_reference", func(t *testing.T) {
		// metadata.componentName is reserved for component-bound resources,
		// which are not currently supported. The base surface must reject it
		// so a future contributor cannot accidentally restore the field
		// without an explicit decision.
		input := renderSingle(t, rawExt(t, map[string]any{
			"apiVersion": "v1",
			"kind":       "Probe",
			"value":      "${metadata.componentName}",
		}))

		got, err := NewPipeline().RenderManifests(input)
		require.Error(t, err, "metadata.componentName must not be exposed in the base surface")
		require.Nil(t, got)
	})

	t.Run("exposes_all_metadata_fields_to_cel", func(t *testing.T) {
		md := fixtureMetadata()
		cases := []struct {
			field string
			expr  string
			want  any
		}{
			{"name", "${metadata.name}", md.Name},
			{"namespace", "${metadata.namespace}", md.Namespace},
			{"resourceNamespace", "${metadata.resourceNamespace}", md.ResourceNamespace},
			{"resourceName", "${metadata.resourceName}", md.ResourceName},
			{"resourceUID", "${metadata.resourceUID}", md.ResourceUID},
			{"projectName", "${metadata.projectName}", md.ProjectName},
			{"projectUID", "${metadata.projectUID}", md.ProjectUID},
			{"environmentName", "${metadata.environmentName}", md.EnvironmentName},
			{"environmentUID", "${metadata.environmentUID}", md.EnvironmentUID},
			{"dataPlaneName", "${metadata.dataPlaneName}", md.DataPlaneName},
			{"dataPlaneUID", "${metadata.dataPlaneUID}", md.DataPlaneUID},
			{
				field: "labels.byKey",
				expr:  "${metadata.labels['openchoreo.dev/resource']}",
				want:  md.Labels["openchoreo.dev/resource"],
			},
			{
				field: "annotations.byKey",
				expr:  "${metadata.annotations['openchoreo.dev/owner']}",
				want:  md.Annotations["openchoreo.dev/owner"],
			},
		}

		for _, tc := range cases {
			t.Run(tc.field, func(t *testing.T) {
				input := renderSingle(t, rawExt(t, map[string]any{
					"apiVersion": "v1",
					"kind":       "Probe",
					"value":      tc.expr,
				}))
				got, err := NewPipeline().RenderManifests(input)
				require.NoError(t, err)
				require.Equal(t, tc.want, got.Entries[0].Object["value"])
			})
		}
	})
}

func TestResolveOutputs(t *testing.T) {
	t.Run("resolves_value_output_against_observed_status", func(t *testing.T) {
		input := resolveInput(v1alpha1.ResourceTypeOutput{
			Name:  "host",
			Value: "${applied.claim.status.host}",
		})
		observed := map[string]map[string]any{
			"claim": {"host": "10.0.0.5"},
		}

		got, err := NewPipeline().ResolveOutputs(input, observed)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "host", got[0].Name)
		assert.Equal(t, "10.0.0.5", got[0].Value)
		assert.Nil(t, got[0].SecretKeyRef)
		assert.Nil(t, got[0].ConfigMapKeyRef)
	})

	t.Run("resolves_secret_key_ref_with_cel_templated_name_and_key", func(t *testing.T) {
		input := resolveInput(v1alpha1.ResourceTypeOutput{
			Name: "password",
			SecretKeyRef: &v1alpha1.SecretKeyRef{
				Name: "${metadata.resourceName}-conn",
				Key:  "password",
			},
		})

		got, err := NewPipeline().ResolveOutputs(input, nil)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "password", got[0].Name)
		require.NotNil(t, got[0].SecretKeyRef)
		assert.Equal(t, "analytics-shared-db-conn", got[0].SecretKeyRef.Name)
		assert.Equal(t, "password", got[0].SecretKeyRef.Key)
		assert.Empty(t, got[0].Value)
		assert.Nil(t, got[0].ConfigMapKeyRef)
	})

	t.Run("resolves_config_map_key_ref", func(t *testing.T) {
		input := resolveInput(v1alpha1.ResourceTypeOutput{
			Name: "caCert",
			ConfigMapKeyRef: &v1alpha1.ConfigMapKeyRef{
				Name: "${metadata.resourceName}-tls",
				Key:  "ca.crt",
			},
		})

		got, err := NewPipeline().ResolveOutputs(input, nil)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.NotNil(t, got[0].ConfigMapKeyRef)
		assert.Equal(t, "analytics-shared-db-tls", got[0].ConfigMapKeyRef.Name)
		assert.Equal(t, "ca.crt", got[0].ConfigMapKeyRef.Key)
		assert.Nil(t, got[0].SecretKeyRef)
	})

	t.Run("resolves_static_ref_when_observed_is_empty", func(t *testing.T) {
		// Static name has no applied.* reference. Locks OutputsResolved
		// independence from ResourcesReady: such an output can resolve
		// before any DP-side object is observed.
		input := resolveInput(v1alpha1.ResourceTypeOutput{
			Name: "password",
			SecretKeyRef: &v1alpha1.SecretKeyRef{
				Name: "${metadata.resourceName}-conn",
				Key:  "password",
			},
		})

		got, err := NewPipeline().ResolveOutputs(input, map[string]map[string]any{})
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.NotNil(t, got[0].SecretKeyRef)
		assert.Equal(t, "analytics-shared-db-conn", got[0].SecretKeyRef.Name)
	})

	t.Run("returns_partial_result_when_applied_reference_missing", func(t *testing.T) {
		// First output references applied.claim (not yet observed); errors.
		// Second output is static; resolves successfully. Both outcomes
		// must surface so the controller can write the partial set into
		// status.outputs and mark OutputsResolved=False.
		input := makeRenderInput(v1alpha1.ResourceTypeSpec{
			Outputs: []v1alpha1.ResourceTypeOutput{
				{
					Name:  "host",
					Value: "${applied.claim.status.host}",
				},
				{
					Name: "password",
					SecretKeyRef: &v1alpha1.SecretKeyRef{
						Name: "${metadata.resourceName}-conn",
						Key:  "password",
					},
				},
			},
		})

		got, err := NewPipeline().ResolveOutputs(input, map[string]map[string]any{})
		require.Error(t, err, "missing applied.<id> reference must surface as an error")
		require.Len(t, got, 1, "successfully-resolved outputs are still returned")
		assert.Equal(t, "password", got[0].Name)
		assert.NotNil(t, got[0].SecretKeyRef)
	})
}

func TestEvaluateReadyWhen(t *testing.T) {
	// emptyInput is enough to satisfy buildBaseContext; readyWhen evaluation
	// only depends on the metadata + applied surface for our tests.
	emptyInput := func() *RenderInput {
		return makeRenderInput(v1alpha1.ResourceTypeSpec{})
	}

	t.Run("returns_true_for_empty_expression", func(t *testing.T) {
		ready, err := NewPipeline().EvaluateReadyWhen(emptyInput(), nil, "")
		require.NoError(t, err)
		require.True(t, ready, "empty readyWhen falls back to per-Kind health inference; pipeline reports ready")
	})

	t.Run("returns_bool_value_for_predicate", func(t *testing.T) {
		t.Run("true", func(t *testing.T) {
			observed := map[string]map[string]any{
				"claim": {"ready": true},
			}
			ready, err := NewPipeline().EvaluateReadyWhen(emptyInput(), observed, "${applied.claim.status.ready == true}")
			require.NoError(t, err)
			require.True(t, ready)
		})

		t.Run("false", func(t *testing.T) {
			observed := map[string]map[string]any{
				"claim": {"ready": false},
			}
			ready, err := NewPipeline().EvaluateReadyWhen(emptyInput(), observed, "${applied.claim.status.ready == true}")
			require.NoError(t, err)
			require.False(t, ready)
		})
	})

	t.Run("errors_on_non_bool_result", func(t *testing.T) {
		// "${applied.claim.status.ready}" by itself returns the string value,
		// not a bool comparison.
		observed := map[string]map[string]any{
			"claim": {"ready": "yes"},
		}
		ready, err := NewPipeline().EvaluateReadyWhen(emptyInput(), observed, "${applied.claim.status.ready}")
		require.Error(t, err)
		require.False(t, ready)
		require.Contains(t, err.Error(), "must evaluate to bool")
	})
}
