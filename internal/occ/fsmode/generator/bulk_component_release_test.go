// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func TestDiscoverComponents(t *testing.T) {
	idx := index.New("/repo")

	addComponent(t, idx, "comp-a", "proj-1", "deployment/service", "/repo/comp-a.yaml")
	addComponent(t, idx, "comp-b", "proj-1", "deployment/service", "/repo/comp-b.yaml")
	addComponent(t, idx, "comp-c", "proj-2", "deployment/worker", "/repo/comp-c.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	t.Run("all flag returns all components", func(t *testing.T) {
		components, err := gen.discoverComponents(BulkReleaseOptions{All: true})
		require.NoError(t, err)
		assert.Len(t, components, 3)
	})

	t.Run("project filter returns matching components", func(t *testing.T) {
		components, err := gen.discoverComponents(BulkReleaseOptions{ProjectName: "proj-1"})
		require.NoError(t, err)
		assert.Len(t, components, 2)
	})

	t.Run("project with no components returns error", func(t *testing.T) {
		_, err := gen.discoverComponents(BulkReleaseOptions{ProjectName: "nonexistent"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no components found")
	})

	t.Run("neither all nor project returns error", func(t *testing.T) {
		_, err := gen.discoverComponents(BulkReleaseOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "either --all or --project must be specified")
	})
}

func TestGenerateBulkReleases(t *testing.T) {
	t.Run("generates releases for all components in a project", func(t *testing.T) {
		idx := index.New("/repo")

		addComponent(t, idx, "svc-a", "myproj", "deployment/service", "/repo/svc-a.yaml")
		addComponent(t, idx, "svc-b", "myproj", "deployment/service", "/repo/svc-b.yaml")

		addComponentType(t, idx, "service", "deployment", "/repo/ct-service.yaml")

		addWorkload(t, idx, "default", "svc-a-workload", "myproj", "svc-a",
			map[string]any{"container": map[string]any{"image": "img-a:v1"}},
			"/repo/svc-a-workload.yaml")
		addWorkload(t, idx, "default", "svc-b-workload", "myproj", "svc-b",
			map[string]any{"container": map[string]any{"image": "img-b:v1"}},
			"/repo/svc-b-workload.yaml")

		ocIndex := fsmode.WrapIndex(idx)
		gen := NewReleaseGenerator(ocIndex)

		result, err := gen.GenerateBulkReleases(BulkReleaseOptions{
			ProjectName: "myproj",
			Namespace:   "default",
			Version:     "1",
		})
		require.NoError(t, err)
		assert.Len(t, result.Releases, 2)
		assert.Empty(t, result.Errors)
	})

	t.Run("no components found returns error", func(t *testing.T) {
		idx := index.New("/repo")
		ocIndex := fsmode.WrapIndex(idx)
		gen := NewReleaseGenerator(ocIndex)

		_, err := gen.GenerateBulkReleases(BulkReleaseOptions{All: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no components found")
	})

	t.Run("partial failures collected in errors", func(t *testing.T) {
		idx := index.New("/repo")

		// svc-good has a workload, svc-bad does not — GenerateRelease will fail for svc-bad
		addComponent(t, idx, "svc-good", "myproj", "deployment/service", "/repo/svc-good.yaml")
		addComponent(t, idx, "svc-bad", "myproj", "deployment/service", "/repo/svc-bad.yaml")

		addComponentType(t, idx, "service", "deployment", "/repo/ct-service.yaml")

		addWorkload(t, idx, "default", "svc-good-workload", "myproj", "svc-good",
			map[string]any{"container": map[string]any{"image": "img:v1"}},
			"/repo/svc-good-workload.yaml")
		// No workload for svc-bad

		ocIndex := fsmode.WrapIndex(idx)
		gen := NewReleaseGenerator(ocIndex)

		result, err := gen.GenerateBulkReleases(BulkReleaseOptions{
			ProjectName: "myproj",
			Namespace:   "default",
			Version:     "1",
		})
		require.NoError(t, err)
		assert.Len(t, result.Releases, 1)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "svc-bad", result.Errors[0].ComponentName)
	})
}
