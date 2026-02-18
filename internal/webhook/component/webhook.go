// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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
	// Note: Cross-resource validation (ComponentType, Trait, schema validation) is handled by the controller

	// Validate unique trait instance names
	allErrs = append(allErrs, validateUniqueTraitInstanceNames(component)...)

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

	// Note: Required field validations (componentType, owner.projectName, traits.name, traits.instanceName) are enforced by the CRD schema
	// Note: spec.componentType immutability is enforced by CEL rules in the CRD schema
	// Note: Cross-resource validation (ComponentType, Trait, schema validation) is handled by the controller

	// Validate unique trait instance names
	allErrs = append(allErrs, validateUniqueTraitInstanceNames(newComponent)...)

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

// validateUniqueTraitInstanceNames validates that trait instance names are unique within a component
func validateUniqueTraitInstanceNames(component *openchoreodevv1alpha1.Component) field.ErrorList {
	allErrs := field.ErrorList{}
	instanceNames := make(map[string]bool)

	for i, trait := range component.Spec.Traits {
		if instanceNames[trait.InstanceName] {
			allErrs = append(allErrs, field.Duplicate(
				field.NewPath("spec", "traits").Index(i).Child("instanceName"),
				trait.InstanceName))
		}
		instanceNames[trait.InstanceName] = true
	}

	return allErrs
}
