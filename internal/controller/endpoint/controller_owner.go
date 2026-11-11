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

// ensureOwnerReference keeps exactly one Component or Project owner reference
// on the Endpoint, as named by spec.owner, so Kubernetes garbage collection
// removes the Endpoint with its owner.
//
// spec.owner.componentName makes the Component the owner; without it the
// Project is. The CRD only accepts a component every backend routes to, so
// deleting a component never deletes a route that still serves another one.
// The owner is recomputed from the spec on every reconcile, so an Endpoint
// whose spec moves from its component to its project changes owner with it.
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

// desiredOwner returns the Component or Project that should own ep, or nil to
// leave its owner references untouched.
//
// An owning component that is going away, or already gone while the Endpoint
// still names it as owner, returns nil so garbage collection can finish
// deleting the Endpoint instead of this controller rescuing it into the
// project. One that does not exist yet (GitOps applying the Endpoint first), or
// that belongs to another project, falls back to the project; the Component
// watch re-enqueues the Endpoint once the component lands.
func (r *Reconciler) desiredOwner(ctx context.Context, ep *openchoreov1alpha1.Endpoint) (client.Object, error) {
	if name := ep.Spec.Owner.ComponentName; name != "" {
		comp := &openchoreov1alpha1.Component{}
		err := r.Get(ctx, types.NamespacedName{Namespace: ep.Namespace, Name: name}, comp)
		switch {
		case apierrors.IsNotFound(err):
			if ownedBy(ep, "Component", name) {
				return nil, nil
			}
		case err != nil:
			return nil, fmt.Errorf("get Component %q: %w", name, err)
		case !comp.DeletionTimestamp.IsZero():
			return nil, nil
		case comp.Spec.Owner.ProjectName == ep.Spec.Owner.ProjectName:
			return withGVK(comp, "Component"), nil
		default:
			log.FromContext(ctx).Info("Owning component belongs to another project; the project owns the Endpoint",
				"component", name, "componentProject", comp.Spec.Owner.ProjectName)
		}
	}

	project := &openchoreov1alpha1.Project{}
	err := r.Get(ctx, types.NamespacedName{Namespace: ep.Namespace, Name: ep.Spec.Owner.ProjectName}, project)
	switch {
	case apierrors.IsNotFound(err):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("get Project %q: %w", ep.Spec.Owner.ProjectName, err)
	}
	return withGVK(project, "Project"), nil
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

// isManagedOwnerKind reports whether ref points at a kind whose ownership this
// controller manages, so a stale one can be replaced.
func isManagedOwnerKind(ref metav1.OwnerReference) bool {
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil || gv.Group != openchoreov1alpha1.GroupVersion.Group {
		return false
	}
	return ref.Kind == "Component" || ref.Kind == "Project"
}

// withGVK stamps the GroupVersionKind on an object read through the typed
// client, which leaves TypeMeta empty, so log lines can name the kind.
func withGVK(obj client.Object, kind string) client.Object {
	obj.GetObjectKind().SetGroupVersionKind(openchoreov1alpha1.GroupVersion.WithKind(kind))
	return obj
}
