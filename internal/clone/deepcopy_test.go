// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clone

import (
	"reflect"
	"testing"
)

const modifiedValue = "modified"

func TestDeepCopy_Nil(t *testing.T) {
	result := DeepCopy(nil)
	if result != nil {
		t.Errorf("DeepCopy(nil) = %v, want nil", result)
	}
}

func TestDeepCopy_EmptyMap(t *testing.T) {
	result := DeepCopy(map[string]any{})
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("DeepCopy(empty map) type = %T, want map[string]any", result)
	}
	if len(m) != 0 {
		t.Errorf("DeepCopy(empty map) len = %d, want 0", len(m))
	}
}

func TestDeepCopy_EmptySlice(t *testing.T) {
	result := DeepCopy([]any{})
	s, ok := result.([]any)
	if !ok {
		t.Fatalf("DeepCopy(empty slice) type = %T, want []any", result)
	}
	if len(s) != 0 {
		t.Errorf("DeepCopy(empty slice) len = %d, want 0", len(s))
	}
}

func TestDeepCopy_Map(t *testing.T) {
	original := map[string]any{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}
	result := DeepCopy(original)
	copied, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("DeepCopy map result type = %T, want map[string]any", result)
	}

	if !reflect.DeepEqual(original, copied) {
		t.Errorf("DeepCopy map = %v, want %v", copied, original)
	}

	// Verify independence
	copied["key1"] = modifiedValue
	if original["key1"] == modifiedValue {
		t.Error("DeepCopy map is not independent: modifying copy affected original")
	}
}

func TestDeepCopy_NestedMap(t *testing.T) {
	original := map[string]any{
		"outer": map[string]any{
			"inner": "value",
		},
	}
	result := DeepCopy(original)
	copied, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("DeepCopy nested map result type = %T, want map[string]any", result)
	}

	if !reflect.DeepEqual(original, copied) {
		t.Errorf("DeepCopy nested map = %v, want %v", copied, original)
	}

	// Verify deep independence
	inner := copied["outer"].(map[string]any)
	inner["inner"] = modifiedValue
	origInner := original["outer"].(map[string]any)
	if origInner["inner"] == modifiedValue {
		t.Error("DeepCopy nested map is not deep-independent")
	}
}

func TestDeepCopy_Slice(t *testing.T) {
	original := []any{"a", "b", "c"}
	result := DeepCopy(original)
	copied, ok := result.([]any)
	if !ok {
		t.Fatalf("DeepCopy slice result type = %T, want []any", result)
	}

	if !reflect.DeepEqual(original, copied) {
		t.Errorf("DeepCopy slice = %v, want %v", copied, original)
	}

	// Verify independence
	copied[0] = modifiedValue
	if original[0] == modifiedValue {
		t.Error("DeepCopy slice is not independent")
	}
}

func TestDeepCopy_NestedSlice(t *testing.T) {
	original := []any{
		[]any{1, 2, 3},
		[]any{4, 5, 6},
	}
	result := DeepCopy(original)
	copied, ok := result.([]any)
	if !ok {
		t.Fatalf("DeepCopy nested slice result type = %T, want []any", result)
	}

	if !reflect.DeepEqual(original, copied) {
		t.Errorf("DeepCopy nested slice = %v, want %v", copied, original)
	}
}

func TestDeepCopy_Primitives(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{"string", "hello"},
		{"int", 42},
		{"int64", int64(100)},
		{"int32", int32(200)},
		{"int16", int16(300)},
		{"int8", int8(127)},
		{"uint", uint(42)},
		{"uint64", uint64(100)},
		{"uint32", uint32(200)},
		{"uint16", uint16(300)},
		{"uint8", uint8(255)},
		{"float64", float64(3.14)},
		{"float32", float32(2.71)},
		{"bool_true", true},
		{"bool_false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeepCopy(tt.input)
			if result != tt.input {
				t.Errorf("DeepCopy(%v) = %v, want %v", tt.input, result, tt.input)
			}
		})
	}
}

func TestDeepCopyMap_Nil(t *testing.T) {
	result := DeepCopyMap(nil)
	if result != nil {
		t.Errorf("DeepCopyMap(nil) = %v, want nil", result)
	}
}

func TestDeepCopyMap_Normal(t *testing.T) {
	original := map[string]any{
		"a": "1",
		"b": 2,
		"c": map[string]any{"d": "nested"},
	}
	copied := DeepCopyMap(original)

	if !reflect.DeepEqual(original, copied) {
		t.Errorf("DeepCopyMap = %v, want %v", copied, original)
	}

	// Verify independence
	copied["a"] = modifiedValue
	if original["a"] == modifiedValue {
		t.Error("DeepCopyMap is not independent")
	}
}

func TestDeepCopyMap_DeepIndependence(t *testing.T) {
	original := map[string]any{
		"outer": map[string]any{
			"inner": "original",
		},
	}
	copied := DeepCopyMap(original)

	// Modify the nested map in the copy
	innerCopy := copied["outer"].(map[string]any)
	innerCopy["inner"] = modifiedValue

	// Original should be unchanged
	innerOrig := original["outer"].(map[string]any)
	if innerOrig["inner"] != "original" {
		t.Error("DeepCopyMap nested modification affected original")
	}
}
