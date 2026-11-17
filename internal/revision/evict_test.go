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

func active(c *EvictionCandidate)     { c.Active = true }
func knownGood(c *EvictionCandidate)  { c.KnownGood = true }
func undoTarget(c *EvictionCandidate) { c.UndoTarget = true }

func numbers(candidates []EvictionCandidate) []int64 {
	out := make([]int64, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.Number)
	}
	return out
}

func TestSelectEvictions(t *testing.T) {
	tests := []struct {
		name          string
		candidates    []EvictionCandidate
		limit         int
		wantEvicted   []int64
		wantOverLimit bool
	}{
		{
			name:        "under the limit evicts nothing",
			candidates:  with(revs(3), 3, active),
			limit:       10,
			wantEvicted: []int64{},
		},
		{
			name:        "exactly at the limit evicts nothing: the active revision does not count",
			candidates:  with(revs(3), 3, active),
			limit:       2,
			wantEvicted: []int64{},
		},
		{
			name:        "limit 0 keeps no old revision",
			candidates:  with(revs(5), 5, active),
			limit:       0,
			wantEvicted: []int64{1, 2, 3, 4},
		},
		{
			name:        "a negative limit is treated as 0",
			candidates:  with(revs(5), 5, active),
			limit:       -1,
			wantEvicted: []int64{1, 2, 3, 4},
		},
		{
			name: "limit 0 evicts the newest known-good revision and the undo target",
			candidates: with(with(with(revs(5), 5, active), 4, knownGood),
				3, undoTarget),
			limit:       0,
			wantEvicted: []int64{1, 2, 3, 4},
		},
		{
			name:        "evicts the oldest first",
			candidates:  with(revs(5), 5, active),
			limit:       2,
			wantEvicted: []int64{1, 2},
		},
		{
			name:        "never evicts the active revision, even when it is the oldest",
			candidates:  with(revs(4), 1, active),
			limit:       1,
			wantEvicted: []int64{2, 3},
		},
		{
			name:        "the only known-good revision is the oldest",
			candidates:  with(with(revs(5), 5, active), 1, knownGood),
			limit:       2,
			wantEvicted: []int64{2, 3},
		},
		{
			name:        "only the newest known-good revision is protected",
			candidates:  with(with(with(revs(5), 5, active), 1, knownGood), 3, knownGood),
			limit:       2,
			wantEvicted: []int64{1, 2},
		},
		{
			name:        "an active known-good revision is the newest known-good one",
			candidates:  with(with(with(revs(4), 4, active), 4, knownGood), 1, knownGood),
			limit:       1,
			wantEvicted: []int64{1, 2},
		},
		{
			name: "protected revisions alone exceed the limit: all kept, over limit reported",
			candidates: with(with(with(revs(3), 3, active), 2, knownGood),
				1, undoTarget),
			limit:         1,
			wantEvicted:   []int64{},
			wantOverLimit: true,
		},
		{
			name: "unprotected evicted, protected still over the limit",
			candidates: with(with(with(revs(5), 5, active), 4, knownGood),
				3, undoTarget),
			limit:         1,
			wantEvicted:   []int64{1, 2},
			wantOverLimit: true,
		},
		{
			name:        "limit 1 keeps the active revision and the newest old one",
			candidates:  with(revs(3), 3, active),
			limit:       1,
			wantEvicted: []int64{1},
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
			evicted, overLimit := SelectEvictions(tt.candidates, tt.limit)
			assert.Equal(t, tt.wantEvicted, numbers(evicted))
			assert.Equal(t, tt.wantOverLimit, overLimit)
		})
	}
}

func TestProtected(t *testing.T) {
	knownGood := func(c *EvictionCandidate) { c.KnownGood = true }
	active := func(c *EvictionCandidate) { c.Active = true }

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

// TestProtectedMatchesSelectEvictions checks that a revision deleted by hand is
// protected exactly when a history limit above zero would never delete it.
func TestProtectedMatchesSelectEvictions(t *testing.T) {
	candidates := with(with(with(revs(6), 6, func(c *EvictionCandidate) { c.Active = true }),
		2, func(c *EvictionCandidate) { c.KnownGood = true }),
		4, func(c *EvictionCandidate) { c.KnownGood = true })
	evicted, _ := SelectEvictions(candidates, 1)
	evictable := map[string]bool{}
	for _, c := range evicted {
		evictable[c.Name] = true
	}
	for _, c := range candidates {
		assert.Equal(t, evictable[c.Name], Protected(candidates, c) == "", c.Name)
	}
}

func TestSelectEvictions_SparesTheUndoTarget(t *testing.T) {
	// r1 is the revision active before r4 (a rollback to r1, then a new deploy):
	// the oldest, but the target of an undo.
	candidates := with(with(revs(4), 4, func(c *EvictionCandidate) { c.Active = true }),
		1, func(c *EvictionCandidate) { c.UndoTarget = true })
	evicted, _ := SelectEvictions(candidates, 2)
	assert.Equal(t, []string{"b-r2"}, names(evicted), "the oldest revision that is not the undo target goes")

	// The undo target stays deletable by hand.
	assert.Equal(t, Protection(""), Protected(candidates, candidates[0]))
}

func names(cs []EvictionCandidate) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}
