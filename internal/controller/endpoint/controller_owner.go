// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	kindReleaseLine = "ReleaseLine"
	kindComponent   = "Component"
	kindProject     = "Project"
)

// ensureOwnerReference keeps exactly one ReleaseLine, Component or Project
// owner reference on the Endpoint, as named by spec.owner, so Kubernetes
// garbage collection removes the Endpoint with its owner.
//
// spec.owner.releaseLineName makes the ReleaseLine the owner,
// spec.owner.componentName the Component; without either the Project is. The
// CRD only accepts a component and line every backend routes to, so deleting
// one never deletes a route that still serves another. The owner is recomputed
// from the spec on every reconcile, so an Endpoint whose spec moves from its
// line to its component, or to its project, changes owner with it.
//
// Returns true when the Endpoint was updated; the update re-enqueues it.
func (r *Reconciler) ensureOwnerReference(ctx context.Context, ep *openchoreov1alpha1.Endpoint) (bool, error) {
	owner, err := r.desiredOwner(ctx, ep)
	if err != nil || owner == nil {
		return false, err
	}

	refs := make([]metav1.OwnerReference, 0, len(ep.OwnerReferences))
	hasOwner := false
	for _, ref := range ep.OwnerReferences {
		if ref.UID == owner.GetUID() {
			hasOwner = true
			refs = append(refs, ref)
			continue
		}
		if isManagedOwnerKind(ref) {
			continue
		}
		refs = append(refs, ref)
	}
	if hasOwner && len(refs) == len(ep.OwnerReferences) {
		return false, nil
	}

	ep.OwnerReferences = refs
	if !hasOwner {
		if err := controllerutil.SetOwnerReference(owner, ep, r.Scheme); err != nil {
			return false, fmt.Errorf("set owner reference: %w", err)
		}
	}
	if err := r.Update(ctx, ep); err != nil {
		return false, fmt.Errorf("update Endpoint owner references: %w", err)
	}
	log.FromContext(ctx).Info("Updated Endpoint owner",
		"kind", owner.GetObjectKind().GroupVersionKind().Kind, "owner", owner.GetName())
	return true, nil
}

// desiredOwner returns the ReleaseLine, Component or Project that should own
// ep, or nil to leave its owner references untouched.
//
// It tries the narrowest owner spec.owner names first. One that is going away,
// or already gone while the Endpoint still names it as owner, returns nil so
// garbage collection can finish deleting the Endpoint instead of this
// controller rescuing it into a wider owner. One that does not exist yet
// (GitOps applying the Endpoint first), or that belongs elsewhere, falls back
// to the next wider owner; the ReleaseLine and Component watches re-enqueue the
// Endpoint once it lands.
func (r *Reconciler) desiredOwner(ctx context.Context, ep *openchoreov1alpha1.Endpoint) (client.Object, error) {
	owner := ep.Spec.Owner
	if owner.ComponentName != "" && owner.ReleaseLineName != "" {
		rl := &openchoreov1alpha1.ReleaseLine{}
		obj, done, err := r.ownerCandidate(ctx, ep, kindReleaseLine, owner.ReleaseLineName, rl,
			func() bool {
				return rl.Spec.Owner.ProjectName == owner.ProjectName && rl.Spec.Owner.ComponentName == owner.ComponentName
			})
		if err != nil || done {
			return obj, err
		}
	}

	if owner.ComponentName != "" {
		comp := &openchoreov1alpha1.Component{}
		obj, done, err := r.ownerCandidate(ctx, ep, kindComponent, owner.ComponentName, comp,
			func() bool { return comp.Spec.Owner.ProjectName == owner.ProjectName })
		if err != nil || done {
			return obj, err
		}
	}

	project := &openchoreov1alpha1.Project{}
	err := r.Get(ctx, types.NamespacedName{Namespace: ep.Namespace, Name: owner.ProjectName}, project)
	switch {
	case apierrors.IsNotFound(err):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("get Project %q: %w", owner.ProjectName, err)
	}
	return withGVK(project, kindProject), nil
}

// ownerCandidate resolves one owner named by spec.owner into obj. done reports
// that the search ends here: with the candidate as the owner, or with nil to
// leave the owner references to garbage collection. When done is false the
// caller tries the next wider owner. belongs reports whether the fetched object
// belongs to the Endpoint's owners above it.
func (r *Reconciler) ownerCandidate(
	ctx context.Context, ep *openchoreov1alpha1.Endpoint, kind, name string,
	obj client.Object, belongs func() bool,
) (client.Object, bool, error) {
	err := r.Get(ctx, types.NamespacedName{Namespace: ep.Namespace, Name: name}, obj)
	switch {
	case apierrors.IsNotFound(err):
		if ownedBy(ep, kind, name) {
			return nil, true, nil
		}
		return nil, false, nil
	case err != nil:
		return nil, true, fmt.Errorf("get %s %q: %w", kind, name, err)
	case !obj.GetDeletionTimestamp().IsZero():
		return nil, true, nil
	case ownedByAnother(ep, kind, name, obj.GetUID()):
		// The owner was deleted and one of the same name created before garbage
		// collection removed this Endpoint. It belonged to the deleted object, so
		// it is left to garbage collection rather than handed to its successor.
		return nil, true, nil
	case belongs():
		return withGVK(obj, kind), true, nil
	default:
		log.FromContext(ctx).Info("Owner named in spec.owner belongs elsewhere; a wider owner owns the Endpoint",
			"kind", kind, "name", name)
		return nil, false, nil
	}
}

// ownedBy reports whether ep carries an owner reference to the named object
// of a kind this controller manages.
func ownedBy(ep *openchoreov1alpha1.Endpoint, kind, name string) bool {
	for _, ref := range ep.OwnerReferences {
		if isManagedOwnerKind(ref) && ref.Kind == kind && ref.Name == name {
			return true
		}
	}
	return false
}

// ownedByAnother reports whether ep carries an owner reference to an object of
// the given kind and name whose UID is not uid: an earlier object of that name.
func ownedByAnother(ep *openchoreov1alpha1.Endpoint, kind, name string, uid types.UID) bool {
	for _, ref := range ep.OwnerReferences {
		if isManagedOwnerKind(ref) && ref.Kind == kind && ref.Name == name && ref.UID != uid {
			return true
		}
	}
	return false
}

// isManagedOwnerKind reports whether ref points at a kind whose ownership this
// controller manages, so a stale one can be replaced.
func isManagedOwnerKind(ref metav1.OwnerReference) bool {
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil || gv.Group != openchoreov1alpha1.GroupVersion.Group {
		return false
	}
	return ref.Kind == kindReleaseLine || ref.Kind == kindComponent || ref.Kind == kindProject
}

// withGVK stamps the GroupVersionKind on an object read through the typed
// client, which leaves TypeMeta empty, so log lines can name the kind.
func withGVK(obj client.Object, kind string) client.Object {
	obj.GetObjectKind().SetGroupVersionKind(openchoreov1alpha1.GroupVersion.WithKind(kind))
	return obj
}
