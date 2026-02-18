// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hash

import (
	"testing"
)

func TestComputeHash_Deterministic(t *testing.T) {
	obj := map[string]string{"key": "value", "foo": "bar"}
	h1 := ComputeHash(obj, nil)
	h2 := ComputeHash(obj, nil)
	if h1 != h2 {
		t.Errorf("ComputeHash is not deterministic: got %q and %q for same input", h1, h2)
	}
}

func TestComputeHash_DifferentInputs(t *testing.T) {
	obj1 := map[string]string{"key": "value1"}
	obj2 := map[string]string{"key": "value2"}
	h1 := ComputeHash(obj1, nil)
	h2 := ComputeHash(obj2, nil)
	if h1 == h2 {
		t.Errorf("ComputeHash produced the same hash for different inputs: %q", h1)
	}
}

func TestComputeHash_WithCollisionCount(t *testing.T) {
	obj := "test-object"
	var cc int32 = 1
	h0 := ComputeHash(obj, nil)
	h1 := ComputeHash(obj, &cc)
	if h0 == h1 {
		t.Errorf("ComputeHash with collisionCount should differ from without: h0=%q h1=%q", h0, h1)
	}

	var cc2 int32 = 2
	h2 := ComputeHash(obj, &cc2)
	if h1 == h2 {
		t.Errorf("Different collision counts should produce different hashes: h1=%q h2=%q", h1, h2)
	}
}

func TestComputeHash_ZeroCollisionCount(t *testing.T) {
	obj := "test"
	var cc int32 = 0
	h0 := ComputeHash(obj, nil)
	h1 := ComputeHash(obj, &cc)
	// Zero collision count should affect the hash
	if h0 == h1 {
		t.Errorf("Zero collision count should differ from nil: h0=%q h1=%q", h0, h1)
	}
}

func TestComputeHash_NilObject(t *testing.T) {
	// Should not panic
	h := ComputeHash(nil, nil)
	if h == "" {
		t.Error("ComputeHash(nil) returned empty string")
	}
}

func TestComputeHash_VariousTypes(t *testing.T) {
	tests := []struct {
		name string
		obj  any
	}{
		{"string", "hello world"},
		{"int", 42},
		{"bool", true},
		{"float", 3.14},
		{"struct", struct{ X, Y int }{1, 2}},
		{"slice", []string{"a", "b", "c"}},
		{"nested map", map[string]any{"nested": map[string]string{"k": "v"}}},
	}

	seen := make(map[string]bool)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := ComputeHash(tt.obj, nil)
			if h == "" {
				t.Error("ComputeHash returned empty string")
			}
			// Each unique type/value should produce a unique hash
			if seen[h] {
				t.Errorf("Hash collision detected for %v: %q", tt.obj, h)
			}
			seen[h] = true
		})
	}
}

func TestEqual_SameObjects(t *testing.T) {
	obj := struct{ Name string }{Name: "test"}
	if !Equal(obj, obj) {
		t.Error("Equal returned false for the same object")
	}
}

func TestEqual_EqualObjects(t *testing.T) {
	obj1 := struct{ Name string }{Name: "test"}
	obj2 := struct{ Name string }{Name: "test"}
	if !Equal(obj1, obj2) {
		t.Error("Equal returned false for equal objects")
	}
}

func TestEqual_DifferentObjects(t *testing.T) {
	obj1 := struct{ Name string }{Name: "test1"}
	obj2 := struct{ Name string }{Name: "test2"}
	if Equal(obj1, obj2) {
		t.Error("Equal returned true for different objects")
	}
}

func TestEqual_NilObjects(t *testing.T) {
	if !Equal(nil, nil) {
		t.Error("Equal returned false for nil == nil")
	}
}

func TestEqual_MapStructure(t *testing.T) {
	m1 := map[string]string{"a": "1", "b": "2"}
	m2 := map[string]string{"a": "1", "b": "2"}
	m3 := map[string]string{"a": "1", "b": "3"}

	if !Equal(m1, m2) {
		t.Error("Equal maps should be equal")
	}
	if Equal(m1, m3) {
		t.Error("Different maps should not be equal")
	}
}
