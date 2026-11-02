// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import "testing"

func percents(weights ...int32) []int32 {
	backends := make([]Backend, 0, len(weights))
	for _, w := range weights {
		backends = append(backends, Backend{Weight: w})
	}
	out := make([]int32, 0, len(weights))
	for _, b := range WithPercents(backends) {
		out = append(out, b.Percent)
	}
	return out
}

func TestWithPercents(t *testing.T) {
	cases := []struct {
		name    string
		weights []int32
		want    []int32
	}{
		{"already a percentage", []int32{95, 5}, []int32{95, 5}},
		{"relative weights", []int32{1, 1, 1}, []int32{34, 33, 33}},
		{"large weights", []int32{9500, 500}, []int32{95, 5}},
		{"a backend at zero", []int32{3, 0, 1}, []int32{75, 0, 25}},
		{"no weight at all", []int32{0, 0}, []int32{0, 0}},
		{"one backend", []int32{7}, []int32{100}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := percents(c.weights...)
			var sum int32
			for i := range got {
				sum += got[i]
				if got[i] != c.want[i] {
					t.Fatalf("percents(%v) = %v, want %v", c.weights, got, c.want)
				}
			}
			if sum != 0 && sum != 100 {
				t.Fatalf("percents(%v) add up to %d, want 100", c.weights, sum)
			}
		})
	}
}
