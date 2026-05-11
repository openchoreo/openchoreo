// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/validation/component"
	"github.com/openchoreo/openchoreo/internal/validation/schemautil"
)

// nolint:unused
// log is for logging in this package.
var componentreleaselog = logf.Log.WithName("componentrelease-resource")

// omitValue is used to omit the value from field.Invalid error messages
var omitValue = field.OmitValueType{}

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

	// Build trait spec map and validate (kind,name) uniqueness
	traitMap, traitMapErrs := buildTraitSpecMap(componentrelease.Spec.Traits)
	allErrs = append(allErrs, traitMapErrs...)

	// Validate unique trait instance names and trait existence (only if ComponentProfile is non-nil)
	if componentrelease.Spec.ComponentProfile != nil {
		instanceNames := make(map[string]bool)
		for i, trait := range componentrelease.Spec.ComponentProfile.Traits {
			traitPath := field.NewPath("spec", "componentProfile", "traits").Index(i)

			if instanceNames[trait.InstanceName] {
				allErrs = append(allErrs, field.Duplicate(
					traitPath.Child("instanceName"),
					trait.InstanceName))
			}
			instanceNames[trait.InstanceName] = true

			// Verify the trait spec exists in the traits slice (by kind+name composite key)
			key := string(trait.Kind) + ":" + trait.Name
			if _, exists := traitMap[key]; !exists {
				allErrs = append(allErrs, field.Invalid(
					traitPath.Child("name"),
					trait.Name,
					fmt.Sprintf("trait '%s' (kind %s) referenced in componentProfile but not found in traits snapshot", trait.Name, trait.Kind)))
			}
		}
	}

	// Validate component profile against embedded schemas
	errs := validateComponentProfileAgainstSchemas(componentrelease)
	allErrs = append(allErrs, errs...)

	// Validate embedded ComponentType and Trait templates have required fields
	errs = validateEmbeddedResourceTemplates(componentrelease)
	allErrs = append(allErrs, errs...)

	// Validate workload container has an image
	if componentrelease.Spec.Workload.Container.Image == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "workload", "container", "image"),
			"workload container must have an image"))
	}

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(componentrelease.GroupVersionKind().GroupKind(), componentrelease.GetName(), allErrs)
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
	var rawParams *runtime.RawExtension
	if release.Spec.ComponentProfile != nil {
		rawParams = release.Spec.ComponentProfile.Parameters
	}
	return validateParamsAgainstSchema(
		release.Spec.ComponentType.Spec.Parameters,
		rawParams,
		field.NewPath("spec", "componentProfile", "parameters"),
		"ComponentType",
	)
}

// validateTraitInstanceParameters validates trait instance parameters against Trait schemas
func validateTraitInstanceParameters(release *openchoreodevv1alpha1.ComponentRelease) field.ErrorList {
	allErrs := field.ErrorList{}

	// If ComponentProfile is nil, there are no trait instances to validate
	if release.Spec.ComponentProfile == nil {
		return allErrs
	}

	basePath := field.NewPath("spec", "componentProfile", "traits")

	// Build lookup map for trait specs by (kind:name)
	traitSpecsByKey := make(map[string]*openchoreodevv1alpha1.TraitSpec, len(release.Spec.Traits))
	for i := range release.Spec.Traits {
		key := string(release.Spec.Traits[i].Kind) + ":" + release.Spec.Traits[i].Name
		traitSpecsByKey[key] = &release.Spec.Traits[i].Spec
	}

	for i, traitInstance := range release.Spec.ComponentProfile.Traits {
		traitPath := basePath.Index(i)

		// Get the trait spec from the snapshot using composite (kind, name) key
		key := string(traitInstance.Kind) + ":" + traitInstance.Name
		traitSpec, exists := traitSpecsByKey[key]
		if !exists {
			// This is already caught by ValidateCreate, skip
			continue
		}

		allErrs = append(allErrs, validateParamsAgainstSchema(
			traitSpec.Parameters,
			traitInstance.Parameters,
			traitPath.Child("parameters"),
			fmt.Sprintf("Trait %q", traitInstance.Name),
		)...)
	}

	return allErrs
}

// validateEmbeddedResourceTemplates validates that embedded ComponentType and Trait templates have required fields
// and validates CEL expressions with schema-aware type checking.
func validateEmbeddedResourceTemplates(release *openchoreodevv1alpha1.ComponentRelease) field.ErrorList {
	allErrs := field.ErrorList{}
	ctSpecPath := field.NewPath("spec", "componentType", "spec")

	// Validate ComponentType resource templates and check for workload type match
	allErrs = append(allErrs, component.ValidateWorkloadResources(
		release.Spec.ComponentType.Spec.WorkloadType,
		release.Spec.ComponentType.Spec.Resources,
		ctSpecPath.Child("resources"))...)

	// Validate CEL expressions in embedded ComponentType resources
	parametersSchema, envConfigsSchema, schemaErrs := schemautil.ExtractStructuralSchemas(
		release.Spec.ComponentType.Spec.Parameters, release.Spec.ComponentType.Spec.EnvironmentConfigs, ctSpecPath,
	)
	allErrs = append(allErrs, schemaErrs...)

	allErrs = append(allErrs, component.ValidateResourcesWithSchema(
		release.Spec.ComponentType.Spec.Resources,
		release.Spec.ComponentType.Spec.Validations,
		parametersSchema, envConfigsSchema,
		ctSpecPath,
	)...)

	// Validate Trait creates templates and CEL expressions
	traitsBasePath := field.NewPath("spec", "traits")
	for i, rt := range release.Spec.Traits {
		traitPath := traitsBasePath.Index(i)
		traitSpecPath := traitPath.Child("spec")

		// Validate trait create templates (structure + no workload resources)
		allErrs = append(allErrs, component.ValidateTraitCreateTemplates(
			rt.Spec.Creates, traitSpecPath.Child("creates"))...)

		// Validate CEL expressions in trait creates and patches
		traitParamsSchema, traitEnvConfigsSchema, traitSchemaErrs := schemautil.ExtractStructuralSchemas(
			rt.Spec.Parameters, rt.Spec.EnvironmentConfigs, traitSpecPath,
		)
		allErrs = append(allErrs, traitSchemaErrs...)

		allErrs = append(allErrs, component.ValidateTraitSpec(
			release.Spec.Traits[i].Spec,
			traitParamsSchema, traitEnvConfigsSchema,
			traitSpecPath,
		)...)
	}

	return allErrs
}

// validateParamsAgainstSchema validates raw parameters against a schema section.
func validateParamsAgainstSchema(
	schemaSection *openchoreodevv1alpha1.SchemaSection,
	rawParams *runtime.RawExtension,
	fieldPath *field.Path,
	schemaOwner string,
) field.ErrorList {
	allErrs := field.ErrorList{}

	if schemaSection == nil {
		return allErrs
	}

	paramsRaw := schemaSection.GetRaw()
	if paramsRaw == nil || len(paramsRaw.Raw) == 0 {
		return allErrs
	}

	jsonSchema, err := schema.SectionToJSONSchema(schemaSection)
	if err != nil {
		return append(allErrs, field.Invalid(fieldPath, omitValue,
			fmt.Sprintf("%s snapshot has invalid schema definition: %v", schemaOwner, err)))
	}

	var params map[string]any
	if rawParams != nil && len(rawParams.Raw) > 0 {
		if err := yaml.Unmarshal(rawParams.Raw, &params); err != nil {
			return append(allErrs, field.Invalid(fieldPath, omitValue,
				fmt.Sprintf("failed to parse parameters: %v", err)))
		}
	} else {
		params = map[string]any{}
	}

	if err := schema.ValidateWithJSONSchema(params, jsonSchema); err != nil {
		allErrs = append(allErrs, field.Invalid(fieldPath, omitValue,
			fmt.Sprintf("parameters do not match %s schema: %v", schemaOwner, err)))
	}

	return allErrs
}

// buildTraitSpecMap builds a map from (kind:name) to *TraitSpec and validates uniqueness.
func buildTraitSpecMap(traits []openchoreodevv1alpha1.ComponentReleaseTrait) (map[string]*openchoreodevv1alpha1.TraitSpec, field.ErrorList) {
	allErrs := field.ErrorList{}
	traitMap := make(map[string]*openchoreodevv1alpha1.TraitSpec, len(traits))
	traitsPath := field.NewPath("spec", "traits")

	for i, rt := range traits {
		key := string(rt.Kind) + ":" + rt.Name
		if _, exists := traitMap[key]; exists {
			allErrs = append(allErrs, field.Duplicate(traitsPath.Index(i), key))
		}
		traitMap[key] = &traits[i].Spec
	}

	return traitMap, allErrs
}
