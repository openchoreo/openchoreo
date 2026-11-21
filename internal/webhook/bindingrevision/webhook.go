// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package bindingrevision validates updates to ReleaseBindingRevisions and
// ResourceReleaseBindingRevisions: a revision records one binding spec, and
// that record must not change after it is created.
//
// The CRDs carry a `self == oldSelf` CEL rule on spec, but CEL cannot compare
// the free-form config objects in a ReleaseBinding spec in full: they are
// declared `type: object` with preserved unknown fields and no properties, and
// CEL equality on them misses a key replaced by another key, for example
// {"replicas":2} becoming {"count":2}. This webhook compares the whole spec.
package bindingrevision

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ImmutableSpecMessage is the message an update that changes a revision's
// spec is rejected with. It matches the CRDs' CEL rule.
const ImmutableSpecMessage = "revision spec is immutable; only status may change"

// SetupReleaseBindingRevisionWebhookWithManager registers the validating
// webhook for ReleaseBindingRevision in the manager.
func SetupReleaseBindingRevisionWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &openchoreov1alpha1.ReleaseBindingRevision{}).
		WithCustomValidator(&ReleaseBindingRevisionValidator{}).
		Complete()
}

// SetupResourceReleaseBindingRevisionWebhookWithManager registers the
// validating webhook for ResourceReleaseBindingRevision in the manager.
func SetupResourceReleaseBindingRevisionWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &openchoreov1alpha1.ResourceReleaseBindingRevision{}).
		WithCustomValidator(&ResourceReleaseBindingRevisionValidator{}).
		Complete()
}

// Only updates can change a spec. Status is written through the status
// subresource, which these rules do not match.

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-releasebindingrevision,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=releasebindingrevisions,verbs=update,versions=v1alpha1,name=vreleasebindingrevision-v1alpha1.kb.io,admissionReviewVersions=v1

// ReleaseBindingRevisionValidator rejects updates that change a
// ReleaseBindingRevision's spec.
type ReleaseBindingRevisionValidator struct{}

var _ webhook.CustomValidator = &ReleaseBindingRevisionValidator{}

// ValidateCreate accepts every create; the CRD schema validates the spec.
func (v *ReleaseBindingRevisionValidator) ValidateCreate(context.Context, runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate rejects an update whose spec differs from the stored one.
func (v *ReleaseBindingRevisionValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldRev, ok := oldObj.(*openchoreov1alpha1.ReleaseBindingRevision)
	if !ok {
		return nil, fmt.Errorf("expected a ReleaseBindingRevision but got %T", oldObj)
	}
	newRev, ok := newObj.(*openchoreov1alpha1.ReleaseBindingRevision)
	if !ok {
		return nil, fmt.Errorf("expected a ReleaseBindingRevision but got %T", newObj)
	}
	return nil, rejectSpecChange(oldRev.Spec, newRev.Spec,
		openchoreov1alpha1.GroupVersion.WithKind("ReleaseBindingRevision").GroupKind(), newRev.Name)
}

// ValidateDelete accepts every delete.
func (v *ReleaseBindingRevisionValidator) ValidateDelete(context.Context, runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-resourcereleasebindingrevision,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=resourcereleasebindingrevisions,verbs=update,versions=v1alpha1,name=vresourcereleasebindingrevision-v1alpha1.kb.io,admissionReviewVersions=v1

// ResourceReleaseBindingRevisionValidator rejects updates that change a
// ResourceReleaseBindingRevision's spec.
type ResourceReleaseBindingRevisionValidator struct{}

var _ webhook.CustomValidator = &ResourceReleaseBindingRevisionValidator{}

// ValidateCreate accepts every create; the CRD schema validates the spec.
func (v *ResourceReleaseBindingRevisionValidator) ValidateCreate(context.Context, runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate rejects an update whose spec differs from the stored one.
func (v *ResourceReleaseBindingRevisionValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldRev, ok := oldObj.(*openchoreov1alpha1.ResourceReleaseBindingRevision)
	if !ok {
		return nil, fmt.Errorf("expected a ResourceReleaseBindingRevision but got %T", oldObj)
	}
	newRev, ok := newObj.(*openchoreov1alpha1.ResourceReleaseBindingRevision)
	if !ok {
		return nil, fmt.Errorf("expected a ResourceReleaseBindingRevision but got %T", newObj)
	}
	return nil, rejectSpecChange(oldRev.Spec, newRev.Spec,
		openchoreov1alpha1.GroupVersion.WithKind("ResourceReleaseBindingRevision").GroupKind(), newRev.Name)
}

// ValidateDelete accepts every delete.
func (v *ResourceReleaseBindingRevisionValidator) ValidateDelete(context.Context, runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// rejectSpecChange returns an Invalid error when the two specs differ.
func rejectSpecChange(oldSpec, newSpec any, gk schema.GroupKind, name string) error {
	same, err := sameJSON(oldSpec, newSpec)
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("compare revision specs: %w", err))
	}
	if same {
		return nil
	}
	return apierrors.NewInvalid(gk, name, field.ErrorList{
		field.Forbidden(field.NewPath("spec"), ImmutableSpecMessage),
	})
}

// sameJSON reports whether a and b serialize to the same JSON document. Both
// are decoded into generic values before comparing, so key order and
// formatting in the free-form fields (raw JSON) do not matter, while any key
// added, removed or renamed at any depth does.
func sameJSON(a, b any) (bool, error) {
	av, err := toGeneric(a)
	if err != nil {
		return false, err
	}
	bv, err := toGeneric(b)
	if err != nil {
		return false, err
	}
	return reflect.DeepEqual(av, bv), nil
}

func toGeneric(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
