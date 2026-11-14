// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codecompletion

import (
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
)

func TestRootKindDropdown(t *testing.T) {
	for _, value := range []string{"kind:", "kind: ", "kind: Com", "kind: Component", "kind: # keep comment"} {
		t.Run(value, func(t *testing.T) {
			text := "apiVersion: openchoreo.dev/v1alpha1\n" + value
			column := len(value) + 1
			if strings.Contains(value, "#") {
				column = 7
			}
			result, err := Handle(text, 2, column)
			if err != nil {
				t.Fatal(err)
			}
			list := result.(protocol.CompletionList)
			if len(list.Items) != len(template.AllKinds()) {
				t.Fatalf("expected all registered kinds, got %d", len(list.Items))
			}
			seen := map[string]bool{}
			for _, item := range list.Items {
				if seen[item.Label] || template.SchemaFor(item.Label) == nil {
					t.Fatalf("invalid choice %q", item.Label)
				}
				seen[item.Label] = true
				edit := item.TextEdit.(*protocol.TextEdit)
				if edit.Range.Start.Line != 1 || edit.Range.End.Line != 1 {
					t.Fatal(edit)
				}
				accepted := "apiVersion: openchoreo.dev/v1alpha1\n" + value[:edit.Range.Start.Character] + edit.NewText + value[edit.Range.End.Character:]
				docs, err := parser.ParseYAML([]byte(accepted))
				if err != nil || len(docs) != 1 || docs[0].Kind != item.Label {
					t.Fatalf("invalid accepted kind: %s (%v)", accepted, err)
				}
				if strings.Contains(value, "#") && !strings.Contains(accepted, "# keep comment") {
					t.Fatal("comment lost")
				}
				if strings.Contains(accepted, "\n\n") {
					t.Fatal("kind selection must not insert extra lines")
				}
			}
		})
	}
}

func TestRootKindDropdownDocumentScope(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		text := strings.Join([]string{"apiVersion: v1", "kind: ConfigMap", "---", "apiVersion: openchoreo.dev/v1alpha1", "kind: "}, eol)
		result, err := Handle(text, 5, 7)
		if err != nil {
			t.Fatal(err)
		}
		list := result.(protocol.CompletionList)
		if len(list.Items) != len(template.AllKinds()) {
			t.Fatal(list)
		}
		for _, item := range list.Items {
			if item.TextEdit.(*protocol.TextEdit).Range.Start.Line != 4 {
				t.Fatal("wrong document")
			}
		}
	}
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\n---\napiVersion: v1\nkind: "
	result, err := Handle(text, 5, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.(protocol.CompletionList).Items) != 0 {
		t.Fatal("OpenChoreo kinds leaked into unrelated document")
	}
}

func TestReferenceKindDropdownUsesSchemaEnum(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nspec:\n  componentType:\n    kind: "
	result, err := Handle(text, 5, 11)
	if err != nil {
		t.Fatal(err)
	}
	list := result.(protocol.CompletionList)
	if len(list.Items) != 2 {
		t.Fatalf("nested reference must use two schema choices, got %+v", list.Items)
	}
	for _, item := range list.Items {
		if item.Label != "ComponentType" && item.Label != "ClusterComponentType" {
			t.Fatalf("unrelated reference kind: %s", item.Label)
		}
	}
}

func TestKindSelectionContinuesToRequiredPreview(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: "
	result, err := Handle(text, 2, 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range result.(protocol.CompletionList).Items {
		if item.Label != "Component" {
			continue
		}
		edit := item.TextEdit.(*protocol.TextEdit)
		line := "kind: "
		line = line[:edit.Range.Start.Character] + edit.NewText + line[edit.Range.End.Character:]
		preview := RequiredPreview("apiVersion: openchoreo.dev/v1alpha1\n"+line, 1, len(line), "  ")
		if !strings.Contains(preview, "metadata:") || !strings.Contains(preview, "componentType:") {
			t.Fatalf("required preview did not follow selection: %q", preview)
		}
		return
	}
	t.Fatal("Component choice missing")
}
