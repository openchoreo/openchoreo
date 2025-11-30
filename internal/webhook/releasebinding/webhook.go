// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"fmt"

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
var releasebindinglog = logf.Log.WithName("releasebinding-resource")

// SetupReleaseBindingWebhookWithManager registers the webhook for ReleaseBinding in the manager.
func SetupReleaseBindingWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.ReleaseBinding{}).
		WithValidator(&Validator{Client: mgr.GetClient()}).
		WithDefaulter(&Defaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-openchoreo-dev-v1alpha1-releasebinding,mutating=true,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=releasebindings,verbs=create;update,versions=v1alpha1,name=mreleasebinding-v1alpha1.kb.io,admissionReviewVersions=v1

// Defaulter struct is responsible for setting default values on the custom resource of the
// Kind ReleaseBinding when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type Defaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &Defaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind ReleaseBinding.
func (d *Defaulter) Default(_ context.Context, obj runtime.Object) error {
	// No-op: Defaulting logic disabled for now
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion component.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-releasebinding,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=releasebindings,verbs=create;update,versions=v1alpha1,name=vreleasebinding-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator struct is responsible for validating the ReleaseBinding resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type Validator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ReleaseBinding.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	releasebinding, ok := obj.(*openchoreodevv1alpha1.ReleaseBinding)
	if !ok {
		return nil, fmt.Errorf("expected a ReleaseBinding object but got %T", obj)
	}
	releasebindinglog.Info("Validation for ReleaseBinding upon creation", "name", releasebinding.GetName())

	allErrs := field.ErrorList{}
	var warnings admission.Warnings

	// Note: Required field validations (owner, environment) are enforced by the CRD schema
	// Note: releaseName is optional - it will be populated by the controller when
	// Component.Spec.AutoDeploy is enabled. Only validate against ComponentRelease
	// if releaseName is already set.

	// Cross-resource validation: validate against ComponentRelease (only if releaseName is set)
	if releasebinding.Spec.ReleaseName != "" {
		warns, errs := v.validateAgainstRelease(ctx, releasebinding)
		warnings = append(warnings, warns...)
		allErrs = append(allErrs, errs...)
	}

	if len(allErrs) > 0 {
		return warnings, allErrs.ToAggregate()
	}

	return warnings, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ReleaseBinding.
func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	_, ok := oldObj.(*openchoreodevv1alpha1.ReleaseBinding)
	if !ok {
		return nil, fmt.Errorf("expected a ReleaseBinding object for the oldObj but got %T", oldObj)
	}

	newBinding, ok := newObj.(*openchoreodevv1alpha1.ReleaseBinding)
	if !ok {
		return nil, fmt.Errorf("expected a ReleaseBinding object for the newObj but got %T", newObj)
	}
	releasebindinglog.Info("Validation for ReleaseBinding upon update", "name", newBinding.GetName())

	allErrs := field.ErrorList{}
	var warnings admission.Warnings

	// Note: spec.environment, spec.owner.projectName, and spec.owner.componentName immutability are enforced by CEL rules in the CRD schema
	// Note: Required field validations (owner, environment) are enforced by the CRD schema

	// Cross-resource validation: validate against ComponentRelease (only if releaseName is set)
	if newBinding.Spec.ReleaseName != "" {
		warns, errs := v.validateAgainstRelease(ctx, newBinding)
		warnings = append(warnings, warns...)
		allErrs = append(allErrs, errs...)
	}

	if len(allErrs) > 0 {
		return warnings, allErrs.ToAggregate()
	}

	return warnings, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ReleaseBinding.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	releasebinding, ok := obj.(*openchoreodevv1alpha1.ReleaseBinding)
	if !ok {
		return nil, fmt.Errorf("expected a ReleaseBinding object but got %T", obj)
	}
	releasebindinglog.Info("Validation for ReleaseBinding upon deletion", "name", releasebinding.GetName())

	// No special validation needed for deletion
	return nil, nil
}

// validateAgainstRelease validates ReleaseBinding against the referenced ComponentRelease
func (v *Validator) validateAgainstRelease(ctx context.Context, binding *openchoreodevv1alpha1.ReleaseBinding) (admission.Warnings, field.ErrorList) {
	var warnings admission.Warnings
	allErrs := field.ErrorList{}

	// Fetch the ComponentRelease
	release := &openchoreodevv1alpha1.ComponentRelease{}
	err := v.Client.Get(ctx, types.NamespacedName{
		Name:      binding.Spec.ReleaseName,
		Namespace: binding.Namespace,
	}, release)

	if err != nil {
		if apierrors.IsNotFound(err) {
			allErrs = append(allErrs, field.NotFound(
				field.NewPath("spec", "releaseName"),
				binding.Spec.ReleaseName))
			return warnings, allErrs
		}
		// For other errors, add a warning and continue
		warnings = append(warnings, fmt.Sprintf("Failed to fetch ComponentRelease %q: %v, skipping cross-resource validation", binding.Spec.ReleaseName, err))
		return warnings, allErrs
	}

	// Validate envOverrides against ComponentType's envOverrides schema
	if binding.Spec.ComponentTypeEnvOverrides != nil && len(binding.Spec.ComponentTypeEnvOverrides.Raw) > 0 {
		allErrs = append(allErrs, v.validateEnvOverridesAgainstSchema(binding, &release.Spec.ComponentType)...)
	}

	// Validate traitOverrides against traits in the release
	allErrs = append(allErrs, v.validateTraitOverridesAgainstRelease(binding, release)...)

	return warnings, allErrs
}

// validateEnvOverridesAgainstSchema validates envOverrides against ComponentType's envOverrides schema
func (v *Validator) validateEnvOverridesAgainstSchema(binding *openchoreodevv1alpha1.ReleaseBinding, componentType *openchoreodevv1alpha1.ComponentTypeSpec) field.ErrorList {
	allErrs := field.ErrorList{}
	basePath := field.NewPath("spec", "componentTypeEnvOverrides")

	// If ComponentType has no envOverrides schema, nothing to validate against
	if componentType.Schema.EnvOverrides == nil || len(componentType.Schema.EnvOverrides.Raw) == 0 {
		return allErrs
	}

	// Build the schema definition
	var types map[string]any
	if componentType.Schema.Types != nil && len(componentType.Schema.Types.Raw) > 0 {
		if err := yaml.Unmarshal(componentType.Schema.Types.Raw, &types); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath,
				"<invalid>",
				fmt.Sprintf("ComponentType in Release has invalid types schema: %v", err)))
			return allErrs
		}
	}

	var envOverridesSchema map[string]any
	if err := yaml.Unmarshal(componentType.Schema.EnvOverrides.Raw, &envOverridesSchema); err != nil {
		allErrs = append(allErrs, field.Invalid(
			basePath,
			"<invalid>",
			fmt.Sprintf("ComponentType in Release has invalid envOverrides schema: %v", err)))
		return allErrs
	}

	schemaDef := schema.Definition{
		Types:   types,
		Schemas: []map[string]any{envOverridesSchema},
	}

	// Convert to JSON schema for validation
	jsonSchema, err := schema.ToJSONSchema(schemaDef)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(
			basePath,
			"<invalid>",
			fmt.Sprintf("ComponentType in Release has invalid schema definition: %v", err)))
		return allErrs
	}

	// Unmarshal binding's envOverrides
	var envOverrides map[string]any
	if err := yaml.Unmarshal(binding.Spec.ComponentTypeEnvOverrides.Raw, &envOverrides); err != nil {
		allErrs = append(allErrs, field.Invalid(
			basePath,
			"<invalid>",
			fmt.Sprintf("failed to parse envOverrides: %v", err)))
		return allErrs
	}

	// Validate envOverrides against schema
	if err := schema.ValidateWithJSONSchema(envOverrides, jsonSchema); err != nil {
		allErrs = append(allErrs, field.Invalid(
			basePath,
			"<invalid>",
			fmt.Sprintf("envOverrides do not match ComponentType schema: %v", err)))
	}

	return allErrs
}

// validateTraitOverridesAgainstRelease validates trait overrides against traits in the release
func (v *Validator) validateTraitOverridesAgainstRelease(binding *openchoreodevv1alpha1.ReleaseBinding, release *openchoreodevv1alpha1.ComponentRelease) field.ErrorList {
	allErrs := field.ErrorList{}
	basePath := field.NewPath("spec", "traitOverrides")

	// Build a map of trait instance names from the component profile
	validInstanceNames := make(map[string]string) // instanceName -> traitName
	for _, traitInstance := range release.Spec.ComponentProfile.Traits {
		validInstanceNames[traitInstance.InstanceName] = traitInstance.Name
	}

	// Validate each trait override
	for instanceName, override := range binding.Spec.TraitOverrides {
		overridePath := basePath.Key(instanceName)

		// Check if instance name exists in the release
		traitName, exists := validInstanceNames[instanceName]
		if !exists {
			allErrs = append(allErrs, field.NotFound(
				overridePath,
				fmt.Sprintf("trait instance %q not found in ComponentRelease %q", instanceName, release.Name)))
			continue
		}

		// If override has no content, skip validation
		if len(override.Raw) == 0 {
			continue
		}

		// Get the trait spec from the release
		traitSpec, exists := release.Spec.Traits[traitName]
		if !exists {
			allErrs = append(allErrs, field.Invalid(
				overridePath,
				instanceName,
				fmt.Sprintf("Trait %q referenced by instance %q not found in ComponentRelease snapshot", traitName, instanceName)))
			continue
		}

		// If Trait has no envOverrides schema, nothing to validate against
		if traitSpec.Schema.EnvOverrides == nil || len(traitSpec.Schema.EnvOverrides.Raw) == 0 {
			continue
		}

		// Build the schema definition
		var types map[string]any
		if traitSpec.Schema.Types != nil && len(traitSpec.Schema.Types.Raw) > 0 {
			if err := yaml.Unmarshal(traitSpec.Schema.Types.Raw, &types); err != nil {
				allErrs = append(allErrs, field.Invalid(
					overridePath,
					"<invalid>",
					fmt.Sprintf("Trait %q in Release has invalid types schema: %v", traitName, err)))
				continue
			}
		}

		var envOverridesSchema map[string]any
		if err := yaml.Unmarshal(traitSpec.Schema.EnvOverrides.Raw, &envOverridesSchema); err != nil {
			allErrs = append(allErrs, field.Invalid(
				overridePath,
				"<invalid>",
				fmt.Sprintf("Trait %q in Release has invalid envOverrides schema: %v", traitName, err)))
			continue
		}

		schemaDef := schema.Definition{
			Types:   types,
			Schemas: []map[string]any{envOverridesSchema},
		}

		// Convert to JSON schema for validation
		jsonSchema, err := schema.ToJSONSchema(schemaDef)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(
				overridePath,
				"<invalid>",
				fmt.Sprintf("Trait %q in Release has invalid schema definition: %v", traitName, err)))
			continue
		}

		// Unmarshal trait override
		var traitOverride map[string]any
		if err := yaml.Unmarshal(override.Raw, &traitOverride); err != nil {
			allErrs = append(allErrs, field.Invalid(
				overridePath,
				"<invalid>",
				fmt.Sprintf("failed to parse trait override: %v", err)))
			continue
		}

		// Validate override against schema
		if err := schema.ValidateWithJSONSchema(traitOverride, jsonSchema); err != nil {
			allErrs = append(allErrs, field.Invalid(
				overridePath,
				"<invalid>",
				fmt.Sprintf("trait override does not match Trait %q envOverrides schema: %v", traitName, err)))
		}
	}

	return allErrs
}
