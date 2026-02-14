// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"os"
	"path/filepath"
	"testing"

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

func TestGenerateBindingWithInfo_Create(t *testing.T) {
	const (
		namespace     = "test-ns"
		projectName   = "my-proj"
		componentName = "my-comp"
		targetEnv     = "dev"
		releaseName   = "my-comp-20260101-0"
	)

	idx := index.New("/repo")

	// Add a ComponentRelease so selectComponentRelease can find the latest release
	addRelease(t, idx, namespace, releaseName, projectName, componentName,
		"/repo/projects/my-proj/components/my-comp/releases/my-comp-20260101-0.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewBindingGenerator(ocIndex)

	info, err := gen.GenerateBindingWithInfo(BindingOptions{
		ProjectName:   projectName,
		ComponentName: componentName,
		TargetEnv:     targetEnv,
		PipelineInfo:  newTestPipelineInfo(targetEnv),
		Namespace:     namespace,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify CREATE path fields
	if info.IsUpdate {
		t.Error("expected IsUpdate to be false for a new binding")
	}
	if info.ExistingFilePath != "" {
		t.Errorf("expected empty ExistingFilePath, got %q", info.ExistingFilePath)
	}
	if info.ProjectName != projectName {
		t.Errorf("ProjectName: got %q, want %q", info.ProjectName, projectName)
	}
	if info.ComponentName != componentName {
		t.Errorf("ComponentName: got %q, want %q", info.ComponentName, componentName)
	}
	if info.Environment != targetEnv {
		t.Errorf("Environment: got %q, want %q", info.Environment, targetEnv)
	}
	if info.ReleaseName != releaseName {
		t.Errorf("ReleaseName: got %q, want %q", info.ReleaseName, releaseName)
	}
	wantBindingName := componentName + "-" + targetEnv
	if info.BindingName != wantBindingName {
		t.Errorf("BindingName: got %q, want %q", info.BindingName, wantBindingName)
	}
	if info.Binding == nil {
		t.Fatal("expected non-nil Binding")
	}
}

func TestGenerateBindingWithInfo_Update(t *testing.T) {
	const (
		namespace     = "test-ns"
		projectName   = "my-proj"
		componentName = "my-comp"
		targetEnv     = "dev"
		newRelease    = "my-comp-20260101-0"
		oldRelease    = "my-comp-20250101-0"
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
	if err := os.WriteFile(bindingFile, []byte(existingYAML), 0600); err != nil {
		t.Fatal(err)
	}

	idx := index.New(tmpDir)

	// Add a ComponentRelease so selectComponentRelease can find the latest release
	addRelease(t, idx, namespace, newRelease, projectName, componentName,
		filepath.Join(tmpDir, "releases", "my-comp-20260101-0.yaml"))

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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify UPDATE path fields
	if !info.IsUpdate {
		t.Error("expected IsUpdate to be true for an existing binding")
	}
	if info.ExistingFilePath != bindingFile {
		t.Errorf("ExistingFilePath: got %q, want %q", info.ExistingFilePath, bindingFile)
	}
	if info.ProjectName != projectName {
		t.Errorf("ProjectName: got %q, want %q", info.ProjectName, projectName)
	}
	if info.ComponentName != componentName {
		t.Errorf("ComponentName: got %q, want %q", info.ComponentName, componentName)
	}
	if info.Environment != targetEnv {
		t.Errorf("Environment: got %q, want %q", info.Environment, targetEnv)
	}
	if info.ReleaseName != newRelease {
		t.Errorf("ReleaseName: got %q, want %q", info.ReleaseName, newRelease)
	}
	if info.Binding == nil {
		t.Fatal("expected non-nil Binding")
	}

	// Verify releaseName was updated in the binding object
	gotRelease := getNestedString(info.Binding.Object, "spec", "releaseName")
	if gotRelease != newRelease {
		t.Errorf("binding spec.releaseName: got %q, want %q", gotRelease, newRelease)
	}

	// Verify extra fields from the original file are preserved
	gotCustom := getNestedString(info.Binding.Object, "spec", "customField")
	if gotCustom != "preserve-this" {
		t.Errorf("binding spec.customField: got %q, want %q (field lost during update)", gotCustom, "preserve-this")
	}

	labels, _, _ := unstructured.NestedStringMap(info.Binding.Object, "metadata", "labels")
	if labels["team"] != "platform" {
		t.Errorf("binding metadata.labels.team: got %q, want %q", labels["team"], "platform")
	}

	annotations, _, _ := unstructured.NestedStringMap(info.Binding.Object, "metadata", "annotations")
	if annotations["note"] != "do-not-lose-me" {
		t.Errorf("binding metadata.annotations.note: got %q, want %q", annotations["note"], "do-not-lose-me")
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
	if err := idx.Add(entry); err != nil {
		t.Fatal(err)
	}
}
