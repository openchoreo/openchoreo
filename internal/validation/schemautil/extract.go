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
// Returns parameters schema, environmentConfigs schema, and any validation errors.
func ExtractStructuralSchemas(
	parameters *v1alpha1.SchemaSection,
	environmentConfigs *v1alpha1.SchemaSection,
	basePath *field.Path,
) (*apiextschema.Structural, *apiextschema.Structural, field.ErrorList) {
	allErrs := field.ErrorList{}

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
