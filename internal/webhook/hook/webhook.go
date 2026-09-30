// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hook

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
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/validation/hooks"
)

// nolint:unused
// log is for logging in this package.
var hooklog = logf.Log.WithName("hook-resource")

// SetupHookWebhookWithManager registers the webhook for Hook in the manager.
func SetupHookWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &openchoreodevv1alpha1.Hook{}).
		WithCustomValidator(&Validator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-hook,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=hooks,verbs=create;update,versions=v1alpha1,name=vhook-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates Hook resources
// +kubebuilder:object:generate=false
type Validator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Hook.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	hk, ok := obj.(*openchoreodevv1alpha1.Hook)
	if !ok {
		return nil, fmt.Errorf("expected a Hook object but got %T", obj)
	}
	hooklog.Info("Validation for Hook upon creation", "name", hk.GetName())

	warnings, allErrs := ValidateHook(ctx, v.Client, hk.Namespace, &hk.Spec, false)
	if len(allErrs) > 0 {
		return warnings, apierrors.NewInvalid(hk.GroupVersionKind().GroupKind(), hk.GetName(), allErrs)
	}
	return warnings, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Hook.
func (v *Validator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	hk, ok := newObj.(*openchoreodevv1alpha1.Hook)
	if !ok {
		return nil, fmt.Errorf("expected a Hook object for the newObj but got %T", newObj)
	}
	hooklog.Info("Validation for Hook upon update", "name", hk.GetName())

	warnings, allErrs := ValidateHook(ctx, v.Client, hk.Namespace, &hk.Spec, false)
	if len(allErrs) > 0 {
		return warnings, apierrors.NewInvalid(hk.GroupVersionKind().GroupKind(), hk.GetName(), allErrs)
	}
	return warnings, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Hook.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	hk, ok := obj.(*openchoreodevv1alpha1.Hook)
	if !ok {
		return nil, fmt.Errorf("expected a Hook object but got %T", obj)
	}
	hooklog.Info("Validation for Hook upon deletion", "name", hk.GetName())

	// No special validation needed for deletion
	return nil, nil
}

// ValidateHook validates a Hook or ClusterHook spec and, when the referenced
// workflow exists, checks the parameter mapping against the workflow's inputs.
// A workflow that does not exist yet is a warning, not an error, so hooks and
// workflows can be applied in any order. Exported for reuse by the ClusterHook webhook.
func ValidateHook(
	ctx context.Context,
	c client.Client,
	namespace string,
	spec *openchoreodevv1alpha1.HookSpec,
	isCluster bool,
) (admission.Warnings, field.ErrorList) {
	specPath := field.NewPath("spec")
	allErrs := hooks.ValidateHookSpec(spec, isCluster, specPath)
	if len(allErrs) > 0 || c == nil || spec.WorkflowRef == nil {
		return nil, allErrs
	}

	result, err := controller.ResolveWorkflow(ctx, c, namespace, spec.WorkflowRef.Kind, spec.WorkflowRef.Name)
	if err != nil {
		return admission.Warnings{fmt.Sprintf("%s: %v; the hook will fail at deployment until the workflow exists",
			specPath.Child("workflowRef"), err)}, allErrs
	}

	wfSpec := result.GetWorkflowSpec()
	allErrs = append(allErrs, hooks.ValidateParametersAgainstWorkflow(
		spec.Parameters, result.GetName(), &wfSpec, specPath.Child("parameters"))...)
	return nil, allErrs
}
