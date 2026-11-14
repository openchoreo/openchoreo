// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codedefinition

import "testing"

type noLookupIndexer struct{ KindIndexer }

// A template-local name must never be looked up in the workspace.
func TestTemplateLocalName(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType\nmetadata:\n  name: demo\nspec:\n  resources:\n    - id: deployment\n      template:\n        spec:\n          containers:\n            - name: main\n"
	if got := Handle(text, 11, 21, "file:///demo.yaml", noLookupIndexer{}); len(got) != 0 {
		t.Fatalf("local container name resolved: %v", got)
	}
}
