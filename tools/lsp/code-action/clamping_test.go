// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codeaction

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"
)

func TestScalarRangePositionConversions(t *testing.T) {
	for _, tc := range []struct {
		name string
		node yaml.Node
		want protocol.Range
	}{
		{"ascii_origin", yaml.Node{Line: 1, Column: 1, Value: "kind"}, protocol.Range{End: protocol.Position{Character: 4}}},
		{"bmp_width", yaml.Node{Line: 3, Column: 5, Value: "añb"}, protocol.Range{Start: protocol.Position{Line: 2, Character: 4}, End: protocol.Position{Line: 2, Character: 7}}},
		{"empty_value", yaml.Node{Line: 1, Column: 1}, protocol.Range{}},
		{"missing_position", yaml.Node{Value: "ab"}, protocol.Range{End: protocol.Position{Character: 2}}},
		{"negative_position", yaml.Node{Line: -2, Column: -3, Value: "ab"}, protocol.Range{End: protocol.Position{Character: 2}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := scalarRange(&tc.node); got != tc.want {
				t.Fatalf("range=%+v, want %+v", got, tc.want)
			}
		})
	}
}
