// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"strings"
	"testing"
)

func TestResolveRefs_SimpleRef(t *testing.T) {
	schema := map[string]any{
		"$defs": map[string]any{
			"Foo": map[string]any{"type": "string"},
		},
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"$ref": "#/$defs/Foo"},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	props := result["properties"].(map[string]any)
	name := props["name"].(map[string]any)
	if name["type"] != "string" {
		t.Fatalf("expected type=string, got %v", name["type"])
	}

	// $defs should be removed
	if _, ok := result["$defs"]; ok {
		t.Fatal("$defs should be removed from output")
	}
}

func TestResolveRefs_NestedRefChain(t *testing.T) {
	schema := map[string]any{
		"$defs": map[string]any{
			"A": map[string]any{"$ref": "#/$defs/B"},
			"B": map[string]any{"$ref": "#/$defs/C"},
			"C": map[string]any{"type": "integer"},
		},
		"type": "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "#/$defs/A"},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	props := result["properties"].(map[string]any)
	val := props["val"].(map[string]any)
	if val["type"] != "integer" {
		t.Fatalf("expected type=integer after chain resolution, got %v", val["type"])
	}
}

func TestResolveRefs_RefWithSiblings(t *testing.T) {
	schema := map[string]any{
		"$defs": map[string]any{
			"Foo": map[string]any{"type": "string", "minLength": 1},
		},
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"$ref":    "#/$defs/Foo",
				"default": "bar",
			},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	props := result["properties"].(map[string]any)
	name := props["name"].(map[string]any)
	if name["type"] != "string" {
		t.Fatalf("expected type=string, got %v", name["type"])
	}
	if name["default"] != "bar" {
		t.Fatalf("expected default=bar, got %v", name["default"])
	}
}

func TestResolveRefs_CircularRef(t *testing.T) {
	schema := map[string]any{
		"$defs": map[string]any{
			"A": map[string]any{"$ref": "#/$defs/B"},
			"B": map[string]any{"$ref": "#/$defs/A"},
		},
		"type": "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "#/$defs/A"},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for circular ref")
	}
	if !strings.Contains(err.Error(), "circular $ref") {
		t.Fatalf("expected circular ref error, got: %v", err)
	}
}

func TestResolveRefs_MissingRef(t *testing.T) {
	schema := map[string]any{
		"$defs": map[string]any{},
		"type":  "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "#/$defs/Missing"},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for missing ref")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got: %v", err)
	}
}

func TestResolveRefs_RemoteRef(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "http://example.com/schema"},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for remote ref")
	}
	if !strings.Contains(err.Error(), "only local $ref supported") {
		t.Fatalf("expected remote ref error, got: %v", err)
	}
}

func TestResolveRefs_RefInItems(t *testing.T) {
	schema := map[string]any{
		"$defs": map[string]any{
			"Item": map[string]any{"type": "string"},
		},
		"type": "object",
		"properties": map[string]any{
			"list": map[string]any{
				"type":  "array",
				"items": map[string]any{"$ref": "#/$defs/Item"},
			},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	props := result["properties"].(map[string]any)
	list := props["list"].(map[string]any)
	items := list["items"].(map[string]any)
	if items["type"] != "string" {
		t.Fatalf("expected items type=string, got %v", items["type"])
	}
}

func TestResolveRefs_RefInAllOf(t *testing.T) {
	schema := map[string]any{
		"$defs": map[string]any{
			"Base": map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}},
		},
		"allOf": []any{
			map[string]any{"$ref": "#/$defs/Base"},
			map[string]any{"type": "object", "properties": map[string]any{"age": map[string]any{"type": "integer"}}},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	allOf := result["allOf"].([]any)
	first := allOf[0].(map[string]any)
	if first["type"] != "object" {
		t.Fatalf("expected first allOf element to have type=object, got %v", first["type"])
	}
}

func TestResolveRefs_NoRefsPresent(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	props := result["properties"].(map[string]any)
	name := props["name"].(map[string]any)
	if name["type"] != "string" {
		t.Fatalf("expected type=string, got %v", name["type"])
	}
}

func TestResolveRefs_BackwardCompatDefinitions(t *testing.T) {
	schema := map[string]any{
		"definitions": map[string]any{
			"Foo": map[string]any{"type": "boolean"},
		},
		"type": "object",
		"properties": map[string]any{
			"flag": map[string]any{"$ref": "#/definitions/Foo"},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	props := result["properties"].(map[string]any)
	flag := props["flag"].(map[string]any)
	if flag["type"] != "boolean" {
		t.Fatalf("expected type=boolean, got %v", flag["type"])
	}

	if _, ok := result["definitions"]; ok {
		t.Fatal("definitions should be removed from output")
	}
}

func TestResolveRefs_NilInput(t *testing.T) {
	result, err := ResolveRefs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestResolveRefs_DepthLimitExceeded(t *testing.T) {
	// Build a deeply nested chain of refs that exceeds maxRefDepth (64).
	defs := map[string]any{}
	for i := 0; i < 70; i++ {
		name := fmt.Sprintf("D%d", i)
		if i < 69 {
			defs[name] = map[string]any{"$ref": fmt.Sprintf("#/$defs/D%d", i+1)}
		} else {
			defs[name] = map[string]any{"type": "string"}
		}
	}
	schema := map[string]any{
		"$defs": defs,
		"type":  "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "#/$defs/D0"},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for depth limit exceeded")
	}
	if !strings.Contains(err.Error(), "maximum depth") {
		t.Fatalf("expected depth limit error, got: %v", err)
	}
}

func TestResolveRefs_RefInAdditionalProperties(t *testing.T) {
	schema := map[string]any{
		"$defs": map[string]any{
			"Value": map[string]any{"type": "string"},
		},
		"type": "object",
		"properties": map[string]any{
			"labels": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"$ref": "#/$defs/Value"},
			},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	props := result["properties"].(map[string]any)
	labels := props["labels"].(map[string]any)
	ap := labels["additionalProperties"].(map[string]any)
	if ap["type"] != "string" {
		t.Fatalf("expected additionalProperties type=string, got %v", ap["type"])
	}
}

func TestResolveRefs_RefInOneOf(t *testing.T) {
	schema := map[string]any{
		"$defs": map[string]any{
			"StringVal": map[string]any{"type": "string"},
			"IntVal":    map[string]any{"type": "integer"},
		},
		"oneOf": []any{
			map[string]any{"$ref": "#/$defs/StringVal"},
			map[string]any{"$ref": "#/$defs/IntVal"},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	oneOf := result["oneOf"].([]any)
	first := oneOf[0].(map[string]any)
	second := oneOf[1].(map[string]any)
	if first["type"] != "string" {
		t.Fatalf("expected first oneOf type=string, got %v", first["type"])
	}
	if second["type"] != "integer" {
		t.Fatalf("expected second oneOf type=integer, got %v", second["type"])
	}
}

func TestResolveRefs_DoesNotMutateInput(t *testing.T) {
	original := map[string]any{
		"$defs": map[string]any{
			"Foo": map[string]any{"type": "string"},
		},
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"$ref": "#/$defs/Foo"},
		},
	}

	_, err := ResolveRefs(original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Original should still have $ref
	props := original["properties"].(map[string]any)
	name := props["name"].(map[string]any)
	if _, ok := name["$ref"]; !ok {
		t.Fatal("original input was mutated: $ref should still be present")
	}
}
