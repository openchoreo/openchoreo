// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrolebinding

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzrolebindingwebhook "github.com/openchoreo/openchoreo/internal/webhook/authzrolebinding"
)

var log = logf.Log.WithName("clusterauthzrolebinding-webhook")

// SetupClusterAuthzRoleBindingWebhookWithManager registers the validating webhook for ClusterAuthzRoleBinding.
func SetupClusterAuthzRoleBindingWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&openchoreodevv1alpha1.ClusterAuthzRoleBinding{}).
		WithValidator(&ClusterAuthzRoleBindingValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-clusterauthzrolebinding,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=clusterauthzrolebindings,verbs=create;update,versions=v1alpha1,name=vclusterauthzrolebinding-v1alpha1.kb.io,admissionReviewVersions=v1

// ClusterAuthzRoleBindingValidator validates ClusterAuthzRoleBinding resources.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ClusterAuthzRoleBindingValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &ClusterAuthzRoleBindingValidator{}

func (v *ClusterAuthzRoleBindingValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rb, ok := obj.(*openchoreodevv1alpha1.ClusterAuthzRoleBinding)
	if !ok {
		return nil, fmt.Errorf("expected ClusterAuthzRoleBinding, got %T", obj)
	}
	log.Info("Validation for ClusterAuthzRoleBinding upon creation", "name", rb.GetName())
	if errs := validateClusterRoleMappings(rb.Spec.RoleMappings); len(errs) > 0 {
		return nil, apierrors.NewInvalid(rb.GroupVersionKind().GroupKind(), rb.GetName(), errs)
	}
	return nil, nil
}

func (v *ClusterAuthzRoleBindingValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	rb, ok := newObj.(*openchoreodevv1alpha1.ClusterAuthzRoleBinding)
	if !ok {
		return nil, fmt.Errorf("expected ClusterAuthzRoleBinding, got %T", newObj)
	}
	log.Info("Validation for ClusterAuthzRoleBinding upon update", "name", rb.GetName())
	if errs := validateClusterRoleMappings(rb.Spec.RoleMappings); len(errs) > 0 {
		return nil, apierrors.NewInvalid(rb.GroupVersionKind().GroupKind(), rb.GetName(), errs)
	}
	return nil, nil
}

func (v *ClusterAuthzRoleBindingValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rb, ok := obj.(*openchoreodevv1alpha1.ClusterAuthzRoleBinding)
	if !ok {
		return nil, fmt.Errorf("expected ClusterAuthzRoleBinding, got %T", obj)
	}
	log.Info("Validation for ClusterAuthzRoleBinding upon deletion", "name", rb.GetName())
	return nil, nil
}

func validateClusterRoleMappings(mappings []openchoreodevv1alpha1.ClusterRoleMapping) field.ErrorList {
	var allErrs field.ErrorList
	basePath := field.NewPath("spec").Child("roleMappings")
	for i, m := range mappings {
		for j, cond := range m.Conditions {
			condPath := basePath.Index(i).Child("conditions").Index(j)
			allErrs = append(allErrs, authzrolebindingwebhook.ValidateCondition(cond, condPath)...)
		}
	}
	return allErrs
}
