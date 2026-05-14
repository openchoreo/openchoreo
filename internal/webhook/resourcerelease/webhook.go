// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

import (
	"context"
	"encoding/json"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
	resourcevalidation "github.com/openchoreo/openchoreo/internal/validation/resource"
)

//nolint:unused // exported via webhook bootstrap; keeps parity with sibling webhooks
var resourcereleaselog = logf.Log.WithName("resourcerelease-resource")

// omitValue suppresses raw values in field.Invalid errors when the value
// itself isn't useful to the operator reading the error.
var omitValue = field.OmitValueType{}

// SetupResourceReleaseWebhookWithManager registers the validating webhook for
// ResourceRelease in the manager.
func SetupResourceReleaseWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreodevv1alpha1.ResourceRelease{}).
		WithValidator(&Validator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-resourcerelease,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=resourcereleases,verbs=create;update,versions=v1alpha1,name=vresourcerelease-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates ResourceRelease resources.
// +kubebuilder:object:generate=false
type Validator struct{}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate runs the on-snapshot checks: parameters against the embedded
// (Cluster)ResourceType.Spec.Parameters schema, and a re-validation of the
// embedded ResourceType.Spec for schema drift since the release was cut.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rr, ok := obj.(*openchoreodevv1alpha1.ResourceRelease)
	if !ok {
		return nil, fmt.Errorf("expected a ResourceRelease object but got %T", obj)
	}
	resourcereleaselog.Info("Validation for ResourceRelease upon creation", "name", rr.GetName())

	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateParametersAgainstSnapshot(rr)...)
	allErrs = append(allErrs, validateEmbeddedResourceType(rr)...)

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(rr.GroupVersionKind().GroupKind(), rr.GetName(), allErrs)
	}
	return nil, nil
}

// ValidateUpdate is a no-op. ResourceRelease.spec is immutable via a
// spec-level CEL XValidation marker (api/v1alpha1/resourcerelease_types.go:16);
// any update either matches the existing spec or is rejected at the CRD layer
// before reaching this webhook.
func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	if _, ok := oldObj.(*openchoreodevv1alpha1.ResourceRelease); !ok {
		return nil, fmt.Errorf("expected a ResourceRelease object for the oldObj but got %T", oldObj)
	}
	if _, ok := newObj.(*openchoreodevv1alpha1.ResourceRelease); !ok {
		return nil, fmt.Errorf("expected a ResourceRelease object for the newObj but got %T", newObj)
	}
	return nil, nil
}

// ValidateDelete is a no-op. ResourceRelease deletion is driven by the
// owning Resource's finalizer.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	rr, ok := obj.(*openchoreodevv1alpha1.ResourceRelease)
	if !ok {
		return nil, fmt.Errorf("expected a ResourceRelease object but got %T", obj)
	}
	resourcereleaselog.Info("Validation for ResourceRelease upon deletion", "name", rr.GetName())
	return nil, nil
}

// validateParametersAgainstSnapshot validates spec.parameters against the
// embedded snapshot's parameters.openAPIV3Schema. Mirrors
// internal/webhook/componentrelease/webhook.go::validateParamsAgainstSchema —
// the snapshot lives on the same object, so no cross-CRD lookup is needed.
func validateParametersAgainstSnapshot(rr *openchoreodevv1alpha1.ResourceRelease) field.ErrorList {
	section := rr.Spec.ResourceType.Spec.Parameters
	if section == nil {
		return nil
	}

	rawSchema := section.GetRaw()
	if rawSchema == nil || len(rawSchema.Raw) == 0 {
		return nil
	}

	fieldPath := field.NewPath("spec", "parameters")

	jsonSchema, err := schema.SectionToJSONSchema(section)
	if err != nil {
		return field.ErrorList{field.Invalid(fieldPath, omitValue,
			fmt.Sprintf("ResourceType snapshot has invalid parameters schema: %v", err))}
	}

	var params map[string]any
	if rr.Spec.Parameters != nil && len(rr.Spec.Parameters.Raw) > 0 {
		if err := json.Unmarshal(rr.Spec.Parameters.Raw, &params); err != nil {
			return field.ErrorList{field.Invalid(fieldPath, omitValue,
				fmt.Sprintf("failed to parse parameters: %v", err))}
		}
	} else {
		params = map[string]any{}
	}

	if err := schema.ValidateWithJSONSchema(params, jsonSchema); err != nil {
		return field.ErrorList{field.Invalid(fieldPath, omitValue,
			fmt.Sprintf("parameters do not match ResourceType schema: %v", err))}
	}
	return nil
}

// validateEmbeddedResourceType re-runs the resource-type spec validator
// against the embedded snapshot. Defends against schema drift between when
// the release was cut and admission today: a webhook upgrade can tighten CEL
// rules that previously admitted a permissive ResourceType, and we want
// those releases to be rejected at admission rather than silently rendering
// failed at runtime. Mirrors componentrelease.validateEmbeddedResourceTemplates.
func validateEmbeddedResourceType(rr *openchoreodevv1alpha1.ResourceRelease) field.ErrorList {
	specPath := field.NewPath("spec", "resourceType", "spec")
	return resourcevalidation.ValidateResourceTypeSpec(&rr.Spec.ResourceType.Spec, specPath)
}
