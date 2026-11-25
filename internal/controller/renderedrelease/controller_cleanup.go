// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// findStaleResources returns the live resources to delete: what this release applied and
// no longer renders. Observed objects are never among them, even ones this release
// applied before they were listed as observed.
//
// The same object is listed once per API version the controller knows for its kind, and
// is returned once.
func findStaleResources(release *openchoreov1alpha1.RenderedRelease, liveResources []*unstructured.Unstructured,
	observed []observedEntry, isRendered func(*unstructured.Unstructured) bool) []*unstructured.Unstructured {
	isObserved := sets.New[objectIdentity]()
	for _, entry := range observed {
		isObserved.Insert(entry.identity())
	}
	seen := sets.New[objectIdentity]()
	var stale []*unstructured.Unstructured
	for _, live := range liveResources {
		identity := identityOf(live)
		if isObserved.Has(identity) || seen.Has(identity) || isRendered(live) || !appliedByRelease(live, release) {
			continue
		}
		seen.Insert(identity)
		stale = append(stale, live)
	}
	return stale
}

// renderedIn returns whether a live resource is among the desired ones. It goes by the
// resource ID, so an object renamed under its ID is kept. An object without an ID cannot
// be told apart from a rendered one, and is kept too.
func renderedIn(desiredResources []*unstructured.Unstructured) func(*unstructured.Unstructured) bool {
	ids := sets.New[string]()
	for _, obj := range desiredResources {
		ids.Insert(obj.GetLabels()[labels.LabelKeyRenderedReleaseResourceID])
	}
	return func(live *unstructured.Unstructured) bool {
		id := live.GetLabels()[labels.LabelKeyRenderedReleaseResourceID]
		return id == "" || ids.Has(id)
	}
}

// nothingIsRendered is the isRendered of a release that is being deleted.
func nothingIsRendered(*unstructured.Unstructured) bool { return false }

// appliedByRelease reports whether this release applied the live resource: its
// managedFields hold a server-side apply entry from this controller's field manager, and
// its labels name the release.
//
// The labels alone do not say that. Operators copy labels onto the objects they create
// (ESO copies an ExternalSecret's labels onto its Secret when the ExternalSecret has no
// target template), and such an object is listed as if it were ours. managedFields are
// not copied, and applyResources records an Apply entry under ControllerName on
// everything it writes. Deleting only what carries that entry leaves those objects to
// their owner: the Secret of an ExternalSecret with creationPolicy Orphan outlives it,
// and one with Merge existed before it.
//
// The entry alone does not say it either, since every release applies as ControllerName.
func appliedByRelease(live *unstructured.Unstructured, release *openchoreov1alpha1.RenderedRelease) bool {
	liveLabels := live.GetLabels()
	if liveLabels[labels.LabelKeyRenderedReleaseNamespace] != release.Namespace ||
		liveLabels[labels.LabelKeyRenderedReleaseName] != release.Name {
		return false
	}
	for _, entry := range live.GetManagedFields() {
		if entry.Manager == ControllerName && entry.Operation == metav1.ManagedFieldsOperationApply {
			return true
		}
	}
	return false
}

// deleteResources deletes the given stale resources from the target plane. It tries all
// of them and reports the ones that fail together.
func (r *Reconciler) deleteResources(ctx context.Context, planeClient client.Client, staleResources []*unstructured.Unstructured) error {
	var errs []error
	for _, obj := range staleResources {
		// Skip resources already terminating on the target plane (re-deleting over
		// the gateway tunnel is a wasted round-trip on the most expensive I/O path).
		if obj.GetDeletionTimestamp() != nil {
			continue
		}

		// An object recreated since it was listed has another UID and may be someone
		// else's, so it is kept and the next pass looks again.
		uid := obj.GetUID()
		if err := planeClient.Delete(ctx, obj, client.Preconditions{UID: &uid}); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete stale resource %s: %w",
				obj.GetLabels()[labels.LabelKeyRenderedReleaseResourceID], err))
			if ctx.Err() != nil {
				break
			}
		}
	}
	return kerrors.NewAggregate(errs)
}
