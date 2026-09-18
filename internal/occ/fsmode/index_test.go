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

// addClusterComponentTypeEntry adds a cluster-scoped ComponentType named "service". Cluster
// resources resolve by bare name, so the name has no reason to vary across the callers.
func addClusterComponentTypeEntry(t *testing.T, idx *index.Index) {
	t.Helper()
	const name = "service"
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

// addClusterTraitEntry adds a cluster-scoped Trait named "global-ingress". Cluster resources
// resolve by bare name, so the name has no reason to vary across the callers.
func addClusterTraitEntry(t *testing.T, idx *index.Index) {
	t.Helper()
	const name = "global-ingress"
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
	addClusterComponentTypeEntry(t, idx)
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
	addClusterTraitEntry(t, idx)
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
	addClusterComponentTypeEntry(t, idx)
	ocIndex := WrapIndex(idx)

	t.Run("found", func(t *testing.T) {
		cct, err := ocIndex.GetTypedClusterComponentType("service")
		require.NoError(t, err)
		assert.Equal(t, "deployment", cct.Spec.WorkloadType)
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
	addClusterTraitEntry(t, idx)
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

// addComponentReleaseEntry adds a ComponentRelease owned by component "web-app". Every caller
// exercises the same owning component across projects/namespaces, so componentName is fixed here.
func addComponentReleaseEntry(t *testing.T, idx *index.Index, namespace, name, projectName string) {
	t.Helper()
	const componentName = "web-app"
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

// addReleaseBindingEntry adds a ReleaseBinding owned by component "web-app". Every caller
// exercises the same owning component across projects/namespaces, so componentName is fixed here.
func addReleaseBindingEntry(t *testing.T, idx *index.Index, namespace, name, projectName, envName string) {
	t.Helper()
	const componentName = "web-app"
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
	addClusterComponentTypeEntry(t, idx)
	addClusterTraitEntry(t, idx)

	// Workloads
	addWorkloadEntry(t, idx, "ns1", "web-app-workload", "proj-a", "web-app")

	// ComponentReleases
	addComponentReleaseEntry(t, idx, "ns1", "web-app-20260401-v1", "proj-a")
	addComponentReleaseEntry(t, idx, "ns1", "web-app-20260401-v2", "proj-a")

	// ReleaseBindings
	addReleaseBindingEntry(t, idx, "ns1", "web-app-dev-binding", "proj-a", "dev")
	addReleaseBindingEntry(t, idx, "ns1", "web-app-staging-binding", "proj-a", "staging")

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
		entry, ok := ocIndex.GetComponentType("ns1", "http-service")
		require.True(t, ok)
		assert.Equal(t, "http-service", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetComponentType("ns1", "nonexistent")
		assert.False(t, ok)
	})
}

func TestGetTrait(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		entry, ok := ocIndex.GetTrait("ns1", "autoscaler")
		require.True(t, ok)
		assert.Equal(t, "autoscaler", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetTrait("ns1", "nonexistent")
		assert.False(t, ok)
	})
}

func TestGetWorkloadForComponent(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		entry, ok := ocIndex.GetWorkloadForComponent("ns1", "proj-a", "web-app")
		require.True(t, ok)
		assert.Equal(t, "web-app-workload", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetWorkloadForComponent("ns1", "proj-a", "nonexistent")
		assert.False(t, ok)
	})
}

func TestGetTypedWorkloadForComponent(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		wl, err := ocIndex.GetTypedWorkloadForComponent("ns1", "proj-a", "web-app")
		require.NoError(t, err)
		require.NotNil(t, wl)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := ocIndex.GetTypedWorkloadForComponent("ns1", "proj-a", "nonexistent")
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
		components := ocIndex.ListComponentsForProject("ns1", "proj-a")
		assert.Len(t, components, 2)
	})

	t.Run("project with no components", func(t *testing.T) {
		components := ocIndex.ListComponentsForProject("ns1", "nonexistent")
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
		releases := ocIndex.ListReleasesForComponent("ns1", "proj-a", "web-app")
		assert.Len(t, releases, 2)
	})

	t.Run("component with no releases", func(t *testing.T) {
		releases := ocIndex.ListReleasesForComponent("ns2", "proj-b", "worker")
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
		ct, err := ocIndex.GetTypedComponentType("ns1", "http-service")
		require.NoError(t, err)
		assert.Equal(t, "deployment", ct.Spec.WorkloadType)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := ocIndex.GetTypedComponentType("ns1", "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonexistent")
	})
}

func TestGetTypedTrait(t *testing.T) {
	ocIndex := buildFullIndex(t)

	t.Run("found", func(t *testing.T) {
		trait, err := ocIndex.GetTypedTrait("ns1", "autoscaler")
		require.NoError(t, err)
		require.NotNil(t, trait)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := ocIndex.GetTypedTrait("ns1", "nonexistent")
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
		entry, ok := ocIndex.GetDeploymentPipeline("ns1", "default-pipeline")
		require.True(t, ok)
		assert.Equal(t, "default-pipeline", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetDeploymentPipeline("ns1", "nonexistent")
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
		entry, ok := ocIndex.GetReleaseBindingForEnv("ns1", "proj-a", "web-app", "dev")
		require.True(t, ok)
		assert.Equal(t, "web-app-dev-binding", entry.Name())
	})

	t.Run("different environment", func(t *testing.T) {
		entry, ok := ocIndex.GetReleaseBindingForEnv("ns1", "proj-a", "web-app", "staging")
		require.True(t, ok)
		assert.Equal(t, "web-app-staging-binding", entry.Name())
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ocIndex.GetReleaseBindingForEnv("ns1", "proj-a", "web-app", "prod")
		assert.False(t, ok)
	})
}

// addComponentTypeEntryWithMarker adds a ComponentType whose spec carries a marker
// value so tests can assert which namespace's resource was resolved.
func addComponentTypeEntryWithMarker(t *testing.T, idx *index.Index, namespace, name, marker string) {
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
					"workloadType": marker,
					"resources":    []any{},
				},
			},
		},
		FilePath: "/repo/" + namespace + "/component-types/" + name + ".yaml",
	}
	require.NoError(t, idx.Add(entry))
}

func addTraitEntryWithMarker(t *testing.T, idx *index.Index, namespace, name, marker string) {
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
				"spec": map[string]any{
					"marker": marker,
				},
			},
		},
		FilePath: "/repo/" + namespace + "/traits/" + name + ".yaml",
	}
	require.NoError(t, idx.Add(entry))
}

func addDeploymentPipelineEntryWithMarker(t *testing.T, idx *index.Index, namespace, name, marker string) {
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
					"marker": marker,
				},
			},
		},
		FilePath: "/repo/" + namespace + "/pipelines/" + name + ".yaml",
	}
	require.NoError(t, idx.Add(entry))
}

// TestNamespaceCollisionIsolation verifies that namespace-scoped specialized
// lookups isolate resources that share a name across namespaces (issue #4148).
func TestNamespaceCollisionIsolation(t *testing.T) {
	idx := index.New("/repo")

	// Same-named namespace-scoped resources with distinct specs in two namespaces.
	addComponentTypeEntryWithMarker(t, idx, "team-a", "http-service", "deployment")
	addComponentTypeEntryWithMarker(t, idx, "team-b", "http-service", "statefulset")
	addTraitEntryWithMarker(t, idx, "team-a", "logging", "marker-a")
	addTraitEntryWithMarker(t, idx, "team-b", "logging", "marker-b")
	addDeploymentPipelineEntryWithMarker(t, idx, "team-a", "pipe", "pipe-a")
	addDeploymentPipelineEntryWithMarker(t, idx, "team-b", "pipe", "pipe-b")

	// Cluster-scoped resources resolve by bare name.
	addClusterComponentTypeEntry(t, idx)
	addClusterTraitEntry(t, idx)

	// Same project/component owners across namespaces.
	addWorkloadEntry(t, idx, "team-a", "web-app-wl", "proj", "web-app")
	addWorkloadEntry(t, idx, "team-b", "web-app-wl", "proj", "web-app")
	addComponentEntry(t, idx, "team-a", "web-app", "proj")
	addComponentEntry(t, idx, "team-b", "web-app", "proj")
	addComponentReleaseEntry(t, idx, "team-a", "web-app-20260401-1", "proj")
	addComponentReleaseEntry(t, idx, "team-b", "web-app-20260401-9", "proj")
	addReleaseBindingEntry(t, idx, "team-a", "web-app-dev-a", "proj", "dev")
	addReleaseBindingEntry(t, idx, "team-b", "web-app-dev-b", "proj", "dev")

	ocIndex := WrapIndex(idx)

	t.Run("component type isolated by namespace", func(t *testing.T) {
		ctA, err := ocIndex.GetTypedComponentType("team-a", "http-service")
		require.NoError(t, err)
		assert.Equal(t, "deployment", ctA.Spec.WorkloadType)

		ctB, err := ocIndex.GetTypedComponentType("team-b", "http-service")
		require.NoError(t, err)
		assert.Equal(t, "statefulset", ctB.Spec.WorkloadType)
	})

	t.Run("trait isolated by namespace", func(t *testing.T) {
		entryA, ok := ocIndex.GetTrait("team-a", "logging")
		require.True(t, ok)
		assert.Equal(t, "marker-a", entryA.GetNestedString("spec", "marker"))

		entryB, ok := ocIndex.GetTrait("team-b", "logging")
		require.True(t, ok)
		assert.Equal(t, "marker-b", entryB.GetNestedString("spec", "marker"))
	})

	t.Run("deployment pipeline isolated by namespace", func(t *testing.T) {
		entryA, ok := ocIndex.GetDeploymentPipeline("team-a", "pipe")
		require.True(t, ok)
		assert.Equal(t, "pipe-a", entryA.GetNestedString("spec", "marker"))

		entryB, ok := ocIndex.GetDeploymentPipeline("team-b", "pipe")
		require.True(t, ok)
		assert.Equal(t, "pipe-b", entryB.GetNestedString("spec", "marker"))
	})

	t.Run("cluster-scoped still resolve by bare name", func(t *testing.T) {
		_, ok := ocIndex.GetClusterComponentType("service")
		assert.True(t, ok)
		_, ok = ocIndex.GetClusterTrait("global-ingress")
		assert.True(t, ok)
	})

	t.Run("owner-based workload isolated by namespace", func(t *testing.T) {
		entryA, ok := ocIndex.GetWorkloadForComponent("team-a", "proj", "web-app")
		require.True(t, ok)
		assert.Equal(t, "team-a", entryA.Namespace())

		entryB, ok := ocIndex.GetWorkloadForComponent("team-b", "proj", "web-app")
		require.True(t, ok)
		assert.Equal(t, "team-b", entryB.Namespace())
	})

	t.Run("owner-based releases isolated by namespace", func(t *testing.T) {
		relA := ocIndex.ListReleasesForComponent("team-a", "proj", "web-app")
		require.Len(t, relA, 1)
		assert.Equal(t, "web-app-20260401-1", relA[0].Name())

		relB := ocIndex.ListReleasesForComponent("team-b", "proj", "web-app")
		require.Len(t, relB, 1)
		assert.Equal(t, "web-app-20260401-9", relB[0].Name())
	})

	t.Run("owner-based bindings isolated by namespace", func(t *testing.T) {
		bindA, ok := ocIndex.GetReleaseBindingForEnv("team-a", "proj", "web-app", "dev")
		require.True(t, ok)
		assert.Equal(t, "web-app-dev-a", bindA.Name())

		bindB, ok := ocIndex.GetReleaseBindingForEnv("team-b", "proj", "web-app", "dev")
		require.True(t, ok)
		assert.Equal(t, "web-app-dev-b", bindB.Name())
	})

	t.Run("components listed per namespace", func(t *testing.T) {
		compsA := ocIndex.ListComponentsForProject("team-a", "proj")
		require.Len(t, compsA, 1)
		assert.Equal(t, "team-a", compsA[0].Namespace())

		compsB := ocIndex.ListComponentsForProject("team-b", "proj")
		require.Len(t, compsB, 1)
		assert.Equal(t, "team-b", compsB[0].Namespace())
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
		comps := ocIndex.ListComponentsForProject("ns1", "proj-x")
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
		_, ok := ocIndex.GetDeploymentPipeline("ns1", "my-pipeline")
		assert.True(t, ok)
	})
}
