// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"maps"
	"slices"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// requeuePriorityActive is the queue priority for a release whose resources are
	// still settling (Progressing or Unknown), so a rollout's health is picked up as
	// promptly as a real change. Watch events for objects that changed arrive at
	// this priority too.
	requeuePriorityActive = 0

	// requeuePriorityPoll is the queue priority for the periodic checks of releases
	// that are stable, scaled to zero, or only Degraded. It is the priority
	// controller-runtime gives unchanged objects from the initial list and resyncs,
	// so a poll never delays a real change.
	requeuePriorityPoll = handler.LowPriority
)

// ignoreStatusOnlyUpdates drops update events that changed nothing but status,
// which in practice are this controller's own status writes. The status mirrors
// live plane objects whose status can change on every check (an ExternalSecret's
// refreshTime, for one). Re-queuing on those writes kept such releases
// reconciling back to back at priority 0 and starved every other release, so the
// reconcile schedules its own next check instead (RequeueAfter).
//
// RenderedRelease has a status subresource, so a status write cannot change any
// field compared here. Spec changes (generation), deletion, labels, annotations
// (including the restartedAt annotation), finalizers and owner references pass.
// Resyncs (unchanged resourceVersion) pass as a safety net for a release with no
// requeue scheduled; controller-runtime queues them at low priority. Create,
// delete and generic events pass.
func ignoreStatusOnlyUpdates() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return true
			}
			o, n := e.ObjectOld, e.ObjectNew
			return o.GetResourceVersion() == n.GetResourceVersion() ||
				o.GetGeneration() != n.GetGeneration() ||
				!o.GetDeletionTimestamp().Equal(n.GetDeletionTimestamp()) ||
				!maps.Equal(o.GetLabels(), n.GetLabels()) ||
				!maps.Equal(o.GetAnnotations(), n.GetAnnotations()) ||
				!slices.Equal(o.GetFinalizers(), n.GetFinalizers()) ||
				!apiequality.Semantic.DeepEqual(o.GetOwnerReferences(), n.GetOwnerReferences())
		},
	}
}

// transitioningRequeuePriority returns the queue priority for a transitioning
// release. Progressing resources, and applied ones still Unknown (just applied),
// settle on their own and keep priority 0. Degraded ones, and observed ones that
// are Unknown (missing or unreadable), may last, so they poll at low priority.
func transitioningRequeuePriority(applied, observed []openchoreov1alpha1.RenderedManifestStatus) int {
	for _, res := range applied {
		if res.HealthStatus == openchoreov1alpha1.HealthStatusProgressing ||
			res.HealthStatus == openchoreov1alpha1.HealthStatusUnknown {
			return requeuePriorityActive
		}
	}
	for _, res := range observed {
		if res.HealthStatus == openchoreov1alpha1.HealthStatusProgressing {
			return requeuePriorityActive
		}
	}
	return requeuePriorityPoll
}
