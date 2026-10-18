// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointtype

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

var endpointtypelog = logf.Log.WithName("endpointtype-resource")

// SetupEndpointTypeWebhookWithManager registers the validating webhook for
// EndpointType in the manager.
func SetupEndpointTypeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &openchoreodevv1alpha1.EndpointType{}).
		WithCustomValidator(&Validator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-endpointtype,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=endpointtypes,verbs=create;update,versions=v1alpha1,name=vendpointtype-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates EndpointType resources.
// +kubebuilder:object:generate=false
type Validator struct{}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate runs the endpoint-type spec validator on creation.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	et, ok := obj.(*openchoreodevv1alpha1.EndpointType)
	if !ok {
		return nil, fmt.Errorf("expected an EndpointType object but got %T", obj)
	}
	endpointtypelog.Info("Validation for EndpointType upon creation", "name", et.GetName())
	return nil, validate(et)
}

// ValidateUpdate re-runs the validator on the new object. Endpoints pin a
// snapshot of the type in their EndpointRelease, so an update affects only
// releases cut after it; validating it here keeps those releases renderable.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	if _, ok := oldObj.(*openchoreodevv1alpha1.EndpointType); !ok {
		return nil, fmt.Errorf("expected an EndpointType object for the oldObj but got %T", oldObj)
	}
	et, ok := newObj.(*openchoreodevv1alpha1.EndpointType)
	if !ok {
		return nil, fmt.Errorf("expected an EndpointType object for the newObj but got %T", newObj)
	}
	endpointtypelog.Info("Validation for EndpointType upon update", "name", et.GetName())
	return nil, validate(et)
}

// ValidateDelete is a no-op. Existing Endpoints keep rendering from the
// snapshot pinned in their EndpointRelease.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	if _, ok := obj.(*openchoreodevv1alpha1.EndpointType); !ok {
		return nil, fmt.Errorf("expected an EndpointType object but got %T", obj)
	}
	return nil, nil
}

func validate(et *openchoreodevv1alpha1.EndpointType) error {
	if errs := endpointvalidation.ValidateEndpointTypeSpec(&et.Spec, field.NewPath("spec")); len(errs) > 0 {
		return apierrors.NewInvalid(et.GroupVersionKind().GroupKind(), et.GetName(), errs)
	}
	return nil
}
