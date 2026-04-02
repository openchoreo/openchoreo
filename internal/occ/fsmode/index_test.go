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

// Helper functions for building test entries

func addComponentEntry(t *testing.T, idx *index.Index, namespace, name, projectName string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Component",
				"metadata": map[string]any{
					"name":      name,
					"namespace": namespace,
				},
				"spec": map[string]any{
					"owner": map[string]any{
						"projectName": projectName,
					},
					"componentType": map[string]any{
						"name": "deployment/http-service",
					},
				},
			},
		},
		FilePath: "/repo/projects/" + projectName + "/components/" + name + ".yaml",
	}
	require.NoError(t, idx.Add(entry))
}

func addComponentTypeEntry(t *testing.T, idx *index.Index, namespace, name string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "ComponentType",
				"metadata": map[string]any{
					"name":      name,
					"namespace": namespace,
				},
				"spec": map[string]any{
					"workloadType": "deployment",
					"resources":    []any{},
				},
			},
		},
		FilePath: "/repo/component-types/" + name + ".yaml",
	}
	require.NoError(t, idx.Add(entry))
}

func addTraitEntry(t *testing.T, idx *index.Index, namespace, name string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Trait",
				"metadata": map[string]any{
					"name":      name,
					"namespace": namespace,
				},
				"spec": map[string]any{},
			},
		},
		FilePath: "/repo/traits/" + name + ".yaml",
	}
	require.NoError(t, idx.Add(entry))
}

func addWorkloadEntry(t *testing.T, idx *index.Index, namespace, name, projectName, componentName string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Workload",
				"metadata": map[string]any{
					"name":      name,
					"namespace": namespace,
				},
				"spec": map[string]any{
					"owner": map[string]any{
						"projectName":   projectName,
						"componentName": componentName,
					},
					"container": map[string]any{
						"image": "nginx:latest",
					},
				},
			},
		},
		FilePath: "/repo/projects/" + projectName + "/workloads/" + name + ".yaml",
	}
	require.NoError(t, idx.Add(entry))
}

func addComponentReleaseEntry(t *testing.T, idx *index.Index, namespace, name, projectName, componentName string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "ComponentRelease",
				"metadata": map[string]any{
					"name":      name,
					"namespace": namespace,
				},
				"spec": map[string]any{
					"owner": map[string]any{
						"projectName":   projectName,
						"componentName": componentName,
					},
				},
			},
		},
		FilePath: "/repo/releases/" + name + ".yaml",
	}
	require.NoError(t, idx.Add(entry))
}

func addReleaseBindingEntry(t *testing.T, idx *index.Index, namespace, name, projectName, componentName, envName string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "ReleaseBinding",
				"metadata": map[string]any{
					"name":      name,
					"namespace": namespace,
				},
				"spec": map[string]any{
					"owner": map[string]any{
						"projectName":   projectName,
						"componentName": componentName,
					},
					"environment": envName,
				},
			},
		},
		FilePath: "/repo/bindings/" + name + ".yaml",
	}
	require.NoError(t, idx.Add(entry))
}

func addProjectEntry(t *testing.T, idx *index.Index, namespace, name string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Project",
				"metadata": map[string]any{
					"name":      name,
					"namespace": namespace,
				},
				"spec": map[string]any{},
			},
		},
		FilePath: "/repo/projects/" + name + ".yaml",
	}
	require.NoError(t, idx.Add(entry))
}

func addDeploymentPipelineEntry(t *testing.T, idx *index.Index, namespace, name string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "DeploymentPipeline",
				"metadata": map[string]any{
					"name":      name,
					"namespace": namespace,
				},
				"spec": map[string]any{
					"promotionPaths": []any{
						map[string]any{
							"sourceEnvironmentRef": map[string]any{"name": "dev"},
							"targetEnvironmentRef": map[string]any{"name": "staging"},
						},
					},
				},
			},
		},
		FilePath: "/repo/pipelines/" + name + ".yaml",
	}
	require.NoError(t, idx.Add(entry))
}

// buildFullIndex creates an index populated with all resource types for comprehensive testing
func buildFullIndex(t *testing.T) *Index {
	t.Helper()
	idx := index.New("/repo")

	// Components
	addComponentEntry(t, idx, "ns1", "web-app", "proj-a")
	addComponentEntry(t, idx, "ns1", "api-service", "proj-a")
	addComponentEntry(t, idx, "ns2", "worker", "proj-b")

	// ComponentTypes
	addComponentTypeEntry(t, idx, "ns1", "http-service")

	// Traits
	addTraitEntry(t, idx, "ns1", "autoscaler")

	// ClusterComponentTypes & ClusterTraits (already have helpers)
	addClusterComponentTypeEntry(t, idx, "service")
	addClusterTraitEntry(t, idx, "global-ingress")

	// Workloads
	addWorkloadEntry(t, idx, "ns1", "web-app-workload", "proj-a", "web-app")

	// ComponentReleases
	addComponentReleaseEntry(t, idx, "ns1", "web-app-20260401-v1", "proj-a", "web-app")
	addComponentReleaseEntry(t, idx, "ns1", "web-app-20260401-v2", "proj-a", "web-app")

	// ReleaseBindings
	addReleaseBindingEntry(t, idx, "ns1", "web-app-dev-binding", "proj-a", "web-app", "dev")
	addReleaseBindingEntry(t, idx, "ns1", "web-app-staging-binding", "proj-a", "web-app", "staging")

	// Projects
	addProjectEntry(t, idx, "ns1", "proj-a")

	// DeploymentPipelines
	addDeploymentPipelineEntry(t, idx, "ns1", "default-pipeline")

	return WrapIndex(idx)
}

func TestGetComponent(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		entry, ok := ocIndex.GetComponent("ns1", "web-app")
		require.True(t, ok)
		assert.Equal(t, "web-app", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetComponent("ns1", "nonexistent")
		assert.False(t, ok)
	})
}

func TestGetComponentType(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		entry, ok := ocIndex.GetComponentType("http-service")
		require.True(t, ok)
		assert.Equal(t, "http-service", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetComponentType("nonexistent")
		assert.False(t, ok)
	})
}

func TestGetTrait(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		entry, ok := ocIndex.GetTrait("autoscaler")
		require.True(t, ok)
		assert.Equal(t, "autoscaler", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetTrait("nonexistent")
		assert.False(t, ok)
	})
}

func TestGetWorkloadForComponent(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		entry, ok := ocIndex.GetWorkloadForComponent("proj-a", "web-app")
		require.True(t, ok)
		assert.Equal(t, "web-app-workload", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetWorkloadForComponent("proj-a", "nonexistent")
		assert.False(t, ok)
	})
}

func TestGetTypedWorkloadForComponent(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		wl, err := ocIndex.GetTypedWorkloadForComponent("proj-a", "web-app")
		require.NoError(t, err)
		require.NotNil(t, wl)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := ocIndex.GetTypedWorkloadForComponent("proj-a", "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonexistent")
	})
}

func TestListComponents(t *testing.T) {
	ocIndex := buildFullIndex(t)
	components := ocIndex.ListComponents()
	assert.Len(t, components, 3)
}

func TestListComponentsForProject(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("project with components", func(t *testing.T) {
		components := ocIndex.ListComponentsForProject("proj-a")
		assert.Len(t, components, 2)
	})

	t.Run("project with no components", func(t *testing.T) {
		components := ocIndex.ListComponentsForProject("nonexistent")
		assert.Empty(t, components)
	})
}

func TestListReleases(t *testing.T) {
	ocIndex := buildFullIndex(t)
	releases := ocIndex.ListReleases()
	assert.Len(t, releases, 2)
}

func TestListReleasesForComponent(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("component with releases", func(t *testing.T) {
		releases := ocIndex.ListReleasesForComponent("proj-a", "web-app")
		assert.Len(t, releases, 2)
	})

	t.Run("component with no releases", func(t *testing.T) {
		releases := ocIndex.ListReleasesForComponent("proj-b", "worker")
		assert.Empty(t, releases)
	})
}

func TestGetTypedComponent(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		comp, err := ocIndex.GetTypedComponent("ns1", "web-app")
		require.NoError(t, err)
		assert.Equal(t, "proj-a", comp.ProjectName())
	})

	t.Run("not found", func(t *testing.T) {
		_, err := ocIndex.GetTypedComponent("ns1", "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonexistent")
	})
}

func TestGetTypedComponentType(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		ct, err := ocIndex.GetTypedComponentType("http-service")
		require.NoError(t, err)
		assert.Equal(t, "deployment", ct.WorkloadType())
	})

	t.Run("not found", func(t *testing.T) {
		_, err := ocIndex.GetTypedComponentType("nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonexistent")
	})
}

func TestGetTypedTrait(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		trait, err := ocIndex.GetTypedTrait("autoscaler")
		require.NoError(t, err)
		require.NotNil(t, trait)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := ocIndex.GetTypedTrait("nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonexistent")
	})
}

func TestGetProject(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		entry, ok := ocIndex.GetProject("ns1", "proj-a")
		require.True(t, ok)
		assert.Equal(t, "proj-a", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetProject("ns1", "nonexistent")
		assert.False(t, ok)
	})
}

func TestGetDeploymentPipeline(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		entry, ok := ocIndex.GetDeploymentPipeline("default-pipeline")
		require.True(t, ok)
		assert.Equal(t, "default-pipeline", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetDeploymentPipeline("nonexistent")
		assert.False(t, ok)
	})
}

func TestListReleaseBindings(t *testing.T) {
	ocIndex := buildFullIndex(t)
	bindings := ocIndex.ListReleaseBindings()
	assert.Len(t, bindings, 2)
}

func TestGetReleaseBindingForEnv(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		entry, ok := ocIndex.GetReleaseBindingForEnv("proj-a", "web-app", "dev")
		require.True(t, ok)
		assert.Equal(t, "web-app-dev-binding", entry.Name())
	})

	t.Run("different environment", func(t *testing.T) {
		entry, ok := ocIndex.GetReleaseBindingForEnv("proj-a", "web-app", "staging")
		require.True(t, ok)
		assert.Equal(t, "web-app-staging-binding", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetReleaseBindingForEnv("proj-a", "web-app", "prod")
		assert.False(t, ok)
	})
}

func TestAddToSpecializedIndexesUnsafe(t *testing.T) {
	// Test that all resource types are properly indexed when added
	idx := index.New("/repo")
	ocIndex := WrapIndex(idx)

	// Add each resource type and verify it's indexed
	t.Run("component indexed by project", func(t *testing.T) {
		entry := &index.ResourceEntry{
			Resource: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "openchoreo.dev/v1alpha1",
					"kind":       "Component",
					"metadata":   map[string]any{"name": "svc-1", "namespace": "ns1"},
					"spec": map[string]any{
						"owner":         map[string]any{"projectName": "proj-x"},
						"componentType": map[string]any{"name": "deployment/web"},
					},
				},
			},
			FilePath: "/repo/comp.yaml",
		}
		require.NoError(t, idx.Add(entry))
		ocIndex.rebuildSpecializedIndexes()
		comps := ocIndex.ListComponentsForProject("proj-x")
		assert.Len(t, comps, 1)
	})

	t.Run("deployment pipeline indexed by name", func(t *testing.T) {
		entry := &index.ResourceEntry{
			Resource: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "openchoreo.dev/v1alpha1",
					"kind":       "DeploymentPipeline",
					"metadata":   map[string]any{"name": "my-pipeline", "namespace": "ns1"},
					"spec":       map[string]any{},
				},
			},
			FilePath: "/repo/pipeline.yaml",
		}
		require.NoError(t, idx.Add(entry))
		ocIndex.rebuildSpecializedIndexes()
		_, ok := ocIndex.GetDeploymentPipeline("my-pipeline")
		assert.True(t, ok)
	})
}
