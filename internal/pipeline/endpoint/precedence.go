// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import "sort"

// MoreSpecific reports whether match a takes precedence over match b, as
// Gateway API ranks HTTPRoute matches: an exact path, then the longer path
// (a match without one, or with the "/" prefix, ranks below every other
// path), then more header conditions. A cookie
// renders as a condition on the Cookie header, so it counts as one.
func MoreSpecific(a, b Match) bool {
	if ae, be := exactPath(a), exactPath(b); ae != be {
		return ae
	}
	if al, bl := pathLength(a), pathLength(b); al != bl {
		return al > bl
	}
	return len(a.Conditions) > len(b.Conditions)
}

// SortBySpecificity orders match rules most specific first, keeping the
// given order between rules that rank the same. Rendered in this order,
// every routing API picks the same backend for a request: Gateway API ranks
// rules this way anyway, and Istio and Envoy take the first match.
func SortBySpecificity(rules []Rule) {
	sort.SliceStable(rules, func(i, j int) bool {
		return MoreSpecific(firstMatch(rules[i]), firstMatch(rules[j]))
	})
}

func firstMatch(r Rule) Match {
	if len(r.Matches) == 0 {
		return Match{}
	}
	return r.Matches[0]
}

func exactPath(m Match) bool {
	return m.Path != nil && m.Path.Type == PathMatchExact
}

func pathLength(m Match) int {
	if m.Path == nil {
		return 0
	}
	return len(m.Path.Value)
}
