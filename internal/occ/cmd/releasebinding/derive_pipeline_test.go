// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
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

		params := api.GenerateReleaseBindingParams{
			ProjectName: projectName,
		}

		err := deriveUsePipeline(ocIndex, namespace, &params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if params.UsePipeline != pipelineName {
			t.Errorf("UsePipeline = %q, want %q", params.UsePipeline, pipelineName)
		}
	})

	t.Run("skips derivation when UsePipeline already set", func(t *testing.T) {
		idx := index.New("/repo")
		addProject(t, idx, namespace, projectName, pipelineName)
		ocIndex := fsmode.WrapIndex(idx)

		explicit := "explicit-pipeline"
		params := api.GenerateReleaseBindingParams{
			ProjectName: projectName,
			UsePipeline: explicit,
		}

		err := deriveUsePipeline(ocIndex, namespace, &params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if params.UsePipeline != explicit {
			t.Errorf("UsePipeline = %q, want %q (should keep explicit value)", params.UsePipeline, explicit)
		}
	})

	t.Run("error when project not found", func(t *testing.T) {
		idx := index.New("/repo")
		ocIndex := fsmode.WrapIndex(idx)

		params := api.GenerateReleaseBindingParams{
			ProjectName: "nonexistent",
		}

		err := deriveUsePipeline(ocIndex, namespace, &params)
		if err == nil {
			t.Fatal("expected error for missing project, got nil")
		}

		want := `project "nonexistent" not found in namespace "test-ns"`
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("error when deploymentPipelineRef is empty", func(t *testing.T) {
		idx := index.New("/repo")
		addProject(t, idx, namespace, projectName, "") // empty pipelineRef
		ocIndex := fsmode.WrapIndex(idx)

		params := api.GenerateReleaseBindingParams{
			ProjectName: projectName,
		}

		err := deriveUsePipeline(ocIndex, namespace, &params)
		if err == nil {
			t.Fatal("expected error for empty deploymentPipelineRef, got nil")
		}

		want := `project "my-proj" has no deploymentPipelineRef set`
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("error when --all is set and UsePipeline is empty", func(t *testing.T) {
		idx := index.New("/repo")
		ocIndex := fsmode.WrapIndex(idx)

		params := api.GenerateReleaseBindingParams{
			All: true,
		}

		err := deriveUsePipeline(ocIndex, namespace, &params)
		if err == nil {
			t.Fatal("expected error when --all is set without --use-pipeline, got nil")
		}

		want := "--use-pipeline is required when using --all"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("no error when --all is set and UsePipeline is provided", func(t *testing.T) {
		idx := index.New("/repo")
		ocIndex := fsmode.WrapIndex(idx)

		params := api.GenerateReleaseBindingParams{
			All:         true,
			UsePipeline: pipelineName,
		}

		err := deriveUsePipeline(ocIndex, namespace, &params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if params.UsePipeline != pipelineName {
			t.Errorf("UsePipeline = %q, want %q", params.UsePipeline, pipelineName)
		}
	})

	t.Run("error when both UsePipeline and ProjectName are empty", func(t *testing.T) {
		idx := index.New("/repo")
		ocIndex := fsmode.WrapIndex(idx)

		params := api.GenerateReleaseBindingParams{}

		err := deriveUsePipeline(ocIndex, namespace, &params)
		if err == nil {
			t.Fatal("expected error when pipeline cannot be derived, got nil")
		}

		want := "--use-pipeline is required (could not derive from project)"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("error when ProjectName is empty and UsePipeline is empty", func(t *testing.T) {
		idx := index.New("/repo")
		addProject(t, idx, namespace, projectName, pipelineName)
		ocIndex := fsmode.WrapIndex(idx)

		// ProjectName is empty, so derivation is skipped; UsePipeline is also empty
		params := api.GenerateReleaseBindingParams{}

		err := deriveUsePipeline(ocIndex, namespace, &params)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		want := "--use-pipeline is required (could not derive from project)"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("project in different namespace is not found", func(t *testing.T) {
		idx := index.New("/repo")
		addProject(t, idx, "other-ns", "other-proj", pipelineName)
		ocIndex := fsmode.WrapIndex(idx)

		params := api.GenerateReleaseBindingParams{
			ProjectName: "other-proj",
		}

		err := deriveUsePipeline(ocIndex, namespace, &params)
		if err == nil {
			t.Fatal("expected error when project is in different namespace, got nil")
		}

		want := `project "other-proj" not found in namespace "test-ns"`
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("project without spec field returns empty pipelineRef error", func(t *testing.T) {
		idx := index.New("/repo")
		// Add a project with no spec at all
		addProjectRaw(t, idx, namespace, projectName, nil)
		ocIndex := fsmode.WrapIndex(idx)

		params := api.GenerateReleaseBindingParams{
			ProjectName: projectName,
		}

		err := deriveUsePipeline(ocIndex, namespace, &params)
		if err == nil {
			t.Fatal("expected error for project without spec, got nil")
		}

		want := `project "my-proj" has no deploymentPipelineRef set`
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})
}

// addProject adds a Project resource entry with a deploymentPipelineRef to the index.
func addProject(t *testing.T, idx *index.Index, namespace, name, pipelineRef string) {
	t.Helper()
	spec := map[string]any{}
	if pipelineRef != "" {
		spec["deploymentPipelineRef"] = pipelineRef
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
	if err := idx.Add(entry); err != nil {
		t.Fatal(err)
	}
}
