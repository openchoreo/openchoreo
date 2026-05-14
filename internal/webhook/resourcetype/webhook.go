// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcetype

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	resourcevalidation "github.com/openchoreo/openchoreo/internal/validation/resource"
)

// log is for logging in this package.
//
//nolint:unused // exported via webhook bootstrap; keeps parity with sibling webhooks
var resourcetypelog = logf.Log.WithName("resourcetype-resource")

// SetupResourceTypeWebhookWithManager registers the validating webhook for
// ResourceType in the manager.
func SetupResourceTypeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.ResourceType{}).
		WithValidator(&Validator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-resourcetype,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=resourcetypes,verbs=create;update,versions=v1alpha1,name=vresourcetype-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates ResourceType resources.
// +kubebuilder:object:generate=false
type Validator struct{}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate runs the resource-type spec validator on creation.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rt, ok := obj.(*openchoreodevv1alpha1.ResourceType)
	if !ok {
		return nil, fmt.Errorf("expected a ResourceType object but got %T", obj)
	}
	resourcetypelog.Info("Validation for ResourceType upon creation", "name", rt.GetName())

	if errs := resourcevalidation.ValidateResourceTypeSpec(&rt.Spec, field.NewPath("spec")); len(errs) > 0 {
		return nil, apierrors.NewInvalid(rt.GroupVersionKind().GroupKind(), rt.GetName(), errs)
	}
	return nil, nil
}

// ValidateUpdate re-runs the validator on the new object. Spec immutability
// for individual fields is enforced by CRD CEL markers; the rest of the spec
// is re-validated to catch CEL drift introduced by an update.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	if _, ok := oldObj.(*openchoreodevv1alpha1.ResourceType); !ok {
		return nil, fmt.Errorf("expected a ResourceType object for the oldObj but got %T", oldObj)
	}
	rt, ok := newObj.(*openchoreodevv1alpha1.ResourceType)
	if !ok {
		return nil, fmt.Errorf("expected a ResourceType object for the newObj but got %T", newObj)
	}
	resourcetypelog.Info("Validation for ResourceType upon update", "name", rt.GetName())

	if errs := resourcevalidation.ValidateResourceTypeSpec(&rt.Spec, field.NewPath("spec")); len(errs) > 0 {
		return nil, apierrors.NewInvalid(rt.GroupVersionKind().GroupKind(), rt.GetName(), errs)
	}
	return nil, nil
}

// ValidateDelete is a no-op. ResourceType deletion is gated by RBAC and the
// runtime impact is bounded by the existing finalizer logic on consumers.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rt, ok := obj.(*openchoreodevv1alpha1.ResourceType)
	if !ok {
		return nil, fmt.Errorf("expected a ResourceType object but got %T", obj)
	}
	resourcetypelog.Info("Validation for ResourceType upon deletion", "name", rt.GetName())
	return nil, nil
}
