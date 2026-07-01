// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectpipeline

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// rawExt marshals v and wraps it in a RawExtension.
func rawExt(t *testing.T, v any) *runtime.RawExtension {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: b}
}

// fixtureMetadata returns a fully-populated MetadataContext used as the
// default for tests that don't care about specific field values.
func fixtureMetadata() MetadataContext {
	return MetadataContext{
		Namespace:        "dp-org-myapp-dev-a1b2c3d",
		ProjectNamespace: "org",
		ProjectName:      "myapp",
		ProjectUID:       "00000000-0000-0000-0000-000000000001",
		EnvironmentName:  "dev",
		EnvironmentUID:   "00000000-0000-0000-0000-000000000002",
		DataPlaneName:    "primary",
		DataPlaneUID:     "00000000-0000-0000-0000-000000000003",
		Labels:           map[string]string{"openchoreo.dev/project": "myapp"},
	}
}

// nsTemplate returns a ResourceTemplate that renders the mandated cell
// Namespace.
func nsTemplate(t *testing.T) v1alpha1.ResourceTemplate {
	return v1alpha1.ResourceTemplate{
		ID: "cell-namespace",
		Template: rawExt(t, map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name": "${metadata.namespace}",
			},
		}),
	}
}

func TestRender_HappyPath(t *testing.T) {
	p := NewPipeline()

	input := &RenderInput{
		ProjectTypeSpec: &v1alpha1.ProjectTypeSpec{
			Resources: []v1alpha1.ResourceTemplate{
				nsTemplate(t),
				{
					ID: "default-deny-egress",
					Template: rawExt(t, map[string]any{
						"apiVersion": "networking.k8s.io/v1",
						"kind":       "NetworkPolicy",
						"metadata": map[string]any{
							"name":      "default-deny-egress",
							"namespace": "${metadata.namespace}",
						},
						"spec": map[string]any{
							"podSelector": map[string]any{},
							"policyTypes": []any{"Egress"},
						},
					}),
				},
			},
		},
		Metadata: fixtureMetadata(),
	}

	out, err := p.Render(input)
	require.NoError(t, err)
	require.Len(t, out.Entries, 2)

	ns := out.Entries[0]
	assert.Equal(t, "cell-namespace", ns.ID)
	assert.Equal(t, "Namespace", ns.Object["kind"])
	nsMeta := ns.Object["metadata"].(map[string]any)
	assert.Equal(t, "dp-org-myapp-dev-a1b2c3d", nsMeta["name"])

	np := out.Entries[1]
	assert.Equal(t, "default-deny-egress", np.ID)
	assert.Equal(t, "NetworkPolicy", np.Object["kind"])
	npMeta := np.Object["metadata"].(map[string]any)
	assert.Equal(t, "dp-org-myapp-dev-a1b2c3d", npMeta["namespace"])
}

func TestRender_IncludeWhenSkipsEntry(t *testing.T) {
	p := NewPipeline()

	input := &RenderInput{
		ProjectTypeSpec: &v1alpha1.ProjectTypeSpec{
			Resources: []v1alpha1.ResourceTemplate{
				nsTemplate(t),
				{
					ID:          "opt-in",
					IncludeWhen: "${parameters.enableOptIn}",
					Template: rawExt(t, map[string]any{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]any{"name": "opt-in"},
					}),
				},
			},
		},
		ProjectParameters: rawExt(t, map[string]any{"enableOptIn": false}),
		Metadata:          fixtureMetadata(),
	}

	out, err := p.Render(input)
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)
	assert.Equal(t, "cell-namespace", out.Entries[0].ID)
}

func TestRender_ForEachExpansion(t *testing.T) {
	p := NewPipeline()

	input := &RenderInput{
		ProjectTypeSpec: &v1alpha1.ProjectTypeSpec{
			Resources: []v1alpha1.ResourceTemplate{
				nsTemplate(t),
				{
					ID:      "allow-egress-cidr",
					ForEach: "${environmentConfigs.allowedEgressCidrs}",
					Var:     "cidr",
					Template: rawExt(t, map[string]any{
						"apiVersion": "networking.k8s.io/v1",
						"kind":       "NetworkPolicy",
						"metadata": map[string]any{
							"name": "allow-egress-${cidr}",
						},
					}),
				},
			},
		},
		EnvironmentConfigs: rawExt(t, map[string]any{
			"allowedEgressCidrs": []any{"10.0.0.0/8", "192.168.0.0/16"},
		}),
		Metadata: fixtureMetadata(),
	}

	out, err := p.Render(input)
	require.NoError(t, err)
	require.Len(t, out.Entries, 3)
	assert.Equal(t, "cell-namespace", out.Entries[0].ID)
	assert.Equal(t, "allow-egress-cidr-0", out.Entries[1].ID)
	assert.Equal(t, "allow-egress-cidr-1", out.Entries[2].ID)
}

func TestRender_ValidationFailureAborts(t *testing.T) {
	p := NewPipeline()

	input := &RenderInput{
		ProjectTypeSpec: &v1alpha1.ProjectTypeSpec{
			Validations: []v1alpha1.ValidationRule{
				{
					Rule:    "${parameters.tier == 'premium'}",
					Message: "tier must be premium",
				},
			},
			Resources: []v1alpha1.ResourceTemplate{nsTemplate(t)},
		},
		ProjectParameters: rawExt(t, map[string]any{"tier": "standard"}),
		Metadata:          fixtureMetadata(),
	}

	_, err := p.Render(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tier must be premium")
}

func TestRender_MissingNamespaceRejected(t *testing.T) {
	p := NewPipeline()

	input := &RenderInput{
		ProjectTypeSpec: &v1alpha1.ProjectTypeSpec{
			Resources: []v1alpha1.ResourceTemplate{nsTemplate(t)},
		},
		Metadata: MetadataContext{}, // Namespace empty
	}

	_, err := p.Render(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Namespace is empty")
}

// fixtureDataPlane returns a DataPlane with a secret store, observability
// plane ref, and both egress and ingress gateways configured. The egress
// external HTTPS host differs from fixtureEnvironment so tests can tell the
// raw dataplane surface apart from the merged effective gateway.
func fixtureDataPlane() *v1alpha1.DataPlane {
	dp := &v1alpha1.DataPlane{}
	dp.Name = "primary"
	dp.Spec.SecretStoreRef = &v1alpha1.SecretStoreRef{Name: "vault-store"}
	dp.Spec.ObservabilityPlaneRef = &v1alpha1.ObservabilityPlaneRef{
		Kind: v1alpha1.ObservabilityPlaneRefKind("ObservabilityPlane"),
		Name: "obs-default",
	}
	dp.Spec.Gateway = v1alpha1.GatewaySpec{
		Egress: &v1alpha1.GatewayNetworkSpec{
			External: &v1alpha1.GatewayEndpointSpec{
				Name:      "egress-gw",
				Namespace: "gateway-system",
				HTTPS:     &v1alpha1.GatewayListenerSpec{Port: 443, Host: "egress.dp.example.com"},
			},
		},
		Ingress: &v1alpha1.GatewayNetworkSpec{
			External: &v1alpha1.GatewayEndpointSpec{
				Name:      "ingress-gw",
				Namespace: "gateway-system",
				HTTPS:     &v1alpha1.GatewayListenerSpec{Port: 443, Host: "ingress.dp.example.com"},
			},
		},
	}
	return dp
}

// fixtureEnvironment returns an Environment that overrides only the egress
// external gateway; ingress is left unset so it falls back to the dataplane.
func fixtureEnvironment() *v1alpha1.Environment {
	env := &v1alpha1.Environment{}
	env.Name = "dev"
	env.Spec.Gateway = v1alpha1.GatewaySpec{
		Egress: &v1alpha1.GatewayNetworkSpec{
			External: &v1alpha1.GatewayEndpointSpec{
				Name:      "egress-gw",
				Namespace: "gateway-system",
				HTTPS:     &v1alpha1.GatewayListenerSpec{Port: 443, Host: "egress.dev.example.com"},
			},
		},
	}
	return env
}

func TestRender_ExposesDataPlaneAndGatewayContext(t *testing.T) {
	p := NewPipeline()

	dp := fixtureDataPlane()
	env := fixtureEnvironment()

	input := &RenderInput{
		ProjectTypeSpec: &v1alpha1.ProjectTypeSpec{
			Resources: []v1alpha1.ResourceTemplate{
				nsTemplate(t),
				{
					ID: "shared-secret",
					Template: rawExt(t, map[string]any{
						"apiVersion": "external-secrets.io/v1beta1",
						"kind":       "ExternalSecret",
						"metadata": map[string]any{
							"name":      "shared-secret",
							"namespace": "${metadata.namespace}",
						},
						"spec": map[string]any{
							"secretStoreRef": map[string]any{
								"name": "${dataplane.secretStore}",
								"kind": "ClusterSecretStore",
							},
							// Effective (merged) egress gateway — env wins.
							"effectiveEgressHost": "${gateway.egress.external.https.host}",
							// Raw dataplane egress gateway — dataplane value.
							"dpEgressHost": "${dataplane.gateway.egress.external.https.host}",
							// Ingress not overridden by env → dataplane fallback.
							"effectiveIngressHost": "${gateway.ingress.external.https.host}",
						},
					}),
				},
			},
		},
		Metadata:    fixtureMetadata(),
		DataPlane:   BuildDataPlaneContext(dp),
		Environment: BuildEnvironmentContext(env, dp),
	}

	out, err := p.Render(input)
	require.NoError(t, err)
	require.Len(t, out.Entries, 2)

	spec := out.Entries[1].Object["spec"].(map[string]any)
	assert.Equal(t, "vault-store", spec["secretStoreRef"].(map[string]any)["name"])
	assert.Equal(t, "egress.dev.example.com", spec["effectiveEgressHost"], "top-level gateway alias should be the env-merged value")
	assert.Equal(t, "egress.dp.example.com", spec["dpEgressHost"], "dataplane.gateway should be the raw dataplane value")
	assert.Equal(t, "ingress.dp.example.com", spec["effectiveIngressHost"], "ingress should fall back to the dataplane when env omits it")
}

func TestRender_GatewayNilGuardFallsBack(t *testing.T) {
	p := NewPipeline()

	input := &RenderInput{
		ProjectTypeSpec: &v1alpha1.ProjectTypeSpec{
			Resources: []v1alpha1.ResourceTemplate{
				nsTemplate(t),
				{
					ID: "guarded",
					Template: rawExt(t, map[string]any{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]any{"name": "guarded"},
						"data": map[string]any{
							"egressHost":  "${has(environment.gateway) ? environment.gateway.egress.external.https.host : 'no-gateway'}",
							"secretStore": "${has(dataplane.secretStore) ? dataplane.secretStore : 'no-store'}",
						},
					}),
				},
			},
		},
		Metadata: fixtureMetadata(),
		// DataPlane and Environment left zero-valued: no gateway, no secret store.
	}

	out, err := p.Render(input)
	require.NoError(t, err)
	require.Len(t, out.Entries, 2)

	data := out.Entries[1].Object["data"].(map[string]any)
	assert.Equal(t, "no-gateway", data["egressHost"])
	assert.Equal(t, "no-store", data["secretStore"])
}

func TestBuildEnvironmentContext_EnvOverridesDataPlaneByLeaf(t *testing.T) {
	envCtx := BuildEnvironmentContext(fixtureEnvironment(), fixtureDataPlane())

	require.NotNil(t, envCtx.Gateway)
	require.NotNil(t, envCtx.Gateway.Egress)
	require.NotNil(t, envCtx.Gateway.Egress.External)
	// Environment-level egress external wins over the dataplane.
	assert.Equal(t, "egress.dev.example.com", envCtx.Gateway.Egress.External.HTTPS.Host)

	require.NotNil(t, envCtx.Gateway.Ingress)
	require.NotNil(t, envCtx.Gateway.Ingress.External)
	// Ingress is unset on the environment, so it falls back to the dataplane.
	assert.Equal(t, "ingress.dp.example.com", envCtx.Gateway.Ingress.External.HTTPS.Host)
}

func TestBuildDataPlaneContext_ExtractsSecretStoreAndObservability(t *testing.T) {
	dpCtx := BuildDataPlaneContext(fixtureDataPlane())

	assert.Equal(t, "vault-store", dpCtx.SecretStore)
	require.NotNil(t, dpCtx.ObservabilityPlaneRef)
	assert.Equal(t, "ObservabilityPlane", dpCtx.ObservabilityPlaneRef.Kind)
	assert.Equal(t, "obs-default", dpCtx.ObservabilityPlaneRef.Name)
	require.NotNil(t, dpCtx.Gateway)
	require.NotNil(t, dpCtx.Gateway.Egress)
	assert.Equal(t, "egress.dp.example.com", dpCtx.Gateway.Egress.External.HTTPS.Host)
}

func TestBuildDataPlaneContext_NilReturnsZero(t *testing.T) {
	dpCtx := BuildDataPlaneContext(nil)
	assert.Empty(t, dpCtx.SecretStore)
	assert.Nil(t, dpCtx.Gateway)
	assert.Nil(t, dpCtx.ObservabilityPlaneRef)
}
