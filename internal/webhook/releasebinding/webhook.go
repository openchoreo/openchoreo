// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

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
var releasebindinglog = logf.Log.WithName("releasebinding-resource")

// SetupReleaseBindingWebhookWithManager registers the webhook for ReleaseBinding in the manager.
func SetupReleaseBindingWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.ReleaseBinding{}).
		WithValidator(&Validator{}).
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
	releasebinding, ok := obj.(*openchoreodevv1alpha1.ReleaseBinding)

	if !ok {
		return fmt.Errorf("expected an ReleaseBinding object but got %T", obj)
	}
	releasebindinglog.Info("Defaulting for ReleaseBinding", "name", releasebinding.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-releasebinding,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=releasebindings,verbs=create;update,versions=v1alpha1,name=vreleasebinding-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator struct is responsible for validating the ReleaseBinding resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type Validator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ReleaseBinding.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	releasebinding, ok := obj.(*openchoreodevv1alpha1.ReleaseBinding)
	if !ok {
		return nil, fmt.Errorf("expected a ReleaseBinding object but got %T", obj)
	}
	releasebindinglog.Info("Validation for ReleaseBinding upon creation", "name", releasebinding.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ReleaseBinding.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	releasebinding, ok := newObj.(*openchoreodevv1alpha1.ReleaseBinding)
	if !ok {
		return nil, fmt.Errorf("expected a ReleaseBinding object for the newObj but got %T", newObj)
	}
	releasebindinglog.Info("Validation for ReleaseBinding upon update", "name", releasebinding.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ReleaseBinding.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	releasebinding, ok := obj.(*openchoreodevv1alpha1.ReleaseBinding)
	if !ok {
		return nil, fmt.Errorf("expected a ReleaseBinding object but got %T", obj)
	}
	releasebindinglog.Info("Validation for ReleaseBinding upon deletion", "name", releasebinding.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
