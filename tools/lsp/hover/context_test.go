// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hover

import (
	"bytes"
	"os"
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lsp/hover/internal/catalog"
)

func TestCatalogReproducible(t *testing.T) {
	data, err := catalog.Generate("../../../config/crd/bases")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile("field_descriptions.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(data), bytes.TrimSpace(stored)) {
		t.Fatal("regenerate hover catalog")
	}
}
func TestContextDescriptions(t *testing.T) {
	source := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: app\nspec:\n  componentType:\n    kind: ComponentType\n    name: deployment/web-application\n  traits:\n    - name: autoscaling\n"
	for _, tc := range []struct {
		line, col int
		want      string
	}{{8, 15, "component type"}, {10, 15, "trait"}, {4, 10, "Name of this resource"}} {
		h := Handle(source, tc.line, tc.col)
		if h == nil {
			t.Fatalf("missing hover at %d", tc.line)
		}
		text := h.Contents.(protocol.MarkupContent).Value
		if !strings.Contains(strings.ToLower(text), strings.ToLower(tc.want)) || strings.Contains(text, "allowedWorkflows") {
			t.Fatalf("wrong contextual hover at %d: %s", tc.line, text)
		}
	}
	if got := contextDescription("name", "spec.unknown.name", "unknown/v1", "Unknown"); got != "" {
		t.Fatal(got)
	}
}

func TestHoverSelectsLaterDocument(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Environment\nmetadata: {name: dev}\n---\napiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata: {name: app}\nspec:\n  componentType:\n    kind: ComponentType\n    name: deployment/web-app\n"
	for _, col := range []int{5, 15} {
		h := Handle(text, 11, col)
		if h == nil {
			t.Fatal("missing later-document hover")
		}
		value := h.Contents.(protocol.MarkupContent).Value
		if !strings.Contains(value, "component type reference") {
			t.Fatal(value)
		}
	}
}
