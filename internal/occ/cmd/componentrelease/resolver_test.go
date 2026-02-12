// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func TestBuildOutputDirResolver(t *testing.T) {
	const namespace = "test-ns"

	t.Run("priority 1: existing releases directory", func(t *testing.T) {
		idx := index.New("/repo")

		// Add a component
		addComponent(t, idx, namespace, "my-comp", "my-proj", "/repo/projects/my-proj/components/my-comp/component.yaml")

		// Add an existing release for the component
		addRelease(t, idx, namespace, "my-comp-20250101-001", "my-proj", "my-comp", "/repo/projects/my-proj/components/my-comp/releases/my-comp-20250101-001.yaml")

		ocIndex := fsmode.WrapIndex(idx)
		resolver := buildOutputDirResolver(ocIndex, namespace)

		got := resolver("my-proj", "my-comp")
		want := "/repo/projects/my-proj/components/my-comp/releases"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("priority 2: releases dir next to component (does not exist on disk)", func(t *testing.T) {
		idx := index.New("/repo")

		// Use a temp directory for the component so os.Stat works
		tmpDir := t.TempDir()
		compDir := filepath.Join(tmpDir, "projects", "my-proj", "components", "my-comp")
		if err := os.MkdirAll(compDir, 0755); err != nil {
			t.Fatal(err)
		}
		compFile := filepath.Join(compDir, "component.yaml")
		if err := os.WriteFile(compFile, []byte(""), 0600); err != nil {
			t.Fatal(err)
		}

		addComponent(t, idx, namespace, "my-comp", "my-proj", compFile)

		ocIndex := fsmode.WrapIndex(idx)
		resolver := buildOutputDirResolver(ocIndex, namespace)

		got := resolver("my-proj", "my-comp")
		want := filepath.Join(compDir, "releases")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("priority 3: releases dir already exists, use releases-<name>", func(t *testing.T) {
		idx := index.New("/repo")

		// Use a temp directory and create the releases/ dir to simulate conflict
		tmpDir := t.TempDir()
		compDir := filepath.Join(tmpDir, "projects", "my-proj", "components", "my-comp")
		releasesDir := filepath.Join(compDir, "releases")
		if err := os.MkdirAll(releasesDir, 0755); err != nil {
			t.Fatal(err)
		}
		compFile := filepath.Join(compDir, "component.yaml")
		if err := os.WriteFile(compFile, []byte(""), 0600); err != nil {
			t.Fatal(err)
		}

		addComponent(t, idx, namespace, "my-comp", "my-proj", compFile)

		ocIndex := fsmode.WrapIndex(idx)
		resolver := buildOutputDirResolver(ocIndex, namespace)

		got := resolver("my-proj", "my-comp")
		want := filepath.Join(compDir, "releases-my-comp")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("component not found: returns empty string", func(t *testing.T) {
		idx := index.New("/repo")
		ocIndex := fsmode.WrapIndex(idx)
		resolver := buildOutputDirResolver(ocIndex, namespace)

		got := resolver("nonexistent-proj", "nonexistent-comp")
		if got != "" {
			t.Errorf("expected empty string for unknown component, got %q", got)
		}
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
	if err := idx.Add(entry); err != nil {
		t.Fatal(err)
	}
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
	if err := idx.Add(entry); err != nil {
		t.Fatal(err)
	}
}
