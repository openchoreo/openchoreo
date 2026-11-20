// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// revs builds candidates r1..rN, none active or known-good.
func revs(n int) []EvictionCandidate {
	out := make([]EvictionCandidate, n)
	for i := range out {
		out[i] = EvictionCandidate{Name: fmt.Sprintf("b-r%d", i+1), Number: int64(i + 1)}
	}
	return out
}

// with returns candidates with mutate applied to revision number n.
func with(candidates []EvictionCandidate, n int64, mutate func(c *EvictionCandidate)) []EvictionCandidate {
	for i := range candidates {
		if candidates[i].Number == n {
			mutate(&candidates[i])
		}
	}
	return candidates
}

func active(c *EvictionCandidate)    { c.Active = true }
func knownGood(c *EvictionCandidate) { c.KnownGood = true }

// existing builds candidates with exactly these numbers, as if every other
// number had been deleted; none is active or known-good.
func existing(ns ...int64) []EvictionCandidate {
	out := make([]EvictionCandidate, len(ns))
	for i, n := range ns {
		out[i] = EvictionCandidate{Name: fmt.Sprintf("b-r%d", n), Number: n}
	}
	return out
}

func numbers(candidates []EvictionCandidate) []int64 {
	out := make([]int64, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.Number)
	}
	return out
}

// TestSelectEvictions pins the history limit: it keeps the active revision and
// the newest limit old revisions, deleting the oldest old revisions first.
func TestSelectEvictions(t *testing.T) {
	tests := []struct {
		name        string
		candidates  []EvictionCandidate
		limit       int
		wantEvicted []int64
	}{
		{
			name:        "limit 0 keeps only the active revision",
			candidates:  with(revs(4), 4, active),
			limit:       0,
			wantEvicted: []int64{1, 2, 3},
		},
		{
			name:        "limit 1 keeps the active revision and one old revision, the newest",
			candidates:  with(revs(4), 4, active),
			limit:       1,
			wantEvicted: []int64{1, 2},
		},
		{
			name:        "limit 2 keeps the active revision and two old revisions",
			candidates:  with(revs(4), 4, active),
			limit:       2,
			wantEvicted: []int64{1},
		},
		{
			name:        "a limit at or above the old revisions deletes nothing: raising a limit keeps all",
			candidates:  with(revs(4), 4, active),
			limit:       3,
			wantEvicted: []int64{},
		},
		{
			name:        "a negative limit is treated as 0",
			candidates:  with(revs(3), 3, active),
			limit:       -1,
			wantEvicted: []int64{1, 2},
		},
		{
			name:        "the active revision is never deleted, even when it is the oldest",
			candidates:  with(revs(4), 1, active),
			limit:       1,
			wantEvicted: []int64{2, 3},
		},
		{
			name:        "the active revision in the middle is skipped; the oldest old revisions go",
			candidates:  with(revs(5), 3, active),
			limit:       1,
			wantEvicted: []int64{1, 2, 4},
		},
		{
			name:        "deleted revisions do not count: r1 active with r2 and r4 left after r3 was deleted, limit 2",
			candidates:  with(existing(1, 2, 4), 1, active),
			limit:       2,
			wantEvicted: []int64{},
		},
		{
			name:        "deleted revisions do not count: the same history with limit 1 deletes the oldest old revision",
			candidates:  with(existing(1, 2, 4), 1, active),
			limit:       1,
			wantEvicted: []int64{2},
		},
		{
			name:        "gaps in the numbering are ignored: only existing old revisions are counted",
			candidates:  with(existing(3, 7, 8, 12, 20), 20, active),
			limit:       2,
			wantEvicted: []int64{3, 7},
		},
		{
			name:        "known-good revisions are not spared: limit 1 keeps only the newest old revision",
			candidates:  with(with(with(revs(5), 5, active), 1, knownGood), 3, knownGood),
			limit:       1,
			wantEvicted: []int64{1, 2, 3},
		},
		{
			name:        "without an active revision every revision is an old one",
			candidates:  revs(3),
			limit:       1,
			wantEvicted: []int64{1, 2},
		},
		{
			name: "input order does not matter",
			candidates: []EvictionCandidate{
				{Name: "b-r4", Number: 4, Active: true},
				{Name: "b-r2", Number: 2},
				{Name: "b-r1", Number: 1},
				{Name: "b-r3", Number: 3},
			},
			limit:       1,
			wantEvicted: []int64{1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantEvicted, numbers(SelectEvictions(tt.candidates, tt.limit)))
		})
	}
}

func TestProtected(t *testing.T) {
	tests := []struct {
		name       string
		candidates []EvictionCandidate
		target     int64
		want       Protection
	}{
		{"an inactive revision may be deleted", with(revs(4), 4, active), 2, ""},
		{"the active revision is protected", with(revs(4), 4, active), 4, ProtectionActive},
		{"the newest known-good revision is protected",
			with(with(with(revs(4), 4, active), 1, knownGood), 3, knownGood), 3, ProtectionNewestKnownGood},
		{"an older known-good revision may be deleted",
			with(with(with(revs(4), 4, active), 1, knownGood), 3, knownGood), 1, ""},
		{"an active known-good revision is the newest known-good",
			with(with(with(revs(4), 4, active), 4, knownGood), 3, knownGood), 3, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var target EvictionCandidate
			for _, c := range tt.candidates {
				if c.Number == tt.target {
					target = c
				}
			}
			assert.Equal(t, tt.want, Protected(tt.candidates, target))
		})
	}
}
