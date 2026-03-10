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
	if name["type"] != typeString {
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
	if val["type"] != typeInteger {
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
	if name["type"] != typeString {
		t.Fatalf("expected type=string, got %v", name["type"])
	}
	if name["default"] != "bar" {
		t.Fatalf("expected default=bar, got %v", name["default"])
	}
}

func TestResolveRefs_RefWithSiblingsContainingRef(t *testing.T) {
	// Sibling keys of a $ref may themselves contain nested $ref that must be resolved.
	schema := map[string]any{
		"$defs": map[string]any{
			"Base":    map[string]any{"type": "object"},
			"Address": map[string]any{"type": "string", "default": "localhost"},
		},
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{
				"$ref": "#/$defs/Base",
				"properties": map[string]any{
					"host": map[string]any{"$ref": "#/$defs/Address"},
				},
			},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	props := result["properties"].(map[string]any)
	server := props["server"].(map[string]any)
	if server["type"] != typeObject {
		t.Fatalf("expected server type=object, got %v", server["type"])
	}

	serverProps := server["properties"].(map[string]any)
	host := serverProps["host"].(map[string]any)
	if host["type"] != typeString {
		t.Fatalf("expected host type=string (resolved from $ref), got %v", host["type"])
	}
	if host["default"] != "localhost" {
		t.Fatalf("expected host default=localhost, got %v", host["default"])
	}

	// Verify no $ref remains in the output
	if _, hasRef := host["$ref"]; hasRef {
		t.Fatal("expected $ref to be resolved in sibling value, but it was left un-inlined")
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
	if items["type"] != typeString {
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
	if first["type"] != typeObject {
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
	if name["type"] != typeString {
		t.Fatalf("expected type=string, got %v", name["type"])
	}
}

func TestResolveRefs_DefinitionsNotSupported(t *testing.T) {
	schema := map[string]any{
		"definitions": map[string]any{
			"Foo": map[string]any{"type": "boolean"},
		},
		"type": "object",
		"properties": map[string]any{
			"flag": map[string]any{"$ref": "#/definitions/Foo"},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for #/definitions/ ref, got nil")
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
	for i := range 70 {
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
	if ap["type"] != typeString {
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
	if first["type"] != typeString {
		t.Fatalf("expected first oneOf type=string, got %v", first["type"])
	}
	if second["type"] != typeInteger {
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

func TestResolveRefs_DeeplyNestedDefPath(t *testing.T) {
	// $ref pointing to a deeply nested definition path like #/$defs/Parent/$defs/Child
	// should fail because only top-level defs are supported (exactly 2 path parts).
	schema := map[string]any{
		"$defs": map[string]any{
			"Parent": map[string]any{
				"type": "object",
				"$defs": map[string]any{
					"Child": map[string]any{"type": "string"},
				},
			},
		},
		"type": "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "#/$defs/Parent/$defs/Child"},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for deeply nested $ref path")
	}
	if !strings.Contains(err.Error(), "unsupported $ref path") {
		t.Fatalf("expected unsupported $ref path error, got: %v", err)
	}
}

func TestResolveRefs_EmptyRefString(t *testing.T) {
	// $ref with an empty string value should fail
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": ""},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for empty $ref string")
	}
	if !strings.Contains(err.Error(), "only local $ref supported") {
		t.Fatalf("expected local ref error, got: %v", err)
	}
}

func TestResolveRefs_BothDefsAndDefinitions(t *testing.T) {
	// Only $defs is supported; "definitions" is ignored (kept as-is in output).
	schema := map[string]any{
		"$defs": map[string]any{
			"Foo": map[string]any{"type": "string"},
		},
		"definitions": map[string]any{
			"Foo": map[string]any{"type": "integer"},
		},
		"type": "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "#/$defs/Foo"},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	props := result["properties"].(map[string]any)
	val := props["val"].(map[string]any)
	// Should resolve from $defs
	if val["type"] != typeString {
		t.Fatalf("expected type=string from $defs, got %v", val["type"])
	}

	// $defs should be removed from output
	if _, ok := result["$defs"]; ok {
		t.Fatal("$defs should be removed from output")
	}
}

func TestResolveRefs_RefInAnyOf(t *testing.T) {
	schema := map[string]any{
		"$defs": map[string]any{
			"StringVal": map[string]any{"type": "string"},
			"IntVal":    map[string]any{"type": "integer"},
			"BoolVal":   map[string]any{"type": "boolean"},
		},
		"anyOf": []any{
			map[string]any{"$ref": "#/$defs/StringVal"},
			map[string]any{"$ref": "#/$defs/IntVal"},
			map[string]any{"$ref": "#/$defs/BoolVal"},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	anyOf := result["anyOf"].([]any)
	if len(anyOf) != 3 {
		t.Fatalf("expected 3 anyOf elements, got %d", len(anyOf))
	}

	expected := []string{typeString, typeInteger, "boolean"}
	for i, exp := range expected {
		elem := anyOf[i].(map[string]any)
		if elem["type"] != exp {
			t.Fatalf("expected anyOf[%d] type=%s, got %v", i, exp, elem["type"])
		}
	}
}

func TestResolveRefs_SelfReferencingRef(t *testing.T) {
	// A definition that references itself should be detected as circular.
	schema := map[string]any{
		"$defs": map[string]any{
			"A": map[string]any{"$ref": "#/$defs/A"},
		},
		"type": "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "#/$defs/A"},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for self-referencing $ref")
	}
	if !strings.Contains(err.Error(), "circular $ref") {
		t.Fatalf("expected circular ref error, got: %v", err)
	}
}

func TestResolveRefs_SpecialCharactersInDefName(t *testing.T) {
	// Definition names with special characters (dots, hyphens, underscores)
	schema := map[string]any{
		"$defs": map[string]any{
			"my-type_v1.0": map[string]any{"type": "string"},
		},
		"type": "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "#/$defs/my-type_v1.0"},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	props := result["properties"].(map[string]any)
	val := props["val"].(map[string]any)
	if val["type"] != typeString {
		t.Fatalf("expected type=string, got %v", val["type"])
	}
}

func TestResolveRefs_LargeSchemaWithManyRefs(t *testing.T) {
	// Schema with many definitions and references to verify correctness at scale.
	defs := map[string]any{}
	properties := map[string]any{}
	const numDefs = 50
	for i := range numDefs {
		name := fmt.Sprintf("Type%d", i)
		defs[name] = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"value": map[string]any{"type": "integer", "default": float64(i)},
			},
		}
		properties[fmt.Sprintf("field%d", i)] = map[string]any{"$ref": fmt.Sprintf("#/$defs/%s", name)}
	}

	schema := map[string]any{
		"$defs":      defs,
		"type":       "object",
		"properties": properties,
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all refs resolved correctly
	props := result["properties"].(map[string]any)
	for i := range numDefs {
		fieldName := fmt.Sprintf("field%d", i)
		field, ok := props[fieldName].(map[string]any)
		if !ok {
			t.Fatalf("expected field %s", fieldName)
		}
		if field["type"] != typeObject {
			t.Fatalf("expected %s type=object, got %v", fieldName, field["type"])
		}
		innerProps := field["properties"].(map[string]any)
		valueProp := innerProps["value"].(map[string]any)
		if valueProp["default"] != float64(i) {
			t.Fatalf("expected %s default=%d, got %v", fieldName, i, valueProp["default"])
		}
	}

	// $defs should be removed
	if _, ok := result["$defs"]; ok {
		t.Fatal("$defs should be removed from output")
	}
}

func TestResolveRefs_RefNonStringValue(t *testing.T) {
	// $ref with a non-string value should produce an error
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": 42},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for non-string $ref")
	}
	if !strings.Contains(err.Error(), "$ref must be a string") {
		t.Fatalf("expected '$ref must be a string' error, got: %v", err)
	}
}

func TestResolveRefs_RefInPatternProperties(t *testing.T) {
	schema := map[string]any{
		"$defs": map[string]any{
			"Value": map[string]any{"type": "integer"},
		},
		"type": "object",
		"patternProperties": map[string]any{
			"^x-": map[string]any{"$ref": "#/$defs/Value"},
		},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pp := result["patternProperties"].(map[string]any)
	xProp := pp["^x-"].(map[string]any)
	if xProp["type"] != typeInteger {
		t.Fatalf("expected patternProperties type=integer, got %v", xProp["type"])
	}
}

func TestResolveRefs_EmptyDefinitionName(t *testing.T) {
	// $ref with an empty definition name: "#/$defs/"
	schema := map[string]any{
		"$defs": map[string]any{},
		"type":  "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "#/$defs/"},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for empty definition name")
	}
	if !strings.Contains(err.Error(), "empty definition name") {
		t.Fatalf("expected empty definition name error, got: %v", err)
	}
}

func TestResolveRefs_HttpsRemoteRef(t *testing.T) {
	// HTTPS remote refs should also be rejected
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "https://example.com/schema.json"},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for HTTPS remote ref")
	}
	if !strings.Contains(err.Error(), "only local $ref supported") {
		t.Fatalf("expected remote ref error, got: %v", err)
	}
}

func TestResolveRefs_RefWithInvalidPrefix(t *testing.T) {
	// $ref with a path that doesn't use $defs
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "#/components/Foo"},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for invalid $ref prefix")
	}
	if !strings.Contains(err.Error(), "unsupported $ref path") {
		t.Fatalf("expected unsupported $ref path error, got: %v", err)
	}
}

func TestResolveRefs_ThreeWayCircularRef(t *testing.T) {
	// A -> B -> C -> A should be detected as circular
	schema := map[string]any{
		"$defs": map[string]any{
			"A": map[string]any{"$ref": "#/$defs/B"},
			"B": map[string]any{"$ref": "#/$defs/C"},
			"C": map[string]any{"$ref": "#/$defs/A"},
		},
		"type": "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "#/$defs/A"},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for three-way circular ref")
	}
	if !strings.Contains(err.Error(), "circular $ref") {
		t.Fatalf("expected circular ref error, got: %v", err)
	}
}

func TestResolveRefs_RefWithNoDefsAvailable(t *testing.T) {
	// $ref used but no $defs section at all
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"val": map[string]any{"$ref": "#/$defs/Missing"},
		},
	}

	_, err := ResolveRefs(schema)
	if err == nil {
		t.Fatal("expected error for ref with no definitions")
	}
	if !strings.Contains(err.Error(), "no definitions available") {
		t.Fatalf("expected 'no definitions available' error, got: %v", err)
	}
}

func TestResolveRefs_RefInIfThenElse(t *testing.T) {
	schema := map[string]any{
		"$defs": map[string]any{
			"StringSchema": map[string]any{"type": "string"},
			"IntSchema":    map[string]any{"type": "integer"},
		},
		"type": "object",
		"if":   map[string]any{"$ref": "#/$defs/StringSchema"},
		"then": map[string]any{"$ref": "#/$defs/IntSchema"},
		"else": map[string]any{"$ref": "#/$defs/StringSchema"},
	}

	result, err := ResolveRefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ifSchema := result["if"].(map[string]any)
	if ifSchema["type"] != typeString {
		t.Fatalf("expected if type=string, got %v", ifSchema["type"])
	}
	thenSchema := result["then"].(map[string]any)
	if thenSchema["type"] != typeInteger {
		t.Fatalf("expected then type=integer, got %v", thenSchema["type"])
	}
	elseSchema := result["else"].(map[string]any)
	if elseSchema["type"] != typeString {
		t.Fatalf("expected else type=string, got %v", elseSchema["type"])
	}
}
