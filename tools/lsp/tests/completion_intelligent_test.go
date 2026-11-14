// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"

	codecompletion "github.com/openchoreo/openchoreo/tools/lsp/code-completion"
)

type fakeResourceIndexer struct {
	byKind map[string][]string
}

func (f fakeResourceIndexer) NamesByKind() map[string][]string {
	return f.byKind
}

func itemsOf(res any) []protocol.CompletionItem {
	if list, ok := res.(protocol.CompletionList); ok {
		return list.Items
	}
	return nil
}

// A cursor in the value slot of an enum field must suggest the enum values,
// each replacing the value region via a TextEdit (not just appending text).
func TestValueSlotEnumCompletion(t *testing.T) {
	// `effect: |` — block scalar indicator typed on the value line of an enum.
	text := "apiVersion: openchoreo.dev/v1alpha1\n" +
		"kind: AuthzRoleBinding\n" +
		"spec:\n" +
		"  effect: |\n" // cursor after the `|` (1-based line 4, col 11)
	res, err := codecompletion.Handle(text, 4, 11)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	items := itemsOf(res)
	byLabel := make(map[string]protocol.CompletionItem)
	for _, it := range items {
		byLabel[it.Label] = it
	}
	for _, want := range []string{"allow", "deny"} {
		it, ok := byLabel[want]
		if !ok {
			t.Errorf("missing enum completion %q; got %v", want, items)
			continue
		}
		if it.TextEdit == nil {
			t.Errorf("enum %q should use a TextEdit that replaces the value slot", want)
			continue
		}
		edit, ok := it.TextEdit.(*protocol.TextEdit)
		if !ok {
			t.Errorf("enum %q: TextEdit has unexpected type %T", want, it.TextEdit)
			continue
		}
		if edit.NewText != want {
			t.Errorf("enum %q: TextEdit.NewText = %q, want %q", want, edit.NewText, want)
		}
		// 0-based line 3, from just past `effect: ` (char 10) to end of line.
		start := edit.Range.Start
		end := edit.Range.End
		if start.Line != 3 || start.Character != 10 || end.Line != 3 || end.Character != 11 {
			t.Errorf("enum %q: unexpected TextEdit range %+v → %+v", want, start, end)
		}
	}

	// The schema default "allow" is also the enum value — it must not be a
	// duplicate item.
	if n := len(items); n != 2 {
		t.Errorf("expected exactly 2 enum items, got %d (%v)", n, items)
	}
}

// A value slot for a leaf with no enum and no default must NOT be filled with
// an invented value: the old engine fabricated `name: default`/timestamps; the
// schema-driven engine returns nothing.
func TestNoInventedLeafValue(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\n" +
		"kind: Component\n" +
		"spec:\n" +
		"  autoDeploy: \n" // boolean leaf, no enum, no default (1-based line 4)
	res, err := codecompletion.Handle(text, 4, 14)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if items := itemsOf(res); len(items) != 0 {
		t.Errorf("expected no invented value completions, got %v", items)
	}
}

// A leaf with a real schema default does surface that default as the value.
func TestRealDefaultValueCompletion(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\n" +
		"kind: AuthzRoleBinding\n" +
		"spec:\n" +
		"  effect: \n" // spec.effect has marker-backed default "allow" + enum
	res, err := codecompletion.Handle(text, 4, 11)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	found := false
	for _, it := range itemsOf(res) {
		if it.Label == "allow" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the schema default %q among value completions, got %v", "allow", itemsOf(res))
	}
}

// An empty `- ` array item line completes the fields of the array's element
// schema (shallow `key:` inserts, no deep expansion).
func TestEmptyArrayItemFieldCompletion(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\n" +
		"kind: AuthzRoleBinding\n" +
		"spec:\n" +
		"  roleMappings:\n" +
		"    - \n" // 1-based line 5, cursor on the dash item line
	res, err := codecompletion.Handle(text, 5, 6)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	byLabel := make(map[string]protocol.CompletionItem)
	for _, it := range itemsOf(res) {
		byLabel[it.Label] = it
	}
	for _, want := range []string{"roleRef", "conditions", "scope"} {
		if _, ok := byLabel[want]; !ok {
			t.Errorf("missing array-item field %q; got %v", want, itemsOf(res))
		}
	}
	// Object fields on a dash line are shallow `key:` (no expanded children).
	it := byLabel["roleRef"]
	insert := ""
	if it.InsertText != nil {
		insert = *it.InsertText
	}
	if insert != "roleRef:" {
		t.Errorf("array-item object field roleRef: InsertText %q, want %q", insert, "roleRef:")
	}
}

// A `name:` value under an object carrying a sibling `kind:` is a resource
// reference: names from the workspace index filtered by that kind.
func TestReferenceNameCompletionWithIndex(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\n" +
		"kind: AuthzRoleBinding\n" +
		"spec:\n" +
		"  roleMappings:\n" +
		"    - roleRef:\n" +
		"        kind: AuthzRole\n" +
		"        name: \n" // 1-based line 7, value slot of name
	idx := fakeResourceIndexer{byKind: map[string][]string{
		"AuthzRole":        {"role-a", "role-b"},
		"ClusterAuthzRole": {"cluster-role-a"},
	}}
	res, err := codecompletion.HandleWithIndex(text, 7, 15, idx)
	if err != nil {
		t.Fatalf("HandleWithIndex error: %v", err)
	}

	byLabel := make(map[string]protocol.CompletionItem)
	for _, it := range itemsOf(res) {
		byLabel[it.Label] = it
	}
	for _, want := range []string{"role-a", "role-b"} {
		if _, ok := byLabel[want]; !ok {
			t.Errorf("missing reference name %q; got %v", want, itemsOf(res))
		}
	}
	// Names of other kinds must NOT leak into a kind-filtered reference.
	if _, ok := byLabel["cluster-role-a"]; ok {
		t.Errorf("name of unrelated kind %q must not be suggested", "cluster-role-a")
	}
	// The index's own copy (when the resource is self-referencing) can appear,
	// but every item is a real resource name from the index.
	for label := range byLabel {
		switch label {
		case "role-a", "role-b":
		default:
			t.Errorf("unexpected completion %q (not an AuthzRole name)", label)
		}
	}
}

// HandleWithIndex with a nil indexer behaves exactly like Handle — no
// reference names, no panic.
func TestHandleWithIndexNilIndexer(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\n" +
		"kind: AuthzRoleBinding\n" +
		"spec:\n" +
		"  roleMappings:\n" +
		"    - roleRef:\n" +
		"        kind: AuthzRole\n" +
		"        name: \n"
	withNil, err := codecompletion.HandleWithIndex(text, 7, 15, nil)
	if err != nil {
		t.Fatalf("HandleWithIndex(nil) error: %v", err)
	}
	res, err := codecompletion.Handle(text, 7, 15)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if got, want := len(itemsOf(withNil)), len(itemsOf(res)); got != want {
		t.Errorf("HandleWithIndex(nil) != Handle: %d vs %d items", got, want)
	}
	// No workspace index must mean no reference-name completions (kind "resource
	// name" detail), only schema-driven enum/default items.
	for _, it := range itemsOf(withNil) {
		if it.Detail != nil && *it.Detail == "resource name" {
			t.Errorf("nil indexer must not suggest resource names, got %q", it.Label)
		}
	}
}
