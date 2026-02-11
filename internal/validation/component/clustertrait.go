// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"

	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ValidateClusterTraitCreatesAndPatchesWithSchema validates all creates and patches in a ClusterTrait with schema-aware type checking.
// It checks CEL expressions, forEach loops, and ensures proper variable usage.
//
// Parameters:
//   - ct: The ClusterTrait to validate
//   - parametersSchema: Structural schema for parameters (from ClusterTrait.Schema.Parameters)
//   - envOverridesSchema: Structural schema for envOverrides (from ClusterTrait.Schema.EnvOverrides)
//
// If schemas are nil, DynType will be used for those variables (no static type checking).
// This provides better error messages by catching type errors at validation time.
func ValidateClusterTraitCreatesAndPatchesWithSchema(
	ct *v1alpha1.ClusterTrait,
	parametersSchema *apiextschema.Structural,
	envOverridesSchema *apiextschema.Structural,
) field.ErrorList {
	allErrs := field.ErrorList{}

	// Create schema-aware validator for trait context
	validator, err := NewCELValidator(TraitResource, SchemaOptions{
		ParametersSchema:   parametersSchema,
		EnvOverridesSchema: envOverridesSchema,
	})
	if err != nil {
		allErrs = append(allErrs, field.InternalError(
			field.NewPath("spec"),
			fmt.Errorf("failed to create CEL validator: %w", err)))
		return allErrs
	}

	basePath := field.NewPath("spec")

	// Validate creates
	for i, create := range ct.Spec.Creates {
		createPath := basePath.Child("creates").Index(i)
		errs := ValidateTraitCreate(create, validator, createPath)
		allErrs = append(allErrs, errs...)
	}

	// Validate patches
	for i, patch := range ct.Spec.Patches {
		patchPath := basePath.Child("patches").Index(i)
		errs := ValidateTraitPatch(patch, validator, patchPath)
		allErrs = append(allErrs, errs...)
	}

	return allErrs
}
