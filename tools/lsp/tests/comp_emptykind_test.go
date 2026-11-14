// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"

	codecompletion "github.com/openchoreo/openchoreo/tools/lsp/code-completion"
)

func completionLabelsOf(anyResult any) map[string]bool {
	labels := make(map[string]bool)
	if list, ok := anyResult.(protocol.CompletionList); ok {
		for _, it := range list.Items {
			labels[it.Label] = true
		}
	}
	return labels
}

// After typing "kind: " the cursor is positioned on the kind-value line, so we
// must suggest valid resource kind names, NOT the root-level fields.
func TestEmptyKindReturnsKindNames(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: \n"
	res, err := codecompletion.Handle(text, 2, 7)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	labels := completionLabelsOf(res)
	if len(labels) == 0 {
		t.Fatal("expected kind-name completions, got none")
	}

	// Must include the resource kind names…
	if !labels["Project"] || !labels["Component"] {
		t.Errorf("expected kind names in completions, got %v", labels)
	}

	// …and must NOT include the root-level fields (those belong on the next line).
	for _, field := range []string{"apiVersion", "kind", "metadata", "spec", "status"} {
		if labels[field] {
			t.Errorf("root-level field %q should not be suggested on the kind-value line; got %v", field, labels)
		}
	}
}

// After typing only "apiVersion: ..." and pressing Enter (blank line), the
// root-level fields must be suggested (kind first).
func TestApiVersionBlankLineSuggestsRootFields(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\n"
	res, err := codecompletion.Handle(text, 2, 1)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	labels := completionLabelsOf(res)
	for _, field := range []string{"kind", "metadata", "spec", "status"} {
		if !labels[field] {
			t.Errorf("expected root field %q to be suggested, got %v", field, labels)
		}
	}
	if labels["apiVersion"] {
		t.Errorf("apiVersion is already present and should not be re-suggested, got %v", labels)
	}
}
