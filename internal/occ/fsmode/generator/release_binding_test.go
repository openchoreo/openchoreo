// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/pipeline"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// newTestPipelineInfo creates a simple pipeline with a single root environment.
func newTestPipelineInfo(rootEnv string) *pipeline.PipelineInfo {
	return &pipeline.PipelineInfo{
		Name:            "test-pipeline",
		RootEnvironment: rootEnv,
		Environments:    []string{rootEnv},
		PromotionPaths:  map[string][]string{rootEnv: {}},
		EnvPosition:     map[string]int{rootEnv: 0},
	}
}

// generateMatchingRelease generates a ComponentRelease whose spec matches the
// current component state, using the ReleaseGenerator. This ensures the release
// spec is exactly what the hash-based comparison expects.
func generateMatchingRelease(t *testing.T, idx *index.Index, namespace, releaseName, projectName, componentName string) *unstructured.Unstructured {
	t.Helper()
	tmpIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(tmpIndex)
	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   releaseName,
	})
	require.NoError(t, err)
	return release
}

func TestGenerateBindingWithInfo_Create(t *testing.T) {
	const (
		namespace         = "test-ns"
		projectName       = "my-proj"
		componentName     = "my-comp"
		targetEnv         = "dev"
		releaseName       = "my-comp-20260101-0"
		componentTypeName = "my-type"
		workloadType      = "Deployment"
		image             = "my-image:latest"
	)

	idx := index.New("/repo")

	// Add component state resources required for release spec generation
	addComponentWithKind(t, idx, namespace, componentName, projectName, componentTypeName,
		"ComponentType",
		"/repo/projects/my-proj/components/my-comp.yaml")
	addComponentType(t, idx, componentTypeName, workloadType,
		"/repo/component-types/my-type.yaml")
	addWorkload(t, idx, namespace, componentName+"-workload", projectName, componentName,
		map[string]any{"container": map[string]any{"image": image}},
		"/repo/projects/my-proj/components/my-comp/workload.yaml")

	// Generate a ComponentRelease whose spec matches the current component state
	release := generateMatchingRelease(t, idx, namespace, releaseName, projectName, componentName)

	// Add the matching release to the index
	releaseEntry := &index.ResourceEntry{
		Resource: release,
		FilePath: "/repo/projects/my-proj/components/my-comp/releases/my-comp-20260101-0.yaml",
	}
	require.NoError(t, idx.Add(releaseEntry))

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewBindingGenerator(ocIndex)

	info, err := gen.GenerateBindingWithInfo(BindingOptions{
		ProjectName:   projectName,
		ComponentName: componentName,
		TargetEnv:     targetEnv,
		PipelineInfo:  newTestPipelineInfo(targetEnv),
		Namespace:     namespace,
	})
	require.NoError(t, err)

	// Verify CREATE path fields
	assert.False(t, info.IsUpdate, "expected IsUpdate to be false for a new binding")
	assert.Empty(t, info.ExistingFilePath)
	assert.Equal(t, projectName, info.ProjectName)
	assert.Equal(t, componentName, info.ComponentName)
	assert.Equal(t, targetEnv, info.Environment)
	assert.Equal(t, releaseName, info.ReleaseName)
	assert.Equal(t, componentName+"-"+targetEnv, info.BindingName)
	require.NotNil(t, info.Binding)
}

func TestGenerateBindingWithInfo_Update(t *testing.T) {
	const (
		namespace         = "test-ns"
		projectName       = "my-proj"
		componentName     = "my-comp"
		targetEnv         = "dev"
		newRelease        = "my-comp-20260101-0"
		oldRelease        = "my-comp-20250101-0"
		componentTypeName = "my-type"
		workloadType      = "Deployment"
		image             = "my-image:latest"
	)

	tmpDir := t.TempDir()
	bindingFile := filepath.Join(tmpDir, "my-comp-dev.yaml")

	// Write a binding file on disk with extra fields that must be preserved
	existingYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseBinding
metadata:
  name: my-comp-dev
  namespace: test-ns
  labels:
    team: platform
  annotations:
    note: do-not-lose-me
spec:
  owner:
    projectName: my-proj
    componentName: my-comp
  environment: dev
  releaseName: my-comp-20250101-0
  customField: preserve-this
`
	require.NoError(t, os.WriteFile(bindingFile, []byte(existingYAML), 0600))

	idx := index.New(tmpDir)

	// Add component state resources required for release spec generation
	addComponentWithKind(t, idx, namespace, componentName, projectName, componentTypeName,
		"ComponentType",
		filepath.Join(tmpDir, "projects", projectName, "components", componentName+".yaml"))
	addComponentType(t, idx, componentTypeName, workloadType,
		filepath.Join(tmpDir, "component-types", componentTypeName+".yaml"))
	addWorkload(t, idx, namespace, componentName+"-workload", projectName, componentName,
		map[string]any{"container": map[string]any{"image": image}},
		filepath.Join(tmpDir, "projects", projectName, "components", componentName, "workload.yaml"))

	// Generate a ComponentRelease whose spec matches the current component state
	release := generateMatchingRelease(t, idx, namespace, newRelease, projectName, componentName)

	// Add the matching release to the index
	releaseEntry := &index.ResourceEntry{
		Resource: release,
		FilePath: filepath.Join(tmpDir, "releases", newRelease+".yaml"),
	}
	require.NoError(t, idx.Add(releaseEntry))

	// Add an existing ReleaseBinding pointing to the file on disk
	addReleaseBinding(t, idx, namespace, componentName+"-"+targetEnv, projectName, componentName, targetEnv,
		oldRelease, bindingFile)

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewBindingGenerator(ocIndex)

	info, err := gen.GenerateBindingWithInfo(BindingOptions{
		ProjectName:   projectName,
		ComponentName: componentName,
		TargetEnv:     targetEnv,
		PipelineInfo:  newTestPipelineInfo(targetEnv),
		Namespace:     namespace,
	})
	require.NoError(t, err)

	// Verify UPDATE path fields
	assert.True(t, info.IsUpdate, "expected IsUpdate to be true for an existing binding")
	assert.Equal(t, bindingFile, info.ExistingFilePath)
	assert.Equal(t, projectName, info.ProjectName)
	assert.Equal(t, componentName, info.ComponentName)
	assert.Equal(t, targetEnv, info.Environment)
	assert.Equal(t, newRelease, info.ReleaseName)
	require.NotNil(t, info.Binding)

	// Verify releaseName was updated in the binding object
	gotRelease := getNestedString(info.Binding.Object, "spec", "releaseName")
	assert.Equal(t, newRelease, gotRelease)

	// Verify extra fields from the original file are preserved
	gotCustom := getNestedString(info.Binding.Object, "spec", "customField")
	assert.Equal(t, "preserve-this", gotCustom, "field lost during update")

	labels, _, _ := unstructured.NestedStringMap(info.Binding.Object, "metadata", "labels")
	assert.Equal(t, "platform", labels["team"])

	annotations, _, _ := unstructured.NestedStringMap(info.Binding.Object, "metadata", "annotations")
	assert.Equal(t, "do-not-lose-me", annotations["note"])
}

// addReleaseBinding adds a ReleaseBinding resource entry to the index.
func addReleaseBinding(t *testing.T, idx *index.Index, namespace, name, project, component, env, releaseName, filePath string) {
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
						"projectName":   project,
						"componentName": component,
					},
					"environment": env,
					"releaseName": releaseName,
				},
			},
		},
		FilePath: filePath,
	}
	require.NoError(t, idx.Add(entry))
}
