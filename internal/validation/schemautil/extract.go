// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemautil

import (
	"fmt"

	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
)

// omitValue is used to omit the value from field.Invalid error messages
var omitValue = field.OmitValueType{}

// ExtractStructuralSchemas extracts and builds structural schemas from SchemaSection inputs.
// It handles both ocSchema and openAPIV3Schema formats transparently.
// Returns parameters schema, environmentConfigs schema, and any validation errors.
func ExtractStructuralSchemas(
	parameters *v1alpha1.SchemaSection,
	environmentConfigs *v1alpha1.SchemaSection,
	basePath *field.Path,
) (*apiextschema.Structural, *apiextschema.Structural, field.ErrorList) {
	allErrs := field.ErrorList{}

	// Validate that parameters and environmentConfigs use the same schema format
	allErrs = append(allErrs, validateSchemaFormatConsistency(parameters, environmentConfigs, basePath)...)

	// Extract and build parameters structural schema
	parametersSchema, err := schema.ResolveSectionToStructural(parameters)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(
			basePath.Child("parameters"),
			omitValue,
			fmt.Sprintf("failed to parse parameters schema: %v", err)))
	}

	// Extract and build environmentConfigs structural schema
	envConfigsSchema, err := schema.ResolveSectionToStructural(environmentConfigs)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(
			basePath.Child("environmentConfigs"),
			omitValue,
			fmt.Sprintf("failed to parse environmentConfigs schema: %v", err)))
	}

	return parametersSchema, envConfigsSchema, allErrs
}

// validateSchemaFormatConsistency checks that when both parameters and environmentConfigs are
// provided, they use the same schema format (both ocSchema or both openAPIV3Schema).
func validateSchemaFormatConsistency(
	parameters *v1alpha1.SchemaSection,
	environmentConfigs *v1alpha1.SchemaSection,
	basePath *field.Path,
) field.ErrorList {
	// Only validate when both sections are provided
	if parameters == nil || environmentConfigs == nil {
		return nil
	}
	// Only validate when both sections have a schema set
	if parameters.GetRaw() == nil || environmentConfigs.GetRaw() == nil {
		return nil
	}

	paramsIsV3 := parameters.IsOpenAPIV3()
	envIsV3 := environmentConfigs.IsOpenAPIV3()

	if paramsIsV3 != envIsV3 {
		return field.ErrorList{field.Invalid(
			basePath.Child("environmentConfigs"),
			omitValue,
			"parameters and environmentConfigs must use the same schema format (both ocSchema or both openAPIV3Schema)",
		)}
	}

	return nil
}
