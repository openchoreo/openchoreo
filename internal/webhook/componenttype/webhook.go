// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/validation/component"
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

	allErrs := field.ErrorList{}

	// 1. Validate schema if present
	schemaErrs := validateComponentTypeSchema(&componenttype.Spec.Schema)
	allErrs = append(allErrs, schemaErrs...)

	// 2. Validate CEL expressions in resources
	celErrs := component.ValidateComponentTypeResources(componenttype)
	allErrs = append(allErrs, celErrs...)

	// 3. Validate resource IDs and workloadType
	resourceErrs := validateResourceStructure(componenttype)
	allErrs = append(allErrs, resourceErrs...)

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

	allErrs := field.ErrorList{}

	// Note: spec.workloadType immutability is enforced by CEL rules in the CRD schema

	// Validate the new spec (same as create)
	schemaErrs := validateComponentTypeSchema(&newComponentType.Spec.Schema)
	allErrs = append(allErrs, schemaErrs...)

	celErrs := component.ValidateComponentTypeResources(newComponentType)
	allErrs = append(allErrs, celErrs...)

	resourceErrs := validateResourceStructure(newComponentType)
	allErrs = append(allErrs, resourceErrs...)

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

// validateComponentTypeSchema validates the schema definition using the same method as the rendering pipeline
func validateComponentTypeSchema(schemaSpec *openchoreodevv1alpha1.ComponentTypeSchema) field.ErrorList {
	allErrs := field.ErrorList{}
	basePath := field.NewPath("spec", "schema")

	// Extract types from RawExtension (same as pipeline)
	var types map[string]any
	if schemaSpec.Types != nil && len(schemaSpec.Types.Raw) > 0 {
		if err := yaml.Unmarshal(schemaSpec.Types.Raw, &types); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("types"),
				"<invalid>",
				fmt.Sprintf("failed to parse types: %v", err)))
			return allErrs // Can't continue validation without valid types
		}
	}

	// Extract schemas from RawExtensions (same as pipeline)
	var schemas []map[string]any
	var params map[string]any
	var envOverrides map[string]any

	if schemaSpec.Parameters != nil && len(schemaSpec.Parameters.Raw) > 0 {
		if err := yaml.Unmarshal(schemaSpec.Parameters.Raw, &params); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("parameters"),
				"<invalid>",
				fmt.Sprintf("failed to parse parameters schema: %v", err)))
		} else {
			schemas = append(schemas, params)
		}
	}

	if schemaSpec.EnvOverrides != nil && len(schemaSpec.EnvOverrides.Raw) > 0 {
		if err := yaml.Unmarshal(schemaSpec.EnvOverrides.Raw, &envOverrides); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("envOverrides"),
				"<invalid>",
				fmt.Sprintf("failed to parse envOverrides schema: %v", err)))
		} else {
			schemas = append(schemas, envOverrides)
		}
	}

	// Validate that parameters and envOverrides don't have overlapping top-level keys
	if params != nil && envOverrides != nil {
		for key := range params {
			if _, exists := envOverrides[key]; exists {
				allErrs = append(allErrs, field.Invalid(
					basePath.Child("envOverrides"),
					key,
					fmt.Sprintf("key '%s' is already defined in parameters; parameters and envOverrides cannot have overlapping keys", key)))
			}
		}
	}

	// If we have schemas, validate them using the same method as the rendering pipeline
	if len(schemas) > 0 || types != nil {
		def := schema.Definition{
			Types:   types,
			Schemas: schemas,
		}

		// This is the same validation the pipeline uses
		if _, err := schema.ToStructural(def); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath,
				"<invalid>",
				fmt.Sprintf("failed to build structural schema: %v", err)))
		}
	}

	return allErrs
}

// validateResourceStructure validates resource templates and ensures workloadType matches a resource kind
func validateResourceStructure(ct *openchoreodevv1alpha1.ComponentType) field.ErrorList {
	return component.ValidateWorkloadResources(
		ct.Spec.WorkloadType,
		ct.Spec.Resources,
		field.NewPath("spec", "resources"))
}
