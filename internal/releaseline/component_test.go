// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
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
	assert.Regexp(t, `^orders-v2-[0-9a-f]{8}-dev$`, ScopedName("orders", "v2", "dev"))
	assert.Equal(t, ComponentLineName("orders", "v2")+"-workload", ScopedName("orders", "v2", "workload"))

	// A line's names never equal those of a component named "{component}-{line}", or of the same
	// component's default line in an environment that starts with the line name.
	assert.NotEqual(t, ScopedName("orders-v2", "", "workload"), ScopedName("orders", "v2", "workload"))
	assert.NotEqual(t, ScopedName("api", "", "pre-prod"), ScopedName("api", "pre", "prod"))
}

func TestReleaseName(t *testing.T) {
	long := strings.Repeat("c", 60)
	assert.Equal(t, "orders-544774ffc5", ReleaseName("orders", "", "544774ffc5"))
	assert.Equal(t, long+"-544774ffc5", ReleaseName(long, "", "544774ffc5"), "the default line's names never change")

	// A release name of a line starts like the line's other names when it fits.
	assert.Equal(t, ScopedName("orders", "v2", "544774ffc5"), ReleaseName("orders", "v2", "544774ffc5"))

	for _, c := range []struct{ component, line string }{
		{"customer-notification-service", "feature-checkout"},
		{long, strings.Repeat("l", 60)},
	} {
		for _, suffix := range []string{"544774ffc5", "20261027-1", "20261027-999"} {
			name := ReleaseName(c.component, c.line, suffix)
			assert.LessOrEqual(t, len(name), 63, name)
			assert.Equal(t, ReleaseNameBase(c.component, c.line)+"-"+suffix, name)
		}
	}
	assert.NotEqual(t, ReleaseNameBase(long, "a"), ReleaseNameBase(long, "b"))
}

func TestComponentLineName(t *testing.T) {
	assert.Equal(t, "hello", ComponentLineName("hello", ""))
	assert.Regexp(t, `^hello-next-[0-9a-f]{8}$`, ComponentLineName("hello", "next"))
	assert.NotEqual(t, ComponentLineName("a-b", "c"), ComponentLineName("a", "b-c"))

	component, line := strings.Repeat("c", 40), strings.Repeat("l", 40)
	a, b := ComponentLineName(component, line), ComponentLineName(component, line+"x")
	assert.LessOrEqual(t, len(a), dpkubernetes.MaxServiceNameLength)
	assert.NotEqual(t, a, b)
}

func TestObjectBaseName(t *testing.T) {
	assert.Regexp(t, `^hello-next-dev-[0-9a-f]{8}$`, ObjectBaseName("hello", "next", "dev"))
	assert.NotEqual(t, ObjectBaseName("a-b", "c", "dev"), ObjectBaseName("a", "b-c", "dev"))
}

func TestRenderedReleaseName(t *testing.T) {
	// A RenderedRelease name of a line is its ObjectBaseName when it fits.
	assert.Equal(t, ObjectBaseName("hello", "next", "development"), RenderedReleaseName("hello", "next", "development"))

	for _, c := range [][3]string{
		{"customer-notification-service", "next", "development"},
		{"customer-notification-service", "feature-checkout", "development"},
		{strings.Repeat("c", 60), strings.Repeat("l", 60), strings.Repeat("e", 60)},
	} {
		name := RenderedReleaseName(c[0], c[1], c[2])
		assert.LessOrEqual(t, len(name+"-observability"), 63, name)
	}
	assert.NotEqual(t,
		RenderedReleaseName("customer-notification-service", "feature-checkout", "development"),
		RenderedReleaseName("customer-notification-service", "feature-checkout", "staging"))
}
