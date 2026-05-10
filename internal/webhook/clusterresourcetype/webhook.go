// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterresourcetype

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

//nolint:unused // exported via webhook bootstrap; keeps parity with sibling webhooks
var clusterresourcetypelog = logf.Log.WithName("clusterresourcetype-resource")

// SetupClusterResourceTypeWebhookWithManager registers the validating webhook
// for ClusterResourceType in the manager.
func SetupClusterResourceTypeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.ClusterResourceType{}).
		WithValidator(&Validator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-clusterresourcetype,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=clusterresourcetypes,verbs=create;update,versions=v1alpha1,name=vclusterresourcetype-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates ClusterResourceType resources.
// +kubebuilder:object:generate=false
type Validator struct{}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate runs the cluster-scoped resource-type spec validator.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	crt, ok := obj.(*openchoreodevv1alpha1.ClusterResourceType)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterResourceType object but got %T", obj)
	}
	clusterresourcetypelog.Info("Validation for ClusterResourceType upon creation", "name", crt.GetName())

	if errs := resourcevalidation.ValidateClusterResourceTypeSpec(&crt.Spec, field.NewPath("spec")); len(errs) > 0 {
		return nil, apierrors.NewInvalid(crt.GroupVersionKind().GroupKind(), crt.GetName(), errs)
	}
	return nil, nil
}

// ValidateUpdate re-runs the validator on the new object. Spec immutability
// for individual fields is enforced by CRD CEL markers; the rest of the spec
// is re-validated to catch CEL drift introduced by an update.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	if _, ok := oldObj.(*openchoreodevv1alpha1.ClusterResourceType); !ok {
		return nil, fmt.Errorf("expected a ClusterResourceType object for the oldObj but got %T", oldObj)
	}
	crt, ok := newObj.(*openchoreodevv1alpha1.ClusterResourceType)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterResourceType object for the newObj but got %T", newObj)
	}
	clusterresourcetypelog.Info("Validation for ClusterResourceType upon update", "name", crt.GetName())

	if errs := resourcevalidation.ValidateClusterResourceTypeSpec(&crt.Spec, field.NewPath("spec")); len(errs) > 0 {
		return nil, apierrors.NewInvalid(crt.GroupVersionKind().GroupKind(), crt.GetName(), errs)
	}
	return nil, nil
}

// ValidateDelete is a no-op. ClusterResourceType deletion is gated by RBAC
// and the runtime impact is bounded by the existing finalizer logic on
// consumers.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	crt, ok := obj.(*openchoreodevv1alpha1.ClusterResourceType)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterResourceType object but got %T", obj)
	}
	clusterresourcetypelog.Info("Validation for ClusterResourceType upon deletion", "name", crt.GetName())
	return nil, nil
}
