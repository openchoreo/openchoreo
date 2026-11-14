// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codeaction

import (
	"math"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestIntegerSafetyScalarRange(t *testing.T) {
	got := scalarRange(&yaml.Node{Line: math.MinInt, Column: math.MaxInt, Value: "kind"})
	if got.Start.Line != 0 || got.End.Character < got.Start.Character || got.End.Character > math.MaxInt32 {
		t.Fatalf("scalar arithmetic wrapped: %+v", got)
	}
}
func TestIntegerSafetyScalarUnicodeWidth(t *testing.T) {
	got := scalarRange(&yaml.Node{Line: 3, Column: 5, Value: "a😀b"})
	if got.End.Character != 8 {
		t.Fatalf("UTF-16 end character=%d, want 8", got.End.Character)
	}
}

func TestIntegerSafetyUTF16TokenPositions(t *testing.T) {
	for _, tc := range []struct {
		name, line, value string
		column            int
		start, end        uint32
	}{
		{"emoji_value", "name: a😀b", "a😀b", 7, 6, 10},
		{"emoji_prefix", "😀 name: old", "old", 9, 9, 12},
		{"bmp_value", "name: añb", "añb", 7, 6, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := &yaml.Node{Line: 1, Column: tc.column, Value: tc.value}
			scalar := scalarRange(node, []string{tc.line})
			edit := replaceTokenAt([]string{tc.line}, node, "new")
			if scalar.Start.Character != tc.start || scalar.End.Character != tc.end {
				t.Fatalf("scalar range=%+v, want %d:%d", scalar, tc.start, tc.end)
			}
			if edit == nil || edit.Range.Start.Character != tc.start || edit.Range.End.Character != tc.end {
				t.Fatalf("token edit=%+v, want %d:%d", edit, tc.start, tc.end)
			}
		})
	}
}

func TestIntegerSafetyParsedYAMLUTF16Range(t *testing.T) {
	const source = "😀 name: a😀b"
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(source), &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Content) != 1 || len(document.Content[0].Content) != 2 {
		t.Fatalf("unexpected parsed YAML: %+v", document)
	}
	node := document.Content[0].Content[1]
	scalar := scalarRange(node, []string{source})
	edit := replaceTokenAt([]string{source}, node, "replacement")
	if scalar.Start.Character != 9 || scalar.End.Character != 13 {
		t.Fatalf("parsed scalar range=%+v", scalar)
	}
	if edit == nil || edit.Range.Start.Character != 9 || edit.Range.End.Character != 13 {
		t.Fatalf("parsed token edit=%+v", edit)
	}
}
