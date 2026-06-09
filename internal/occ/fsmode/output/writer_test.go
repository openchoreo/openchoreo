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
			ExistingPaths: map[string]string{"svc-dev": "/repo/existing/path/svc-dev.yaml"},
		}
		path, err := w.resolveBindingOutputPath(binding, "proj", "svc", opts)
		require.NoError(t, err)
		assert.Equal(t, "/repo/existing/path/svc-dev.yaml", path)
	})

	t.Run("priority 0: existing path escaping base dir is rejected", func(t *testing.T) {
		opts := BulkBindingWriteOptions{
			ExistingPaths: map[string]string{"svc-dev": "/repo/../etc/svc-dev.yaml"},
		}
		_, err := w.resolveBindingOutputPath(binding, "proj", "svc", opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "escapes base directory")
	})

	t.Run("priority 0: existing path outside base dir is rejected", func(t *testing.T) {
		opts := BulkBindingWriteOptions{
			ExistingPaths: map[string]string{"svc-dev": "/outside/path/svc-dev.yaml"},
		}
		_, err := w.resolveBindingOutputPath(binding, "proj", "svc", opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "escapes base directory")
	})

	t.Run("priority 1: output dir flag", func(t *testing.T) {
		path, err := w.resolveBindingOutputPath(binding, "proj", "svc", BulkBindingWriteOptions{OutputDir: "/out"})
		require.NoError(t, err)
		assert.Equal(t, "/out/svc-dev.yaml", path)
	})

	t.Run("priority 1: relative output dir resolved against baseDir", func(t *testing.T) {
		path, err := w.resolveBindingOutputPath(binding, "proj", "svc", BulkBindingWriteOptions{OutputDir: "out"})
		require.NoError(t, err)
		assert.Equal(t, "/repo/out/svc-dev.yaml", path)
	})

	t.Run("priority 3: resolver function", func(t *testing.T) {
		resolver := func(project, component string) string {
			return "/resolved/" + project + "/" + component
		}
		path, err := w.resolveBindingOutputPath(binding, "proj", "svc", BulkBindingWriteOptions{Resolver: resolver})
		require.NoError(t, err)
		assert.Equal(t, "/resolved/proj/svc/svc-dev.yaml", path)
	})

	t.Run("priority 4: default path", func(t *testing.T) {
		path, err := w.resolveBindingOutputPath(binding, "proj", "svc", BulkBindingWriteOptions{})
		require.NoError(t, err)
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

func TestWriteRelease_SkipIfUnchanged(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	// Create a release and write it
	release := makeReleaseObj("my-comp-20250315-1", "proj", "my-comp")
	path, skipped, err := w.WriteRelease(release, WriteOptions{OutputDir: dir})
	require.NoError(t, err)
	assert.False(t, skipped)
	assert.FileExists(t, path)

	// Try writing the same release again with SkipIfUnchanged
	_, skipped, err = w.WriteRelease(release, WriteOptions{OutputDir: dir, SkipIfUnchanged: true})
	require.NoError(t, err)
	assert.True(t, skipped, "should skip writing identical release")
}

func TestWriteRelease_AutoIncrementVersion(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir)

	// Write first release
	release1 := makeReleaseObj("my-comp-20250315-0", "proj", "my-comp")
	path1, _, err := w.WriteRelease(release1, WriteOptions{OutputDir: dir})
	require.NoError(t, err)
	assert.Contains(t, path1, "my-comp-20250315-0.yaml")

	// Write a different release with the same name - should auto-increment
	release2 := makeReleaseObj("my-comp-20250315-0", "proj", "my-comp")
	// Add a different field to make it not identical
	_ = unstructured.SetNestedField(release2.Object, "new-value", "spec", "extraField")
	path2, _, err := w.WriteRelease(release2, WriteOptions{OutputDir: dir})
	require.NoError(t, err)
	assert.Contains(t, path2, "my-comp-20250315-1.yaml")
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

func TestWriteResource(t *testing.T) {
	t.Run("writes to disk", func(t *testing.T) {
		dir := t.TempDir()
		w := NewWriter(dir)
		resource := makeReleaseObj("test-resource", "proj", "comp")
		outputPath := filepath.Join(dir, "subdir", "test-resource.yaml")

		err := w.WriteResource(resource, outputPath, false)
		require.NoError(t, err)

		data, err := os.ReadFile(outputPath)
		require.NoError(t, err)
		assert.Contains(t, string(data), "test-resource")
	})

	t.Run("dry run does not write file", func(t *testing.T) {
		dir := t.TempDir()
		w := NewWriter(dir)
		resource := makeReleaseObj("test-resource", "proj", "comp")
		outputPath := filepath.Join(dir, "test-resource.yaml")

		err := w.WriteResource(resource, outputPath, true)
		require.NoError(t, err)

		// File should not exist
		_, err = os.Stat(outputPath)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestWriteBulkReleases(t *testing.T) {
	t.Run("dry run writes all to stdout", func(t *testing.T) {
		w := NewWriter("/repo")
		releases := []*unstructured.Unstructured{
			makeReleaseObj("comp-a-20250315-0", "proj", "comp-a"),
			makeReleaseObj("comp-b-20250315-0", "proj", "comp-b"),
		}
		var buf bytes.Buffer

		result, err := w.WriteBulkReleases(releases, BulkWriteOptions{DryRun: true, Stdout: &buf})
		require.NoError(t, err)
		assert.Empty(t, result.OutputPaths)
		assert.Empty(t, result.Errors)
		assert.Contains(t, buf.String(), "comp-a-20250315-0")
		assert.Contains(t, buf.String(), "comp-b-20250315-0")
	})

	t.Run("writes files to disk", func(t *testing.T) {
		dir := t.TempDir()
		w := NewWriter(dir)
		releases := []*unstructured.Unstructured{
			makeReleaseObj("comp-a-20250315-0", "proj", "comp-a"),
			makeReleaseObj("comp-b-20250315-0", "proj", "comp-b"),
		}

		result, err := w.WriteBulkReleases(releases, BulkWriteOptions{OutputDir: dir})
		require.NoError(t, err)
		assert.Len(t, result.OutputPaths, 2)
		assert.Empty(t, result.Errors)

		for _, p := range result.OutputPaths {
			assert.FileExists(t, p)
		}
	})

	t.Run("skip if unchanged", func(t *testing.T) {
		dir := t.TempDir()
		w := NewWriter(dir)
		release := makeReleaseObj("comp-a-20250315-0", "proj", "comp-a")

		// Write first time
		_, err := w.WriteBulkReleases([]*unstructured.Unstructured{release}, BulkWriteOptions{OutputDir: dir})
		require.NoError(t, err)

		// Write again with SkipIfUnchanged
		result, err := w.WriteBulkReleases([]*unstructured.Unstructured{release}, BulkWriteOptions{
			OutputDir:       dir,
			SkipIfUnchanged: true,
		})
		require.NoError(t, err)
		assert.Len(t, result.Skipped, 1)
		assert.Equal(t, "comp-a-20250315-0", result.Skipped[0])
	})
}

func TestWriteBulkBindings(t *testing.T) {
	t.Run("dry run writes all to stdout", func(t *testing.T) {
		w := NewWriter("/repo")
		bindings := []*unstructured.Unstructured{
			makeBindingObj("comp-a-dev", "proj", "comp-a"),
			makeBindingObj("comp-b-dev", "proj", "comp-b"),
		}
		var buf bytes.Buffer

		result, err := w.WriteBulkBindings(bindings, BulkBindingWriteOptions{DryRun: true, Stdout: &buf})
		require.NoError(t, err)
		assert.Empty(t, result.OutputPaths)
		assert.Empty(t, result.Errors)
		assert.Contains(t, buf.String(), "comp-a-dev")
		assert.Contains(t, buf.String(), "comp-b-dev")
	})

	t.Run("writes files to disk", func(t *testing.T) {
		dir := t.TempDir()
		w := NewWriter(dir)
		bindings := []*unstructured.Unstructured{
			makeBindingObj("comp-a-dev", "proj", "comp-a"),
			makeBindingObj("comp-b-dev", "proj", "comp-b"),
		}

		result, err := w.WriteBulkBindings(bindings, BulkBindingWriteOptions{OutputDir: dir})
		require.NoError(t, err)
		assert.Len(t, result.OutputPaths, 2)
		assert.Empty(t, result.Errors)

		for _, p := range result.OutputPaths {
			assert.FileExists(t, p)
		}
	})

	t.Run("with existing paths for update-in-place", func(t *testing.T) {
		dir := t.TempDir()
		w := NewWriter(dir)
		binding := makeBindingObj("comp-a-dev", "proj", "comp-a")

		existingPath := filepath.Join(dir, "existing", "comp-a-dev.yaml")

		result, err := w.WriteBulkBindings([]*unstructured.Unstructured{binding}, BulkBindingWriteOptions{
			ExistingPaths: map[string]string{"comp-a-dev": existingPath},
		})
		require.NoError(t, err)
		assert.Len(t, result.OutputPaths, 1)
		assert.Equal(t, existingPath, result.OutputPaths[0])
		assert.FileExists(t, existingPath)
	})

	t.Run("with existing paths escaping base dir are rejected", func(t *testing.T) {
		dir := t.TempDir()
		w := NewWriter(dir)
		binding := makeBindingObj("comp-a-dev", "proj", "comp-a")

		escapedPath := filepath.Join(dir, "..", "escaped", "comp-a-dev.yaml")

		result, err := w.WriteBulkBindings([]*unstructured.Unstructured{binding}, BulkBindingWriteOptions{
			ExistingPaths: map[string]string{"comp-a-dev": escapedPath},
		})
		require.NoError(t, err) // WriteBulkBindings itself doesn't return error, it collects them
		assert.Empty(t, result.OutputPaths)
		assert.Len(t, result.Errors, 1)
		assert.Contains(t, result.Errors[0].Error(), "escapes base directory")
	})
}
