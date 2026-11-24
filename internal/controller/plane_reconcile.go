// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"math/rand/v2"
	"slices"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// StatusUpdateJitter is the most that is added to StatusUpdateInterval for one
// requeue. Every plane CR is reconciled once per interval to refresh its agent
// connection status, and plane CRs created together would otherwise refresh in
// the same instant every interval, for as long as they exist.
const StatusUpdateJitter = 15 * time.Second

// StatusRequeueAfter returns the delay before the next periodic status
// refresh: StatusUpdateInterval plus up to StatusUpdateJitter.
func StatusRequeueAfter() time.Duration {
	//nolint:gosec // Breaking refresh synchrony, not a security decision.
	return StatusUpdateInterval + rand.N(StatusUpdateJitter)
}

// PlaneChangedPredicate drops the update events a plane controller causes
// itself by writing status. Each periodic refresh writes agent connection
// status, and without this filter that write queues the CR again, so every CR
// is reconciled twice per interval.
//
// An update passes when the spec, labels, annotations or finalizers changed, or
// when the CR was marked for deletion. Deletion and finalizer changes do not
// move metadata.generation, so they are checked explicitly: finalization must
// not wait for the next periodic refresh. A reconcile that relies on its own
// status write to run again has to requeue instead.
func PlaneChangedPredicate() predicate.Predicate {
	return predicate.Or(
		predicate.GenerationChangedPredicate{},
		predicate.LabelChangedPredicate{},
		predicate.AnnotationChangedPredicate{},
		predicate.Funcs{
			CreateFunc:  func(event.CreateEvent) bool { return false },
			DeleteFunc:  func(event.DeleteEvent) bool { return false },
			GenericFunc: func(event.GenericEvent) bool { return false },
			UpdateFunc: func(e event.UpdateEvent) bool {
				if e.ObjectOld == nil || e.ObjectNew == nil {
					return true
				}
				if e.ObjectOld.GetDeletionTimestamp().IsZero() != e.ObjectNew.GetDeletionTimestamp().IsZero() {
					return true
				}
				return !slices.Equal(e.ObjectOld.GetFinalizers(), e.ObjectNew.GetFinalizers())
			},
		},
	)
}
