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
		CellNamespace:    "dp-org-myapp-dev-a1b2c3d",
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
				"name": "${metadata.cellNamespace}",
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
							"namespace": "${metadata.cellNamespace}",
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

func TestRender_MissingCellNamespaceRejected(t *testing.T) {
	p := NewPipeline()

	input := &RenderInput{
		ProjectTypeSpec: &v1alpha1.ProjectTypeSpec{
			Resources: []v1alpha1.ResourceTemplate{nsTemplate(t)},
		},
		Metadata: MetadataContext{}, // CellNamespace empty
	}

	_, err := p.Render(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CellNamespace is empty")
}
