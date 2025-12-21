// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/validation/component"
	"github.com/openchoreo/openchoreo/internal/webhook/schemautil"
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
	basePath := field.NewPath("spec", "schema")
	parametersSchema, envOverridesSchema, schemaErrs := schemautil.ExtractStructuralSchemas(&trait.Spec.Schema, basePath)
	allErrs = append(allErrs, schemaErrs...)

	templateErrs := validateTraitCreatesTemplateStructure(trait)
	allErrs = append(allErrs, templateErrs...)

	// Validate CEL expressions with schema-aware type checking
	celErrs := component.ValidateTraitCreatesAndPatchesWithSchema(
		trait,
		parametersSchema,
		envOverridesSchema,
	)
	allErrs = append(allErrs, celErrs...)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
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
	basePath := field.NewPath("spec", "schema")
	parametersSchema, envOverridesSchema, schemaErrs := schemautil.ExtractStructuralSchemas(&newTrait.Spec.Schema, basePath)
	allErrs = append(allErrs, schemaErrs...)

	templateErrs := validateTraitCreatesTemplateStructure(newTrait)
	allErrs = append(allErrs, templateErrs...)

	// Validate CEL expressions with schema-aware type checking
	celErrs := component.ValidateTraitCreatesAndPatchesWithSchema(
		newTrait,
		parametersSchema,
		envOverridesSchema,
	)
	allErrs = append(allErrs, celErrs...)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
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

// validateTraitCreatesTemplateStructure validates that trait creates templates have required K8s resource fields (apiVersion, kind, metadata.name)
func validateTraitCreatesTemplateStructure(trait *openchoreodevv1alpha1.Trait) field.ErrorList {
	allErrs := field.ErrorList{}
	basePath := field.NewPath("spec", "creates")

	for i, create := range trait.Spec.Creates {
		createPath := basePath.Index(i)
		templatePath := createPath.Child("template")

		if create.Template == nil {
			allErrs = append(allErrs, field.Required(templatePath, "template is required"))
			continue
		}

		_, errs := component.ValidateResourceTemplateStructure(*create.Template, templatePath)
		allErrs = append(allErrs, errs...)
	}

	return allErrs
}
