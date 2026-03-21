// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fsmode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func TestExtractOwnerRef(t *testing.T) {
	tests := []struct {
		name          string
		entry         *index.ResourceEntry
		wantNil       bool
		wantProject   string
		wantComponent string
	}{
		{
			name: "component kind uses metadata.name as componentName",
			entry: &index.ResourceEntry{
				Resource: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "openchoreo.dev/v1alpha1",
						"kind":       "Component",
						"metadata": map[string]any{
							"name": "my-component",
						},
						"spec": map[string]any{
							"owner": map[string]any{
								"projectName": "my-project",
							},
						},
					},
				},
			},
			wantProject:   "my-project",
			wantComponent: "my-component",
		},
		{
			name: "component release kind uses spec.owner for both fields",
			entry: &index.ResourceEntry{
				Resource: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "openchoreo.dev/v1alpha1",
						"kind":       "ComponentRelease",
						"metadata": map[string]any{
							"name": "release-1",
						},
						"spec": map[string]any{
							"owner": map[string]any{
								"projectName":   "proj-a",
								"componentName": "comp-b",
							},
						},
					},
				},
			},
			wantProject:   "proj-a",
			wantComponent: "comp-b",
		},
		{
			name:    "nil entry",
			entry:   nil,
			wantNil: true,
		},
		{
			name: "missing owner in spec",
			entry: &index.ResourceEntry{
				Resource: &unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "openchoreo.dev/v1alpha1",
						"kind":       "Workload",
						"metadata": map[string]any{
							"name": "wl-1",
						},
						"spec": map[string]any{},
					},
				},
			},
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := ExtractOwnerRef(tt.entry)
			if tt.wantNil {
				assert.Nil(t, ref)
				return
			}
			require.NotNil(t, ref)
			assert.Equal(t, tt.wantProject, ref.ProjectName)
			assert.Equal(t, tt.wantComponent, ref.ComponentName)
		})
	}
}

func addClusterComponentTypeEntry(t *testing.T, idx *index.Index, name string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "ClusterComponentType",
				"metadata":   map[string]any{"name": name},
				"spec": map[string]any{
					"workloadType": "deployment",
					"resources":    []any{},
				},
			},
		},
		FilePath: "/repo/platform/cluster-component-types/" + name + ".yaml",
	}
	require.NoError(t, idx.Add(entry))
}

func addClusterTraitEntry(t *testing.T, idx *index.Index, name string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "ClusterTrait",
				"metadata":   map[string]any{"name": name},
				"spec":       map[string]any{},
			},
		},
		FilePath: "/repo/platform/cluster-traits/" + name + ".yaml",
	}
	require.NoError(t, idx.Add(entry))
}

func TestGetClusterComponentType(t *testing.T) {
	idx := index.New("/repo")
	addClusterComponentTypeEntry(t, idx, "service")
	ocIndex := WrapIndex(idx)

	t.Run("found", func(t *testing.T) {
		entry, ok := ocIndex.GetClusterComponentType("service")
		require.True(t, ok)
		assert.Equal(t, "service", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetClusterComponentType("nonexistent")
		assert.False(t, ok)
	})
}

func TestGetClusterTrait(t *testing.T) {
	idx := index.New("/repo")
	addClusterTraitEntry(t, idx, "global-ingress")
	ocIndex := WrapIndex(idx)

	t.Run("found", func(t *testing.T) {
		entry, ok := ocIndex.GetClusterTrait("global-ingress")
		require.True(t, ok)
		assert.Equal(t, "global-ingress", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetClusterTrait("nonexistent")
		assert.False(t, ok)
	})
}

func TestGetTypedClusterComponentType(t *testing.T) {
	idx := index.New("/repo")
	addClusterComponentTypeEntry(t, idx, "service")
	ocIndex := WrapIndex(idx)

	t.Run("found", func(t *testing.T) {
		cct, err := ocIndex.GetTypedClusterComponentType("service")
		require.NoError(t, err)
		assert.Equal(t, "deployment", cct.WorkloadType())
	})

	t.Run("not found", func(t *testing.T) {
		_, err := ocIndex.GetTypedClusterComponentType("nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cluster component type")
		assert.Contains(t, err.Error(), "nonexistent")
	})
}

func TestGetTypedClusterTrait(t *testing.T) {
	idx := index.New("/repo")
	addClusterTraitEntry(t, idx, "global-ingress")
	ocIndex := WrapIndex(idx)

	t.Run("found", func(t *testing.T) {
		ct, err := ocIndex.GetTypedClusterTrait("global-ingress")
		require.NoError(t, err)
		require.NotNil(t, ct)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := ocIndex.GetTypedClusterTrait("nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cluster trait")
		assert.Contains(t, err.Error(), "nonexistent")
	})
}
