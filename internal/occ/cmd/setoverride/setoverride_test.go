// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package setoverride

import (
	"testing"
)

func TestToSjsonPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple dot path unchanged",
			input:    "spec.replicas",
			expected: "spec.replicas",
		},
		{
			name:     "bracket index at end",
			input:    "spec.containers[0]",
			expected: "spec.containers.0",
		},
		{
			name:     "bracket index in middle",
			input:    "spec.containers[0].name",
			expected: "spec.containers.0.name",
		},
		{
			name:     "multiple bracket indices",
			input:    "spec.containers[0].ports[1].containerPort",
			expected: "spec.containers.0.ports.1.containerPort",
		},
		{
			name:     "negative index for append",
			input:    "spec.containers[-1]",
			expected: "spec.containers.-1",
		},
		{
			name:     "nested array indices",
			input:    "matrix[0][1]",
			expected: "matrix.0.1",
		},
		{
			name:     "single key unchanged",
			input:    "name",
			expected: "name",
		},
		{
			name:     "deeply nested with mixed notation",
			input:    "a.b[0].c.d[2].e",
			expected: "a.b.0.c.d.2.e",
		},
		{
			name:     "bare numeric segment becomes object key",
			input:    "spec.containers.0.image",
			expected: "spec.containers.:0.image",
		},
		{
			name:     "multiple bare numeric segments become object keys",
			input:    "a.1.b.2.c",
			expected: "a.:1.b.:2.c",
		},
		{
			name:     "bare numeric only",
			input:    "0",
			expected: ":0",
		},
		{
			name:     "mixed bracket and bare numeric",
			input:    "spec.items[0].configs.1.name",
			expected: "spec.items.0.configs.:1.name",
		},
		{
			name:     "escaped dot preserves single key",
			input:    `labels.foo\.0`,
			expected: `labels.foo\.0`,
		},
		{
			name:     "escaped dot with deeper path",
			input:    `metadata.labels.app\.kubernetes\.io/name`,
			expected: `metadata.labels.app\.kubernetes\.io/name`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToSjsonPath(tt.input)
			if got != tt.expected {
				t.Errorf("ToSjsonPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestToJSONLiteral(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Valid JSON numbers (returned raw)
		{name: "integer", input: "1", expected: "1"},
		{name: "negative integer", input: "-1", expected: "-1"},
		{name: "zero", input: "0", expected: "0"},
		{name: "decimal", input: "1.5", expected: "1.5"},
		{name: "negative decimal", input: "-1.5", expected: "-1.5"},
		{name: "exponent", input: "1e10", expected: "1e10"},
		{name: "exponent uppercase", input: "1E10", expected: "1E10"},
		{name: "exponent with plus", input: "1.0e+2", expected: "1.0e+2"},
		{name: "exponent with minus", input: "1.0e-2", expected: "1.0e-2"},
		// Booleans and null (returned raw)
		{name: "true", input: "true", expected: "true"},
		{name: "false", input: "false", expected: "false"},
		{name: "null", input: "null", expected: "null"},
		// Invalid JSON numbers (quoted as strings)
		{name: "leading zero", input: "01", expected: `"01"`},
		{name: "leading plus", input: "+1", expected: `"+1"`},
		{name: "leading plus decimal", input: "+1.5", expected: `"+1.5"`},
		{name: "double zero", input: "00", expected: `"00"`},
		{name: "leading zero decimal", input: "01.5", expected: `"01.5"`},
		// Plain strings (quoted)
		{name: "word", input: "hello", expected: `"hello"`},
		{name: "empty", input: "", expected: `""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toJSONLiteral(tt.input)
			if got != tt.expected {
				t.Errorf("toJSONLiteral(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestApply(t *testing.T) {
	tests := []struct {
		name      string
		jsonStr   string
		setValues []string
		expected  string
		wantErr   bool
	}{
		{
			name:      "set simple string value",
			jsonStr:   `{"spec":{"name":"old"}}`,
			setValues: []string{"spec.name=new"},
			expected:  `{"spec":{"name":"new"}}`,
		},
		{
			name:      "set numeric value",
			jsonStr:   `{"spec":{"replicas":1}}`,
			setValues: []string{"spec.replicas=3"},
			expected:  `{"spec":{"replicas":3}}`,
		},
		{
			name:      "set boolean value",
			jsonStr:   `{"spec":{"enabled":false}}`,
			setValues: []string{"spec.enabled=true"},
			expected:  `{"spec":{"enabled":true}}`,
		},
		{
			name:      "set array element with bracket notation",
			jsonStr:   `{"spec":{"items":["a","b","c"]}}`,
			setValues: []string{"spec.items[1]=x"},
			expected:  `{"spec":{"items":["a","x","c"]}}`,
		},
		{
			name:      "set nested object in array with bracket notation",
			jsonStr:   `{"spec":{"containers":[{"name":"app","image":"old"}]}}`,
			setValues: []string{"spec.containers[0].image=new"},
			expected:  `{"spec":{"containers":[{"name":"app","image":"new"}]}}`,
		},
		{
			name:      "multiple set values",
			jsonStr:   `{"spec":{"name":"old","replicas":1}}`,
			setValues: []string{"spec.name=new", "spec.replicas=5"},
			expected:  `{"spec":{"name":"new","replicas":5}}`,
		},
		{
			name:      "bare numeric creates object key not array index",
			jsonStr:   `{"spec":{}}`,
			setValues: []string{"spec.0=hello"},
			expected:  `{"spec":{"0":"hello"}}`,
		},
		{
			name:      "empty set values is no-op",
			jsonStr:   `{"spec":{"name":"old"}}`,
			setValues: []string{},
			expected:  `{"spec":{"name":"old"}}`,
		},
		{
			name:      "missing equals sign",
			jsonStr:   `{}`,
			setValues: []string{"spec.name"},
			wantErr:   true,
		},
		{
			name:      "empty key",
			jsonStr:   `{}`,
			setValues: []string{"=value"},
			wantErr:   true,
		},
		{
			name:      "set null value",
			jsonStr:   `{"spec":{"name":"old"}}`,
			setValues: []string{"spec.name=null"},
			expected:  `{"spec":{"name":null}}`,
		},
		{
			name:      "leading zero stays as string",
			jsonStr:   `{"spec":{}}`,
			setValues: []string{"spec.id=01"},
			expected:  `{"spec":{"id":"01"}}`,
		},
		{
			name:      "leading plus stays as string",
			jsonStr:   `{"spec":{}}`,
			setValues: []string{"spec.val=+1"},
			expected:  `{"spec":{"val":"+1"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Apply(tt.jsonStr, tt.setValues)
			if (err != nil) != tt.wantErr {
				t.Errorf("Apply() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("Apply() = %q, want %q", got, tt.expected)
			}
		})
	}
}
