// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package strdist provides the Levenshtein edit distance and closest-match
// selection used to recognize misspelled OpenChoreo field, kind and enum values.
package strdist

import "strings"

// Distance returns the Levenshtein edit distance between a and b: the number
// of single-character insertions, deletions or substitutions needed to turn one
// into the other. Comparison is case-insensitive, since YAML field names and
// OpenChoreo kinds differ only in casing in the most common typo.
func Distance(a, b string) int {
	ra := []rune(strings.ToLower(a))
	rb := []rune(strings.ToLower(b))
	if string(ra) == string(rb) {
		return 0
	}

	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// Closest returns the candidate nearest to typo, comparing case-insensitively.
// It reports ok=false when no candidate is within maxDist, and also when two
// candidates tie at that distance, because guessing between equally close
// candidates would risk renaming a correct field to the wrong one.
func Closest(typo string, candidates []string, maxDist int) (match string, dist int, ok bool) {
	if typo == "" {
		return "", 0, false
	}
	best := ""
	bestDist := maxDist + 1
	tied := false
	for _, c := range candidates {
		if c == "" {
			continue
		}
		d := Distance(typo, c)
		if d > maxDist {
			continue
		}
		switch {
		case d < bestDist:
			best, bestDist, tied = c, d, false
		case d == bestDist && c != best:
			tied = true
		}
	}
	if best == "" || tied {
		return "", bestDist, false
	}
	return best, bestDist, true
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
