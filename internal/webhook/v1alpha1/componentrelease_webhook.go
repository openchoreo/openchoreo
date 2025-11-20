// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var componentreleaselog = logf.Log.WithName("componentrelease-resource")

// SetupComponentReleaseWebhookWithManager registers the webhook for ComponentRelease in the manager.
func SetupComponentReleaseWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.ComponentRelease{}).
		WithValidator(&ComponentReleaseCustomValidator{}).
		WithDefaulter(&ComponentReleaseCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-openchoreo-dev-v1alpha1-componentrelease,mutating=true,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=componentreleases,verbs=create;update,versions=v1alpha1,name=mcomponentrelease-v1alpha1.kb.io,admissionReviewVersions=v1

// ComponentReleaseCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind ComponentRelease when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ComponentReleaseCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &ComponentReleaseCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind ComponentRelease.
func (d *ComponentReleaseCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	componentrelease, ok := obj.(*openchoreodevv1alpha1.ComponentRelease)

	if !ok {
		return fmt.Errorf("expected an ComponentRelease object but got %T", obj)
	}
	componentreleaselog.Info("Defaulting for ComponentRelease", "name", componentrelease.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-componentrelease,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=componentreleases,verbs=create;update,versions=v1alpha1,name=vcomponentrelease-v1alpha1.kb.io,admissionReviewVersions=v1

// ComponentReleaseCustomValidator struct is responsible for validating the ComponentRelease resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ComponentReleaseCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &ComponentReleaseCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ComponentRelease.
func (v *ComponentReleaseCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	componentrelease, ok := obj.(*openchoreodevv1alpha1.ComponentRelease)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentRelease object but got %T", obj)
	}
	componentreleaselog.Info("Validation for ComponentRelease upon creation", "name", componentrelease.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ComponentRelease.
func (v *ComponentReleaseCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	componentrelease, ok := newObj.(*openchoreodevv1alpha1.ComponentRelease)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentRelease object for the newObj but got %T", newObj)
	}
	componentreleaselog.Info("Validation for ComponentRelease upon update", "name", componentrelease.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ComponentRelease.
func (v *ComponentReleaseCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	componentrelease, ok := obj.(*openchoreodevv1alpha1.ComponentRelease)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentRelease object but got %T", obj)
	}
	componentreleaselog.Info("Validation for ComponentRelease upon deletion", "name", componentrelease.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
