// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import "sort"

// EvictionCandidate is the part of a revision the history limit looks at.
type EvictionCandidate struct {
	Name      string
	Number    int64
	Active    bool
	KnownGood bool
	// UndoTarget is true for the revision that was active before the active one
	// (AnnotationKeyPreviousRevision on the active revision): the target of a
	// rollback that names no revision. Only the history limit spares it; it may
	// still be deleted by hand.
	UndoTarget bool
}

// SelectEvictions returns the revisions to delete so that at most limit old
// revisions remain, oldest by spec.revision first. Old revisions are all but the
// active one, which is always kept and does not count -- as a Deployment's
// revisionHistoryLimit counts old ReplicaSets. A limit of zero or less keeps no
// old revision.
//
// While the limit is above zero, the newest known-good revision (the rollback
// target of last resort) and the UndoTarget are never selected -- otherwise a
// rollback to an old revision followed by a new deploy would evict the very
// revision an undo returns to. When the
// spared revisions alone outnumber the limit, every other old revision is
// selected, the spared ones stay, and overLimit reports that the history is
// still above the limit.
//
// A limit of zero asks for no history at all, so the newest known-good revision
// and the UndoTarget are selected too.
func SelectEvictions(candidates []EvictionCandidate, limit int) (evict []EvictionCandidate, overLimit bool) {
	limit = max(limit, 0)

	sorted := append([]EvictionCandidate(nil), candidates...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Number < sorted[j].Number })

	old := 0
	for _, c := range sorted {
		if !c.Active {
			old++
		}
	}
	excess := old - limit
	if excess <= 0 {
		return nil, false
	}

	newestKnownGood := newestKnownGood(sorted)
	for _, c := range sorted {
		if excess == 0 {
			break
		}
		if c.Active {
			continue
		}
		if limit > 0 && (c.UndoTarget || c.Number == newestKnownGood) {
			continue
		}
		evict = append(evict, c)
		excess--
	}
	return evict, excess > 0
}

// Protection is why a revision may not be deleted, by the history limit or by
// hand. It reads as the end of "cannot delete revision N because ...".
type Protection string

const (
	// ProtectionActive is the revision the binding runs.
	ProtectionActive Protection = "it is currently active"
	// ProtectionNewestKnownGood is the binding's rollback target of last resort.
	ProtectionNewestKnownGood Protection = "it is the newest known-good revision, kept as the rollback target of last resort"
)

// Protected returns why target may not be deleted by hand, or "" when it may.
// These are the revisions SelectEvictions spares under a limit above zero,
// except the UndoTarget.
// The UndoTarget is left deletable on purpose: it is often the failed revision
// a user just rolled back from and wants gone. candidates are all of the
// binding's revisions, target among them.
func Protected(candidates []EvictionCandidate, target EvictionCandidate) Protection {
	switch {
	case target.Active:
		return ProtectionActive
	case target.KnownGood && target.Number == newestKnownGood(candidates):
		return ProtectionNewestKnownGood
	}
	return ""
}

// newestKnownGood returns the highest number among the known-good candidates,
// or -1 when none is known good.
func newestKnownGood(candidates []EvictionCandidate) int64 {
	newest := int64(-1)
	for _, c := range candidates {
		if c.KnownGood && c.Number > newest {
			newest = c.Number
		}
	}
	return newest
}
