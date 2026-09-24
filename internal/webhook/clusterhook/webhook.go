// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterhook

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
	hookwebhook "github.com/openchoreo/openchoreo/internal/webhook/hook"
)

// nolint:unused
// log is for logging in this package.
var clusterhooklog = logf.Log.WithName("clusterhook-resource")

// SetupClusterHookWebhookWithManager registers the webhook for ClusterHook in the manager.
func SetupClusterHookWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &openchoreodevv1alpha1.ClusterHook{}).
		WithCustomValidator(&Validator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-clusterhook,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=clusterhooks,verbs=create;update,versions=v1alpha1,name=vclusterhook-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates ClusterHook resources
// +kubebuilder:object:generate=false
type Validator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ClusterHook.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	chk, ok := obj.(*openchoreodevv1alpha1.ClusterHook)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterHook object but got %T", obj)
	}
	clusterhooklog.Info("Validation for ClusterHook upon creation", "name", chk.GetName())

	warnings, allErrs := hookwebhook.ValidateHook(ctx, v.Client, "", &chk.Spec, true)
	if len(allErrs) > 0 {
		return warnings, apierrors.NewInvalid(chk.GroupVersionKind().GroupKind(), chk.GetName(), allErrs)
	}
	return warnings, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ClusterHook.
func (v *Validator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	chk, ok := newObj.(*openchoreodevv1alpha1.ClusterHook)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterHook object for the newObj but got %T", newObj)
	}
	clusterhooklog.Info("Validation for ClusterHook upon update", "name", chk.GetName())

	warnings, allErrs := hookwebhook.ValidateHook(ctx, v.Client, "", &chk.Spec, true)
	if len(allErrs) > 0 {
		return warnings, apierrors.NewInvalid(chk.GroupVersionKind().GroupKind(), chk.GetName(), allErrs)
	}
	return warnings, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ClusterHook.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	chk, ok := obj.(*openchoreodevv1alpha1.ClusterHook)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterHook object but got %T", obj)
	}
	clusterhooklog.Info("Validation for ClusterHook upon deletion", "name", chk.GetName())

	// No special validation needed for deletion
	return nil, nil
}
