// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package specdiff

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func num(s string) json.Number { return json.Number(s) }

func TestCompare(t *testing.T) {
	tests := []struct {
		name string
		from any
		to   any
		want []Change
	}{
		{
			name: "identical",
			from: map[string]any{"image": "v1", "env": []any{"A"}},
			to:   map[string]any{"image": "v1", "env": []any{"A"}},
			want: nil,
		},
		{
			name: "modified scalar",
			from: map[string]any{"image": "nginx:v1"},
			to:   map[string]any{"image": "nginx:v2"},
			want: []Change{{Path: "spec.image", Type: Modified, Old: "nginx:v1", New: "nginx:v2"}},
		},
		{
			name: "added and removed fields",
			from: map[string]any{"a": 1, "b": 2},
			to:   map[string]any{"b": 2, "c": 3},
			want: []Change{
				{Path: "spec.a", Type: Removed, Old: num("1")},
				{Path: "spec.c", Type: Added, New: num("3")},
			},
		},
		{
			name: "nested fields",
			from: map[string]any{"workload": map[string]any{"container": map[string]any{"image": "v1", "args": []any{"x"}}}},
			to:   map[string]any{"workload": map[string]any{"container": map[string]any{"image": "v2", "args": []any{"x"}}}},
			want: []Change{{Path: "spec.workload.container.image", Type: Modified, Old: "v1", New: "v2"}},
		},
		{
			name: "list elements by index",
			from: map[string]any{"env": []any{map[string]any{"key": "A", "value": "1"}, "gone"}},
			to:   map[string]any{"env": []any{map[string]any{"key": "A", "value": "2"}}},
			want: []Change{
				{Path: "spec.env[0].value", Type: Modified, Old: "1", New: "2"},
				{Path: "spec.env[1]", Type: Removed, Old: "gone"},
			},
		},
		{
			name: "list grows",
			from: map[string]any{"ports": []any{80}},
			to:   map[string]any{"ports": []any{80, 443}},
			want: []Change{{Path: "spec.ports[1]", Type: Added, New: num("443")}},
		},
		{
			name: "whole subtree added",
			from: map[string]any{},
			to:   map[string]any{"parameters": map[string]any{"replicas": 2}},
			want: []Change{{Path: "spec.parameters", Type: Added, New: map[string]any{"replicas": num("2")}}},
		},
		{
			name: "null and missing are both unset",
			from: map[string]any{"a": nil},
			to:   map[string]any{},
			want: nil,
		},
		{
			name: "empty value is not unset",
			from: map[string]any{},
			to:   map[string]any{"parameters": map[string]any{}},
			want: []Change{{Path: "spec.parameters", Type: Added, New: map[string]any{}}},
		},
		{
			name: "type change",
			from: map[string]any{"v": "3"},
			to:   map[string]any{"v": 3},
			want: []Change{{Path: "spec.v", Type: Modified, Old: "3", New: num("3")}},
		},
		{
			name: "map replaced by list",
			from: map[string]any{"v": map[string]any{"a": 1}},
			to:   map[string]any{"v": []any{1}},
			want: []Change{{Path: "spec.v", Type: Modified, Old: map[string]any{"a": num("1")}, New: []any{num("1")}}},
		},
		{
			name: "keys needing quotes",
			from: map[string]any{"labels": map[string]any{"app.kubernetes.io/name": "a"}},
			to:   map[string]any{"labels": map[string]any{"app.kubernetes.io/name": "b"}},
			want: []Change{{Path: `spec.labels["app.kubernetes.io/name"]`, Type: Modified, Old: "a", New: "b"}},
		},
		{
			name: "nil root",
			from: nil,
			to:   map[string]any{"a": 1},
			want: []Change{{Path: "spec", Type: Added, New: map[string]any{"a": num("1")}}},
		},
		{
			name: "both nil",
			from: nil,
			to:   nil,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Compare("spec", tt.from, tt.to)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompare_UsesJSONFieldNames(t *testing.T) {
	type spec struct {
		Image    string  `json:"image"`
		Replicas *int    `json:"replicas,omitempty"`
		Note     *string `json:"note,omitempty"`
	}
	two := 2
	got, err := Compare("spec", &spec{Image: "v1"}, &spec{Image: "v1", Replicas: &two})
	require.NoError(t, err)
	assert.Equal(t, []Change{{Path: "spec.replicas", Type: Added, New: num("2")}}, got)
}

func TestCompare_SortedOutput(t *testing.T) {
	got, err := Compare("spec", map[string]any{}, map[string]any{"c": 1, "a": 1, "b": 1})
	require.NoError(t, err)
	paths := make([]string, len(got))
	for i, c := range got {
		paths[i] = c.Path
	}
	assert.Equal(t, []string{"spec.a", "spec.b", "spec.c"}, paths)
}

func TestCompare_UnmarshalableValue(t *testing.T) {
	bad := map[string]any{"f": func() {}}

	_, err := Compare("spec", bad, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read the first value")

	_, err = Compare("spec", nil, bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read the second value")
}

func TestJoinKey(t *testing.T) {
	assert.Equal(t, "a", joinKey("", "a"))
	assert.Equal(t, "spec.my-key_1", joinKey("spec", "my-key_1"))
	assert.Equal(t, `spec["a b"]`, joinKey("spec", "a b"))
	assert.Equal(t, `spec["1abc"]`, joinKey("spec", "1abc"))
}

func TestFormatValue(t *testing.T) {
	assert.Equal(t, `"a<b>"`, formatValue("a<b>"))
	assert.Equal(t, `{"a":[1,"x"]}`, formatValue(map[string]any{"a": []any{num("1"), "x"}}))
	assert.Equal(t, "null", formatValue(nil))
	assert.Equal(t, `"line1\nline2"`, formatValue("line1\nline2"))
	// JSON has no NaN, so the value is printed with fmt instead.
	assert.Equal(t, "NaN", formatValue(math.NaN()))
}
