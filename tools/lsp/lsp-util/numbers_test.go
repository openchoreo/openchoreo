// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsputil

import (
	"math"
	"strconv"
	"testing"
)

func TestPositionIndexBounds(t *testing.T) {
	maximum := uint32(math.MaxInt32)
	type testCase struct {
		name  string
		input int
		want  uint32
	}
	cases := []testCase{
		{"minimum_machine_integer", math.MinInt, 0},
		{"negative_normal", -42, 0},
		{"just_below_unsigned_minimum", -1, 0},
		{"unsigned_minimum", 0, 0},
		{"just_above_unsigned_minimum", 1, 1},
		{"normal", 42, 42},
		{"protocol_unsigned_maximum", math.MaxInt32, math.MaxInt32},
		{"maximum_machine_integer", math.MaxInt, maximum},
	}
	// int cannot express the uint32 upper boundary on 32-bit targets.
	if strconv.IntSize == 64 {
		signedMaximum := math.MaxInt32
		unsignedMaximum := signedMaximum*2 + 1
		cases = append(cases,
			testCase{"just_above_protocol_maximum", signedMaximum + 1, math.MaxInt32},
			testCase{"just_below_uint32_maximum", unsignedMaximum - 1, math.MaxInt32},
			testCase{"uint32_maximum", unsignedMaximum, math.MaxInt32},
			testCase{"just_above_uint32_maximum", unsignedMaximum + 1, math.MaxInt32},
			testCase{"far_above_uint32_maximum", unsignedMaximum + 1024, math.MaxInt32},
		)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PositionIndex(tc.input); got != tc.want {
				t.Fatalf("PositionIndex(%d) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestIntegerBounds(t *testing.T) {
	type testCase struct {
		name  string
		input int
		want  int32
	}
	cases := []testCase{
		{"minimum_machine_integer", math.MinInt, math.MinInt32},
		{"signed_minimum", math.MinInt32, math.MinInt32},
		{"just_above_signed_minimum", math.MinInt32 + 1, math.MinInt32 + 1},
		{"negative_normal", -42, -42},
		{"negative_one", -1, -1},
		{"zero", 0, 0},
		{"positive_one", 1, 1},
		{"positive_normal", 42, 42},
		{"just_below_signed_maximum", math.MaxInt32 - 1, math.MaxInt32 - 1},
		{"signed_maximum", math.MaxInt32, math.MaxInt32},
		{"maximum_machine_integer", math.MaxInt, math.MaxInt32},
	}
	if strconv.IntSize == 64 {
		minimum, maximum := math.MinInt32, math.MaxInt32
		cases = append(cases,
			testCase{"just_below_signed_minimum", minimum - 1, math.MinInt32},
			testCase{"far_below_signed_minimum", minimum - 1024, math.MinInt32},
			testCase{"just_above_signed_maximum", maximum + 1, math.MaxInt32},
			testCase{"far_above_signed_maximum", maximum + 1024, math.MaxInt32},
		)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Integer(tc.input); got != tc.want {
				t.Fatalf("Integer(%d) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}
