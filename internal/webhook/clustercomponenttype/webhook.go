// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

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
var clustercomponenttypelog = logf.Log.WithName("clustercomponenttype-resource")

// SetupClusterComponentTypeWebhookWithManager registers the webhook for ClusterComponentType in the manager.
func SetupClusterComponentTypeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.ClusterComponentType{}).
		WithValidator(&Validator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-clustercomponenttype,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=clustercomponenttypes,verbs=create;update,versions=v1alpha1,name=vclustercomponenttype-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates ClusterComponentType resources
// +kubebuilder:object:generate=false
type Validator struct{}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ClusterComponentType.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	clustercomponenttype, ok := obj.(*openchoreodevv1alpha1.ClusterComponentType)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterComponentType object but got %T", obj)
	}
	clustercomponenttypelog.Info("Validation for ClusterComponentType upon creation", "name", clustercomponenttype.GetName())

	allErrs := validateClusterComponentType(clustercomponenttype)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ClusterComponentType.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	_, ok := oldObj.(*openchoreodevv1alpha1.ClusterComponentType)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterComponentType object for the oldObj but got %T", oldObj)
	}

	newClusterComponentType, ok := newObj.(*openchoreodevv1alpha1.ClusterComponentType)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterComponentType object for the newObj but got %T", newObj)
	}
	clustercomponenttypelog.Info("Validation for ClusterComponentType upon update", "name", newClusterComponentType.GetName())

	// Note: spec.workloadType immutability is enforced by CEL rules in the CRD schema

	allErrs := validateClusterComponentType(newClusterComponentType)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ClusterComponentType.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	clustercomponenttype, ok := obj.(*openchoreodevv1alpha1.ClusterComponentType)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterComponentType object but got %T", obj)
	}
	clustercomponenttypelog.Info("Validation for ClusterComponentType upon deletion", "name", clustercomponenttype.GetName())

	// No special validation needed for deletion
	return nil, nil
}

// validateClusterComponentType performs all validation for a ClusterComponentType.
func validateClusterComponentType(cct *openchoreodevv1alpha1.ClusterComponentType) field.ErrorList {
	allErrs := field.ErrorList{}

	// Extract and validate schemas, getting structural schemas for CEL validation
	basePath := field.NewPath("spec", "schema")
	parametersSchema, envOverridesSchema, schemaErrs := schemautil.ExtractStructuralSchemas(&cct.Spec.Schema, basePath)
	allErrs = append(allErrs, schemaErrs...)

	// Validate CEL expressions with schema-aware type checking
	celErrs := component.ValidateClusterComponentTypeResourcesWithSchema(
		cct,
		parametersSchema,
		envOverridesSchema,
	)
	allErrs = append(allErrs, celErrs...)

	// Validate resource IDs and workloadType
	resourceErrs := validateResourceStructure(cct)
	allErrs = append(allErrs, resourceErrs...)

	// Validate embedded traits
	embeddedTraitErrs := validateEmbeddedTraits(cct)
	allErrs = append(allErrs, embeddedTraitErrs...)

	// Validate allowedTraits
	allowedTraitErrs := validateAllowedTraits(cct)
	allErrs = append(allErrs, allowedTraitErrs...)

	return allErrs
}

// validateEmbeddedTraits validates the embedded traits in a ClusterComponentType.
// ClusterComponentType can only reference ClusterTrait (not namespace-scoped Trait).
func validateEmbeddedTraits(cct *openchoreodevv1alpha1.ClusterComponentType) field.ErrorList {
	allErrs := field.ErrorList{}
	traitsPath := field.NewPath("spec", "traits")

	instanceNames := make(map[string]int)
	for i, trait := range cct.Spec.Traits {
		traitPath := traitsPath.Index(i)

		// Validate kind is ClusterTrait (schema enforces this, but validate explicitly for safety)
		if trait.Kind != openchoreodevv1alpha1.ClusterTraitRefKindClusterTrait {
			allErrs = append(allErrs, field.Invalid(
				traitPath.Child("kind"),
				string(trait.Kind),
				"ClusterComponentType can only reference ClusterTrait, not namespace-scoped Trait",
			))
		}

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

// validateAllowedTraits validates the allowedTraits list in a ClusterComponentType.
// AllowedTraits uses []ClusterTraitRef (kind is always ClusterTrait).
func validateAllowedTraits(cct *openchoreodevv1alpha1.ClusterComponentType) field.ErrorList {
	allErrs := field.ErrorList{}
	allowedPath := field.NewPath("spec", "allowedTraits")

	// Build set of embedded trait names for overlap check (kind is always ClusterTrait)
	embeddedTraitNames := make(map[string]bool)
	for _, trait := range cct.Spec.Traits {
		embeddedTraitNames[trait.Name] = true
	}

	seen := make(map[string]bool)
	for i, ref := range cct.Spec.AllowedTraits {
		entryPath := allowedPath.Index(i)

		// Validate non-empty name
		if ref.Name == "" {
			allErrs = append(allErrs, field.Required(entryPath.Child("name"), "allowed trait name must not be empty"))
			continue
		}

		// Check for duplicates by name (kind is always ClusterTrait)
		if seen[ref.Name] {
			allErrs = append(allErrs, field.Duplicate(entryPath, ref.Name))
		}
		seen[ref.Name] = true

		// Check for overlap with embedded traits by name (both are ClusterTrait)
		if embeddedTraitNames[ref.Name] {
			allErrs = append(allErrs, field.Invalid(
				entryPath,
				ref.Name,
				"trait is already embedded in spec.traits and cannot also be in allowedTraits",
			))
		}
	}

	return allErrs
}

// validateResourceStructure validates resource templates and ensures workloadType matches a resource kind
func validateResourceStructure(cct *openchoreodevv1alpha1.ClusterComponentType) field.ErrorList {
	return component.ValidateWorkloadResources(
		cct.Spec.WorkloadType,
		cct.Spec.Resources,
		field.NewPath("spec", "resources"))
}
