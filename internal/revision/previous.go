// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	"errors"
	"fmt"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrNoActiveRevision is returned when the binding has no revision yet.
	ErrNoActiveRevision = errors.New("the binding has no active revision")
	// ErrNoPreviousRevision is returned when the current revision was the
	// binding's first activation: nothing was active before it.
	ErrNoPreviousRevision = errors.New("no revision was active before the current one")
	// ErrPreviousRevisionDeleted is returned when the revision active before the
	// current one has been deleted, by hand or by the history limit.
	ErrPreviousRevisionDeleted = errors.New("the previously active revision has been deleted")
	// ErrPreviousRevisionUnknown is returned when the activation history cannot
	// say which revision was active before the current one.
	ErrPreviousRevisionUnknown = errors.New("the previously active revision cannot be determined")
)

// Activation is one of a binding's revisions, as PreviousActive sees it.
type Activation struct {
	Object client.Object
	Number int64
}

// PreviousActive returns the binding's current revision and the revision that
// was active immediately before the current one became active -- the target
// of a rollback that names none, as kubectl rollout undo.
//
// It follows activation history, never revision-number order: a rollback makes
// an older revision active again, so the previously active revision can be
// numbered above or below the current one. The current revision records the
// revision it replaced (AnnotationKeyPreviousRevision) each time it becomes
// active.
//
// A current revision recorded before that annotation existed falls back to
// activation order (AnnotationKeyBindingGeneration): the revision activated
// most recently before the current one. That is certainly the previously
// active revision when the history has no gap -- revisions numbered
// 1..highest all exist. With a gap, a deleted revision may have been active in
// between, so the result is a best inference and inferred explains why; the
// caller must say so. Two revisions sharing a generation leave no order to
// follow: ErrPreviousRevisionUnknown.
//
// A recorded previous revision is returned only when it still exists; a
// deleted one is ErrPreviousRevisionDeleted, never replaced by another.
func PreviousActive(revs []Activation, highest int64) (current, previous Activation, inferred string, err error) {
	if len(revs) == 0 {
		return Activation{}, Activation{}, "", ErrNoActiveRevision
	}
	cur := 0
	for i := range revs {
		if MoreRecentlyActive(revs[i].Object, revs[i].Number, revs[cur].Object, revs[cur].Number) {
			cur = i
		}
	}
	current = revs[cur]

	if raw, ok := current.Object.GetAnnotations()[AnnotationKeyPreviousRevision]; ok {
		n, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil || n < 0 {
			return current, Activation{}, "", fmt.Errorf("%w: revision %d records %s %q",
				ErrPreviousRevisionUnknown, current.Number, AnnotationKeyPreviousRevision, raw)
		}
		if n == 0 {
			return current, Activation{}, "", fmt.Errorf("%w: revision %d was the binding's first", ErrNoPreviousRevision, current.Number)
		}
		for _, r := range revs {
			if r.Number == n {
				return current, r, "", nil
			}
		}
		return current, Activation{}, "", fmt.Errorf("%w: revision %d, active before revision %d", ErrPreviousRevisionDeleted,
			n, current.Number)
	}
	previous, inferred, err = previousByGeneration(revs, cur, highest)
	return current, previous, inferred, err
}

// previousByGeneration is PreviousActive for a current revision without
// AnnotationKeyPreviousRevision.
func previousByGeneration(revs []Activation, cur int, highest int64) (Activation, string, error) {
	current := revs[cur]
	if len(revs) == 1 {
		return Activation{}, "", fmt.Errorf("%w: revision %d is the binding's only revision", ErrNoPreviousRevision,
			current.Number)
	}
	seen := make(map[int64]bool, len(revs))
	prev := -1
	for i := range revs {
		gen := recordedGeneration(revs[i].Object)
		if seen[gen] {
			return Activation{}, "", fmt.Errorf("%w: two revisions record activation generation %d",
				ErrPreviousRevisionUnknown, gen)
		}
		seen[gen] = true
		if i == cur {
			continue
		}
		if prev < 0 || MoreRecentlyActive(revs[i].Object, revs[i].Number, revs[prev].Object, revs[prev].Number) {
			prev = i
		}
	}
	inferred := ""
	if int64(len(revs)) != highest {
		inferred = fmt.Sprintf("revision %d predates activation tracking and %d of %d revisions remain, so revision %d "+
			"is the revision activated just before it; a deleted revision may have been active in between",
			current.Number, len(revs), highest, revs[prev].Number)
	}
	return revs[prev], inferred, nil
}
