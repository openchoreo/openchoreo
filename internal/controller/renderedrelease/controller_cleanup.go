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
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// cleanup is what happens to the live objects a release owns and no longer renders.
type cleanup struct {
	// giveBack are the kept objects: claimed, or labeled because they were claimed once.
	// They are never deleted.
	giveBack []*unstructured.Unstructured
	// remove are the rest, which the release created, or adopted without a claim.
	remove []*unstructured.Unstructured
}

// planCleanup returns what to do with the live objects. One the release doesn't own is left
// alone, and so is one it still renders.
//
// The claims count as they are in the spec, whether or not the objects could be read. An
// object is looked at once, though it is listed once per API version the controller knows
// for its kind.
func planCleanup(release *openchoreov1alpha1.RenderedRelease, rendered renderedSet, live []*unstructured.Unstructured) cleanup {
	claimed := sets.New[objectIdentity]()
	for _, claim := range resolveObservedResources(release.Spec.ObservedResources) {
		claimed.Insert(claim.identity())
	}
	var plan cleanup
	seen := sets.New[objectIdentity]()
	for _, obj := range live {
		identity := identityOf(obj)
		if seen.Has(identity) || rendered.objects.Has(identity) || ownerOf(obj, release) != ownedByRelease {
			continue
		}
		seen.Insert(identity)
		switch {
		case claimed.Has(identity) || hasClaimedLabel(obj):
			plan.giveBack = append(plan.giveBack, obj)
		case !rendered.ids.Has(obj.GetLabels()[labels.LabelKeyRenderedReleaseResourceID]):
			plan.remove = append(plan.remove, obj)
		}
	}
	return plan
}

// renderedSet says what a release renders. Its zero value is for a release that renders
// nothing any more, because it is being deleted.
type renderedSet struct {
	objects sets.Set[objectIdentity]
	// ids are the resource IDs. An object goes stale by its ID and not by its name, so one
	// that was renamed under its ID is kept unless it is claimed.
	ids sets.Set[string]
}

func renderedSetOf(desired []*unstructured.Unstructured) renderedSet {
	rendered := renderedSet{
		objects: sets.New[objectIdentity](),
		// An object without an ID can't be told apart from a rendered one, and is kept.
		ids: sets.New(""),
	}
	for _, obj := range desired {
		rendered.objects.Insert(identityOf(obj))
		rendered.ids.Insert(obj.GetLabels()[labels.LabelKeyRenderedReleaseResourceID])
	}
	return rendered
}

// cleanUp carries out a plan. It tries all of it, and reports what fails together. The next
// pass plans again from what is there then.
func (r *Reconciler) cleanUp(ctx context.Context, planeClient client.Client, plan cleanup) error {
	var errs []error
	for _, obj := range plan.giveBack {
		if err := giveBack(ctx, planeClient, obj); err != nil {
			errs = append(errs, err)
			if ctx.Err() != nil {
				break
			}
			continue
		}
		log.FromContext(ctx).Info("Gave back an object that is or was claimed", "object", describeObject(obj))
	}
	errs = append(errs, r.deleteResources(ctx, planeClient, plan.remove))
	return kerrors.NewAggregate(errs)
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
