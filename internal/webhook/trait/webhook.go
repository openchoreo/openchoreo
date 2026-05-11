// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

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
	"github.com/openchoreo/openchoreo/internal/validation/component"
	"github.com/openchoreo/openchoreo/internal/validation/schemautil"
)

// nolint:unused
// log is for logging in this package.
var traitlog = logf.Log.WithName("trait-resource")

// SetupTraitWebhookWithManager registers the webhook for Trait in the manager.
func SetupTraitWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.Trait{}).
		WithValidator(&Validator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-trait,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=traits,verbs=create;update,versions=v1alpha1,name=vtrait-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates Trait resources
// +kubebuilder:object:generate=false
type Validator struct{}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Trait.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	trait, ok := obj.(*openchoreodevv1alpha1.Trait)
	if !ok {
		return nil, fmt.Errorf("expected a Trait object but got %T", obj)
	}
	traitlog.Info("Validation for Trait upon creation", "name", trait.GetName())

	allErrs := field.ErrorList{}

	// Extract and validate schemas, getting structural schemas for CEL validation
	parametersSchema, envConfigsSchema, schemaErrs := schemautil.ExtractAndValidateSchemas(
		trait.Spec.Parameters, trait.Spec.EnvironmentConfigs, field.NewPath("spec"),
	)
	allErrs = append(allErrs, schemaErrs...)

	allErrs = append(allErrs, component.ValidateTraitCreateTemplates(
		trait.Spec.Creates, field.NewPath("spec", "creates"))...)

	allErrs = append(allErrs, component.ValidateTraitCreatesAndPatchesWithSchema(
		trait, parametersSchema, envConfigsSchema,
	)...)

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(trait.GroupVersionKind().GroupKind(), trait.GetName(), allErrs)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Trait.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newTrait, ok := newObj.(*openchoreodevv1alpha1.Trait)
	if !ok {
		return nil, fmt.Errorf("expected a Trait object for the newObj but got %T", newObj)
	}
	traitlog.Info("Validation for Trait upon update", "name", newTrait.GetName())

	allErrs := field.ErrorList{}

	// Extract and validate schemas, getting structural schemas for CEL validation
	parametersSchema, envConfigsSchema, schemaErrs := schemautil.ExtractAndValidateSchemas(
		newTrait.Spec.Parameters, newTrait.Spec.EnvironmentConfigs, field.NewPath("spec"),
	)
	allErrs = append(allErrs, schemaErrs...)

	allErrs = append(allErrs, component.ValidateTraitCreateTemplates(
		newTrait.Spec.Creates, field.NewPath("spec", "creates"))...)

	allErrs = append(allErrs, component.ValidateTraitCreatesAndPatchesWithSchema(
		newTrait, parametersSchema, envConfigsSchema,
	)...)

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(newTrait.GroupVersionKind().GroupKind(), newTrait.GetName(), allErrs)
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Trait.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	trait, ok := obj.(*openchoreodevv1alpha1.Trait)
	if !ok {
		return nil, fmt.Errorf("expected a Trait object but got %T", obj)
	}
	traitlog.Info("Validation for Trait upon deletion", "name", trait.GetName())

	// No special validation needed for deletion
	return nil, nil
}
