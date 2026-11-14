// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codedefinition

import (
	"math"
	"testing"

	"github.com/openchoreo/openchoreo/tools/lint/indexer"
)

func TestIntegerSafetyDefinitionRange(t *testing.T) {
	got := toLSPLocations([]indexer.Location{{Line: math.MinInt, Column: math.MaxInt}})[0].Range
	if got.Start.Line != 0 || got.End.Character < got.Start.Character || got.End.Character > math.MaxInt32 {
		t.Fatalf("definition arithmetic wrapped: %+v", got)
	}
}
