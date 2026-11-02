// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import "sort"

// WithPercents sets each backend's Percent from its weight, as whole numbers
// that add up to exactly 100 (largest remainder). When no backend has a
// weight, every share is 0.
func WithPercents(backends []Backend) []Backend {
	out := make([]Backend, len(backends))
	copy(out, backends)
	var total int64
	for _, b := range out {
		total += int64(b.Weight)
	}
	if total <= 0 {
		for i := range out {
			out[i].Percent = 0
		}
		return out
	}
	type remainder struct {
		index int
		rest  int64
	}
	rests := make([]remainder, len(out))
	left := int64(100)
	for i, b := range out {
		share := int64(b.Weight) * 100
		out[i].Percent = int32(min(share/total, 100)) //nolint:gosec // a share of 100 is at most 100
		rests[i] = remainder{index: i, rest: share % total}
		left -= int64(out[i].Percent)
	}
	sort.SliceStable(rests, func(a, b int) bool { return rests[a].rest > rests[b].rest })
	for _, r := range rests {
		if left == 0 {
			break
		}
		out[r.index].Percent++
		left--
	}
	return out
}
