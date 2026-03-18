// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func TestBuildOutputDirResolver(t *testing.T) {
	const namespace = "test-ns"

	t.Run("priority 1: existing releases directory", func(t *testing.T) {
		idx := index.New("/repo")

		addComponent(t, idx, namespace, "my-comp", "my-proj", "/repo/projects/my-proj/components/my-comp/component.yaml")
		addRelease(t, idx, namespace, "my-comp-20250101-001", "my-proj", "my-comp", "/repo/projects/my-proj/components/my-comp/releases/my-comp-20250101-001.yaml")

		ocIndex := fsmode.WrapIndex(idx)
		resolver := buildOutputDirResolver(ocIndex, namespace)

		assert.Equal(t, "/repo/projects/my-proj/components/my-comp/releases", resolver("my-proj", "my-comp"))
	})

	t.Run("priority 2: releases dir next to component (does not exist on disk)", func(t *testing.T) {
		idx := index.New("/repo")

		tmpDir := t.TempDir()
		compDir := filepath.Join(tmpDir, "projects", "my-proj", "components", "my-comp")
		require.NoError(t, os.MkdirAll(compDir, 0755))
		compFile := filepath.Join(compDir, "component.yaml")
		require.NoError(t, os.WriteFile(compFile, []byte(""), 0600))

		addComponent(t, idx, namespace, "my-comp", "my-proj", compFile)

		ocIndex := fsmode.WrapIndex(idx)
		resolver := buildOutputDirResolver(ocIndex, namespace)

		assert.Equal(t, filepath.Join(compDir, "releases"), resolver("my-proj", "my-comp"))
	})

	t.Run("releases dir already exists (empty): still uses releases/", func(t *testing.T) {
		idx := index.New("/repo")

		tmpDir := t.TempDir()
		compDir := filepath.Join(tmpDir, "projects", "my-proj", "components", "my-comp")
		releasesDir := filepath.Join(compDir, "releases")
		require.NoError(t, os.MkdirAll(releasesDir, 0755))
		compFile := filepath.Join(compDir, "component.yaml")
		require.NoError(t, os.WriteFile(compFile, []byte(""), 0600))

		addComponent(t, idx, namespace, "my-comp", "my-proj", compFile)

		ocIndex := fsmode.WrapIndex(idx)
		resolver := buildOutputDirResolver(ocIndex, namespace)

		assert.Equal(t, filepath.Join(compDir, "releases"), resolver("my-proj", "my-comp"))
	})

	t.Run("component not found: returns empty string", func(t *testing.T) {
		idx := index.New("/repo")
		ocIndex := fsmode.WrapIndex(idx)
		resolver := buildOutputDirResolver(ocIndex, namespace)

		assert.Empty(t, resolver("nonexistent-proj", "nonexistent-comp"))
	})
}

// addComponent adds a Component resource entry to the index.
func addComponent(t *testing.T, idx *index.Index, namespace, name, project, filePath string) {
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
						"projectName": project,
					},
				},
			},
		},
		FilePath: filePath,
	}
	require.NoError(t, idx.Add(entry))
}

// addRelease adds a ComponentRelease resource entry to the index.
func addRelease(t *testing.T, idx *index.Index, namespace, name, project, component, filePath string) {
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
						"projectName":   project,
						"componentName": component,
					},
				},
			},
		},
		FilePath: filePath,
	}
	require.NoError(t, idx.Add(entry))
}
