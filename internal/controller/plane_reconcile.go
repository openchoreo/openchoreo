// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"math/rand/v2"
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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

// RecordAgentHeartbeat sets status.LastHeartbeatTime from the LastSeen a
// gateway reported, without ever moving it backwards or clearing it.
//
// The status request can land on any gateway replica. A replica that does not
// own the agent's connection may report the agent connected with a zero
// LastSeen (it predates published liveness, or has not yet received the
// owner's refresh), or one older than a previous reading from the owner. Such a
// reading says nothing new about the agent, so the recorded heartbeat stays.
func RecordAgentHeartbeat(status *openchoreov1alpha1.AgentConnectionStatus, lastSeen time.Time) {
	if status == nil || lastSeen.IsZero() {
		return
	}
	// LastHeartbeatTime is stored at second precision, so compare at that
	// precision: a sub-second-later reading of the same heartbeat is not news.
	lastSeen = lastSeen.Truncate(time.Second)
	if status.LastHeartbeatTime != nil && !lastSeen.After(status.LastHeartbeatTime.Time) {
		return
	}
	status.LastHeartbeatTime = &metav1.Time{Time: lastSeen}
}
