// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

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
	"github.com/openchoreo/openchoreo/internal/validation/schemautil"
)

// nolint:unused
// log is for logging in this package.
var componenttypelog = logf.Log.WithName("componenttype-resource")

// SetupComponentTypeWebhookWithManager registers the webhook for ComponentType in the manager.
func SetupComponentTypeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.ComponentType{}).
		WithValidator(&Validator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-componenttype,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=componenttypes,verbs=create;update,versions=v1alpha1,name=vcomponenttype-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates ComponentType resources
// +kubebuilder:object:generate=false
type Validator struct{}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ComponentType.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	componenttype, ok := obj.(*openchoreodevv1alpha1.ComponentType)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentType object but got %T", obj)
	}
	componenttypelog.Info("Validation for ComponentType upon creation", "name", componenttype.GetName())

	allErrs := validateComponentType(componenttype)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ComponentType.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	_, ok := oldObj.(*openchoreodevv1alpha1.ComponentType)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentType object for the oldObj but got %T", oldObj)
	}

	newComponentType, ok := newObj.(*openchoreodevv1alpha1.ComponentType)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentType object for the newObj but got %T", newObj)
	}
	componenttypelog.Info("Validation for ComponentType upon update", "name", newComponentType.GetName())

	// Note: spec.workloadType immutability is enforced by CEL rules in the CRD schema

	allErrs := validateComponentType(newComponentType)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ComponentType.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	componenttype, ok := obj.(*openchoreodevv1alpha1.ComponentType)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentType object but got %T", obj)
	}
	componenttypelog.Info("Validation for ComponentType upon deletion", "name", componenttype.GetName())

	// No special validation needed for deletion
	return nil, nil
}

// validateComponentType performs all validation for a ComponentType.
func validateComponentType(ct *openchoreodevv1alpha1.ComponentType) field.ErrorList {
	allErrs := field.ErrorList{}

	// Extract and validate schemas, getting structural schemas for CEL validation
	basePath := field.NewPath("spec", "schema")
	parametersSchema, envOverridesSchema, schemaErrs := schemautil.ExtractStructuralSchemas(&ct.Spec.Schema, basePath)
	allErrs = append(allErrs, schemaErrs...)

	// Validate CEL expressions with schema-aware type checking
	celErrs := component.ValidateComponentTypeResourcesWithSchema(
		ct,
		parametersSchema,
		envOverridesSchema,
	)
	allErrs = append(allErrs, celErrs...)

	// Validate resource IDs and workloadType
	resourceErrs := validateResourceStructure(ct)
	allErrs = append(allErrs, resourceErrs...)

	// Validate embedded traits
	embeddedTraitErrs := validateEmbeddedTraits(ct)
	allErrs = append(allErrs, embeddedTraitErrs...)

	// Validate allowedTraits
	allowedTraitErrs := validateAllowedTraits(ct)
	allErrs = append(allErrs, allowedTraitErrs...)

	return allErrs
}

// validateResourceStructure validates resource templates and ensures workloadType matches a resource kind
func validateResourceStructure(ct *openchoreodevv1alpha1.ComponentType) field.ErrorList {
	return component.ValidateWorkloadResources(
		ct.Spec.WorkloadType,
		ct.Spec.Resources,
		field.NewPath("spec", "resources"))
}

// validateEmbeddedTraits validates the embedded traits in a ComponentType.
func validateEmbeddedTraits(ct *openchoreodevv1alpha1.ComponentType) field.ErrorList {
	allErrs := field.ErrorList{}
	traitsPath := field.NewPath("spec", "traits")

	instanceNames := make(map[string]int)
	for i, trait := range ct.Spec.Traits {
		traitPath := traitsPath.Index(i)

		// Validate non-empty name
		if trait.Name == "" {
			allErrs = append(allErrs, field.Required(traitPath.Child("name"), "trait name is required"))
		}

		// Validate non-empty instanceName
		if trait.InstanceName == "" {
			allErrs = append(allErrs, field.Required(traitPath.Child("instanceName"), "trait instanceName is required"))
		}

		// Check for duplicate instanceNames
		if prevIdx, exists := instanceNames[trait.InstanceName]; exists {
			allErrs = append(allErrs, field.Duplicate(
				traitPath.Child("instanceName"),
				fmt.Sprintf("instanceName %q is already used by trait at index %d", trait.InstanceName, prevIdx),
			))
		}
		instanceNames[trait.InstanceName] = i
	}

	return allErrs
}

// validateAllowedTraits validates the allowedTraits list in a ComponentType.
func validateAllowedTraits(ct *openchoreodevv1alpha1.ComponentType) field.ErrorList {
	allErrs := field.ErrorList{}
	allowedPath := field.NewPath("spec", "allowedTraits")

	// Build set of embedded trait names for overlap check
	embeddedTraitNames := make(map[string]bool)
	for _, trait := range ct.Spec.Traits {
		embeddedTraitNames[trait.Name] = true
	}

	seen := make(map[string]bool)
	for i, traitName := range ct.Spec.AllowedTraits {
		entryPath := allowedPath.Index(i)

		// Validate non-empty
		if traitName == "" {
			allErrs = append(allErrs, field.Required(entryPath, "allowed trait name must not be empty"))
			continue
		}

		// Check for duplicates
		if seen[traitName] {
			allErrs = append(allErrs, field.Duplicate(entryPath, traitName))
		}
		seen[traitName] = true

		// Check for overlap with embedded traits
		if embeddedTraitNames[traitName] {
			allErrs = append(allErrs, field.Invalid(
				entryPath,
				traitName,
				"trait is already embedded in spec.traits and cannot also be in allowedTraits",
			))
		}
	}

	return allErrs
}
