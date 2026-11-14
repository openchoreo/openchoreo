// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	codedefinition "github.com/openchoreo/openchoreo/tools/lsp/code-definition"
	lsputil "github.com/openchoreo/openchoreo/tools/lsp/lsp-util"
)

func TestFindKindByFieldName(t *testing.T) {
	// Simulate registered kinds from the indexer
	registeredKinds := []string{
		"Workflow",
		"Trait",
		"DeploymentPipeline",
		"Project",
		"Component",
		"Environment",
		"ClusterRoleBinding",
		"ServiceAccount",
	}

	tests := []struct {
		field    string
		expected string
	}{
		{"allowedWorkflows", "Workflow"},
		{"allowedTraits", "Trait"},
		{"deploymentPipelineRef", "DeploymentPipeline"},
		{"projectName", "Project"},
		{"componentName", "Component"},
		{"sourceEnvironmentRef", "Environment"},
		{"targetEnvironmentRefs", "Environment"},
		{"environment", "Environment"},
		{"component", "Component"},
		{"clusterRoleBinding", "ClusterRoleBinding"},
		{"serviceAccount", "ServiceAccount"},
		// Edge cases
		{"unknownField", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got := lsputil.FindKindByFieldName(tt.field, registeredKinds)
			if got != tt.expected {
				t.Errorf("lsputil.FindKindByFieldName(%q) = %q, want %q", tt.field, got, tt.expected)
			}
		})
	}
}

func TestDefinition_MetadataName(t *testing.T) {
	src := releaseWorkflowYAML

	docs, err := parser.ParseYAML([]byte(src))
	if err != nil || len(docs) == 0 {
		t.Fatalf("parse failed: %v", err)
	}
	document := docs[0]

	cursorLine := 4
	cursorCol := 26

	node := lsputil.FindNodeAtPosition(document.Root, cursorLine, cursorCol)
	if node == nil {
		t.Fatal("lsputil.FindNodeAtPosition returned nil")
	}
	if node.Kind != yaml.ScalarNode {
		t.Fatalf("expected scalar, got kind=%d", node.Kind)
	}

	parentMapping, parentKey := lsputil.FindParentMappingAndKey(document.Root, node)
	if parentMapping == nil {
		t.Fatal("lsputil.FindParentMappingAndKey returned nil parent")
	}
	if parentKey != testValueName {
		t.Fatalf("expected parent key 'name', got %q", parentKey)
	}

	idx := indexer.NewIndexer()
	err = idx.UpdateFile("file:///test/workflow.yaml", []byte(src))
	if err != nil {
		t.Fatalf("indexer update failed: %v", err)
	}

	locs := idx.FindDefinition("Workflow", "docker-gitops-release")
	if len(locs) == 0 {
		t.Fatal("FindDefinition returned no results")
	}
	if locs[0].Line != 2 {
		t.Fatalf("expected line 2 (kind line), got line %d", locs[0].Line)
	}
}

func TestDefinition_CrossFileReference(t *testing.T) {
	componentSrc := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-app
spec:
  allowedWorkflows:
    - name: docker-gitops-release
`

	workflowSrc := releaseWorkflowYAML

	docs, err := parser.ParseYAML([]byte(componentSrc))
	if err != nil || len(docs) == 0 {
		t.Fatalf("parse failed: %v", err)
	}
	document := docs[0]

	cursorLine := 7
	cursorCol := 24

	node := lsputil.FindNodeAtPosition(document.Root, cursorLine, cursorCol)
	if node == nil {
		t.Fatal("lsputil.FindNodeAtPosition returned nil")
	}

	parentMapping, parentKey := lsputil.FindParentMappingAndKey(document.Root, node)
	if parentMapping == nil {
		t.Fatal("parent mapping is nil")
	}
	if parentKey != testValueName {
		t.Fatalf("expected parent key 'name', got %q", parentKey)
	}

	// Test the new kind inference approach
	yamlPath := lsputil.BuildPathToNode(document.Root, parentMapping)
	normalized := lsputil.NormalizePath(yamlPath)
	segments := lsputil.SplitPath(normalized)
	if len(segments) == 0 {
		t.Fatal("lsputil.SplitPath returned empty segments")
	}
	fieldName := segments[len(segments)-1]

	// Register Workflow kind in indexer
	idx := indexer.NewIndexer()
	err = idx.UpdateFile("file:///test/workflow.yaml", []byte(workflowSrc))
	if err != nil {
		t.Fatalf("indexer update failed: %v", err)
	}

	registeredKinds := idx.FindAllKinds()
	inferredKind := lsputil.FindKindByFieldName(fieldName, registeredKinds)
	if inferredKind != "Workflow" {
		t.Fatalf("expected inferred kind 'Workflow', got %q (fieldName=%q, kinds=%v)", inferredKind, fieldName, registeredKinds)
	}

	locs := idx.FindDefinition("Workflow", "docker-gitops-release")
	if len(locs) == 0 {
		t.Fatal("FindDefinition returned no results")
	}
}

func TestDefinition_ComponentType_AllowedWorkflows(t *testing.T) {
	componentTypeSrc := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: web-application
  namespace: default
spec:
  workloadType: deployment

  allowedWorkflows:
    - kind: Workflow
      name: docker-gitops-release
`

	workflowSrc := releaseWorkflowYAML

	docs, err := parser.ParseYAML([]byte(componentTypeSrc))
	if err != nil || len(docs) == 0 {
		t.Fatalf("parse failed: %v", err)
	}
	document := docs[0]

	cursorLine := 11
	cursorCol := 32

	node := lsputil.FindNodeAtPosition(document.Root, cursorLine, cursorCol)
	if node == nil {
		t.Fatal("lsputil.FindNodeAtPosition returned nil")
	}

	parentMapping, parentKey := lsputil.FindParentMappingAndKey(document.Root, node)
	if parentMapping == nil {
		t.Fatal("lsputil.FindParentMappingAndKey returned nil")
	}
	if parentKey != testValueName {
		t.Fatalf("expected parent key 'name', got %q", parentKey)
	}

	// Sibling kind should be found directly
	kind := ""
	kindNode := lsputil.FindSiblingKey(parentMapping, "kind")
	if kindNode != nil && kindNode.Kind == yaml.ScalarNode {
		kind = kindNode.Value
	}
	if kind != "Workflow" {
		t.Fatalf("expected kind 'Workflow', got %q", kind)
	}

	idx := indexer.NewIndexer()
	err = idx.UpdateFile("file:///test/workflow.yaml", []byte(workflowSrc))
	if err != nil {
		t.Fatalf("indexer update failed: %v", err)
	}

	locs := idx.FindDefinition("Workflow", "docker-gitops-release")
	if len(locs) == 0 {
		t.Fatal("FindDefinition returned no results")
	}
}

func TestDefinition_FindByName_Fallback(t *testing.T) {
	// When kind is unknown, FindByName should find resources across all kinds
	src := `apiVersion: openchoreo.dev/v1alpha1
kind: Workflow
metadata:
  name: my-resource
`

	idx := indexer.NewIndexer()
	err := idx.UpdateFile("file:///test/workflow.yaml", []byte(src))
	if err != nil {
		t.Fatalf("indexer update failed: %v", err)
	}

	// FindByName should find it regardless of kind
	locs := idx.FindByName("my-resource")
	if len(locs) == 0 {
		t.Fatal("FindByName returned no results")
	}
	t.Logf("FindByName found %d result(s)", len(locs))
}

// TestHandle_SameGitProjectOnly reproduces the reported go-to-definition bug:
// the same (Trait, persistent-volume) resource exists in two sibling git
// repositories of one workspace. Go-to-definition must resolve inside the
// source file's own git project (identified via git), not the sibling checkout.
func TestHandle_SameGitProjectOnly(t *testing.T) {
	sourceRepo := t.TempDir()
	otherRepo := t.TempDir()
	for _, dir := range []string{sourceRepo, otherRepo} {
		if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
			t.Fatalf("git init %s: %v (%s)", dir, err, out)
		}
	}

	trait := persistentVolumeTraitYAML
	doc := webApplicationTypeYAML

	idx := indexer.NewIndexer()
	// Foreign-repo candidate indexed first (mimics alphabetical workspace walk).
	otherTrait := filepath.Join(otherRepo, "component-with-traits.yaml")
	if err := idx.UpdateFile("file://"+otherTrait, []byte(trait)); err != nil {
		t.Fatalf("index foreign repo: %v", err)
	}
	sameTrait := filepath.Join(sourceRepo, "persistent-volume.yaml")
	if err := idx.UpdateFile("file://"+sameTrait, []byte(trait)); err != nil {
		t.Fatalf("index source repo: %v", err)
	}

	sourceURI := "file://" + filepath.Join(sourceRepo, "webapp.yaml")
	locs := codedefinition.Handle(doc, 7, 15, sourceURI, idx)
	if len(locs) != 1 {
		t.Fatalf("expected exactly 1 definition from the source git project, got %d: %v", len(locs), locs)
	}
	if want := "file://" + sameTrait; locs[0].URI != want {
		t.Errorf("definition = %s, want %s", locs[0].URI, want)
	}
}

// TestHandle_EncodedSourceURI_SameGitProject mirrors the real workspace: the
// parent folder name contains "&", the client sends it percent-encoded
// (OpenChoreoLSP%26MCPToolkit) while the indexer stores decoded paths. The
// same-project candidate must still win over the sibling git checkout.
func TestHandle_EncodedSourceURI_SameGitProject(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "OpenChoreoLSP&MCPToolkit")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	sourceRepo := filepath.Join(workspace, "sample-gitops")
	otherRepo := filepath.Join(workspace, "openchoreo")
	for _, dir := range []string{sourceRepo, otherRepo} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
			t.Fatalf("git init %s: %v (%s)", dir, err, out)
		}
	}

	trait := persistentVolumeTraitYAML
	doc := webApplicationTypeYAML

	idx := indexer.NewIndexer()
	otherTrait := filepath.Join(otherRepo, "component-with-traits.yaml")
	if err := idx.UpdateFile("file://"+otherTrait, []byte(trait)); err != nil {
		t.Fatalf("index foreign repo: %v", err)
	}
	sameTrait := filepath.Join(sourceRepo, "persistent-volume.yaml")
	if err := idx.UpdateFile("file://"+sameTrait, []byte(trait)); err != nil {
		t.Fatalf("index source repo: %v", err)
	}

	// Client sends the source URI percent-encoded; indexer paths are decoded.
	srcPath := filepath.Join(sourceRepo, "webapp.yaml")
	sourceURI := "file://" + strings.ReplaceAll(srcPath, "&", "%26")

	locs := codedefinition.Handle(doc, 7, 15, sourceURI, idx)
	if len(locs) != 1 {
		t.Fatalf("expected exactly 1 definition, got %d: %v", len(locs), locs)
	}
	if want := "file://" + sameTrait; locs[0].URI != want {
		t.Errorf("definition = %s, want %s", locs[0].URI, want)
	}
}

// TestHandle_SameProjectOnly reproduces a go-to-definition bug: the same
// (Trait, persistent-volume) resource exists in an unrelated upstream checkout
// AND next to the source document. Go-to-definition must only return the one
// belonging to the same project as the source file.
func TestHandle_SameProjectOnly(t *testing.T) {
	upstreamTrait := persistentVolumeTraitYAML
	sampleTrait := `apiVersion: openchoreo.dev/v1alpha1
kind: Trait
metadata:
  name: persistent-volume
  namespace: default
`
	doc := webApplicationTypeYAML

	idx := indexer.NewIndexer()
	if err := idx.UpdateFile(
		"file:///workspace/openchoreo/internal/pipeline/component/testdata/component-with-traits.yaml",
		[]byte(upstreamTrait),
	); err != nil {
		t.Fatalf("index upstream: %v", err)
	}
	if err := idx.UpdateFile(
		"file:///workspace/sample-gitops/namespaces/default/platform/traits/persistent-volume.yaml",
		[]byte(sampleTrait),
	); err != nil {
		t.Fatalf("index sample: %v", err)
	}

	sourceURI := "file:///workspace/sample-gitops/namespaces/default/platform/component-types/webapp.yaml"
	locs := codedefinition.Handle(doc, 7, 15, sourceURI, idx)
	if len(locs) != 1 {
		t.Fatalf("expected exactly 1 definition, got %d: %v", len(locs), locs)
	}
	want := "file:///workspace/sample-gitops/namespaces/default/platform/traits/persistent-volume.yaml"
	if locs[0].URI != want {
		t.Errorf("first definition = %s, want %s", locs[0].URI, want)
	}
}
