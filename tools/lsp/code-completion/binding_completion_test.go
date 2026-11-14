// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codecompletion

import (
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

func TestBindingInitialReferences(t *testing.T) {
	for kind, rel := range bindingRelationships {
		text := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + kind
		got := RequiredPreviewWithIndex(text, 1, len("kind: "+kind), "  ", "", nil)
		if !strings.Contains(got, rel.pin+":") {
			t.Fatalf("%s: %q", kind, got)
		}
	}
}
func TestBindingReleaseNames(t *testing.T) {
	docs, err := parser.ParseYAML([]byte("apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentRelease\nmetadata: {name: frozen, namespace: default}\nspec: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	idx := dynamicTestIndex{[]indexer.SnapshotDocument{{Document: docs[0]}}}
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: ReleaseBinding\nmetadata: {name: binding}\nspec:\n  releaseName: "
	items, ok := bindingReleaseItems(text, 5, 16, "", idx)
	if !ok || len(items) != 1 || items[0].Label != "frozen" {
		t.Fatal(ok, items)
	}
	idx.docs = append(idx.docs, idx.docs[0])
	items, _ = bindingReleaseItems(text, 5, 16, "", idx)
	if len(items) != 0 {
		t.Fatal("ambiguous release suggested")
	}
}
