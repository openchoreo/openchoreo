// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codedefinition

import (
	"math"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lint/indexer"
)

func TestDefinitionLocationConversions(t *testing.T) {
	const uri = "file:///definition-clamp.yaml"
	for _, tc := range []struct {
		name         string
		line, column int
		start, end   protocol.Position
	}{
		{"origin", 1, 1, protocol.Position{}, protocol.Position{Character: 4}},
		{"normal", 15, 20, protocol.Position{Line: 14, Character: 19}, protocol.Position{Line: 14, Character: 23}},
		{"negative_line", -1, 1, protocol.Position{}, protocol.Position{Character: 4}},
		{"upper_valid_endpoint", math.MaxInt32, math.MaxInt32 - 3, protocol.Position{Line: math.MaxInt32 - 1, Character: math.MaxInt32 - 4}, protocol.Position{Line: math.MaxInt32 - 1, Character: math.MaxInt32}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := toLSPLocations([]indexer.Location{{URI: uri, Line: tc.line, Column: tc.column}})
			if len(got) != 1 || got[0].URI != uri || got[0].Range.Start != tc.start || got[0].Range.End != tc.end {
				t.Fatalf("locations=%+v", got)
			}
		})
	}
	if got := toLSPLocations(nil); got != nil {
		t.Fatalf("nil input changed: %+v", got)
	}
}
