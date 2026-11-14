// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

func TestParseYAML_SingleDocument(t *testing.T) {
	input := []byte("apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: demo\nspec:\n  owner:\n    projectName: default\n  componentType:\n    kind: ComponentType\n    name: deployment/web-app\n")

	docs, err := parser.ParseYAML(input)
	if err != nil {
		t.Fatalf("ParseYAML returned error: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	doc := docs[0]

	if doc.Kind != testValueComponent {
		t.Errorf("expected kind Component, got %q", doc.Kind)
	}
	if doc.APIVersion != "openchoreo.dev/v1alpha1" {
		t.Errorf("expected apiVersion openchoreo.dev/v1alpha1, got %q", doc.APIVersion)
	}
	if doc.Metadata.Name != "demo" {
		t.Errorf("expected metadata.name demo, got %q", doc.Metadata.Name)
	}
	if doc.Spec == nil {
		t.Error("expected spec to be non-nil")
	}

	if doc.Range.Start.Line != 1 {
		t.Errorf("expected document start at line 1, got %d", doc.Range.Start.Line)
	}
}

func TestParseYAML_MultiDocument(t *testing.T) {
	input := []byte("apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: proj-a\nspec: {}\n---\napiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: comp-a\nspec: {}\n")

	docs, err := parser.ParseYAML(input)
	if err != nil {
		t.Fatalf("ParseYAML returned error: %v", err)
	}

	if len(docs) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(docs))
	}

	if docs[0].Kind != "Project" {
		t.Errorf("expected first kind Project, got %q", docs[0].Kind)
	}
	if docs[0].Metadata.Name != "proj-a" {
		t.Errorf("expected first metadata.name proj-a, got %q", docs[0].Metadata.Name)
	}
	if docs[0].DocumentIndex != 0 {
		t.Errorf("expected first document index 0, got %d", docs[0].DocumentIndex)
	}

	if docs[1].Kind != testValueComponent {
		t.Errorf("expected second kind Component, got %q", docs[1].Kind)
	}
	if docs[1].Metadata.Name != "comp-a" {
		t.Errorf("expected second metadata.name comp-a, got %q", docs[1].Metadata.Name)
	}
	if docs[1].DocumentIndex != 1 {
		t.Errorf("expected second document index 1, got %d", docs[1].DocumentIndex)
	}
}

func TestParseYAML_EmptyDocument(t *testing.T) {
	input := []byte("---\n")

	docs, err := parser.ParseYAML(input)
	if err != nil {
		t.Fatalf("ParseYAML returned error: %v", err)
	}

	if len(docs) != 0 {
		t.Fatalf("expected no documents, got %d", len(docs))
	}
}

func TestParseYAML_PositionTracking(t *testing.T) {
	input := []byte("apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: my-project\n  namespace: my-ns\nspec:\n  deploymentPipelineRef:\n    name: default\n")

	docs, err := parser.ParseYAML(input)
	if err != nil {
		t.Fatalf("ParseYAML returned error: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	doc := docs[0]

	apiVerField := doc.GetField("apiVersion")
	if apiVerField == nil {
		t.Fatal("expected apiVersion field to exist")
	}
	if apiVerField.Range.Start.Line != 1 {
		t.Errorf("expected apiVersion at line 1, got %d", apiVerField.Range.Start.Line)
	}

	kindField := doc.GetField("kind")
	if kindField == nil {
		t.Fatal("expected kind field to exist")
	}
	if kindField.Range.Start.Line != 2 {
		t.Errorf("expected kind at line 2, got %d", kindField.Range.Start.Line)
	}

	metaField := doc.GetField("metadata")
	if metaField == nil {
		t.Fatal("expected metadata field to exist")
	}
	if metaField.Range.Start.Line != 3 {
		t.Errorf("expected metadata at line 3, got %d", metaField.Range.Start.Line)
	}

	if doc.Metadata.Name != "my-project" {
		t.Errorf("expected metadata.name my-project, got %q", doc.Metadata.Name)
	}
	if doc.Metadata.Namespace != "my-ns" {
		t.Errorf("expected metadata.namespace my-ns, got %q", doc.Metadata.Namespace)
	}

	specField := doc.GetField("spec")
	if specField == nil {
		t.Fatal("expected spec field to exist")
	}
	if specField.Range.Start.Line != 6 {
		t.Errorf("expected spec at line 6, got %d", specField.Range.Start.Line)
	}
}

func TestParseYAML_FieldLookup(t *testing.T) {
	input := []byte("apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: svc\nspec:\n  owner:\n    projectName: default\n")

	docs, err := parser.ParseYAML(input)
	if err != nil {
		t.Fatalf("ParseYAML returned error: %v", err)
	}

	doc := docs[0]

	if doc.GetField("nonexistent") != nil {
		t.Error("expected nil for nonexistent field")
	}

	f := doc.GetField("kind")
	if f == nil {
		t.Fatal("expected kind field")
	}
	if f.Key != "kind" {
		t.Errorf("expected key 'kind', got %q", f.Key)
	}
}

func TestParseYAML_DocumentForPosition(t *testing.T) {
	input := []byte("apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: proj-a\nspec: {}\n---\napiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: comp-a\nspec: {}\n")

	docs, err := parser.ParseYAML(input)
	if err != nil {
		t.Fatalf("ParseYAML returned error: %v", err)
	}

	doc := parser.DocumentForPosition(docs, 1, 1)
	if doc == nil || doc.Kind != "Project" {
		t.Errorf("expected Project doc at line 1, got %v", doc)
	}

	doc = parser.DocumentForPosition(docs, 7, 1)
	if doc == nil || doc.Kind != testValueComponent {
		t.Errorf("expected Component doc at line 7, got %v", doc)
	}

	doc = parser.DocumentForPosition(docs, 100, 1)
	if doc != nil {
		t.Error("expected nil for out-of-range position")
	}
}

func TestParseYAML_RealisticFixtures(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		expected int
		kinds    []string
		names    []string
	}{
		{
			name:     "component",
			fixture:  "component.yaml",
			expected: 1,
			kinds:    []string{testValueComponent},
			names:    []string{"demo"},
		},
		{
			name:     "project and component",
			fixture:  "project-and-component.yaml",
			expected: 2,
			kinds:    []string{"Project", testValueComponent},
			names:    []string{"proj-a", "comp-a"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join("..", "testData", "fixtures", test.fixture)
			input, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read fixture %q: %v", path, err)
			}

			docs, err := parser.ParseYAML(input)
			if err != nil {
				t.Fatalf("ParseYAML returned error: %v", err)
			}

			if len(docs) != test.expected {
				t.Fatalf("expected %d documents, got %d", test.expected, len(docs))
			}

			for i, doc := range docs {
				if doc.APIVersion != "openchoreo.dev/v1alpha1" {
					t.Errorf("document %d: expected apiVersion openchoreo.dev/v1alpha1, got %q", i, doc.APIVersion)
				}
				if doc.Kind != test.kinds[i] {
					t.Errorf("document %d: expected kind %q, got %q", i, test.kinds[i], doc.Kind)
				}
				if doc.Metadata.Name != test.names[i] {
					t.Errorf("document %d: expected metadata.name %q, got %q", i, test.names[i], doc.Metadata.Name)
				}
			}
		})
	}
}
func TestParseYAML_CRDDataPlane(t *testing.T) {
	path := filepath.Join("..", "testData", "fixtures", "crd-dataplane.yaml")
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	docs, err := parser.ParseYAML(input)
	if err != nil {
		t.Fatalf("ParseYAML returned error: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	doc := docs[0]

	if doc.APIVersion != "apiextensions.k8s.io/v1" {
		t.Errorf("expected apiVersion apiextensions.k8s.io/v1, got %q", doc.APIVersion)
	}
	if doc.Kind != "CustomResourceDefinition" {
		t.Errorf("expected kind CustomResourceDefinition, got %q", doc.Kind)
	}
	if doc.Metadata.Name != "dataplanes.openchoreo.dev" {
		t.Errorf("expected metadata.name dataplanes.openchoreo.dev, got %q", doc.Metadata.Name)
	}

	if doc.Metadata.Annotations == nil {
		t.Fatal("expected annotations to be non-nil")
	}
	if doc.Metadata.Annotations.Kind != yaml.MappingNode {
		t.Error("expected annotations to be a mapping node")
	}

	if doc.Spec == nil {
		t.Fatal("expected spec to be non-nil")
	}

	specGroup := findMappingValue(doc.Spec, "group")
	if specGroup == nil || specGroup.ValueNode.Value != "openchoreo.dev" {
		t.Errorf("expected spec.group openchoreo.dev, got %v", specGroup)
	}

	specScope := findMappingValue(doc.Spec, "scope")
	if specScope == nil || specScope.ValueNode.Value != "Namespaced" {
		t.Errorf("expected spec.scope Namespaced, got %v", specScope)
	}

	namesNode := findMappingValue(doc.Spec, "names")
	if namesNode == nil {
		t.Fatal("expected spec.names to exist")
	}
	namesKind := findMappingValue(namesNode.ValueNode, "kind")
	if namesKind == nil || namesKind.ValueNode.Value != "DataPlane" {
		t.Errorf("expected spec.names.kind DataPlane, got %v", namesKind)
	}
	namesPlural := findMappingValue(namesNode.ValueNode, "plural")
	if namesPlural == nil || namesPlural.ValueNode.Value != "dataplanes" {
		t.Errorf("expected spec.names.plural dataplanes, got %v", namesPlural)
	}

	shortNames := findMappingValue(namesNode.ValueNode, "shortNames")
	if shortNames == nil || shortNames.ValueNode.Kind != yaml.SequenceNode {
		t.Error("expected spec.names.shortNames to be a sequence")
	}

	assertDataPlaneVersions(t, doc.Spec)

	t.Logf("CRD parsed successfully: kind=%s name=%s group=%s scope=%s",
		doc.Kind, doc.Metadata.Name, specGroup.ValueNode.Value, specScope.ValueNode.Value)
}

func findMappingValue(node *yaml.Node, key string) *parser.Field {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return &parser.Field{
				Key:       key,
				KeyNode:   node.Content[i],
				ValueNode: node.Content[i+1],
				Range: parser.Range{
					Start: parser.Position{Line: node.Content[i].Line, Column: node.Content[i].Column},
					End:   parser.Position{Line: node.Content[i+1].Line, Column: node.Content[i+1].Column},
				},
			}
		}
	}
	return nil
}

func TestParseYAML_MissingDashSequenceIndicator(t *testing.T) {
	input := []byte(`apiVersion: openchoreo.dev/v1alpha1
kind: ClusterDataPlane
metadata:
  name: default
status:
  conditions:
    lastTransitionTime: 2026-07-23T10:09:08Z
      message: ClusterDataplane is created
`)
	_, err := parser.ParseYAML(input)
	if err == nil {
		t.Fatal("expected a parse error for missing dash")
	}
	if !strings.Contains(err.Error(), "missing '-' sign (sequence indicator)") {
		t.Errorf("expected missing-dash hint in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "lastTransitionTime") {
		t.Errorf("expected error to name the affected key, got: %v", err)
	}
}

func TestParseYAML_ValidSequenceNoFalsePositive(t *testing.T) {
	input := []byte(`apiVersion: openchoreo.dev/v1alpha1
kind: ClusterDataPlane
metadata:
  name: default
status:
  conditions:
    - lastTransitionTime: 2026-07-23T10:09:08Z
      message: ClusterDataplane is created
`)
	docs, err := parser.ParseYAML(input)
	if err != nil {
		t.Fatalf("valid yaml with dash should parse, got error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
}

func TestParseYAML_CRDTest1(t *testing.T) {
	path := filepath.Join("..", "testData", "fixtures", "crd-test1.yaml")
	input, err := os.ReadFile(path)

	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	docs, err := parser.ParseYAML(input)
	if err != nil {
		t.Fatalf("ParseYAML returned error: %v", err)
	}

	b, err := json.MarshalIndent(docs[0], "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	fmt.Println(string(b))
}

func assertDataPlaneVersions(t *testing.T, spec *yaml.Node) {
	t.Helper()
	versionsNode := findMappingValue(spec, "versions")
	if versionsNode == nil || versionsNode.ValueNode.Kind != yaml.SequenceNode {
		t.Fatal("expected spec.versions to be a sequence")
	}
	if len(versionsNode.ValueNode.Content) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versionsNode.ValueNode.Content))
	}

	v1alpha1Name := findMappingValue(versionsNode.ValueNode.Content[0], "name")
	if v1alpha1Name == nil || v1alpha1Name.ValueNode.Value != "v1alpha1" {
		t.Errorf("expected versions[0].name v1alpha1, got %v", v1alpha1Name)
	}

	schemaNode := findMappingValue(versionsNode.ValueNode.Content[0], "schema")
	if schemaNode == nil {
		t.Fatal("expected versions[0].schema to exist")
	}

	openAPIProps := findMappingValue(schemaNode.ValueNode, "openAPIV3Schema")
	if openAPIProps == nil {
		t.Fatal("expected schema.openAPIV3Schema to exist")
	}

	propsProperties := findMappingValue(openAPIProps.ValueNode, "properties")
	if propsProperties == nil {
		t.Fatal("expected openAPIV3Schema.properties to exist")
	}

	specProp := findMappingValue(propsProperties.ValueNode, "spec")
	if specProp == nil {
		t.Fatal("expected openAPIV3Schema.properties.spec to exist")
	}

	caNode := findMappingValue(specProp.ValueNode, "properties")
	if caNode == nil {
		t.Fatal("expected spec.properties to exist")
	}
	clusterAgent := findMappingValue(caNode.ValueNode, "clusterAgent")
	if clusterAgent == nil {
		t.Fatal("expected spec.properties.clusterAgent to exist")
	}
	clusterAgentDesc := findMappingValue(clusterAgent.ValueNode, "description")
	if clusterAgentDesc == nil || clusterAgentDesc.ValueNode.Value == "" {
		t.Error("expected clusterAgent to have a description")
	}
}
