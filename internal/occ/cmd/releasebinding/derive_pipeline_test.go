// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func TestDeriveUsePipeline(t *testing.T) {
	const (
		namespace    = "test-ns"
		projectName  = "my-proj"
		pipelineName = "my-pipeline"
	)

	t.Run("derives pipeline from project deploymentPipelineRef", func(t *testing.T) {
		idx := index.New("/repo")
		addProject(t, idx, namespace, projectName, pipelineName)
		ocIndex := fsmode.WrapIndex(idx)

		params := GenerateParams{ProjectName: projectName}
		require.NoError(t, deriveUsePipeline(ocIndex, namespace, &params))
		assert.Equal(t, pipelineName, params.UsePipeline)
	})

	t.Run("skips derivation when UsePipeline already set", func(t *testing.T) {
		idx := index.New("/repo")
		addProject(t, idx, namespace, projectName, pipelineName)
		ocIndex := fsmode.WrapIndex(idx)

		params := GenerateParams{ProjectName: projectName, UsePipeline: "explicit-pipeline"}
		require.NoError(t, deriveUsePipeline(ocIndex, namespace, &params))
		assert.Equal(t, "explicit-pipeline", params.UsePipeline)
	})

	t.Run("error when project not found", func(t *testing.T) {
		idx := index.New("/repo")
		ocIndex := fsmode.WrapIndex(idx)

		params := GenerateParams{ProjectName: "nonexistent"}
		err := deriveUsePipeline(ocIndex, namespace, &params)
		require.Error(t, err)
		assert.Equal(t, `project "nonexistent" not found in namespace "test-ns"`, err.Error())
	})

	t.Run("error when deploymentPipelineRef is empty", func(t *testing.T) {
		idx := index.New("/repo")
		addProject(t, idx, namespace, projectName, "")
		ocIndex := fsmode.WrapIndex(idx)

		params := GenerateParams{ProjectName: projectName}
		err := deriveUsePipeline(ocIndex, namespace, &params)
		require.Error(t, err)
		assert.Equal(t, `project "my-proj" has no deploymentPipelineRef set`, err.Error())
	})

	t.Run("error when --all is set and UsePipeline is empty", func(t *testing.T) {
		idx := index.New("/repo")
		ocIndex := fsmode.WrapIndex(idx)

		params := GenerateParams{All: true}
		err := deriveUsePipeline(ocIndex, namespace, &params)
		require.Error(t, err)
		assert.Equal(t, "--use-pipeline is required when using --all", err.Error())
	})

	t.Run("no error when --all is set and UsePipeline is provided", func(t *testing.T) {
		idx := index.New("/repo")
		ocIndex := fsmode.WrapIndex(idx)

		params := GenerateParams{All: true, UsePipeline: pipelineName}
		require.NoError(t, deriveUsePipeline(ocIndex, namespace, &params))
		assert.Equal(t, pipelineName, params.UsePipeline)
	})

	t.Run("error when both UsePipeline and ProjectName are empty", func(t *testing.T) {
		idx := index.New("/repo")
		ocIndex := fsmode.WrapIndex(idx)

		params := GenerateParams{}
		err := deriveUsePipeline(ocIndex, namespace, &params)
		require.Error(t, err)
		assert.Equal(t, "--use-pipeline is required (could not derive from project)", err.Error())
	})

	t.Run("error when ProjectName is empty and UsePipeline is empty", func(t *testing.T) {
		idx := index.New("/repo")
		addProject(t, idx, namespace, projectName, pipelineName)
		ocIndex := fsmode.WrapIndex(idx)

		params := GenerateParams{}
		err := deriveUsePipeline(ocIndex, namespace, &params)
		require.Error(t, err)
		assert.Equal(t, "--use-pipeline is required (could not derive from project)", err.Error())
	})

	t.Run("project in different namespace is not found", func(t *testing.T) {
		idx := index.New("/repo")
		addProject(t, idx, "other-ns", "other-proj", pipelineName)
		ocIndex := fsmode.WrapIndex(idx)

		params := GenerateParams{ProjectName: "other-proj"}
		err := deriveUsePipeline(ocIndex, namespace, &params)
		require.Error(t, err)
		assert.Equal(t, `project "other-proj" not found in namespace "test-ns"`, err.Error())
	})

	t.Run("project without spec field returns empty pipelineRef error", func(t *testing.T) {
		idx := index.New("/repo")
		addProjectRaw(t, idx, namespace, projectName, nil)
		ocIndex := fsmode.WrapIndex(idx)

		params := GenerateParams{ProjectName: projectName}
		err := deriveUsePipeline(ocIndex, namespace, &params)
		require.Error(t, err)
		assert.Equal(t, `project "my-proj" has no deploymentPipelineRef set`, err.Error())
	})
}

// addProject adds a Project resource entry with a deploymentPipelineRef to the index.
func addProject(t *testing.T, idx *index.Index, namespace, name, pipelineRef string) {
	t.Helper()
	spec := map[string]any{}
	if pipelineRef != "" {
		spec["deploymentPipelineRef"] = map[string]any{"name": pipelineRef}
	}
	addProjectRaw(t, idx, namespace, name, spec)
}

// addProjectRaw adds a Project resource entry with an arbitrary spec to the index.
func addProjectRaw(t *testing.T, idx *index.Index, namespace, name string, spec map[string]any) {
	t.Helper()
	obj := map[string]any{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       "Project",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
	}
	if spec != nil {
		obj["spec"] = spec
	}
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{Object: obj},
		FilePath: "/repo/projects/" + name + "/project.yaml",
	}
	require.NoError(t, idx.Add(entry))
}
