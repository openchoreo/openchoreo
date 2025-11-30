// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
)

// nolint:unused
// log is for logging in this package.
var componentlog = logf.Log.WithName("component-resource")

// SetupComponentWebhookWithManager registers the webhook for Component in the manager.
func SetupComponentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.Component{}).
		WithValidator(&Validator{Client: mgr.GetClient()}).
		WithDefaulter(&Defaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-openchoreo-dev-v1alpha1-component,mutating=true,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=components,verbs=create;update,versions=v1alpha1,name=mcomponent-v1alpha1.kb.io,admissionReviewVersions=v1

// Defaulter struct is responsible for setting default values on the custom resource of the
// Kind Component when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type Defaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &Defaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Component.
func (d *Defaulter) Default(_ context.Context, obj runtime.Object) error {
	// No-op: Defaulting logic disabled for now
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion component.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-component,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=components,verbs=create;update,versions=v1alpha1,name=vcomponent-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator struct is responsible for validating the Component resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type Validator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Component.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	component, ok := obj.(*openchoreodevv1alpha1.Component)
	if !ok {
		return nil, fmt.Errorf("expected a Component object but got %T", obj)
	}
	componentlog.Info("Validation for Component upon creation", "name", component.GetName())

	allErrs := field.ErrorList{}
	var warnings admission.Warnings

	// Note: Required field validations (componentType, owner.projectName, traits.name, traits.instanceName) are enforced by the CRD schema

	// Validate unique trait instance names
	instanceNames := make(map[string]bool)
	for i, trait := range component.Spec.Traits {
		if instanceNames[trait.InstanceName] {
			allErrs = append(allErrs, field.Duplicate(
				field.NewPath("spec", "traits").Index(i).Child("instanceName"),
				trait.InstanceName))
		}
		instanceNames[trait.InstanceName] = true
	}

	// Cross-resource validation: validate parameters against ComponentType schema
	if component.Spec.ComponentType != "" {
		allErrs = append(allErrs, v.validateComponentTypeParameters(ctx, component)...)
	}

	// Cross-resource validation: validate trait configs against Trait schemas
	allErrs = append(allErrs, v.validateTraitParameters(ctx, component)...)

	if len(allErrs) > 0 {
		return warnings, allErrs.ToAggregate()
	}

	return warnings, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Component.
func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	_, ok := oldObj.(*openchoreodevv1alpha1.Component)
	if !ok {
		return nil, fmt.Errorf("expected a Component object for the oldObj but got %T", oldObj)
	}

	newComponent, ok := newObj.(*openchoreodevv1alpha1.Component)
	if !ok {
		return nil, fmt.Errorf("expected a Component object for the newObj but got %T", newObj)
	}
	componentlog.Info("Validation for Component upon update", "name", newComponent.GetName())

	allErrs := field.ErrorList{}
	var warnings admission.Warnings

	// Note: spec.componentType and spec.type immutability are enforced by CEL rules in the CRD schema
	// Note: Required field validations (componentType, owner.projectName, traits.name, traits.instanceName) are enforced by the CRD schema

	// Validate unique trait instance names
	instanceNames := make(map[string]bool)
	for i, trait := range newComponent.Spec.Traits {
		if instanceNames[trait.InstanceName] {
			allErrs = append(allErrs, field.Duplicate(
				field.NewPath("spec", "traits").Index(i).Child("instanceName"),
				trait.InstanceName))
		}
		instanceNames[trait.InstanceName] = true
	}

	// Cross-resource validation: validate parameters against ComponentType schema
	if newComponent.Spec.ComponentType != "" {
		allErrs = append(allErrs, v.validateComponentTypeParameters(ctx, newComponent)...)
	}

	// Cross-resource validation: validate trait configs against Trait schemas
	allErrs = append(allErrs, v.validateTraitParameters(ctx, newComponent)...)

	if len(allErrs) > 0 {
		return warnings, allErrs.ToAggregate()
	}

	return warnings, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Component.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	component, ok := obj.(*openchoreodevv1alpha1.Component)
	if !ok {
		return nil, fmt.Errorf("expected a Component object but got %T", obj)
	}
	componentlog.Info("Validation for Component upon deletion", "name", component.GetName())

	// No special validation needed for deletion
	return nil, nil
}

// parseComponentType parses the componentType format: {workloadType}/{componentTypeName}
// Returns workloadType, componentTypeName, and error
func parseComponentType(componentType string) (string, string, error) {
	parts := strings.SplitN(componentType, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("componentType must be in format 'workloadType/componentTypeName', got: %s", componentType)
	}
	return parts[0], parts[1], nil
}

// validateComponentTypeParameters validates component parameters against ComponentType schema
func (v *Validator) validateComponentTypeParameters(ctx context.Context, component *openchoreodevv1alpha1.Component) field.ErrorList {
	allErrs := field.ErrorList{}
	basePath := field.NewPath("spec", "parameters")

	// Parse componentType to get the ComponentType name
	_, componentTypeName, err := parseComponentType(component.Spec.ComponentType)
	if err != nil {
		// This should have been caught by validateComponentStructure, but double-check
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "componentType"),
			component.Spec.ComponentType,
			err.Error()))
		return allErrs
	}

	// Fetch the ComponentType CRD
	componentType := &openchoreodevv1alpha1.ComponentType{}
	err = v.Client.Get(ctx, types.NamespacedName{
		Name:      componentTypeName,
		Namespace: component.Namespace,
	}, componentType)

	if err != nil {
		if apierrors.IsNotFound(err) {
			allErrs = append(allErrs, field.NotFound(
				field.NewPath("spec", "componentType"),
				fmt.Sprintf("ComponentType %q not found in namespace %q", componentTypeName, component.Namespace)))
			return allErrs
		}
		// For other errors, return as error
		allErrs = append(allErrs, field.InternalError(
			field.NewPath("spec", "componentType"),
			fmt.Errorf("failed to fetch ComponentType %q: %w", componentTypeName, err)))
		return allErrs
	}

	// If ComponentType has no schema, nothing to validate against
	if componentType.Spec.Schema.Parameters == nil || len(componentType.Spec.Schema.Parameters.Raw) == 0 {
		return allErrs
	}

	// Build the schema definition
	var types map[string]any
	if componentType.Spec.Schema.Types != nil && len(componentType.Spec.Schema.Types.Raw) > 0 {
		if err := yaml.Unmarshal(componentType.Spec.Schema.Types.Raw, &types); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath,
				"<invalid>",
				fmt.Sprintf("ComponentType has invalid types schema: %v", err)))
			return allErrs
		}
	}

	var params map[string]any
	if err := yaml.Unmarshal(componentType.Spec.Schema.Parameters.Raw, &params); err != nil {
		allErrs = append(allErrs, field.Invalid(
			basePath,
			"<invalid>",
			fmt.Sprintf("ComponentType has invalid parameters schema: %v", err)))
		return allErrs
	}

	schemaDef := schema.Definition{
		Types:   types,
		Schemas: []map[string]any{params},
	}

	// Convert to JSON schema for validation
	jsonSchema, err := schema.ToJSONSchema(schemaDef)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(
			basePath,
			"<invalid>",
			fmt.Sprintf("ComponentType has invalid schema definition: %v", err)))
		return allErrs
	}

	// Unmarshal component parameters (treat nil/empty as empty object)
	var componentParams map[string]any
	if component.Spec.Parameters != nil && len(component.Spec.Parameters.Raw) > 0 {
		if err := yaml.Unmarshal(component.Spec.Parameters.Raw, &componentParams); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath,
				"<invalid>",
				fmt.Sprintf("failed to parse parameters: %v", err)))
			return allErrs
		}
	} else {
		// No parameters provided - validate against empty object
		componentParams = map[string]any{}
	}

	// Validate parameters against schema (will catch missing required fields)
	if err := schema.ValidateWithJSONSchema(componentParams, jsonSchema); err != nil {
		allErrs = append(allErrs, field.Invalid(
			basePath,
			"<invalid>",
			fmt.Sprintf("parameters do not match ComponentType schema: %v", err)))
	}

	return allErrs
}

// validateTraitParameters validates trait configs against Trait schemas
func (v *Validator) validateTraitParameters(ctx context.Context, component *openchoreodevv1alpha1.Component) field.ErrorList {
	allErrs := field.ErrorList{}
	basePath := field.NewPath("spec", "traits")

	for i, traitInstance := range component.Spec.Traits {
		traitPath := basePath.Index(i)

		// Fetch the Trait CRD
		trait := &openchoreodevv1alpha1.Trait{}
		err := v.Client.Get(ctx, types.NamespacedName{
			Name:      traitInstance.Name,
			Namespace: component.Namespace,
		}, trait)

		if err != nil {
			if apierrors.IsNotFound(err) {
				allErrs = append(allErrs, field.NotFound(
					traitPath.Child("name"),
					fmt.Sprintf("Trait %q not found in namespace %q", traitInstance.Name, component.Namespace)))
				continue
			}
			// For other errors, return as error
			allErrs = append(allErrs, field.InternalError(
				traitPath.Child("name"),
				fmt.Errorf("failed to fetch Trait %q: %w", traitInstance.Name, err)))
			continue
		}

		// If Trait has no schema, nothing to validate against
		hasParams := trait.Spec.Schema.Parameters != nil && len(trait.Spec.Schema.Parameters.Raw) > 0
		hasEnvOverrides := trait.Spec.Schema.EnvOverrides != nil && len(trait.Spec.Schema.EnvOverrides.Raw) > 0
		if !hasParams && !hasEnvOverrides {
			continue
		}

		// Build the schema definition (must combine both parameters and envOverrides, like the pipeline does)
		var types map[string]any
		if trait.Spec.Schema.Types != nil && len(trait.Spec.Schema.Types.Raw) > 0 {
			if err := yaml.Unmarshal(trait.Spec.Schema.Types.Raw, &types); err != nil {
				allErrs = append(allErrs, field.Invalid(
					traitPath.Child("parameters"),
					"<invalid>",
					fmt.Sprintf("Trait %q has invalid types schema: %v", traitInstance.Name, err)))
				continue
			}
		}

		// Extract schemas (both parameters and envOverrides)
		var schemas []map[string]any

		if hasParams {
			var params map[string]any
			if err := yaml.Unmarshal(trait.Spec.Schema.Parameters.Raw, &params); err != nil {
				allErrs = append(allErrs, field.Invalid(
					traitPath.Child("parameters"),
					"<invalid>",
					fmt.Sprintf("Trait %q has invalid parameters schema: %v", traitInstance.Name, err)))
				continue
			}
			schemas = append(schemas, params)
		}

		if hasEnvOverrides {
			var envOverrides map[string]any
			if err := yaml.Unmarshal(trait.Spec.Schema.EnvOverrides.Raw, &envOverrides); err != nil {
				allErrs = append(allErrs, field.Invalid(
					traitPath.Child("parameters"),
					"<invalid>",
					fmt.Sprintf("Trait %q has invalid envOverrides schema: %v", traitInstance.Name, err)))
				continue
			}
			schemas = append(schemas, envOverrides)
		}

		schemaDef := schema.Definition{
			Types:   types,
			Schemas: schemas,
		}

		// Convert to JSON schema for validation
		jsonSchema, err := schema.ToJSONSchema(schemaDef)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(
				traitPath.Child("parameters"),
				"<invalid>",
				fmt.Sprintf("Trait %q has invalid schema definition: %v", traitInstance.Name, err)))
			continue
		}

		// Unmarshal trait parameters (treat nil/empty as empty object)
		var traitParams map[string]any
		if traitInstance.Parameters != nil && len(traitInstance.Parameters.Raw) > 0 {
			if err := yaml.Unmarshal(traitInstance.Parameters.Raw, &traitParams); err != nil {
				allErrs = append(allErrs, field.Invalid(
					traitPath.Child("parameters"),
					"<invalid>",
					fmt.Sprintf("failed to parse trait parameters: %v", err)))
				continue
			}
		} else {
			// No parameters provided - validate against empty object
			traitParams = map[string]any{}
		}

		// Validate parameters against schema (will catch missing required fields)
		if err := schema.ValidateWithJSONSchema(traitParams, jsonSchema); err != nil {
			allErrs = append(allErrs, field.Invalid(
				traitPath.Child("parameters"),
				"<invalid>",
				fmt.Sprintf("trait parameters do not match Trait %q schema: %v", traitInstance.Name, err)))
		}
	}

	return allErrs
}
