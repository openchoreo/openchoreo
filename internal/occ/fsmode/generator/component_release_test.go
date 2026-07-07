// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// addComponent adds a Component resource entry to the index.
func addComponent(t *testing.T, idx *index.Index, name, project, componentTypeName string, filePath string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Component",
				"metadata": map[string]any{
					"name":      name,
					"namespace": "default",
				},
				"spec": map[string]any{
					"owner": map[string]any{
						"projectName": project,
					},
					"componentType": map[string]any{
						"name": componentTypeName,
						"kind": "ComponentType",
					},
				},
			},
		},
		FilePath: filePath,
	}
	require.NoError(t, idx.Add(entry))
}

// addComponentType adds a ComponentType resource entry to the index.
func addComponentType(t *testing.T, idx *index.Index, name, workloadType string, filePath string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "ComponentType",
				"metadata": map[string]any{
					"name":      name,
					"namespace": "default",
				},
				"spec": map[string]any{
					"workloadType": workloadType,
					"resources":    []any{},
					"schema":       map[string]any{},
				},
			},
		},
		FilePath: filePath,
	}
	require.NoError(t, idx.Add(entry))
}

// addWorkload adds a Workload resource entry to the index.
func addWorkload(t *testing.T, idx *index.Index, namespace, name, project, component string, workloadObj map[string]any, filePath string) {
	t.Helper()
	spec := map[string]any{
		"owner": map[string]any{
			"projectName":   project,
			"componentName": component,
		},
	}
	for k, v := range workloadObj {
		spec[k] = v
	}
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Workload",
				"metadata": map[string]any{
					"name":      name,
					"namespace": namespace,
				},
				"spec": spec,
			},
		},
		FilePath: filePath,
	}
	require.NoError(t, idx.Add(entry))
}

// addTrait adds a Trait resource entry to the index.
func addTrait(t *testing.T, idx *index.Index, name string, spec map[string]any, filePath string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Trait",
				"metadata": map[string]any{
					"name":      name,
					"namespace": "default",
				},
				"spec": spec,
			},
		},
		FilePath: filePath,
	}
	require.NoError(t, idx.Add(entry))
}

// addComponentWithTraits adds a Component with trait references to the index.
func addComponentWithTraits(t *testing.T, idx *index.Index, namespace string, traits []map[string]any, filePath string) {
	t.Helper()
	const name = "my-svc"
	const project = "myproj"
	const componentTypeName = "deployment/service"
	spec := map[string]any{
		"owner": map[string]any{
			"projectName": project,
		},
		"componentType": map[string]any{
			"name": componentTypeName,
			"kind": "ComponentType",
		},
	}
	if len(traits) > 0 {
		spec["traits"] = traits
	}
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Component",
				"metadata": map[string]any{
					"name":      name,
					"namespace": namespace,
				},
				"spec": spec,
			},
		},
		FilePath: filePath,
	}
	require.NoError(t, idx.Add(entry))
}

func TestGenerateRelease_ManifestShape(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "myproj"
		componentName = "my-svc"
		releaseName   = "my-svc-release-1"
	)

	idx := index.New("/repo")

	addComponentWithTraits(t, idx, namespace,
		[]map[string]any{
			{"kind": "Trait", "name": "ingress", "instanceName": "ingress-1"},
			{"name": "logging", "instanceName": "logging-1"},
		},
		"/repo/projects/myproj/components/my-svc/component.yaml")

	addComponentType(t, idx, "service", "deployment",
		"/repo/platform/component-types/service.yaml")

	addWorkload(t, idx, namespace, "my-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{"image": "reg/my-svc:v1"},
		},
		"/repo/projects/myproj/components/my-svc/workload.yaml")

	addTrait(t, idx, "ingress",
		map[string]any{"creates": []any{map[string]any{"template": map[string]any{"apiVersion": "networking.k8s.io/v1", "kind": "Ingress"}}}},
		"/repo/platform/traits/ingress.yaml")

	addTrait(t, idx, "logging",
		map[string]any{"creates": []any{map[string]any{"template": map[string]any{"apiVersion": "v1", "kind": "ConfigMap"}}}},
		"/repo/platform/traits/logging.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   releaseName,
	})
	require.NoError(t, err)

	// Verify top-level metadata
	assert.Equal(t, "ComponentRelease", release.GetKind())
	assert.Equal(t, releaseName, release.GetName())
	assert.Equal(t, namespace, release.GetNamespace())

	// Verify spec.componentType
	ctKind, _, _ := unstructured.NestedString(release.Object, "spec", "componentType", "kind")
	ctName, _, _ := unstructured.NestedString(release.Object, "spec", "componentType", "name")
	ctWorkloadType, _, _ := unstructured.NestedString(release.Object, "spec", "componentType", "spec", "workloadType")
	assert.Equal(t, "ComponentType", ctKind)
	assert.Equal(t, "deployment/service", ctName)
	assert.Equal(t, "deployment", ctWorkloadType)

	// Verify spec.traits[]
	traitsSlice, ok, _ := unstructured.NestedSlice(release.Object, "spec", "traits")
	require.True(t, ok, "expected spec.traits to exist")
	require.Len(t, traitsSlice, 2)

	for i, expected := range []struct{ kind, name string }{
		{"Trait", "ingress"},
		{"Trait", "logging"},
	} {
		traitMap, ok := traitsSlice[i].(map[string]interface{})
		require.True(t, ok, "spec.traits[%d] is not a map", i)
		assert.Equal(t, expected.kind, traitMap["kind"], "spec.traits[%d].kind", i)
		assert.Equal(t, expected.name, traitMap["name"], "spec.traits[%d].name", i)
		assert.NotNil(t, traitMap["spec"], "spec.traits[%d].spec should not be nil", i)
	}

	// Verify spec.componentProfile.traits[]
	profileTraits, ok, _ := unstructured.NestedSlice(release.Object, "spec", "componentProfile", "traits")
	require.True(t, ok, "expected spec.componentProfile.traits to exist")
	require.Len(t, profileTraits, 2)

	for i, expected := range []struct{ kind, name, instanceName string }{
		{"Trait", "ingress", "ingress-1"},
		{"Trait", "logging", "logging-1"},
	} {
		pt, ok := profileTraits[i].(map[string]interface{})
		require.True(t, ok, "spec.componentProfile.traits[%d] is not a map", i)
		assert.Equal(t, expected.kind, pt["kind"], "spec.componentProfile.traits[%d].kind", i)
		assert.Equal(t, expected.name, pt["name"], "spec.componentProfile.traits[%d].name", i)
		assert.Equal(t, expected.instanceName, pt["instanceName"], "spec.componentProfile.traits[%d].instanceName", i)
	}

	// Verify spec.owner
	ownerComp, _, _ := unstructured.NestedString(release.Object, "spec", "owner", "componentName")
	ownerProj, _, _ := unstructured.NestedString(release.Object, "spec", "owner", "projectName")
	assert.Equal(t, componentName, ownerComp)
	assert.Equal(t, projectName, ownerProj)
}

// addClusterComponentType adds a ClusterComponentType resource entry to the index.
func addClusterComponentType(t *testing.T, idx *index.Index, name, workloadType string, filePath string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "ClusterComponentType",
				"metadata": map[string]any{
					"name": name,
				},
				"spec": map[string]any{
					"workloadType": workloadType,
					"resources":    []any{},
					"schema":       map[string]any{},
				},
			},
		},
		FilePath: filePath,
	}
	require.NoError(t, idx.Add(entry))
}

// addClusterTrait adds a ClusterTrait resource entry to the index.
func addClusterTrait(t *testing.T, idx *index.Index, name string, spec map[string]any, filePath string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "ClusterTrait",
				"metadata": map[string]any{
					"name": name,
				},
				"spec": spec,
			},
		},
		FilePath: filePath,
	}
	require.NoError(t, idx.Add(entry))
}

// addComponentWithKind adds a Component with a specific componentType kind to the index.
func addComponentWithKind(t *testing.T, idx *index.Index, namespace, name, project, componentTypeName, ctKind string, filePath string) {
	t.Helper()
	spec := map[string]any{
		"owner": map[string]any{
			"projectName": project,
		},
		"componentType": map[string]any{
			"name": componentTypeName,
			"kind": ctKind,
		},
	}
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Component",
				"metadata": map[string]any{
					"name":      name,
					"namespace": namespace,
				},
				"spec": spec,
			},
		},
		FilePath: filePath,
	}
	require.NoError(t, idx.Add(entry))
}

func TestGenerateRelease_ClusterComponentType(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "myproj"
		componentName = "my-svc"
		releaseName   = "my-svc-release-1"
	)

	idx := index.New("/repo")

	addComponentWithKind(t, idx, namespace, componentName, projectName, "deployment/service",
		"ClusterComponentType",
		"/repo/projects/myproj/components/my-svc/component.yaml")

	addClusterComponentType(t, idx, "service", "deployment",
		"/repo/platform/cluster-component-types/service.yaml")

	addWorkload(t, idx, namespace, "my-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{"image": "reg/my-svc:v1"},
		},
		"/repo/projects/myproj/components/my-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   releaseName,
	})
	require.NoError(t, err)

	// Verify spec.componentType
	ctKind, _, _ := unstructured.NestedString(release.Object, "spec", "componentType", "kind")
	ctName, _, _ := unstructured.NestedString(release.Object, "spec", "componentType", "name")
	ctWorkloadType, _, _ := unstructured.NestedString(release.Object, "spec", "componentType", "spec", "workloadType")
	assert.Equal(t, "ClusterComponentType", ctKind)
	assert.Equal(t, "deployment/service", ctName)
	assert.Equal(t, "deployment", ctWorkloadType)
}

func TestGenerateRelease_ClusterTrait(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "myproj"
		componentName = "my-svc"
		releaseName   = "my-svc-release-1"
	)

	idx := index.New("/repo")

	addComponentWithTraits(t, idx, namespace,
		[]map[string]any{
			{"kind": "ClusterTrait", "name": "global-ingress", "instanceName": "gi-1"},
		},
		"/repo/projects/myproj/components/my-svc/component.yaml")

	addComponentType(t, idx, "service", "deployment",
		"/repo/platform/component-types/service.yaml")

	addClusterTrait(t, idx, "global-ingress",
		map[string]any{"creates": []any{map[string]any{"template": map[string]any{"apiVersion": "networking.k8s.io/v1", "kind": "Ingress"}}}},
		"/repo/platform/cluster-traits/global-ingress.yaml")

	addWorkload(t, idx, namespace, "my-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{"image": "reg/my-svc:v1"},
		},
		"/repo/projects/myproj/components/my-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   releaseName,
	})
	require.NoError(t, err)

	// Verify spec.traits[]
	traitsSlice, ok, _ := unstructured.NestedSlice(release.Object, "spec", "traits")
	require.True(t, ok, "expected spec.traits to exist")
	require.Len(t, traitsSlice, 1)

	traitMap, ok := traitsSlice[0].(map[string]interface{})
	require.True(t, ok, "spec.traits[0] is not a map")
	assert.Equal(t, "ClusterTrait", traitMap["kind"])
	assert.Equal(t, "global-ingress", traitMap["name"])
	assert.NotNil(t, traitMap["spec"])

	// Verify spec.componentProfile.traits[]
	profileTraits, ok, _ := unstructured.NestedSlice(release.Object, "spec", "componentProfile", "traits")
	require.True(t, ok, "expected spec.componentProfile.traits to exist")
	require.Len(t, profileTraits, 1)

	pt, ok := profileTraits[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ClusterTrait", pt["kind"])
	assert.Equal(t, "global-ingress", pt["name"])
	assert.Equal(t, "gi-1", pt["instanceName"])
}

func TestGenerateRelease_MissingClusterTraitErrors(t *testing.T) {
	const (
		namespace     = "staging"
		projectName   = "myproj"
		componentName = "my-svc"
	)

	idx := index.New("/repo")

	addComponentWithTraits(t, idx, namespace,
		[]map[string]any{
			{"kind": "ClusterTrait", "name": "global-ingress", "instanceName": "gi-1"},
		},
		"/repo/projects/myproj/components/my-svc/component.yaml")

	addComponentType(t, idx, "service", "deployment",
		"/repo/platform/component-types/service.yaml")

	addWorkload(t, idx, namespace, "my-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{"image": "reg/my-svc:v1"},
		},
		"/repo/projects/myproj/components/my-svc/workload.yaml")

	// Note: ClusterTrait "global-ingress" is NOT added to the index

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	_, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   "test-release",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cluster trait")
	assert.Contains(t, err.Error(), "global-ingress")
}

func TestGenerateRelease_MissingClusterComponentTypeErrors(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "myproj"
		componentName = "my-svc"
	)

	idx := index.New("/repo")

	// Component references ClusterComponentType "service" but it doesn't exist in the index
	addComponentWithKind(t, idx, namespace, componentName, projectName, "deployment/service",
		"ClusterComponentType",
		"/repo/projects/myproj/components/my-svc/component.yaml")

	addWorkload(t, idx, namespace, "my-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{"image": "reg/my-svc:v1"},
		},
		"/repo/projects/myproj/components/my-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	_, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   "test-release",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cluster component type")
	assert.Contains(t, err.Error(), "service")
}

func TestGenerateRelease_UnsupportedComponentTypeKindErrors(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "myproj"
		componentName = "my-svc"
	)

	idx := index.New("/repo")

	addComponentWithKind(t, idx, namespace, componentName, projectName, "deployment/service",
		"InvalidKind",
		"/repo/projects/myproj/components/my-svc/component.yaml")

	addWorkload(t, idx, namespace, "my-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{"image": "reg/my-svc:v1"},
		},
		"/repo/projects/myproj/components/my-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	_, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   "test-release",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported component type kind")
	assert.Contains(t, err.Error(), "InvalidKind")
}

func TestGenerateRelease_WorkloadEndpointsIncluded(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "doclet"
		componentName = "document-svc"
		releaseName   = "document-svc-abc123"
	)

	idx := index.New("/repo")

	addComponent(t, idx, componentName, projectName, "deployment/service",
		"/repo/projects/doclet/components/document-svc/component.yaml")

	addComponentType(t, idx, "service", "deployment",
		"/repo/platform/component-types/service.yaml")

	addWorkload(t, idx, namespace, "document-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{
				"image": "my-registry/document-svc:v1",
				"env": []any{
					map[string]any{"key": "PORT", "value": "8080"},
				},
			},
			"endpoints": map[string]any{
				"http": map[string]any{
					"type": "HTTP",
					"port": int64(8080),
				},
			},
		},
		"/repo/projects/doclet/components/document-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   releaseName,
	})
	require.NoError(t, err)

	workload, ok, err := unstructured.NestedMap(release.Object, "spec", "workload")
	require.NoError(t, err)
	require.True(t, ok, "expected spec.workload to exist")

	assert.NotNil(t, workload["container"], "expected spec.workload.container to exist")

	endpoints, ok := workload["endpoints"]
	require.True(t, ok, "expected spec.workload.endpoints to exist")
	require.NotNil(t, endpoints)

	endpointsMap, ok := endpoints.(map[string]interface{})
	require.True(t, ok, "expected endpoints to be a map")

	httpEndpoint, ok := endpointsMap["http"]
	require.True(t, ok, "expected 'http' endpoint in endpoints map")

	httpMap, ok := httpEndpoint.(map[string]interface{})
	require.True(t, ok, "expected http endpoint to be a map")

	assert.Equal(t, "HTTP", httpMap["type"])
	assert.Equal(t, int64(8080), httpMap["port"])
}

func TestGenerateRelease_WorkloadConnectionsIncluded(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "doclet"
		componentName = "document-svc"
		releaseName   = "document-svc-abc123"
	)

	idx := index.New("/repo")

	addComponent(t, idx, componentName, projectName, "deployment/service",
		"/repo/projects/doclet/components/document-svc/component.yaml")

	addComponentType(t, idx, "service", "deployment",
		"/repo/platform/component-types/service.yaml")

	addWorkload(t, idx, namespace, "document-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{
				"image": "my-registry/document-svc:v1",
			},
			"endpoints": map[string]any{
				"http": map[string]any{
					"type": "HTTP",
					"port": int64(8080),
				},
			},
			"dependencies": map[string]any{
				"endpoints": []any{
					map[string]any{
						"component":  "postgres",
						"name":       "tcp",
						"visibility": "project",
						"envBindings": map[string]any{
							"address": "DATABASE_URL",
						},
					},
					map[string]any{
						"component":  "nats",
						"name":       "tcp",
						"visibility": "project",
						"envBindings": map[string]any{
							"address": "NATS_URL",
						},
					},
				},
			},
		},
		"/repo/projects/doclet/components/document-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   releaseName,
	})
	require.NoError(t, err)

	workload, ok, err := unstructured.NestedMap(release.Object, "spec", "workload")
	require.NoError(t, err)
	require.True(t, ok, "expected spec.workload to exist")

	dependencies, ok := workload["dependencies"]
	require.True(t, ok, "expected spec.workload.dependencies to exist")
	require.NotNil(t, dependencies)

	depsMap, ok := dependencies.(map[string]interface{})
	require.True(t, ok, "expected dependencies to be a map")

	connSlice, ok := depsMap["endpoints"].([]interface{})
	require.True(t, ok, "expected dependencies.endpoints to be a slice")
	require.Len(t, connSlice, 2)

	first := connSlice[0].(map[string]interface{})
	assert.Equal(t, "postgres", first["component"])

	second := connSlice[1].(map[string]interface{})
	assert.Equal(t, "nats", second["component"])
}

func TestGenerateRelease_ProjectNameMismatch(t *testing.T) {
	idx := index.New("/repo")

	addComponent(t, idx, "my-svc", "actual-project", "deployment/service",
		"/repo/projects/actual-project/components/my-svc/component.yaml")
	addComponentType(t, idx, "service", "deployment",
		"/repo/platform/component-types/service.yaml")
	addWorkload(t, idx, "default", "my-svc-workload", "actual-project", "my-svc",
		map[string]any{"container": map[string]any{"image": "img:v1"}},
		"/repo/projects/actual-project/components/my-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	_, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: "my-svc",
		ProjectName:   "wrong-project",
		Namespace:     "default",
		ReleaseName:   "test-release",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "belongs to project")
}

func TestGenerateRelease_UnsupportedTraitKindErrors(t *testing.T) {
	idx := index.New("/repo")

	addComponentWithTraits(t, idx, "default",
		[]map[string]any{
			{"kind": "UnknownTraitKind", "name": "my-trait", "instanceName": "t-1"},
		},
		"/repo/projects/myproj/components/my-svc/component.yaml")

	addComponentType(t, idx, "service", "deployment",
		"/repo/platform/component-types/service.yaml")

	addWorkload(t, idx, "default", "my-svc-workload", "myproj", "my-svc",
		map[string]any{"container": map[string]any{"image": "img:v1"}},
		"/repo/projects/myproj/components/my-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	_, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: "my-svc",
		ProjectName:   "myproj",
		Namespace:     "default",
		ReleaseName:   "test-release",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported trait kind")
}

func TestGenerateRelease_WithComponentParameters(t *testing.T) {
	idx := index.New("/repo")

	// Add component with parameters
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Component",
				"metadata":   map[string]any{"name": "my-svc", "namespace": "default"},
				"spec": map[string]any{
					"owner":         map[string]any{"projectName": "myproj"},
					"componentType": map[string]any{"name": "deployment/service", "kind": "ComponentType"},
					"parameters":    map[string]any{"port": float64(8080), "replicas": float64(3)},
				},
			},
		},
		FilePath: "/repo/comp.yaml",
	}
	require.NoError(t, idx.Add(entry))

	addComponentType(t, idx, "service", "deployment",
		"/repo/platform/component-types/service.yaml")
	addWorkload(t, idx, "default", "my-svc-workload", "myproj", "my-svc",
		map[string]any{"container": map[string]any{"image": "img:v1"}},
		"/repo/wl.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: "my-svc",
		ProjectName:   "myproj",
		Namespace:     "default",
		ReleaseName:   "test-release",
	})
	require.NoError(t, err)

	// Verify componentProfile.parameters is present
	params, ok, _ := unstructured.NestedMap(release.Object, "spec", "componentProfile", "parameters")
	require.True(t, ok, "expected spec.componentProfile.parameters")
	assert.Equal(t, float64(8080), params["port"])
	assert.Equal(t, float64(3), params["replicas"])
}

func TestGenerateRelease_DuplicateTraitsDeduped(t *testing.T) {
	idx := index.New("/repo")

	// Component references the same trait twice with different instance names
	addComponentWithTraits(t, idx, "default",
		[]map[string]any{
			{"kind": "Trait", "name": "ingress", "instanceName": "ingress-a"},
			{"kind": "Trait", "name": "ingress", "instanceName": "ingress-b"},
		},
		"/repo/comp.yaml")

	addComponentType(t, idx, "service", "deployment", "/repo/ct.yaml")
	addTrait(t, idx, "ingress", map[string]any{}, "/repo/traits/ingress.yaml")
	addWorkload(t, idx, "default", "my-svc-workload", "myproj", "my-svc",
		map[string]any{"container": map[string]any{"image": "img:v1"}}, "/repo/wl.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: "my-svc",
		ProjectName:   "myproj",
		Namespace:     "default",
		ReleaseName:   "test-release",
	})
	require.NoError(t, err)

	// spec.traits should be deduped to 1 entry
	traitsSlice, ok, _ := unstructured.NestedSlice(release.Object, "spec", "traits")
	require.True(t, ok)
	assert.Len(t, traitsSlice, 1)

	// spec.componentProfile.traits should have both instances
	profileTraits, ok, _ := unstructured.NestedSlice(release.Object, "spec", "componentProfile", "traits")
	require.True(t, ok)
	assert.Len(t, profileTraits, 2)
}

func TestGenerateRelease_WorkloadWithoutEndpoints(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "doclet"
		componentName = "worker-svc"
		releaseName   = "worker-svc-abc123"
	)

	idx := index.New("/repo")

	addComponent(t, idx, componentName, projectName, "deployment/worker",
		"/repo/projects/doclet/components/worker-svc/component.yaml")

	addComponentType(t, idx, "worker", "statefulset",
		"/repo/platform/component-types/worker.yaml")

	addWorkload(t, idx, namespace, "worker-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{
				"image": "my-registry/worker-svc:v1",
			},
		},
		"/repo/projects/doclet/components/worker-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   releaseName,
	})
	require.NoError(t, err)

	workload, ok, err := unstructured.NestedMap(release.Object, "spec", "workload")
	require.NoError(t, err)
	require.True(t, ok, "expected spec.workload to exist")

	assert.NotNil(t, workload["container"], "expected spec.workload.container to exist")
	assert.Nil(t, workload["endpoints"], "expected spec.workload.endpoints to be absent")
	assert.Nil(t, workload["connections"], "expected spec.workload.connections to be absent")
}

// addComponentTypeWithSpec adds a ComponentType with a caller-provided spec, allowing
// tests to include fields (e.g. validations) that the minimal addComponentType helper omits.
func addComponentTypeWithSpec(t *testing.T, idx *index.Index, name string, spec map[string]any, filePath string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "ComponentType",
				"metadata": map[string]any{
					"name":      name,
					"namespace": "default",
				},
				"spec": spec,
			},
		},
		FilePath: filePath,
	}
	require.NoError(t, idx.Add(entry))
}

// TestGenerateRelease_ValidationsPropagated verifies that the generated ComponentRelease
// carries both the ComponentType's validations and the trait's validation rules
// (validations, preRenderValidations, postRenderValidations) plus removes into the frozen
// snapshot, so the rendering pipeline enforces them. Regression test for the occ fsmode
// mappers silently dropping these fields.
func TestGenerateRelease_ValidationsPropagated(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "myproj"
		componentName = "my-svc"
		releaseName   = "my-svc-release-1"
	)

	idx := index.New("/repo")

	addComponentWithTraits(t, idx, namespace,
		[]map[string]any{
			{"kind": "Trait", "name": "resource-limits", "instanceName": "rl-1"},
		},
		"/repo/projects/myproj/components/my-svc/component.yaml")

	addComponentTypeWithSpec(t, idx, "service",
		map[string]any{
			"workloadType": "deployment",
			"resources":    []any{},
			"validations": []any{
				map[string]any{"rule": "${parameters.replicas > 0}", "message": "replicas must be positive"},
			},
			"preRenderValidations": []any{
				map[string]any{"rule": "${has(parameters.image)}", "message": "image required"},
			},
			"postRenderValidations": []any{
				map[string]any{
					"target":  map[string]any{"group": "apps", "version": "v1", "kind": "Deployment"},
					"rule":    "${resource.spec.replicas > 0}",
					"message": "must scale",
				},
			},
		},
		"/repo/platform/component-types/service.yaml")

	addTrait(t, idx, "resource-limits",
		map[string]any{
			"validations": []any{
				map[string]any{"rule": "${parameters.cpu != ''}", "message": "cpu required"},
			},
			"preRenderValidations": []any{
				map[string]any{"rule": "${parameters.memory != ''}", "message": "memory required"},
			},
			"postRenderValidations": []any{
				map[string]any{
					"target":  map[string]any{"group": "apps", "version": "v1", "kind": "Deployment"},
					"rule":    "${resource.spec.template.spec.containers.all(c, has(c.resources))}",
					"message": "containers must set resources",
				},
			},
			"removes": []any{
				map[string]any{
					"target": map[string]any{"group": "", "version": "v1", "kind": "Service"},
				},
			},
		},
		"/repo/platform/traits/resource-limits.yaml")

	addWorkload(t, idx, namespace, "my-svc-workload", projectName, componentName,
		map[string]any{"container": map[string]any{"image": "reg/my-svc:v1"}},
		"/repo/projects/myproj/components/my-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   releaseName,
	})
	require.NoError(t, err)

	// ComponentType validations must be carried into componentType.spec.validations.
	ctValidations, ok, _ := unstructured.NestedSlice(release.Object, "spec", "componentType", "spec", "validations")
	require.True(t, ok, "expected spec.componentType.spec.validations to exist")
	require.Len(t, ctValidations, 1)
	ctv := ctValidations[0].(map[string]interface{})
	assert.Equal(t, "${parameters.replicas > 0}", ctv["rule"])
	assert.Equal(t, "replicas must be positive", ctv["message"])

	// ComponentType pre/post-render validations must be carried too (added in #4088).
	ctPre, ok, _ := unstructured.NestedSlice(release.Object, "spec", "componentType", "spec", "preRenderValidations")
	require.True(t, ok, "expected spec.componentType.spec.preRenderValidations to exist")
	require.Len(t, ctPre, 1)
	assert.Equal(t, "${has(parameters.image)}", ctPre[0].(map[string]interface{})["rule"])

	ctPost, ok, _ := unstructured.NestedSlice(release.Object, "spec", "componentType", "spec", "postRenderValidations")
	require.True(t, ok, "expected spec.componentType.spec.postRenderValidations to exist")
	require.Len(t, ctPost, 1)
	assert.Equal(t, "must scale", ctPost[0].(map[string]interface{})["message"])

	// Trait validation fields and removes must be carried into spec.traits[0].spec.
	traitsSlice, ok, _ := unstructured.NestedSlice(release.Object, "spec", "traits")
	require.True(t, ok, "expected spec.traits to exist")
	require.Len(t, traitsSlice, 1)
	traitSpec := traitsSlice[0].(map[string]interface{})["spec"].(map[string]interface{})

	require.Contains(t, traitSpec, "validations")
	require.Contains(t, traitSpec, "preRenderValidations")
	require.Contains(t, traitSpec, "postRenderValidations")
	require.Contains(t, traitSpec, "removes")

	preRender := traitSpec["preRenderValidations"].([]interface{})
	require.Len(t, preRender, 1)
	assert.Equal(t, "${parameters.memory != ''}", preRender[0].(map[string]interface{})["rule"])

	postRender := traitSpec["postRenderValidations"].([]interface{})
	require.Len(t, postRender, 1)
	prv := postRender[0].(map[string]interface{})
	assert.Equal(t, "containers must set resources", prv["message"])
	prTarget := prv["target"].(map[string]interface{})
	assert.Equal(t, "Deployment", prTarget["kind"])

	removes := traitSpec["removes"].([]interface{})
	require.Len(t, removes, 1)
	rmTarget := removes[0].(map[string]interface{})["target"].(map[string]interface{})
	assert.Equal(t, "Service", rmTarget["kind"])
}

// TestGenerateRelease_WorkloadResourceDependenciesPropagated verifies that a Workload's
// spec.dependencies.resources (external Resource env/file bindings) is carried into the
// generated ComponentRelease's spec.workload.dependencies.resources, where
// ${dependencies.toContainerEnvs()} injects it. Regression test for discussion #4103.
func TestGenerateRelease_WorkloadResourceDependenciesPropagated(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "myproj"
		componentName = "my-svc"
		releaseName   = "my-svc-release-1"
	)

	idx := index.New("/repo")

	addComponentWithTraits(t, idx, namespace, nil,
		"/repo/projects/myproj/components/my-svc/component.yaml")

	addComponentTypeWithSpec(t, idx, "service",
		map[string]any{
			"workloadType": "deployment",
			"resources":    []any{},
		},
		"/repo/platform/component-types/service.yaml")

	addWorkload(t, idx, namespace, "my-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{"image": "reg/my-svc:v1"},
			"dependencies": map[string]any{
				"resources": []any{
					map[string]any{
						"ref": "swa-hetzner-ducklake",
						"envBindings": map[string]any{
							"DUCKLAKE_PG_DSN": "DUCKLAKE_PG_DSN",
							"SEAWEEDFS_KEY":   "SEAWEEDFS_KEY",
						},
					},
				},
			},
		},
		"/repo/projects/myproj/components/my-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   releaseName,
	})
	require.NoError(t, err)

	resources, ok, _ := unstructured.NestedSlice(release.Object, "spec", "workload", "dependencies", "resources")
	require.True(t, ok, "expected spec.workload.dependencies.resources to exist")
	require.Len(t, resources, 1)
	res := resources[0].(map[string]interface{})
	assert.Equal(t, "swa-hetzner-ducklake", res["ref"])
	envBindings := res["envBindings"].(map[string]interface{})
	assert.Equal(t, "DUCKLAKE_PG_DSN", envBindings["DUCKLAKE_PG_DSN"])
	assert.Equal(t, "SEAWEEDFS_KEY", envBindings["SEAWEEDFS_KEY"])
}
