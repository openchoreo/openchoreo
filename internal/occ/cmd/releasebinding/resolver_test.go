// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func TestBuildBindingOutputDirResolver(t *testing.T) {
	const namespace = "test-ns"

	t.Run("priority 1: existing bindings directory", func(t *testing.T) {
		idx := index.New("/repo")

		// Add a component
		addComponent(t, idx, namespace, "my-comp", "my-proj", "/repo/projects/my-proj/components/my-comp/component.yaml")

		// Add an existing binding for the component
		addBinding(t, idx, namespace, "my-comp-dev", "my-proj", "my-comp", "dev", "/repo/projects/my-proj/components/my-comp/bindings/my-comp-dev.yaml")

		ocIndex := fsmode.WrapIndex(idx)
		resolver := buildBindingOutputDirResolver(ocIndex, namespace)

		got := resolver("my-proj", "my-comp")
		want := "/repo/projects/my-proj/components/my-comp/bindings"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("priority 2: bindings dir next to component (does not exist on disk)", func(t *testing.T) {
		idx := index.New("/repo")

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
		resolver := buildBindingOutputDirResolver(ocIndex, namespace)

		got := resolver("my-proj", "my-comp")
		want := filepath.Join(compDir, "release-bindings")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("priority 3: release-bindings dir already exists, use release-bindings-<name>", func(t *testing.T) {
		idx := index.New("/repo")

		tmpDir := t.TempDir()
		compDir := filepath.Join(tmpDir, "projects", "my-proj", "components", "my-comp")
		bindingsDir := filepath.Join(compDir, "release-bindings")
		if err := os.MkdirAll(bindingsDir, 0755); err != nil {
			t.Fatal(err)
		}
		compFile := filepath.Join(compDir, "component.yaml")
		if err := os.WriteFile(compFile, []byte(""), 0600); err != nil {
			t.Fatal(err)
		}

		addComponent(t, idx, namespace, "my-comp", "my-proj", compFile)

		ocIndex := fsmode.WrapIndex(idx)
		resolver := buildBindingOutputDirResolver(ocIndex, namespace)

		got := resolver("my-proj", "my-comp")
		want := filepath.Join(compDir, "release-bindings-my-comp")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("component not found: returns empty string", func(t *testing.T) {
		idx := index.New("/repo")
		ocIndex := fsmode.WrapIndex(idx)
		resolver := buildBindingOutputDirResolver(ocIndex, namespace)

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

// addBinding adds a ReleaseBinding resource entry to the index.
func addBinding(t *testing.T, idx *index.Index, namespace, name, project, component, env, filePath string) {
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
				},
			},
		},
		FilePath: filePath,
	}
	if err := idx.Add(entry); err != nil {
		t.Fatal(err)
	}
}
