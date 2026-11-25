// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"fmt"

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
		if isObserved.Has(identity) || seen.Has(identity) || isRendered(live) || ownerOf(live, release) != ownedByRelease {
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
