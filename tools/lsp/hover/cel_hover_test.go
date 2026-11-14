// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hover

import (
	"strings"
	"testing"
	"unicode/utf16"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestCELIdentifierHover(t *testing.T) {
	for _, tc := range []struct{ value, token, want string }{
		{"'${metadata.podSelectors}'", "podSelectors", "platform-injected selectors"},
		{"'${metadata.podSelectores}'", "podSelectores", "Did you mean `podSelectors`"},
		{"'${parameters.retries}'", "retries", "Retry attempts for this service"},
		{"'${workload.container.image}'", "image", "string"},
		{"|\n          ${metadata.podSelectors}", "podSelectors", "map(string, string)"},
		{"'🌟 ${metadata.podSelectors}'", "podSelectors", "platform-injected selectors"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			source := "apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType\nmetadata: {name: demo}\nspec:\n  parameters:\n    openAPIV3Schema:\n      type: object\n      properties:\n        retries: {type: integer, description: Retry attempts for this service}\n  resources:\n    - id: demo\n      template:\n        value: " + tc.value + "\n"
			for _, crlf := range []bool{false, true} {
				text := source
				if crlf {
					text = strings.ReplaceAll(text, "\n", "\r\n")
				}
				line, column := 0, 0
				for i, value := range strings.Split(text, "\n") {
					if at := strings.Index(value, tc.token); at >= 0 {
						line = i + 1
						column = len(utf16.Encode([]rune(value[:at]))) + 2
					}
				}
				h := Handle(text, line, column)
				if h == nil {
					t.Fatalf("missing CEL hover: %s", text)
				}
				value := h.Contents.(protocol.MarkupContent).Value
				if !strings.Contains(value, tc.want) {
					t.Fatalf("wrong CEL hover: %s", value)
				}
				if h.Range == nil || int(h.Range.Start.Line) != line-1 || int(h.Range.Start.Character) != column-2 {
					t.Fatalf("incorrect hovered token range: %+v expected %d:%d", h.Range, line, column)
				}
			}
		})
	}
}
func TestPostRenderCELHoverUsesTarget(t *testing.T) {
	source := "apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType\nmetadata: {name: demo}\nspec:\n  postRenderValidations:\n    - target: {group: apps, version: v1, kind: Deployment}\n      rule: '${resource.spec.replicas <= 2}'\n      message: demo\n"
	line := strings.Split(source, "\n")[6]
	h := Handle(source, 7, strings.Index(line, "replicas")+2)
	if h == nil || !strings.Contains(h.Contents.(protocol.MarkupContent).Value, "**Type:** `int`") {
		t.Fatalf("missing typed target hover: %+v", h)
	}
}

func TestCELHoverListsParentChildren(t *testing.T) {
	source := "apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType\nmetadata: {name: demo}\nspec:\n  resources:\n    - id: demo\n      template:\n        labels: '${metadata.podSelectors}'\n"
	line := strings.Split(source, "\n")[7]
	for _, token := range []string{"metadata", "podSelectors"} {
		h := Handle(source, 8, strings.Index(line, token)+2)
		if h == nil {
			t.Fatal("missing parent child hover")
		}
		value := h.Contents.(protocol.MarkupContent).Value
		for _, child := range []string{"metadata.name", "metadata.namespace", "metadata.podSelectors"} {
			if !strings.Contains(value, "`"+child+"`") {
				t.Fatalf("missing child %s: %s", child, value)
			}
		}
	}
}
