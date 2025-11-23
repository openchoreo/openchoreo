// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

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
var componentreleaselog = logf.Log.WithName("componentrelease-resource")

// SetupComponentReleaseWebhookWithManager registers the webhook for ComponentRelease in the manager.
func SetupComponentReleaseWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.ComponentRelease{}).
		WithValidator(&Validator{}).
		WithDefaulter(&Defaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-openchoreo-dev-v1alpha1-componentrelease,mutating=true,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=componentreleases,verbs=create;update,versions=v1alpha1,name=mcomponentrelease-v1alpha1.kb.io,admissionReviewVersions=v1

// Defaulter struct is responsible for setting default values on the custom resource of the
// Kind ComponentRelease when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type Defaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &Defaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind ComponentRelease.
func (d *Defaulter) Default(_ context.Context, obj runtime.Object) error {
	// No-op: Defaulting logic disabled for now
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion component.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-componentrelease,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=componentreleases,verbs=create;update,versions=v1alpha1,name=vcomponentrelease-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator struct is responsible for validating the ComponentRelease resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type Validator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ComponentRelease.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	componentrelease, ok := obj.(*openchoreodevv1alpha1.ComponentRelease)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentRelease object but got %T", obj)
	}
	componentreleaselog.Info("Validation for ComponentRelease upon creation", "name", componentrelease.GetName())

	allErrs := field.ErrorList{}

	// Note: Required field validations (owner, componentType, workload, traits.name, traits.instanceName) are enforced by the CRD schema

	// Validate unique trait instance names and trait existence
	instanceNames := make(map[string]bool)
	for i, trait := range componentrelease.Spec.ComponentProfile.Traits {
		traitPath := field.NewPath("spec", "componentProfile", "traits").Index(i)

		if instanceNames[trait.InstanceName] {
			allErrs = append(allErrs, field.Duplicate(
				traitPath.Child("instanceName"),
				trait.InstanceName))
		}
		instanceNames[trait.InstanceName] = true

		// Verify the trait spec exists in the traits map
		if _, exists := componentrelease.Spec.Traits[trait.Name]; !exists {
			allErrs = append(allErrs, field.Invalid(
				traitPath.Child("name"),
				trait.Name,
				fmt.Sprintf("trait '%s' referenced in componentProfile but not found in traits snapshot", trait.Name)))
		}
	}

	// Validate component profile against embedded schemas
	errs := validateComponentProfileAgainstSchemas(componentrelease)
	allErrs = append(allErrs, errs...)

	// Validate embedded ComponentType and Trait templates have required fields
	errs = validateEmbeddedResourceTemplates(componentrelease)
	allErrs = append(allErrs, errs...)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ComponentRelease.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	_, ok := oldObj.(*openchoreodevv1alpha1.ComponentRelease)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentRelease object for the oldObj but got %T", oldObj)
	}

	newRelease, ok := newObj.(*openchoreodevv1alpha1.ComponentRelease)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentRelease object for the newObj but got %T", newObj)
	}
	componentreleaselog.Info("Validation for ComponentRelease upon update", "name", newRelease.GetName())

	// Note: spec immutability is enforced by CEL rules in the CRD schema

	// No additional validation needed for updates
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ComponentRelease.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	componentrelease, ok := obj.(*openchoreodevv1alpha1.ComponentRelease)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentRelease object but got %T", obj)
	}
	componentreleaselog.Info("Validation for ComponentRelease upon deletion", "name", componentrelease.GetName())

	// No special validation needed for deletion
	// In the future, we might want to check if this release is referenced by ReleaseBindings
	return nil, nil
}

// validateComponentProfileAgainstSchemas validates the component profile against embedded schemas
func validateComponentProfileAgainstSchemas(release *openchoreodevv1alpha1.ComponentRelease) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate component profile parameters against ComponentType schema
	errs := validateComponentParameters(release)
	allErrs = append(allErrs, errs...)

	// Validate trait instance parameters against Trait schemas
	errs = validateTraitInstanceParameters(release)
	allErrs = append(allErrs, errs...)

	return allErrs
}

// validateComponentParameters validates component profile parameters against ComponentType schema
func validateComponentParameters(release *openchoreodevv1alpha1.ComponentRelease) field.ErrorList {
	allErrs := field.ErrorList{}
	basePath := field.NewPath("spec", "componentProfile", "parameters")

	// Build the schema definition from ComponentType snapshot
	var types map[string]any
	if release.Spec.ComponentType.Schema.Types != nil && len(release.Spec.ComponentType.Schema.Types.Raw) > 0 {
		if err := yaml.Unmarshal(release.Spec.ComponentType.Schema.Types.Raw, &types); err != nil {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "componentType", "schema", "types"),
				"<invalid>",
				fmt.Sprintf("ComponentType snapshot has invalid types schema: %v", err)))
			return allErrs
		}
	}

	// Extract parameters schema
	var schemas []map[string]any
	if release.Spec.ComponentType.Schema.Parameters != nil && len(release.Spec.ComponentType.Schema.Parameters.Raw) > 0 {
		var paramsSchema map[string]any
		if err := yaml.Unmarshal(release.Spec.ComponentType.Schema.Parameters.Raw, &paramsSchema); err != nil {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "componentType", "schema", "parameters"),
				"<invalid>",
				fmt.Sprintf("ComponentType snapshot has invalid parameters schema: %v", err)))
			return allErrs
		}
		schemas = append(schemas, paramsSchema)
	}

	// If no parameters schema, no validation needed
	if len(schemas) == 0 {
		return allErrs
	}

	// Build JSON schema
	schemaDef := schema.Definition{
		Types:   types,
		Schemas: schemas,
	}

	jsonSchema, err := schema.ToJSONSchema(schemaDef)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(
			basePath,
			"<invalid>",
			fmt.Sprintf("ComponentType snapshot has invalid schema definition: %v", err)))
		return allErrs
	}

	// Unmarshal component profile parameters (treat nil/empty as empty object)
	var componentParams map[string]any
	if release.Spec.ComponentProfile.Parameters != nil && len(release.Spec.ComponentProfile.Parameters.Raw) > 0 {
		if err := yaml.Unmarshal(release.Spec.ComponentProfile.Parameters.Raw, &componentParams); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath,
				"<invalid>",
				fmt.Sprintf("failed to parse component parameters: %v", err)))
			return allErrs
		}
	} else {
		// No parameters provided - validate against empty object
		componentParams = map[string]any{}
	}

	// Validate parameters against schema
	if err := schema.ValidateWithJSONSchema(componentParams, jsonSchema); err != nil {
		allErrs = append(allErrs, field.Invalid(
			basePath,
			"<invalid>",
			fmt.Sprintf("component parameters do not match ComponentType schema: %v", err)))
	}

	return allErrs
}

// validateTraitInstanceParameters validates trait instance parameters against Trait schemas
func validateTraitInstanceParameters(release *openchoreodevv1alpha1.ComponentRelease) field.ErrorList {
	allErrs := field.ErrorList{}
	basePath := field.NewPath("spec", "componentProfile", "traits")

	for i, traitInstance := range release.Spec.ComponentProfile.Traits {
		traitPath := basePath.Index(i)

		// Get the trait spec from the snapshot
		traitSpec, exists := release.Spec.Traits[traitInstance.Name]
		if !exists {
			// This is already caught by validateReleaseTraits, skip
			continue
		}

		// Build the schema definition from Trait snapshot
		var types map[string]any
		if traitSpec.Schema.Types != nil && len(traitSpec.Schema.Types.Raw) > 0 {
			if err := yaml.Unmarshal(traitSpec.Schema.Types.Raw, &types); err != nil {
				allErrs = append(allErrs, field.Invalid(
					traitPath.Child("name"),
					traitInstance.Name,
					fmt.Sprintf("Trait %q snapshot has invalid types schema: %v", traitInstance.Name, err)))
				continue
			}
		}

		// Extract parameters schema from the trait
		var schemas []map[string]any
		if traitSpec.Schema.Parameters != nil && len(traitSpec.Schema.Parameters.Raw) > 0 {
			var paramsSchema map[string]any
			if err := yaml.Unmarshal(traitSpec.Schema.Parameters.Raw, &paramsSchema); err != nil {
				allErrs = append(allErrs, field.Invalid(
					traitPath.Child("name"),
					traitInstance.Name,
					fmt.Sprintf("Trait %q snapshot has invalid parameters schema: %v", traitInstance.Name, err)))
				continue
			}
			schemas = append(schemas, paramsSchema)
		}

		// If no parameters schema, no validation needed for this trait
		if len(schemas) == 0 {
			continue
		}

		// Build JSON schema
		schemaDef := schema.Definition{
			Types:   types,
			Schemas: schemas,
		}

		jsonSchema, err := schema.ToJSONSchema(schemaDef)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(
				traitPath.Child("parameters"),
				"<invalid>",
				fmt.Sprintf("Trait %q snapshot has invalid schema definition: %v", traitInstance.Name, err)))
			continue
		}

		// Unmarshal trait instance parameters (treat nil/empty as empty object)
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

		// Validate parameters against schema
		if err := schema.ValidateWithJSONSchema(traitParams, jsonSchema); err != nil {
			allErrs = append(allErrs, field.Invalid(
				traitPath.Child("parameters"),
				"<invalid>",
				fmt.Sprintf("trait %q parameters do not match Trait schema: %v", traitInstance.Name, err)))
		}
	}

	return allErrs
}

// validateEmbeddedResourceTemplates validates that embedded ComponentType and Trait templates have required fields
func validateEmbeddedResourceTemplates(release *openchoreodevv1alpha1.ComponentRelease) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate ComponentType resource templates and check for workload type match
	errs := component.ValidateWorkloadResources(
		release.Spec.ComponentType.WorkloadType,
		release.Spec.ComponentType.Resources,
		field.NewPath("spec", "componentType", "resources"))
	allErrs = append(allErrs, errs...)

	// Validate Trait creates templates
	traitsBasePath := field.NewPath("spec", "traits")
	for traitName, traitSpec := range release.Spec.Traits {
		traitPath := traitsBasePath.Key(traitName).Child("creates")
		for i, create := range traitSpec.Creates {
			createPath := traitPath.Index(i)
			if create.Template != nil && len(create.Template.Raw) > 0 {
				_, errs := component.ValidateResourceTemplateStructure(*create.Template, createPath.Child("template"))
				allErrs = append(allErrs, errs...)
			}
		}
	}

	return allErrs
}
