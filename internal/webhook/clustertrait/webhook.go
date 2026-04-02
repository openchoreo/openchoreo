// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

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
	"github.com/openchoreo/openchoreo/internal/validation/component"
	"github.com/openchoreo/openchoreo/internal/validation/schemautil"
)

// nolint:unused
// log is for logging in this package.
var clustertraitlog = logf.Log.WithName("clustertrait-resource")

// SetupClusterTraitWebhookWithManager registers the webhook for ClusterTrait in the manager.
func SetupClusterTraitWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.ClusterTrait{}).
		WithValidator(&Validator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-clustertrait,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=clustertraits,verbs=create;update,versions=v1alpha1,name=vclustertrait-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates ClusterTrait resources
// +kubebuilder:object:generate=false
type Validator struct{}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ClusterTrait.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ct, ok := obj.(*openchoreodevv1alpha1.ClusterTrait)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterTrait object but got %T", obj)
	}
	clustertraitlog.Info("Validation for ClusterTrait upon creation", "name", ct.GetName())

	allErrs := field.ErrorList{}

	// Extract and validate schemas, getting structural schemas for CEL validation
	parametersSchema, envConfigsSchema, schemaErrs := schemautil.ExtractAndValidateSchemas(
		ct.Spec.Parameters, ct.Spec.EnvironmentConfigs, field.NewPath("spec"),
	)
	allErrs = append(allErrs, schemaErrs...)

	allErrs = append(allErrs, component.ValidateTraitCreateTemplates(
		ct.Spec.Creates, field.NewPath("spec", "creates"))...)

	allErrs = append(allErrs, component.ValidateClusterTraitCreatesAndPatchesWithSchema(
		ct, parametersSchema, envConfigsSchema,
	)...)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ClusterTrait.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newClusterTrait, ok := newObj.(*openchoreodevv1alpha1.ClusterTrait)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterTrait object for the newObj but got %T", newObj)
	}
	clustertraitlog.Info("Validation for ClusterTrait upon update", "name", newClusterTrait.GetName())

	allErrs := field.ErrorList{}

	// Extract and validate schemas, getting structural schemas for CEL validation
	parametersSchema, envConfigsSchema, schemaErrs := schemautil.ExtractAndValidateSchemas(
		newClusterTrait.Spec.Parameters, newClusterTrait.Spec.EnvironmentConfigs, field.NewPath("spec"),
	)
	allErrs = append(allErrs, schemaErrs...)

	allErrs = append(allErrs, component.ValidateTraitCreateTemplates(
		newClusterTrait.Spec.Creates, field.NewPath("spec", "creates"))...)

	allErrs = append(allErrs, component.ValidateClusterTraitCreatesAndPatchesWithSchema(
		newClusterTrait, parametersSchema, envConfigsSchema,
	)...)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ClusterTrait.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ct, ok := obj.(*openchoreodevv1alpha1.ClusterTrait)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterTrait object but got %T", obj)
	}
	clustertraitlog.Info("Validation for ClusterTrait upon deletion", "name", ct.GetName())

	// No special validation needed for deletion
	return nil, nil
}
