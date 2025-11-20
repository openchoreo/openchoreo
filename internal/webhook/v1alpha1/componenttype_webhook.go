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
var componenttypelog = logf.Log.WithName("componenttype-resource")

// SetupComponentTypeWebhookWithManager registers the webhook for ComponentType in the manager.
func SetupComponentTypeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.ComponentType{}).
		WithValidator(&ComponentTypeCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-componenttype,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=componenttypes,verbs=create;update,versions=v1alpha1,name=vcomponenttype-v1alpha1.kb.io,admissionReviewVersions=v1

// ComponentTypeCustomValidator struct is responsible for validating the ComponentType resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ComponentTypeCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &ComponentTypeCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ComponentType.
func (v *ComponentTypeCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	componenttype, ok := obj.(*openchoreodevv1alpha1.ComponentType)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentType object but got %T", obj)
	}
	componenttypelog.Info("Validation for ComponentType upon creation", "name", componenttype.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ComponentType.
func (v *ComponentTypeCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	componenttype, ok := newObj.(*openchoreodevv1alpha1.ComponentType)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentType object for the newObj but got %T", newObj)
	}
	componenttypelog.Info("Validation for ComponentType upon update", "name", componenttype.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ComponentType.
func (v *ComponentTypeCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	componenttype, ok := obj.(*openchoreodevv1alpha1.ComponentType)
	if !ok {
		return nil, fmt.Errorf("expected a ComponentType object but got %T", obj)
	}
	componenttypelog.Info("Validation for ComponentType upon deletion", "name", componenttype.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
