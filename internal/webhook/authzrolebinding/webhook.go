// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrolebinding

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var log = logf.Log.WithName("authzrolebinding-webhook")

// SetupAuthzRoleBindingWebhookWithManager registers the validating webhook for AuthzRoleBinding.
func SetupAuthzRoleBindingWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&openchoreodevv1alpha1.AuthzRoleBinding{}).
		WithValidator(&AuthzRoleBindingValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-authzrolebinding,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=authzrolebindings,verbs=create;update,versions=v1alpha1,name=vauthzrolebinding-v1alpha1.kb.io,admissionReviewVersions=v1

// AuthzRoleBindingValidator validates AuthzRoleBinding resources.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type AuthzRoleBindingValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &AuthzRoleBindingValidator{}

func (v *AuthzRoleBindingValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rb, ok := obj.(*openchoreodevv1alpha1.AuthzRoleBinding)
	if !ok {
		return nil, fmt.Errorf("expected AuthzRoleBinding, got %T", obj)
	}
	log.Info("Validation for AuthzRoleBinding upon creation", "name", rb.GetName())
	if errs := validateRoleMappings(rb.Spec.RoleMappings); len(errs) > 0 {
		return nil, apierrors.NewInvalid(rb.GroupVersionKind().GroupKind(), rb.GetName(), errs)
	}
	return nil, nil
}

func (v *AuthzRoleBindingValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	rb, ok := newObj.(*openchoreodevv1alpha1.AuthzRoleBinding)
	if !ok {
		return nil, fmt.Errorf("expected AuthzRoleBinding, got %T", newObj)
	}
	log.Info("Validation for AuthzRoleBinding upon update", "name", rb.GetName())
	if errs := validateRoleMappings(rb.Spec.RoleMappings); len(errs) > 0 {
		return nil, apierrors.NewInvalid(rb.GroupVersionKind().GroupKind(), rb.GetName(), errs)
	}
	return nil, nil
}

func (v *AuthzRoleBindingValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rb, ok := obj.(*openchoreodevv1alpha1.AuthzRoleBinding)
	if !ok {
		return nil, fmt.Errorf("expected AuthzRoleBinding, got %T", obj)
	}
	log.Info("Validation for AuthzRoleBinding upon deletion", "name", rb.GetName())
	return nil, nil
}
