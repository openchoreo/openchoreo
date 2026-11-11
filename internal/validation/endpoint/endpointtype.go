// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	componentvalidation "github.com/openchoreo/openchoreo/internal/validation/component"
	"github.com/openchoreo/openchoreo/internal/validation/schemautil"
)

// ValidateEndpointTypeSpec validates an EndpointTypeSpec. Returned errors carry
// full field paths under basePath, so callers can validate an embedded
// snapshot (an EndpointRelease's spec.endpointType.spec) the same way.
//
// What this validates:
//   - parameters / environmentConfigs schemas parse as well-formed OpenAPI v3.
//   - every CEL expression in resources[] (template, includeWhen, forEach),
//     validations[] and urls (every field) type-checks against the endpoint
//     render context; literal URL schemes, security schemes and API key
//     locations are among the allowed values.
//   - resources[].targetPlane, when set, is the data plane: a binding emits one
//     data-plane RenderedRelease, so a template aimed elsewhere could never be
//     delivered.
func ValidateEndpointTypeSpec(spec *v1alpha1.EndpointTypeSpec, basePath *field.Path) field.ErrorList {
	if spec == nil {
		return nil
	}
	return validateEndpointTypeSpecCommon(spec.Parameters, spec.EnvironmentConfigs,
		spec.Validations, spec.Resources, spec.URLs, basePath)
}

// ValidateClusterEndpointTypeSpec is the cluster-scoped sibling. The spec shape
// is currently identical to EndpointTypeSpec; this delegates through the shared
// inner function so future divergence has a single place to extend.
func ValidateClusterEndpointTypeSpec(spec *v1alpha1.ClusterEndpointTypeSpec, basePath *field.Path) field.ErrorList {
	if spec == nil {
		return nil
	}
	return validateEndpointTypeSpecCommon(spec.Parameters, spec.EnvironmentConfigs,
		spec.Validations, spec.Resources, spec.URLs, basePath)
}

func validateEndpointTypeSpecCommon(
	parameters *v1alpha1.SchemaSection,
	environmentConfigs *v1alpha1.SchemaSection,
	validations []v1alpha1.ValidationRule,
	resources []v1alpha1.ResourceTemplate,
	urls *v1alpha1.EndpointTypeURLs,
	basePath *field.Path,
) field.ErrorList {
	parametersSchema, envConfigsSchema, allErrs := schemautil.ExtractAndValidateSchemas(
		parameters, environmentConfigs, basePath,
	)

	for i := range resources {
		if tp := resources[i].TargetPlane; tp != "" && tp != v1alpha1.TargetPlaneDataPlane {
			allErrs = append(allErrs, field.NotSupported(
				basePath.Child("resources").Index(i).Child("targetPlane"), tp,
				[]string{v1alpha1.TargetPlaneDataPlane}))
		}
	}

	allErrs = append(allErrs, componentvalidation.ValidateEndpointTypeResourcesWithSchema(
		resources, validations, urls, parametersSchema, envConfigsSchema, basePath,
	)...)
	return allErrs
}

// ValidateValuesField reports ValidateValues as a field error at path, for
// admission callers that collect a field.ErrorList.
func ValidateValuesField(section *v1alpha1.SchemaSection, raw *runtime.RawExtension, path *field.Path, schemaOwner string) field.ErrorList {
	if err := ValidateValues(section, raw); err != nil {
		return field.ErrorList{field.Invalid(path, field.OmitValueType{},
			fmt.Sprintf("does not match %s schema: %v", schemaOwner, err))}
	}
	return nil
}
