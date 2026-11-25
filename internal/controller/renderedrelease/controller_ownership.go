// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// owner says whose an object on a plane is, seen from one release.
type owner int

const (
	// ownedOutside is an object no release applied: made by hand, by another tool or by an
	// operator.
	ownedOutside owner = iota
	// ownedByRelease is an object this release applied.
	ownedByRelease
	// ownedByAnotherRelease is an object a different release applied.
	ownedByAnotherRelease
)

// ownerOf returns whose a live object is. Two marks decide, and it takes both.
//
// Its managedFields hold a server-side apply entry from this controller's field manager.
// Every release applies as that one manager, so the entry says that some release applied
// the object, and the labels say which one.
//
// The labels alone say nothing. Operators copy labels onto the objects they create (ESO
// copies an ExternalSecret's labels onto its Secret when the ExternalSecret has no target
// template), and managedFields are not copied. Such an object belongs to its creator: the
// Secret of an ExternalSecret with creationPolicy Orphan outlives it, and one with Merge
// existed before it.
//
// The release is matched by name and not by UID, so that a release created again under
// its name, for example when the control plane is rebuilt, still owns its objects.
func ownerOf(live *unstructured.Unstructured, release *openchoreov1alpha1.RenderedRelease) owner {
	namespace := live.GetLabels()[labels.LabelKeyRenderedReleaseNamespace]
	name := live.GetLabels()[labels.LabelKeyRenderedReleaseName]
	switch {
	case namespace == "" || name == "" || !appliedByAnyRelease(live):
		return ownedOutside
	case namespace == release.Namespace && name == release.Name:
		return ownedByRelease
	default:
		return ownedByAnotherRelease
	}
}

// appliedByAnyRelease reports whether a live object has an apply entry from this
// controller.
func appliedByAnyRelease(live *unstructured.Unstructured) bool {
	for _, entry := range live.GetManagedFields() {
		if entry.Manager == ControllerName && entry.Operation == metav1.ManagedFieldsOperationApply {
			return true
		}
	}
	return false
}

// guardIsOn reports whether the release must not touch an existing object it doesn't own.
//
// A release that renders into its project's own namespace, or the Namespace itself, may
// adopt what it finds there and elsewhere. One that doesn't has no place of its own, such
// as a release in a namespace that existed before OpenChoreo, so whatever it finds is
// taken to be someone else's. The namespace is compared by its exact name, which the
// release's spec gives, so there is no field that could be left unset.
func guardIsOn(release *openchoreov1alpha1.RenderedRelease, desired []*unstructured.Unstructured, planeClient client.Client) bool {
	own := dpkubernetes.DataPlaneNamespace(release.Namespace, release.Spec.Owner.ProjectName, release.Spec.EnvironmentName)
	for _, obj := range desired {
		if gvk := obj.GroupVersionKind(); gvk.Group == "" && gvk.Kind == namespaceKind && obj.GetName() == own {
			return false
		}
		if identityOnPlane(obj, planeClient).namespace == own {
			return false
		}
	}
	return true
}

// maxReportedRefusals caps how many refused objects the ResourcesApplied condition names
// one by one.
const maxReportedRefusals = 10

// refusedError says which objects a release must not touch.
type refusedError struct {
	// objects names each object and whose it is.
	objects []string
}

func (e *refusedError) Error() string {
	named := e.objects[:min(len(e.objects), maxReportedRefusals)]
	msg := strings.Join(named, "; ")
	if extra := len(e.objects) - len(named); extra > 0 {
		msg += fmt.Sprintf("; and %d more", extra)
	}
	return msg
}

// checkedResource is a resource to apply, and what the check found in its place.
type checkedResource struct {
	desired *unstructured.Unstructured
	// claimed says that the release also lists the object in spec.observedResources.
	claimed bool
	// live is the object it is applied over. Nil when there was none, and when it was not
	// looked for.
	live *unstructured.Unstructured
	// takeOver says that live is claimed, and that no release applied it.
	takeOver bool
	// missing says that there was none, so applying it creates the object.
	missing bool
}

// checkOwnership looks for the objects a release is about to apply, before anything is
// written, and returns what it found. It looks for the ones that are claimed, and with the
// guard on for all of them. One that no release applied is taken over when it is claimed.
//
// It returns a refusedError when another release owns one of them, or when the guard is on
// and one that no release applied isn't claimed. It returns the error of a read that fails,
// since that object may be someone else's too, and of a takeover the plane would reject.
func checkOwnership(ctx context.Context, planeClient client.Client, release *openchoreov1alpha1.RenderedRelease,
	desired []*unstructured.Unstructured) ([]checkedResource, error) {
	guarded := guardIsOn(release, desired, planeClient)
	claims := sets.New[objectIdentity]()
	for _, claim := range resolveObservedResources(release.Spec.ObservedResources) {
		claims.Insert(claim.identity())
	}

	resources := make([]checkedResource, 0, len(desired))
	missing := sets.New[objectIdentity]()
	var refused []string
	for _, obj := range desired {
		identity := identityOnPlane(obj, planeClient)
		claimed := claims.Has(identity)
		if !claimed && !guarded {
			resources = append(resources, checkedResource{desired: obj})
			continue
		}
		live, err := readLive(ctx, planeClient, obj)
		if err != nil {
			return nil, fmt.Errorf("failed to check %s: %w", describeObject(obj), err)
		}
		if live == nil {
			// When a release renders an object twice, the first one creates it.
			resources = append(resources, checkedResource{desired: obj, claimed: claimed, missing: !missing.Has(identity)})
			missing.Insert(identity)
			continue
		}
		owner := ownerOf(live, release)
		resources = append(resources, checkedResource{desired: obj, claimed: claimed, live: live, takeOver: claimed && owner == ownedOutside})
		switch {
		case owner == ownedByAnotherRelease:
			refused = append(refused, fmt.Sprintf("%s belongs to %s", describeObject(live), describeOwner(live, release)))
		case owner == ownedOutside && !claimed:
			refused = append(refused, describeObject(live)+" exists and is not managed by this release")
		}
	}
	if len(refused) > 0 {
		return nil, &refusedError{objects: refused}
	}
	if err := dryRunTakeovers(ctx, planeClient, resources); err != nil {
		return nil, err
	}
	return resources, nil
}

// dryRunTakeovers has the plane run every takeover without storing it, and returns the ones
// it rejects.
//
// Resources are applied one by one, so a takeover that fails leaves the ones before it
// applied. A Service that selects the release's pods, next to a workload whose immutable
// selector can't change, would select nothing.
func dryRunTakeovers(ctx context.Context, planeClient client.Client, resources []checkedResource) error {
	var errs []error
	for _, resource := range resources {
		if !resource.takeOver {
			continue
		}
		dryRun := resource.desired.DeepCopy() // the answer replaces it
		dryRun.SetUID(resource.live.GetUID())
		if err := planeClient.Patch(ctx, dryRun, client.Apply, client.ForceOwnership,
			client.FieldOwner(ControllerName), client.DryRunAll); err != nil {
			errs = append(errs, fmt.Errorf("%s can't be taken over: %w", describeObject(resource.live), err))
			if ctx.Err() != nil {
				break
			}
		}
	}
	return kerrors.NewAggregate(errs)
}

// identityOnPlane returns the identity a rendered object has on the plane. A cluster-scoped
// object is in no namespace, even if its manifest sets one.
func identityOnPlane(obj *unstructured.Unstructured, planeClient client.Client) objectIdentity {
	identity := identityOf(obj)
	if namespaced, err := planeClient.IsObjectNamespaced(obj); err == nil && !namespaced {
		identity.namespace = ""
	}
	return identity
}

// readLive reads the object desired would be applied as, in desired's API version. It
// returns nil when there is none. ProxyClient answers the same for a kind the plane does
// not serve, and applying that fails anyway.
func readLive(ctx context.Context, planeClient client.Client, desired *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(desired.GroupVersionKind())
	err := planeClient.Get(ctx, client.ObjectKeyFromObject(desired), live)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return live, nil
}

// describeOwner names the release that owns a live object. Two organizations can use one
// namespace of a plane, so a release in another control plane namespace is not named.
func describeOwner(live *unstructured.Unstructured, release *openchoreov1alpha1.RenderedRelease) string {
	if live.GetLabels()[labels.LabelKeyRenderedReleaseNamespace] != release.Namespace {
		return "another RenderedRelease"
	}
	return "RenderedRelease " + live.GetLabels()[labels.LabelKeyRenderedReleaseName]
}
