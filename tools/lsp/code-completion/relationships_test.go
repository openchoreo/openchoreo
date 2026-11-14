// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codecompletion

import (
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

const completionSection = `    openAPIV3Schema:
      type: object
      properties:
        setting: {type: string, default: configured}
`

func TestSchemaBackedRelationships(t *testing.T) {
	for _, tc := range []struct{ kind, ref, target, values string }{
		{"Resource", "type", "ResourceType", "parameters"},
		{"Project", "type", "ProjectType", "parameters"},
		{"WorkflowRun", "workflow", "ClusterWorkflow", "workflow.parameters"},
		{"Component", "workflow", "ClusterWorkflow", "workflow.parameters"},
	} {
		definition := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + tc.target + "\nmetadata: {name: definition}\nspec:\n  parameters:\n" + completionSection
		docs, err := parser.ParseYAML([]byte(definition))
		if err != nil {
			t.Fatal(err)
		}
		idx := dynamicTestIndex{[]indexer.SnapshotDocument{{Document: docs[0]}}}
		body := "  " + tc.ref + ": {kind: " + tc.target + ", name: definition}\n  parameters:\n    "
		if tc.ref == "workflow" {
			body = "  workflow:\n    kind: ClusterWorkflow\n    name: definition\n    parameters:\n      "
		}
		text := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + tc.kind + "\nmetadata: {name: instance}\nspec:\n" + body
		assertDynamicRelationship(t, text, idx)
	}
	for _, tc := range []struct{ kind, definition, values, binding, pin, configs string }{
		{testValueComponentRelease, "componentType", "componentProfile.parameters", "ReleaseBinding", "releaseName", "componentTypeEnvironmentConfigs"},
		{"ResourceRelease", "resourceType", "parameters", "ResourceReleaseBinding", "resourceRelease", "resourceTypeEnvironmentConfigs"},
		{"ProjectRelease", "projectType", "parameters", "ProjectReleaseBinding", "projectRelease", "environmentConfigs"},
	} {
		// Frozen schemas are deliberately different from any live definitions.
		spec := "  " + tc.definition + ":\n    spec:\n      parameters:\n" + indentSection(4) + "      environmentConfigs:\n" + indentSection(4)
		body := "  parameters:\n    "
		if tc.kind == testValueComponentRelease {
			body = "  componentProfile:\n    parameters:\n      "
		}
		header := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + tc.kind + "\nmetadata: {name: frozen}\nspec:\n"
		assertDynamicRelationship(t, header+spec+body, dynamicTestIndex{})
		docs, err := parser.ParseYAML([]byte(header + spec))
		if err != nil {
			t.Fatal(err)
		}
		idx := dynamicTestIndex{[]indexer.SnapshotDocument{{Document: docs[0]}}}
		text := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + tc.binding + "\nmetadata: {name: binding}\nspec:\n  " + tc.pin + ": frozen\n  " + tc.configs + ":\n    "
		assertDynamicRelationship(t, text, idx)
	}
}
func indentSection(n int) string {
	s := completionSection

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = strings.Repeat(" ", n) + line
		}
	}
	return strings.Join(lines, "\n")
}
func assertDynamicRelationship(t *testing.T, text string, idx dynamicTestIndex) {
	t.Helper()
	lines := strings.Split(text, "\n")
	line, col := len(lines), len(lines[len(lines)-1])
	items, ok := dynamicItems(text, line, col+1, idx, "")
	if !ok || len(items) != 1 || items[0].Label != "setting" {
		t.Fatalf("%s\n%v %+v", text, ok, items)
	}
	preview := RequiredPreviewWithIndex(text, line-1, col, "  ", "", idx)
	if !strings.Contains(preview, "setting: configured") {
		t.Fatalf("missing ghost text for %s: %q", text, preview)
	}
}

func TestTraitBindingRelationships(t *testing.T) {
	definition := "apiVersion: openchoreo.dev/v1alpha1\nkind: Trait\nmetadata: {name: storage}\nspec:\n  parameters:\n" + completionSection + "  environmentConfigs:\n" + completionSection
	docs, err := parser.ParseYAML([]byte(definition))
	if err != nil {
		t.Fatal(err)
	}
	idx := dynamicTestIndex{[]indexer.SnapshotDocument{{Document: docs[0]}}}
	for _, kind := range []string{"ComponentType", "ClusterComponentType"} {
		for _, section := range []string{"parameters", "environmentConfigs"} {
			text := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + kind + "\nmetadata: {name: type}\nspec:\n  traits:\n    - kind: Trait\n      name: storage\n      instanceName: volume\n      " + section + ":\n        "
			assertDynamicRelationship(t, text, idx)
		}
	}
}

func TestFrozenTraitParametersAndConfigs(t *testing.T) {
	frozen := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentRelease
metadata: {name: frozen}
spec:
  traits:
    - kind: Trait
      name: storage
      spec:
        parameters:
` + indentSection(6) + "        environmentConfigs:\n" + indentSection(6) + `  componentProfile:
    traits:
      - kind: Trait
        name: storage
        instanceName: volume
        parameters:
          `
	assertDynamicRelationship(t, frozen, dynamicTestIndex{})
	docs, err := parser.ParseYAML([]byte(frozen))
	if err != nil {
		t.Fatal(err)
	}
	idx := dynamicTestIndex{[]indexer.SnapshotDocument{{Document: docs[0]}}}
	binding := "apiVersion: openchoreo.dev/v1alpha1\nkind: ReleaseBinding\nmetadata: {name: binding}\nspec:\n  releaseName: frozen\n  traitEnvironmentConfigs:\n    volume:\n      "
	assertDynamicRelationship(t, binding, idx)
	rootBinding := strings.Replace(binding, "    volume:\n      ", "    ", 1)
	lines := strings.Split(rootBinding, "\n")
	items, ok := dynamicItems(rootBinding, len(lines), 5, idx, "")
	if !ok || len(items) != 1 || items[0].Label != "volume" {
		t.Fatalf("trait map: %v %+v", ok, items)
	}
}

func TestUnknownFlexibleFieldHasNoDynamicSchema(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: app\n  labels:\n    "
	if _, ok := dynamicItems(text, 7, 5, dynamicTestIndex{}, ""); ok {
		t.Fatal("invented schema for labels")
	}
}

func TestStoragePreviewHasOnlyDeclaredFields(t *testing.T) {
	source := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentRelease
metadata: {name: frozen}
spec:
  componentProfile:
    traits:
      - kind: Trait
        name: storage
        instanceName: data-storage
  traits:
    - kind: Trait
      name: storage
      spec:
        environmentConfigs:
          openAPIV3Schema:
            type: object
            properties:
              size: {type: string, default: 1Gi}
              storageClass: {type: string, default: local-path}
`
	docs, err := parser.ParseYAML([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	idx := dynamicTestIndex{[]indexer.SnapshotDocument{{Document: docs[0]}}}
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: ReleaseBinding\nmetadata: {name: binding}\nspec:\n  releaseName: frozen\n  traitEnvironmentConfigs:\n    data-storage:\n      "
	got := RequiredPreviewWithIndex(text, 7, 6, "  ", "", idx)
	if got != "size: 1Gi\n      storageClass: local-path" {
		t.Fatalf("unexpected fields: %q", got)
	}
}
