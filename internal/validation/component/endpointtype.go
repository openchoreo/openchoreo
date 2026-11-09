// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"

	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ValidateEndpointTypeResourcesWithSchema type-checks the CEL in an
// (Cluster)EndpointType's resources and validations against the endpoint
// pipeline's context (metadata, parameters, environmentConfigs, visibility,
// backends, dataplane, environment, gateway). EndpointType templates share the
// ResourceTemplate shape with ComponentType, so the same template walk —
// including forEach loop-variable scoping — applies; only the environment the
// expressions are checked against differs.
//
// basePath is the field path prefix for error reporting, e.g. spec for an
// EndpointType or spec.endpointType.spec for the snapshot on an EndpointRelease.
func ValidateEndpointTypeResourcesWithSchema(
	resources []v1alpha1.ResourceTemplate,
	validations []v1alpha1.ValidationRule,
	parametersSchema *apiextschema.Structural,
	environmentConfigsSchema *apiextschema.Structural,
	basePath *field.Path,
) field.ErrorList {
	validator, err := NewCELValidator(EndpointTypeResource, SchemaOptions{
		ParametersSchema:         parametersSchema,
		EnvironmentConfigsSchema: environmentConfigsSchema,
	})
	if err != nil {
		return field.ErrorList{field.InternalError(basePath, fmt.Errorf("failed to create CEL validator: %w", err))}
	}

	var allErrs field.ErrorList
	for i := range resources {
		allErrs = append(allErrs, validateResourceTemplate(resources[i], validator, basePath.Child("resources").Index(i))...)
	}
	for i := range validations {
		allErrs = append(allErrs, validateValidationRule(validations[i], validator, basePath.Child("validations").Index(i))...)
	}
	return allErrs
}
