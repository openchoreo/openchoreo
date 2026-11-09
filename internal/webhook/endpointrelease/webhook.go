// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrelease

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

var endpointreleaselog = logf.Log.WithName("endpointrelease-resource")

// SetupEndpointReleaseWebhookWithManager registers the validating webhook for
// EndpointRelease in the manager.
func SetupEndpointReleaseWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &openchoreodevv1alpha1.EndpointRelease{}).
		WithCustomValidator(&Validator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-endpointrelease,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=endpointreleases,verbs=create;update,versions=v1alpha1,name=vendpointrelease-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates EndpointRelease resources.
// +kubebuilder:object:generate=false
type Validator struct{}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate runs the on-snapshot checks: spec.parameters against the
// embedded type's parameters schema, and a re-validation of the embedded type
// spec. The Endpoint controller already checks parameters before it cuts a
// release; this is the backstop for releases created any other way, and for
// validator tightening since the type was admitted.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	release, ok := obj.(*openchoreodevv1alpha1.EndpointRelease)
	if !ok {
		return nil, fmt.Errorf("expected an EndpointRelease object but got %T", obj)
	}
	endpointreleaselog.Info("Validation for EndpointRelease upon creation", "name", release.GetName())

	snapshot := &release.Spec.EndpointType
	allErrs := endpointvalidation.ValidateValuesField(snapshot.Spec.Parameters, release.Spec.Parameters,
		field.NewPath("spec", "parameters"), fmt.Sprintf("%s %q", snapshot.Kind, snapshot.Name))
	allErrs = append(allErrs, endpointvalidation.ValidateEndpointTypeSpec(
		&snapshot.Spec, field.NewPath("spec", "endpointType", "spec"))...)

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(release.GroupVersionKind().GroupKind(), release.GetName(), allErrs)
	}
	return nil, nil
}

// ValidateUpdate is a no-op. EndpointRelease.spec is immutable via a spec-level
// CEL XValidation marker, so any update either matches the existing spec or is
// rejected at the CRD layer before reaching this webhook.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	if _, ok := oldObj.(*openchoreodevv1alpha1.EndpointRelease); !ok {
		return nil, fmt.Errorf("expected an EndpointRelease object for the oldObj but got %T", oldObj)
	}
	if _, ok := newObj.(*openchoreodevv1alpha1.EndpointRelease); !ok {
		return nil, fmt.Errorf("expected an EndpointRelease object for the newObj but got %T", newObj)
	}
	return nil, nil
}

// ValidateDelete is a no-op. EndpointRelease deletion is driven by the owning
// Endpoint's finalizer.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	if _, ok := obj.(*openchoreodevv1alpha1.EndpointRelease); !ok {
		return nil, fmt.Errorf("expected an EndpointRelease object but got %T", obj)
	}
	return nil, nil
}
