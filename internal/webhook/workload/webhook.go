// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

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
var workloadlog = logf.Log.WithName("workload-resource")

// SetupWorkloadWebhookWithManager registers the webhook for Workload in the manager.
func SetupWorkloadWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.Workload{}).
		WithValidator(&Validator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-workload,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=workloads,verbs=create;update,versions=v1alpha1,name=vworkload-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator struct is responsible for validating the Workload resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
// +kubebuilder:object:generate=false
type Validator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Workload.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	workload, ok := obj.(*openchoreodevv1alpha1.Workload)
	if !ok {
		return nil, fmt.Errorf("expected a Workload object but got %T", obj)
	}
	workloadlog.Info("Validation for Workload upon creation", "name", workload.GetName())

	allErrs := field.ErrorList{}

	// Check that no other Workload exists for the same Component in this namespace
	errs := v.validateUniqueWorkloadPerComponent(ctx, workload, "")
	allErrs = append(allErrs, errs...)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Workload.
func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	_, ok := oldObj.(*openchoreodevv1alpha1.Workload)
	if !ok {
		return nil, fmt.Errorf("expected a Workload object for the oldObj but got %T", oldObj)
	}

	newWorkload, ok := newObj.(*openchoreodevv1alpha1.Workload)
	if !ok {
		return nil, fmt.Errorf("expected a Workload object for the newObj but got %T", newObj)
	}
	workloadlog.Info("Validation for Workload upon update", "name", newWorkload.GetName())

	allErrs := field.ErrorList{}

	// Owner immutability is enforced by CEL, but also check uniqueness in case of reassignment
	errs := v.validateUniqueWorkloadPerComponent(ctx, newWorkload, newWorkload.Name)
	allErrs = append(allErrs, errs...)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Workload.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	workload, ok := obj.(*openchoreodevv1alpha1.Workload)
	if !ok {
		return nil, fmt.Errorf("expected a Workload object but got %T", obj)
	}
	workloadlog.Info("Validation for Workload upon deletion", "name", workload.GetName())

	// No special validation needed for deletion
	return nil, nil
}

// validateUniqueWorkloadPerComponent checks that only one Workload exists per Component.
// excludeName is the name of the current Workload to exclude from the check (used during updates).
func (v *Validator) validateUniqueWorkloadPerComponent(
	ctx context.Context,
	workload *openchoreodevv1alpha1.Workload,
	excludeName string,
) field.ErrorList {
	allErrs := field.ErrorList{}

	existingWorkloads := &openchoreodevv1alpha1.WorkloadList{}
	if err := v.Client.List(ctx, existingWorkloads, client.InNamespace(workload.Namespace)); err != nil {
		allErrs = append(allErrs, field.InternalError(
			field.NewPath("spec", "owner"),
			fmt.Errorf("failed to list existing workloads: %w", err),
		))
		return allErrs
	}

	for i := range existingWorkloads.Items {
		existing := &existingWorkloads.Items[i]
		if existing.Name == excludeName {
			continue
		}
		if existing.Spec.Owner.ProjectName == workload.Spec.Owner.ProjectName &&
			existing.Spec.Owner.ComponentName == workload.Spec.Owner.ComponentName {
			allErrs = append(allErrs, field.Forbidden(
				field.NewPath("spec", "owner"),
				fmt.Sprintf(
					"a workload %q already exists for component %q in project %q",
					existing.Name,
					workload.Spec.Owner.ComponentName,
					workload.Spec.Owner.ProjectName,
				),
			))
			break
		}
	}

	return allErrs
}
