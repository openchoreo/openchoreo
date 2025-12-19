// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
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

	// Extract and validate schemas, getting structural schemas for CEL validation
	parametersSchema, envOverridesSchema, schemaErrs := extractAndValidateSchemas(&componenttype.Spec.Schema)
	allErrs = append(allErrs, schemaErrs...)

	// Validate CEL expressions with schema-aware type checking
	celErrs := component.ValidateComponentTypeResourcesWithSchema(
		componenttype,
		parametersSchema,
		envOverridesSchema,
	)
	allErrs = append(allErrs, celErrs...)

	// Validate resource IDs and workloadType
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

	// Extract and validate schemas, getting structural schemas for CEL validation
	parametersSchema, envOverridesSchema, schemaErrs := extractAndValidateSchemas(&newComponentType.Spec.Schema)
	allErrs = append(allErrs, schemaErrs...)

	// Validate CEL expressions with schema-aware type checking
	celErrs := component.ValidateComponentTypeResourcesWithSchema(
		newComponentType,
		parametersSchema,
		envOverridesSchema,
	)
	allErrs = append(allErrs, celErrs...)

	// Validate resource IDs and workloadType
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

// extractAndValidateSchemas extracts and validates schemas, returning structural schemas for CEL validation.
// Returns the parameters schema, envOverrides schema, and any validation errors.
func extractAndValidateSchemas(schemaSpec *openchoreodevv1alpha1.ComponentTypeSchema) (
	*apiextschema.Structural, *apiextschema.Structural, field.ErrorList,
) {
	allErrs := field.ErrorList{}
	basePath := field.NewPath("spec", "schema")

	// Extract types from RawExtension
	var types map[string]any
	if schemaSpec.Types != nil && len(schemaSpec.Types.Raw) > 0 {
		if err := yaml.Unmarshal(schemaSpec.Types.Raw, &types); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("types"),
				"<invalid>",
				fmt.Sprintf("failed to parse types: %v", err)))
			return nil, nil, allErrs
		}
	}

	// Extract and build parameters structural schema
	var parametersSchema *apiextschema.Structural
	var params map[string]any
	if schemaSpec.Parameters != nil && len(schemaSpec.Parameters.Raw) > 0 {
		if err := yaml.Unmarshal(schemaSpec.Parameters.Raw, &params); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("parameters"),
				"<invalid>",
				fmt.Sprintf("failed to parse parameters schema: %v", err)))
		} else {
			def := schema.Definition{
				Types:   types,
				Schemas: []map[string]any{params},
			}
			structural, err := schema.ToStructural(def)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(
					basePath.Child("parameters"),
					"<invalid>",
					fmt.Sprintf("failed to build structural schema: %v", err)))
			} else {
				parametersSchema = structural
			}
		}
	}

	// Extract and build envOverrides structural schema
	var envOverridesSchema *apiextschema.Structural
	var envOverrides map[string]any
	if schemaSpec.EnvOverrides != nil && len(schemaSpec.EnvOverrides.Raw) > 0 {
		if err := yaml.Unmarshal(schemaSpec.EnvOverrides.Raw, &envOverrides); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("envOverrides"),
				"<invalid>",
				fmt.Sprintf("failed to parse envOverrides schema: %v", err)))
		} else {
			def := schema.Definition{
				Types:   types,
				Schemas: []map[string]any{envOverrides},
			}
			structural, err := schema.ToStructural(def)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(
					basePath.Child("envOverrides"),
					"<invalid>",
					fmt.Sprintf("failed to build structural schema: %v", err)))
			} else {
				envOverridesSchema = structural
			}
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

	return parametersSchema, envOverridesSchema, allErrs
}

// validateResourceStructure validates resource templates and ensures workloadType matches a resource kind
func validateResourceStructure(ct *openchoreodevv1alpha1.ComponentType) field.ErrorList {
	return component.ValidateWorkloadResources(
		ct.Spec.WorkloadType,
		ct.Spec.Resources,
		field.NewPath("spec", "resources"))
}
