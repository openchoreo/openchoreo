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
}

// SelectEvictions returns the revisions to delete so that at most limit old
// revisions remain, oldest by spec.revision first. Old revisions are all but the
// active one, which is always kept and does not count -- as a Deployment's
// revisionHistoryLimit counts old ReplicaSets. A limit of zero or less keeps no
// old revision.
//
// Only the revisions in candidates count: a revision deleted earlier, by hand or
// by a previous trim, is simply absent, and gaps in the numbering mean nothing.
// The active revision is the one marked Active, whatever its number, so after a
// rollback an old number can be active; it is skipped and the next-oldest old
// revision goes instead.
func SelectEvictions(candidates []EvictionCandidate, limit int) []EvictionCandidate {
	limit = max(limit, 0)

	old := make([]EvictionCandidate, 0, len(candidates))
	for _, c := range candidates {
		if !c.Active {
			old = append(old, c)
		}
	}
	excess := len(old) - limit
	if excess <= 0 {
		return nil
	}
	sort.Slice(old, func(i, j int) bool { return old[i].Number < old[j].Number })
	return old[:excess]
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

// Protected returns why target may not be deleted by hand, or "" when it may:
// the active revision, and the newest known-good revision, the rollback target
// of last resort. The history limit applies only the first rule; a limit that
// is lowered can delete the newest known-good revision. candidates are all of
// the binding's revisions, target among them.
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
