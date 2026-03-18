// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	workflowwebhook "github.com/openchoreo/openchoreo/internal/webhook/workflow"
)

// nolint:unused
// log is for logging in this package.
var clusterworkflowlog = logf.Log.WithName("clusterworkflow-resource")

// SetupClusterWorkflowWebhookWithManager registers the webhook for ClusterWorkflow in the manager.
func SetupClusterWorkflowWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.ClusterWorkflow{}).
		WithValidator(&Validator{}).
		WithDefaulter(&Defaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-clusterworkflow,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=clusterworkflows,verbs=create;update,versions=v1alpha1,name=vclusterworkflow-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates ClusterWorkflow resources
// +kubebuilder:object:generate=false
type Validator struct{}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ClusterWorkflow.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cwf, ok := obj.(*openchoreodevv1alpha1.ClusterWorkflow)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterWorkflow object but got %T", obj)
	}
	clusterworkflowlog.Info("Validation for ClusterWorkflow upon creation", "name", cwf.GetName())

	allErrs := validateClusterWorkflow(cwf)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ClusterWorkflow.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newCwf, ok := newObj.(*openchoreodevv1alpha1.ClusterWorkflow)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterWorkflow object for the newObj but got %T", newObj)
	}
	clusterworkflowlog.Info("Validation for ClusterWorkflow upon update", "name", newCwf.GetName())

	allErrs := validateClusterWorkflow(newCwf)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ClusterWorkflow.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cwf, ok := obj.(*openchoreodevv1alpha1.ClusterWorkflow)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterWorkflow object but got %T", obj)
	}
	clusterworkflowlog.Info("Validation for ClusterWorkflow upon deletion", "name", cwf.GetName())

	return nil, nil
}

// +kubebuilder:webhook:path=/mutate-openchoreo-dev-v1alpha1-clusterworkflow,mutating=true,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=clusterworkflows,verbs=create;update,versions=v1alpha1,name=mclusterworkflow-v1alpha1.kb.io,admissionReviewVersions=v1

// Defaulter sets defaults on ClusterWorkflow resources
// +kubebuilder:object:generate=false
type Defaulter struct{}

var _ webhook.CustomDefaulter = &Defaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type ClusterWorkflow.
func (d *Defaulter) Default(_ context.Context, obj runtime.Object) error {
	cwf, ok := obj.(*openchoreodevv1alpha1.ClusterWorkflow)
	if !ok {
		return fmt.Errorf("expected a ClusterWorkflow object but got %T", obj)
	}
	clusterworkflowlog.Info("Defaulting for ClusterWorkflow", "name", cwf.GetName())

	return workflowwebhook.InjectServiceAccountName(cwf.Spec.RunTemplate)
}

// validateClusterWorkflow performs all validation for a ClusterWorkflow.
func validateClusterWorkflow(cwf *openchoreodevv1alpha1.ClusterWorkflow) field.ErrorList {
	allErrs := field.ErrorList{}

	// ClusterWorkflow scoping constraint: workflowPlaneRef.kind must be ClusterWorkflowPlane
	if cwf.Spec.WorkflowPlaneRef != nil &&
		cwf.Spec.WorkflowPlaneRef.Kind != openchoreodevv1alpha1.ClusterWorkflowPlaneRefKindClusterWorkflowPlane {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "workflowPlaneRef", "kind"),
			string(cwf.Spec.WorkflowPlaneRef.Kind),
			"ClusterWorkflow can only reference ClusterWorkflowPlane, not namespace-scoped WorkflowPlane",
		))
	}

	// Reuse shared validation logic
	allErrs = append(allErrs, workflowwebhook.ValidateWorkflowSpec(
		cwf.Spec.RunTemplate, cwf.Spec.Resources, cwf.Spec.ExternalRefs, cwf.Spec.Parameters,
	)...)

	return allErrs
}
