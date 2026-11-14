// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"strings"
	"testing"
)

func TestResolveRefsDepthBoundary(t *testing.T) {
	for _, count := range []int{1, maxRefDepth - 1, maxRefDepth, maxRefDepth + 1} {
		t.Run(fmt.Sprintf("references_%d", count), func(t *testing.T) {
			defs := make(map[string]any)
			for i := range count {
				definition := map[string]any{"type": "string"}
				if i+1 < count {
					definition = map[string]any{"$ref": fmt.Sprintf("#/$defs/D%d", i+1)}
				}
				defs[fmt.Sprintf("D%d", i)] = definition
			}
			input := map[string]any{"$defs": defs, "$ref": "#/$defs/D0"}
			result, err := ResolveRefs(input)
			if count > maxRefDepth {
				if err == nil || !strings.Contains(err.Error(), "maximum depth") {
					t.Fatalf("expected depth error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result["type"] != "string" {
				t.Fatalf("unexpected resolution: %v", result)
			}
			if _, exists := result["$defs"]; exists {
				t.Fatal("definitions retained in result")
			}
			if input["$ref"] != "#/$defs/D0" {
				t.Fatal("input mutated")
			}
		})
	}
}

func TestResolveRefAllocationBounds(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, tc := range []struct {
		name         string
		stack, depth int
		wantError    bool
	}{
		{"last_stack_entry", maxRefDepth - 1, 0, false},
		{"full_stack", maxRefDepth, 0, true},
		{"oversized_stack", maxRefDepth + 1, 0, true},
		{"last_depth", 0, maxRefDepth - 1, false},
		{"full_depth", 0, maxRefDepth, true},
		{"maximum_integer_depth", 0, maxInt, true},
		{"negative_depth", 0, -1, true},
		{"minimum_integer_depth", 0, -maxInt - 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defs := map[string]any{"Leaf": map[string]any{"type": "string"}}
			result, err := resolveRef("#/$defs/Leaf", nil, defs, make([]string, tc.stack), tc.depth)
			if tc.wantError {
				if err == nil || !strings.Contains(err.Error(), "maximum depth") {
					t.Fatalf("expected depth error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.(map[string]any)["type"] != "string" {
				t.Fatalf("unexpected result: %v", result)
			}
		})
	}
}
