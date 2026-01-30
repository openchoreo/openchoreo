// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemautil

import (
	"fmt"

	"gopkg.in/yaml.v3"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/internal/schema"
)

// omitValue is used to omit the value from field.Invalid error messages
var omitValue = field.OmitValueType{}

// ExtractStructuralSchemas extracts and builds structural schemas from a Source.
// It parses the raw extensions and converts them to Kubernetes structural schemas.
// Returns parameters schema, envOverrides schema, and any validation errors.
func ExtractStructuralSchemas(
	source schema.Source,
	basePath *field.Path,
) (*apiextschema.Structural, *apiextschema.Structural, field.ErrorList) {
	allErrs := field.ErrorList{}

	// Extract types from RawExtension
	var types map[string]any
	if typesRaw := source.GetTypes(); typesRaw != nil && len(typesRaw.Raw) > 0 {
		if err := yaml.Unmarshal(typesRaw.Raw, &types); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("types"),
				omitValue,
				fmt.Sprintf("failed to parse types: %v", err)))
			return nil, nil, allErrs
		}
	}

	// Extract and build parameters structural schema
	var parametersSchema *apiextschema.Structural
	var params map[string]any
	if paramsRaw := source.GetParameters(); paramsRaw != nil && len(paramsRaw.Raw) > 0 {
		if err := yaml.Unmarshal(paramsRaw.Raw, &params); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("parameters"),
				omitValue,
				fmt.Sprintf("failed to parse parameters schema: %v", err)))
		} else {
			def := schema.Definition{
				Types:   types,
				Schemas: []map[string]any{params},
			}
			structural, err := schema.ToStructural(def)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(
					basePath.Child("parameters"),
					omitValue,
					fmt.Sprintf("failed to build structural schema: %v", err)))
			} else {
				parametersSchema = structural
			}
		}
	}

	// Extract and build envOverrides structural schema
	var envOverridesSchema *apiextschema.Structural
	var envOverrides map[string]any
	if envRaw := source.GetEnvOverrides(); envRaw != nil && len(envRaw.Raw) > 0 {
		if err := yaml.Unmarshal(envRaw.Raw, &envOverrides); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("envOverrides"),
				omitValue,
				fmt.Sprintf("failed to parse envOverrides schema: %v", err)))
		} else {
			def := schema.Definition{
				Types:   types,
				Schemas: []map[string]any{envOverrides},
			}
			structural, err := schema.ToStructural(def)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(
					basePath.Child("envOverrides"),
					omitValue,
					fmt.Sprintf("failed to build structural schema: %v", err)))
			} else {
				envOverridesSchema = structural
			}
		}
	}

	return parametersSchema, envOverridesSchema, allErrs
}
