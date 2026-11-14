// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codecompletion

import (
	"math"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestValueEditRangeRejectsInvalidLines(t *testing.T) {
	const text = "name: demo"
	for _, tc := range []struct {
		name string
		line int
	}{
		{"minimum_integer", math.MinInt}, {"negative", -1}, {"zero", 0}, {"beyond_document", 2}, {"maximum_integer", math.MaxInt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := valueEditRange(completionContext{text: text, line: tc.line}); ok {
				t.Fatalf("invalid line accepted: %+v", got)
			}
		})
	}
	got, ok := valueEditRange(completionContext{text: text, line: 1})
	want := protocol.Range{Start: protocol.Position{Character: 6}, End: protocol.Position{Character: 10}}
	if !ok || got != want {
		t.Fatalf("valid edit=%+v, ok=%v, want %+v", got, ok, want)
	}
}

func TestIntegerSafetyCompletionUTF16Ranges(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		start, end uint32
	}{
		{"emoji_value", "name: 😀", 6, 8},
		{"emoji_key", "😀name: value", 8, 13},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := valueEditRange(completionContext{text: tc.text, line: 1})
			if !ok || got.Start.Character != tc.start || got.End.Character != tc.end {
				t.Fatalf("range=%+v, want %d:%d", got, tc.start, tc.end)
			}
		})
	}
	source := "apiVersion: openchoreo.dev/v1alpha1\nkind: 😀"
	items, ok := rootKindChoices(source, 2, 9)
	if !ok || len(items) == 0 {
		t.Fatal("missing root kind choices")
	}
	for _, item := range items {
		edit, ok := item.TextEdit.(*protocol.TextEdit)
		if !ok {
			t.Fatalf("unexpected text edit type %T", item.TextEdit)
		}
		if edit.Range.Start.Character != 6 || edit.Range.End.Character != 8 {
			t.Fatalf("kind range=%+v", edit.Range)
		}
	}
}

func TestIntegerSafetyKindCursorAfterEmoji(t *testing.T) {
	const source = "apiVersion: openchoreo.dev/v1alpha1\nkind: 😀 # note"
	items, handled := rootKindChoices(source, 2, 11)
	if !handled || len(items) != 0 {
		t.Fatalf("cursor in comment offered kind choices: %v", items)
	}
	for _, column := range []int{math.MinInt, -1, 0, math.MaxInt} {
		if items, ok := rootKindChoices(source, 2, column); ok || len(items) != 0 {
			t.Fatalf("invalid cursor %d accepted", column)
		}
	}
}
