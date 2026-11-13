package codecompletion

import (
	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"strings"
	"testing"
)

type dynamicTestIndex struct{ docs []indexer.SnapshotDocument }

func (d dynamicTestIndex) NamesByKind() map[string][]string           { return nil }
func (d dynamicTestIndex) Snapshot(string) []indexer.SnapshotDocument { return d.docs }
func TestDynamicParameterCompletion(t *testing.T) {
	defs, err := parser.ParseYAML([]byte(`apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata: {name: database, namespace: default}
spec:
  workloadType: deployment
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        port: {type: integer, default: 5432}
`))
	if err != nil {
		t.Fatal(err)
	}
	idx := dynamicTestIndex{[]indexer.SnapshotDocument{{Document: defs[0]}}}
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata: {name: app}\nspec:\n  componentType: {kind: ComponentType, name: deployment/database}\n  parameters:\n    "
	items, ok := dynamicItems(text, 7, 5, idx, "")
	if !ok {
		t.Fatal("not resolved")
	}
	found := false
	for _, item := range items {
		if strings.Contains(item.Label, "port") {
			found = true
		}
	}
	if !found {
		t.Fatal(items)
	}
	idx.docs = append(idx.docs, idx.docs[0])
	if _, ok := dynamicItems(text, 7, 5, idx, ""); ok {
		t.Fatal("ambiguous schema selected")
	}
}

func TestDynamicTraitAndNamespace(t *testing.T) {
	for _, kind := range []string{"Trait", "ClusterTrait"} {
		docs, err := parser.ParseYAML([]byte("apiVersion: openchoreo.dev/v1alpha1\nkind: " + kind + "\nmetadata: {name: storage, namespace: other}\nspec:\n  parameters:\n    openAPIV3Schema:\n      type: object\n      properties:\n        mountPath: {type: string, description: Mount directory}\n"))
		if err != nil {
			t.Fatal(err)
		}
		idx := dynamicTestIndex{[]indexer.SnapshotDocument{{Document: docs[0]}}}
		text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata: {name: app}\nspec:\n  traits:\n    - kind: " + kind + "\n      name: storage\n      parameters:\n        "
		items, ok := dynamicItems(text, 9, 9, idx, "")
		if kind == "Trait" {
			if ok {
				t.Fatal("namespace leak")
			}
			continue
		}
		if !ok || len(items) != 1 || items[0].Label != "mountPath" || items[0].Documentation != "Mount directory" {
			t.Fatalf("%v %+v", ok, items)
		}
		text = "apiVersion: v1\nkind: ConfigMap\n---\n" + text
		if items, ok := dynamicItems(text, 12, 9, idx, ""); !ok || len(items) != 1 {
			t.Fatalf("multi-document: %v %v", ok, items)
		}
	}
}

func TestDynamicNestedArrayAndValues(t *testing.T) {
	definition := `apiVersion: openchoreo.dev/v1alpha1
kind: ClusterComponentType
metadata: {name: database}
spec:
  workloadType: deployment
  parameters:
    openAPIV3Schema:
      type: object
      $defs:
        Settings:
          type: object
          properties:
            mode: {type: string, enum: [fast, safe], default: safe}
      properties:
        tuning: {$ref: '#/$defs/Settings'}
        workers:
          type: array
          items: {$ref: '#/$defs/Settings'}
`
	docs, err := parser.ParseYAML([]byte(definition))
	if err != nil {
		t.Fatal(err)
	}
	idx := dynamicTestIndex{[]indexer.SnapshotDocument{{Document: docs[0]}}}
	header := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata: {name: app}\nspec:\n  componentType: {kind: ClusterComponentType, name: deployment/database}\n  parameters:\n"
	for _, tc := range []struct{ tail, want string }{
		{"    tuning:\n      ", "mode"},
		{"    tuning:\n      mode: ", "safe"},
		{"    workers:\n      - ", "mode"},
		{"    tuning:\n      mo", "mode"},
	} {
		text := header + tc.tail
		lines := strings.Split(text, "\n")
		items, ok := dynamicItems(text, len(lines), len(lines[len(lines)-1])+1, idx, "")
		found := false
		for _, item := range items {
			if item.Label == tc.want {
				found = true
			}
		}
		if !ok || !found {
			t.Fatalf("%q: %v %+v", tc.tail, ok, items)
		}
	}
}

func TestDynamicGhostText(t *testing.T) {
	for _, trait := range []bool{false, true} {
		kind, name, prop := "ComponentType", "database", "port: {type: integer, default: 5432}"
		if trait {
			kind, name, prop = "Trait", "storage", "mountPath: {type: string, default: /data}"
		}
		definition := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + kind + "\nmetadata: {name: " + name + "}\nspec:\n  workloadType: deployment\n  parameters:\n    openAPIV3Schema:\n      type: object\n      properties:\n        " + prop + "\n        optional: {type: string}\n"
		docs, err := parser.ParseYAML([]byte(definition))
		if err != nil {
			t.Fatal(err)
		}
		idx := dynamicTestIndex{[]indexer.SnapshotDocument{{Document: docs[0]}}}
		text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata: {name: app, namespace: default}\nspec:\n  componentType: {kind: ComponentType, name: deployment/database}\n  parameters:\n    "
		if trait {
			text = "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata: {name: app, namespace: default}\nspec:\n  traits:\n    - kind: Trait\n      name: storage\n      parameters:\n        "
		}
		lines := strings.Split(text, "\n")
		got := RequiredPreviewWithIndex(text, len(lines)-1, len(lines[len(lines)-1]), "  ", "", idx)
		want := "port: 5432"
		if trait {
			want = "mountPath: /data"
		}
		if !strings.Contains(got, want) || !strings.Contains(got, "optional:") {
			t.Fatalf("trait=%v: %q", trait, got)
		}
		if !trait {
			completed := text + "port: 5432\n    "
			lines := strings.Split(completed, "\n")
			again := RequiredPreviewWithIndex(completed, len(lines)-1, len(lines[len(lines)-1]), "  ", "", idx)
			if strings.Contains(again, "port:") {
				t.Fatalf("duplicate field: %q", again)
			}
		}

	}
}

func TestDynamicPreviewAfterMultilineReference(t *testing.T) {
	defs, err := parser.ParseYAML([]byte("apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType\nmetadata: {name: database}\nspec:\n  workloadType: deployment\n  parameters:\n    openAPIV3Schema:\n      type: object\n      properties:\n        port: {type: integer, default: 5432}\n"))
	if err != nil {
		t.Fatal(err)
	}
	idx := dynamicTestIndex{[]indexer.SnapshotDocument{{Document: defs[0]}}}
	for _, indent := range []string{"  ", "    "} {
		text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: component-name\n  namespace: default\nspec:\n  componentType:\n    kind: ComponentType\n    name: deployment/database\n  "
		got := RequiredPreviewWithIndex(text, 9, 2, indent, "", idx)
		if !strings.Contains(got, "parameters:") || !strings.Contains(got, "port: 5432") {
			t.Fatalf("indent %q: %q", indent, got)
		}
	}
}

func TestDynamicPreviewIncludesAllDeclaredFields(t *testing.T) {
	definition := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata: {name: database}
spec:
  workloadType: deployment
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        port: {type: integer, default: 5432}
        databaseName: {type: string}
        tuning:
          type: object
          properties:
            timeout: {type: integer}
        users:
          type: array
          items:
            type: object
            properties:
              username: {type: string}
`
	docs, err := parser.ParseYAML([]byte(definition))
	if err != nil {
		t.Fatal(err)
	}
	idx := dynamicTestIndex{[]indexer.SnapshotDocument{{Document: docs[0]}}}
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata: {name: demo}\nspec:\n  componentType: {kind: ComponentType, name: deployment/database}\n  parameters:\n    "
	got := RequiredPreviewWithIndex(text, 6, 4, "  ", "", idx)
	for _, want := range []string{"port: 5432", "databaseName:", "<database-name>", "tuning:", "timeout:", "users:", "- username:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s: %s", want, got)
		}
	}
	text += "databaseName: existing\n    "
	got = RequiredPreviewWithIndex(text, 7, 4, "  ", "", idx)
	if strings.Contains(got, "databaseName:") {
		t.Fatalf("duplicate existing field: %s", got)
	}
	// Completion must not mutate the linked schema or its required list.
	if n := dynamicNode(dynamicAt(docs[0].Spec, "parameters.openAPIV3Schema"), "required"); n != nil {
		t.Fatal("mutated definition")
	}
}
