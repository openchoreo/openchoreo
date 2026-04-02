// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemautil

import (
	"bytes"
	"encoding/json"
	"fmt"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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

// ExtractAndValidateSchemas extracts structural schemas and validates openAPIV3Schema fields.
// Combines ExtractStructuralSchemas and ValidateOpenAPIV3SchemaFields into a single call.
func ExtractAndValidateSchemas(
	parameters *v1alpha1.SchemaSection,
	environmentConfigs *v1alpha1.SchemaSection,
	basePath *field.Path,
) (*apiextschema.Structural, *apiextschema.Structural, field.ErrorList) {
	parametersSchema, envConfigsSchema, allErrs := ExtractStructuralSchemas(
		parameters, environmentConfigs, basePath,
	)

	allErrs = append(allErrs, ValidateOpenAPIV3SchemaFields(
		parameters, basePath.Child("parameters"),
	)...)
	allErrs = append(allErrs, ValidateOpenAPIV3SchemaFields(
		environmentConfigs, basePath.Child("environmentConfigs"),
	)...)

	return parametersSchema, envConfigsSchema, allErrs
}

// ValidateOpenAPIV3SchemaFields performs strict field validation on an openAPIV3Schema.
// It rejects unknown fields (e.g., "types" instead of "type") by resolving $ref/$defs,
// stripping vendor extensions (x-*), and then strict-decoding into JSONSchemaProps.
func ValidateOpenAPIV3SchemaFields(section *v1alpha1.SchemaSection, fieldPath *field.Path) field.ErrorList {
	if section == nil || section.OpenAPIV3Schema == nil || len(section.OpenAPIV3Schema.Raw) == 0 {
		return nil
	}

	// Parse raw schema into map
	var rawSchema map[string]any
	if err := json.Unmarshal(section.OpenAPIV3Schema.Raw, &rawSchema); err != nil {
		// Parse errors are already caught by ExtractStructuralSchemas
		return nil
	}

	// Resolve $ref/$defs so they don't appear as unknown fields
	resolved, err := schema.ResolveRefs(rawSchema)
	if err != nil {
		// Ref resolution errors are already caught by ExtractStructuralSchemas
		return nil
	}

	// Strip vendor extensions (x-*) which are valid but not in JSONSchemaProps
	stripped := schema.StripVendorExtensions(resolved)

	// Re-marshal and strict-decode to catch unknown fields
	data, err := json.Marshal(stripped)
	if err != nil {
		return nil
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var props extv1.JSONSchemaProps
	if err := decoder.Decode(&props); err != nil {
		return field.ErrorList{field.Invalid(
			fieldPath,
			omitValue,
			fmt.Sprintf("openAPIV3Schema contains unknown or invalid fields: %v", err),
		)}
	}

	return nil
}
