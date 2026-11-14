// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsputil

import "math"

// PositionIndex bounds an internal position to the LSP unsigned integer range.
func PositionIndex(value int) uint32 {
	bounded := int64(value)
	if bounded < 0 {
		return 0
	}
	if bounded > math.MaxInt32 {
		return math.MaxInt32
	}
	return uint32(bounded)
}

// Integer bounds internal integers to the range supported by LSP messages.
func Integer(value int) int32 {
	if value < math.MinInt32 {
		return math.MinInt32
	}
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(value)
}
