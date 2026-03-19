// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode/config"
)

func makeReleaseObj(name, project, component string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "openchoreo.dev/v1alpha1",
			"kind":       "ComponentRelease",
			"metadata":   map[string]any{"name": name},
			"spec": map[string]any{
				"owner": map[string]any{
					"projectName":   project,
					"componentName": component,
				},
			},
		},
	}
}

func makeBindingObj(name, project, component string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "openchoreo.dev/v1alpha1",
			"kind":       "ReleaseBinding",
			"metadata":   map[string]any{"name": name},
			"spec": map[string]any{
				"owner": map[string]any{
					"projectName":   project,
					"componentName": component,
				},
			},
		},
	}
}

func TestGetNestedString(t *testing.T) {
	obj := map[string]any{
		"spec": map[string]any{
			"owner": map[string]any{
				"projectName": "my-proj",
			},
		},
	}

	tests := []struct {
		name   string
		fields []string
		want   string
	}{
		{name: "valid nested path", fields: []string{"spec", "owner", "projectName"}, want: "my-proj"},
		{name: "missing field", fields: []string{"spec", "owner", "missing"}, want: ""},
		{name: "missing intermediate", fields: []string{"spec", "nonexistent", "field"}, want: ""},
		{name: "top-level missing", fields: []string{"missing"}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, getNestedString(obj, tt.fields...))
		})
	}
}

func TestDetermineOutputPath(t *testing.T) {
	w := NewWriter("/repo")

	t.Run("explicit output dir", func(t *testing.T) {
		release := makeReleaseObj("my-comp-20250315-1", "proj", "my-comp")
		path := w.determineOutputPath(release, WriteOptions{OutputDir: "/custom/dir"})
		assert.Equal(t, "/custom/dir/my-comp-20250315-1.yaml", path)
	})

	t.Run("default path structure", func(t *testing.T) {
		release := makeReleaseObj("my-comp-20250315-1", "my-proj", "my-comp")
		path := w.determineOutputPath(release, WriteOptions{})
		assert.Equal(t, "/repo/projects/my-proj/components/my-comp/releases/my-comp-20250315-1.yaml", path)
	})
}

func TestResolveOutputPath(t *testing.T) {
	w := NewWriter("/repo")
	release := makeReleaseObj("svc-20250315-1", "proj", "svc")

	t.Run("priority 1: config override", func(t *testing.T) {
		cfg := &config.ReleaseConfig{
			APIVersion: "openchoreo.dev/v1alpha1",
			Kind:       "ReleaseConfig",
			ComponentReleaseDefaults: &config.ComponentReleaseDefaults{
				Projects: map[string]config.ProjectReleaseConfig{
					"proj": {Components: map[string]string{"svc": "./custom/releases"}},
				},
			},
		}
		path := w.resolveOutputPath(release, "proj", "svc", BulkWriteOptions{Config: cfg})
		assert.Equal(t, "/repo/custom/releases/svc-20250315-1.yaml", path)
	})

	t.Run("priority 2: output dir flag", func(t *testing.T) {
		path := w.resolveOutputPath(release, "proj", "svc", BulkWriteOptions{OutputDir: "/out"})
		assert.Equal(t, "/out/svc-20250315-1.yaml", path)
	})

	t.Run("priority 2: relative output dir resolved against baseDir", func(t *testing.T) {
		path := w.resolveOutputPath(release, "proj", "svc", BulkWriteOptions{OutputDir: "out"})
		assert.Equal(t, "/repo/out/svc-20250315-1.yaml", path)
	})

	t.Run("priority 3: resolver function", func(t *testing.T) {
		resolver := func(project, component string) string {
			return "/resolved/" + project + "/" + component
		}
		path := w.resolveOutputPath(release, "proj", "svc", BulkWriteOptions{Resolver: resolver})
		assert.Equal(t, "/resolved/proj/svc/svc-20250315-1.yaml", path)
	})

	t.Run("priority 4: default path", func(t *testing.T) {
		path := w.resolveOutputPath(release, "proj", "svc", BulkWriteOptions{})
		assert.Equal(t, "/repo/projects/proj/components/svc/releases/svc-20250315-1.yaml", path)
	})
}

func TestDetermineBindingOutputPath(t *testing.T) {
	w := NewWriter("/repo")

	t.Run("explicit output dir", func(t *testing.T) {
		binding := makeBindingObj("my-comp-dev", "proj", "my-comp")
		path := w.determineBindingOutputPath(binding, "/custom/dir")
		assert.Equal(t, "/custom/dir/my-comp-dev.yaml", path)
	})

	t.Run("default path structure", func(t *testing.T) {
		binding := makeBindingObj("my-comp-dev", "my-proj", "my-comp")
		path := w.determineBindingOutputPath(binding, "")
		assert.Equal(t, "/repo/projects/my-proj/components/my-comp/release-bindings/my-comp-dev.yaml", path)
	})
}

func TestResolveBindingOutputPath(t *testing.T) {
	w := NewWriter("/repo")
	binding := makeBindingObj("svc-dev", "proj", "svc")

	t.Run("priority 0: existing path for update-in-place", func(t *testing.T) {
		opts := BulkBindingWriteOptions{
			ExistingPaths: map[string]string{"svc-dev": "/existing/path/svc-dev.yaml"},
		}
		path := w.resolveBindingOutputPath(binding, "proj", "svc", opts)
		assert.Equal(t, "/existing/path/svc-dev.yaml", path)
	})

	t.Run("priority 1: output dir flag", func(t *testing.T) {
		path := w.resolveBindingOutputPath(binding, "proj", "svc", BulkBindingWriteOptions{OutputDir: "/out"})
		assert.Equal(t, "/out/svc-dev.yaml", path)
	})

	t.Run("priority 1: relative output dir resolved against baseDir", func(t *testing.T) {
		path := w.resolveBindingOutputPath(binding, "proj", "svc", BulkBindingWriteOptions{OutputDir: "out"})
		assert.Equal(t, "/repo/out/svc-dev.yaml", path)
	})

	t.Run("priority 3: resolver function", func(t *testing.T) {
		resolver := func(project, component string) string {
			return "/resolved/" + project + "/" + component
		}
		path := w.resolveBindingOutputPath(binding, "proj", "svc", BulkBindingWriteOptions{Resolver: resolver})
		assert.Equal(t, "/resolved/proj/svc/svc-dev.yaml", path)
	})

	t.Run("priority 4: default path", func(t *testing.T) {
		path := w.resolveBindingOutputPath(binding, "proj", "svc", BulkBindingWriteOptions{})
		assert.Equal(t, "/repo/projects/proj/components/svc/release-bindings/svc-dev.yaml", path)
	})
}

func TestWriteRelease(t *testing.T) {
	t.Run("dry run writes to stdout buffer", func(t *testing.T) {
		w := NewWriter("/repo")
		release := makeReleaseObj("my-comp-20250315-1", "proj", "my-comp")
		var buf bytes.Buffer

		path, skipped, err := w.WriteRelease(release, WriteOptions{DryRun: true, Stdout: &buf})
		require.NoError(t, err)
		assert.Empty(t, path)
		assert.False(t, skipped)
		assert.Contains(t, buf.String(), "my-comp-20250315-1")
		assert.Contains(t, buf.String(), "---")
	})

	t.Run("writes file to disk", func(t *testing.T) {
		dir := t.TempDir()
		w := NewWriter(dir)
		release := makeReleaseObj("my-comp-20250315-1", "proj", "my-comp")

		path, skipped, err := w.WriteRelease(release, WriteOptions{OutputDir: dir})
		require.NoError(t, err)
		assert.False(t, skipped)
		assert.Equal(t, filepath.Join(dir, "my-comp-20250315-1.yaml"), path)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "my-comp-20250315-1")
	})
}

func TestWriteBinding(t *testing.T) {
	t.Run("dry run writes to stdout buffer", func(t *testing.T) {
		w := NewWriter("/repo")
		binding := makeBindingObj("my-comp-dev", "proj", "my-comp")
		var buf bytes.Buffer

		path, _, err := w.WriteBinding(binding, WriteOptions{DryRun: true, Stdout: &buf})
		require.NoError(t, err)
		assert.Empty(t, path)
		assert.Contains(t, buf.String(), "my-comp-dev")
		assert.Contains(t, buf.String(), "---")
	})

	t.Run("writes file to disk", func(t *testing.T) {
		dir := t.TempDir()
		w := NewWriter(dir)
		binding := makeBindingObj("my-comp-dev", "proj", "my-comp")

		path, _, err := w.WriteBinding(binding, WriteOptions{OutputDir: dir})
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "my-comp-dev.yaml"), path)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "my-comp-dev")
	})
}
