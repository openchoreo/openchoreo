// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func raw(t *testing.T, v any) *runtime.RawExtension {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: b}
}

func decode(t *testing.T, ext *runtime.RawExtension) map[string]any {
	t.Helper()
	require.NotNil(t, ext)
	var m map[string]any
	require.NoError(t, json.Unmarshal(ext.Raw, &m))
	return m
}

func TestEffectiveComponent(t *testing.T) {
	comp := &openchoreov1alpha1.Component{
		Spec: openchoreov1alpha1.ComponentSpec{
			Parameters: raw(t, map[string]any{
				"replicas": 2,
				"ports":    []any{80, 443},
				"limits":   map[string]any{"cpu": "1", "memory": "1Gi"},
			}),
			Traits: []openchoreov1alpha1.ComponentTrait{{
				Name:         "alert",
				InstanceName: "errors",
				Parameters:   raw(t, map[string]any{"enabled": true, "threshold": 100}),
			}},
		},
	}
	line := &openchoreov1alpha1.ReleaseLine{
		Spec: openchoreov1alpha1.ReleaseLineSpec{
			Parameters: raw(t, map[string]any{
				"ports":  []any{8080},
				"limits": map[string]any{"memory": "2Gi"},
			}),
			Traits: []openchoreov1alpha1.ComponentTrait{
				// Same identity as the inherited trait: empty kind defaults to Trait.
				{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "alert", InstanceName: "errors", Parameters: raw(t, map[string]any{"threshold": 20})},
				{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "hpa", InstanceName: "scale"},
			},
		},
	}

	got, err := EffectiveComponent(comp, line)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"replicas": float64(2),
		"ports":    []any{float64(8080)},
		"limits":   map[string]any{"cpu": "1", "memory": "2Gi"},
	}, decode(t, got.Spec.Parameters), "maps merge, lists and scalars replace")

	require.Len(t, got.Spec.Traits, 2)
	assert.Equal(t, map[string]any{"enabled": true, "threshold": float64(20)}, decode(t, got.Spec.Traits[0].Parameters))
	assert.Equal(t, "hpa", got.Spec.Traits[1].Name)

	assert.Equal(t, map[string]any{"enabled": true, "threshold": float64(100)}, decode(t, comp.Spec.Traits[0].Parameters),
		"the Component itself is not mutated")
}

func TestEffectiveComponent_NoOverrides(t *testing.T) {
	comp := &openchoreov1alpha1.Component{Spec: openchoreov1alpha1.ComponentSpec{Parameters: raw(t, map[string]any{"replicas": 1})}}
	got, err := EffectiveComponent(comp, &openchoreov1alpha1.ReleaseLine{})
	require.NoError(t, err)
	assert.Equal(t, comp.Spec, got.Spec)
}

func TestScopedName(t *testing.T) {
	assert.Equal(t, "orders-dev", ScopedName("orders", "", "dev"))
	assert.Equal(t, "orders-v2-dev", ScopedName("orders", "v2", "dev"))
}
