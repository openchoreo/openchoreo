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
// Types are extracted from within each ocSchema blob via the "$types" key.
// Returns parameters schema, environmentConfigs schema, and any validation errors.
func ExtractStructuralSchemas(
	source schema.Source,
	basePath *field.Path,
) (*apiextschema.Structural, *apiextschema.Structural, field.ErrorList) {
	allErrs := field.ErrorList{}

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

	// Extract and build environmentConfigs structural schema
	var envConfigsSchema *apiextschema.Structural
	var envConfigs map[string]any
	if envRaw := source.GetEnvironmentConfigs(); envRaw != nil && len(envRaw.Raw) > 0 {
		if err := yaml.Unmarshal(envRaw.Raw, &envConfigs); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("environmentConfigs"),
				omitValue,
				fmt.Sprintf("failed to parse environmentConfigs schema: %v", err)))
		} else {
			def := schema.Definition{
				Schemas: []map[string]any{envConfigs},
			}
			structural, err := schema.ToStructural(def)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(
					basePath.Child("environmentConfigs"),
					omitValue,
					fmt.Sprintf("failed to build structural schema: %v", err)))
			} else {
				envConfigsSchema = structural
			}
		}
	}

	return parametersSchema, envConfigsSchema, allErrs
}
