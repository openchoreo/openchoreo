// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterendpointtype

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
	endpointvalidation "github.com/openchoreo/openchoreo/internal/validation/endpoint"
)

var clusterendpointtypelog = logf.Log.WithName("clusterendpointtype-resource")

// SetupClusterEndpointTypeWebhookWithManager registers the validating webhook
// for ClusterEndpointType in the manager.
func SetupClusterEndpointTypeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &openchoreodevv1alpha1.ClusterEndpointType{}).
		WithCustomValidator(&Validator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-clusterendpointtype,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=clusterendpointtypes,verbs=create;update,versions=v1alpha1,name=vclusterendpointtype-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates ClusterEndpointType resources.
// +kubebuilder:object:generate=false
type Validator struct{}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate runs the endpoint-type spec validator on creation.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cet, ok := obj.(*openchoreodevv1alpha1.ClusterEndpointType)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterEndpointType object but got %T", obj)
	}
	clusterendpointtypelog.Info("Validation for ClusterEndpointType upon creation", "name", cet.GetName())
	return nil, validate(cet)
}

// ValidateUpdate re-runs the validator on the new object. Endpoints pin a
// snapshot of the type in their EndpointRelease, so an update affects only
// releases cut after it; validating it here keeps those releases renderable.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	if _, ok := oldObj.(*openchoreodevv1alpha1.ClusterEndpointType); !ok {
		return nil, fmt.Errorf("expected a ClusterEndpointType object for the oldObj but got %T", oldObj)
	}
	cet, ok := newObj.(*openchoreodevv1alpha1.ClusterEndpointType)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterEndpointType object for the newObj but got %T", newObj)
	}
	clusterendpointtypelog.Info("Validation for ClusterEndpointType upon update", "name", cet.GetName())
	return nil, validate(cet)
}

// ValidateDelete is a no-op. Existing Endpoints keep rendering from the
// snapshot pinned in their EndpointRelease.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	if _, ok := obj.(*openchoreodevv1alpha1.ClusterEndpointType); !ok {
		return nil, fmt.Errorf("expected a ClusterEndpointType object but got %T", obj)
	}
	return nil, nil
}

func validate(cet *openchoreodevv1alpha1.ClusterEndpointType) error {
	if errs := endpointvalidation.ValidateClusterEndpointTypeSpec(&cet.Spec, field.NewPath("spec")); len(errs) > 0 {
		return apierrors.NewInvalid(cet.GroupVersionKind().GroupKind(), cet.GetName(), errs)
	}
	return nil
}
