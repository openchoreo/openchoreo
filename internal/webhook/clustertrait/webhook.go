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
	basePath := field.NewPath("spec", "schema")
	parametersSchema, envOverridesSchema, schemaErrs := schemautil.ExtractStructuralSchemas(&ct.Spec.Schema, basePath)
	allErrs = append(allErrs, schemaErrs...)

	templateErrs := validateClusterTraitCreatesTemplateStructure(ct)
	allErrs = append(allErrs, templateErrs...)

	// Validate CEL expressions with schema-aware type checking
	celErrs := component.ValidateClusterTraitCreatesAndPatchesWithSchema(
		ct,
		parametersSchema,
		envOverridesSchema,
	)
	allErrs = append(allErrs, celErrs...)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ClusterTrait.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newCT, ok := newObj.(*openchoreodevv1alpha1.ClusterTrait)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterTrait object for the newObj but got %T", newObj)
	}
	clustertraitlog.Info("Validation for ClusterTrait upon update", "name", newCT.GetName())

	allErrs := field.ErrorList{}

	// Extract and validate schemas, getting structural schemas for CEL validation
	basePath := field.NewPath("spec", "schema")
	parametersSchema, envOverridesSchema, schemaErrs := schemautil.ExtractStructuralSchemas(&newCT.Spec.Schema, basePath)
	allErrs = append(allErrs, schemaErrs...)

	templateErrs := validateClusterTraitCreatesTemplateStructure(newCT)
	allErrs = append(allErrs, templateErrs...)

	// Validate CEL expressions with schema-aware type checking
	celErrs := component.ValidateClusterTraitCreatesAndPatchesWithSchema(
		newCT,
		parametersSchema,
		envOverridesSchema,
	)
	allErrs = append(allErrs, celErrs...)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ClusterTrait.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	ct, ok := obj.(*openchoreodevv1alpha1.ClusterTrait)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterTrait object but got %T", obj)
	}
	clustertraitlog.Info("Validation for ClusterTrait upon deletion", "name", ct.GetName())

	// No special validation needed for deletion
	return nil, nil
}

// validateClusterTraitCreatesTemplateStructure validates that cluster trait creates templates have required K8s resource fields (apiVersion, kind, metadata.name)
func validateClusterTraitCreatesTemplateStructure(ct *openchoreodevv1alpha1.ClusterTrait) field.ErrorList {
	allErrs := field.ErrorList{}
	basePath := field.NewPath("spec", "creates")

	for i, create := range ct.Spec.Creates {
		createPath := basePath.Index(i)
		templatePath := createPath.Child("template")

		if create.Template == nil {
			allErrs = append(allErrs, field.Required(templatePath, "template is required"))
			continue
		}

		_, errs := component.ValidateResourceTemplateStructure(*create.Template, templatePath)
		allErrs = append(allErrs, errs...)
	}

	return allErrs
}
