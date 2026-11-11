// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/google/cel-go/cel"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	endpointpipeline "github.com/openchoreo/openchoreo/internal/pipeline/endpoint"
	"github.com/openchoreo/openchoreo/internal/template"
)

// ValidateEndpointTypeResourcesWithSchema type-checks the CEL in an
// (Cluster)EndpointType's resources, validations and urls against the endpoint
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
	urls *v1alpha1.EndpointTypeURLs,
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
	if urls != nil {
		urlsPath := basePath.Child("urls")
		for _, u := range []struct {
			key  string
			tmpl *v1alpha1.EndpointURLTemplate
		}{
			{"project", urls.Project},
			{"namespace", urls.Namespace},
			{"internal", urls.Internal},
			{"external", urls.External},
		} {
			if u.tmpl != nil {
				allErrs = append(allErrs, validateEndpointURLTemplate(u.tmpl, validator, urlsPath.Child(u.key))...)
			}
		}
	}
	return allErrs
}

// validateEndpointURLTemplate type-checks one URL template. String fields must
// type-check, and a field that is a single standalone expression must return a
// string, since the engine returns such a value as its native type. Literal
// enum fields are checked against their allowed values here; templated ones
// are checked when they resolve.
func validateEndpointURLTemplate(
	tmpl *v1alpha1.EndpointURLTemplate,
	validator *CELValidator,
	path *field.Path,
) field.ErrorList {
	var allErrs field.ErrorList
	env := validator.GetBaseEnv()

	if tmpl.IncludeWhen != "" {
		includeWhenCEL, ok := extractCELFromTemplate(tmpl.IncludeWhen)
		if !ok {
			allErrs = append(allErrs, field.Invalid(path.Child("includeWhen"), tmpl.IncludeWhen,
				"includeWhen must be wrapped in ${...}"))
		} else if err := validator.ValidateBooleanExpression(includeWhenCEL, env); err != nil {
			allErrs = append(allErrs, field.Invalid(path.Child("includeWhen"), tmpl.IncludeWhen,
				fmt.Sprintf("includeWhen must return boolean: %v", err)))
		}
	}

	allErrs = append(allErrs, validateTemplatedEnum(tmpl.Scheme, endpointpipeline.URLSchemes, env, path.Child("scheme"))...)
	allErrs = append(allErrs, validateTemplatedString(tmpl.Host, env, path.Child("host"))...)
	allErrs = append(allErrs, validateTemplatedPort(tmpl.Port, env, path.Child("port"))...)
	allErrs = append(allErrs, validateTemplatedString(tmpl.Path, env, path.Child("path"))...)

	if sec := tmpl.Security; sec != nil {
		secPath := path.Child("security")
		allErrs = append(allErrs, validateTemplatedEnum(sec.Scheme, endpointpipeline.SecuritySchemes, env, secPath.Child("scheme"))...)
		if sec.APIKey != nil {
			keyPath := secPath.Child("apiKey")
			allErrs = append(allErrs, validateTemplatedEnum(sec.APIKey.In, endpointpipeline.APIKeyLocations, env, keyPath.Child("in"))...)
			allErrs = append(allErrs, validateTemplatedString(sec.APIKey.Name, env, keyPath.Child("name"))...)
		}
		if isLiteral(sec.Scheme) {
			isAPIKey := sec.Scheme == string(v1alpha1.EndpointSecuritySchemeAPIKey)
			switch {
			case isAPIKey && sec.APIKey == nil:
				allErrs = append(allErrs, field.Required(secPath.Child("apiKey"),
					"apiKey is required when scheme is apiKey"))
			case !isAPIKey && sec.APIKey != nil:
				allErrs = append(allErrs, field.Forbidden(secPath.Child("apiKey"),
					"apiKey is only valid when scheme is apiKey"))
			}
		}
	}
	return allErrs
}

// isLiteral reports whether value contains no ${...} expression.
func isLiteral(value string) bool {
	expressions, err := template.FindCELExpressions(value)
	return err == nil && len(expressions) == 0
}

// validateTemplatedString type-checks every ${...} in value; a standalone
// expression must return a string.
func validateTemplatedString(value string, env *cel.Env, path *field.Path) field.ErrorList {
	return validateTemplatedField(value, env, path, cel.StringType)
}

// validateTemplatedEnum is validateTemplatedString plus, for a literal value, a
// check against the allowed values.
func validateTemplatedEnum(value string, allowed []string, env *cel.Env, path *field.Path) field.ErrorList {
	if isLiteral(value) {
		if !slices.Contains(allowed, value) {
			return field.ErrorList{field.NotSupported(path, value, allowed)}
		}
		return nil
	}
	return validateTemplatedString(value, env, path)
}

// validateTemplatedPort accepts a literal integer between 1 and 65535 or a
// template; a standalone expression must return a number or a string.
func validateTemplatedPort(value string, env *cel.Env, path *field.Path) field.ErrorList {
	if value == "" {
		return nil
	}
	if isLiteral(value) {
		if n, err := strconv.Atoi(value); err != nil || n < 1 || n > 65535 {
			return field.ErrorList{field.Invalid(path, value, "port must be an integer between 1 and 65535")}
		}
		return nil
	}
	return validateTemplatedField(value, env, path, cel.IntType, cel.UintType, cel.DoubleType, cel.StringType)
}

// validateTemplatedField type-checks every ${...} in value. When value is a
// single standalone expression, its result type must be one of allowed (or
// dyn, decided at runtime).
func validateTemplatedField(value string, env *cel.Env, path *field.Path, allowed ...*cel.Type) field.ErrorList {
	expressions, err := template.FindCELExpressions(value)
	if err != nil {
		return field.ErrorList{field.Invalid(path, value, fmt.Sprintf("failed to parse CEL expressions: %v", err))}
	}
	standalone := len(expressions) == 1 && expressions[0].FullExpr == strings.TrimSpace(value)

	var allErrs field.ErrorList
	for _, expr := range expressions {
		parsed, issues := env.Parse(expr.InnerExpr)
		if issues != nil && issues.Err() != nil {
			allErrs = append(allErrs, field.Invalid(path, value,
				fmt.Sprintf("invalid CEL expression '%s': parse error: %v", expr.InnerExpr, issues.Err())))
			continue
		}
		checked, issues := env.Check(parsed)
		if issues != nil && issues.Err() != nil {
			allErrs = append(allErrs, field.Invalid(path, value,
				fmt.Sprintf("invalid CEL expression '%s': type check error: %v", expr.InnerExpr, issues.Err())))
			continue
		}
		if standalone {
			out := checked.OutputType()
			ok := out == cel.DynType
			for _, t := range allowed {
				ok = ok || out.IsExactType(t)
			}
			if !ok {
				allErrs = append(allErrs, field.Invalid(path, value,
					fmt.Sprintf("expression '%s' returns %s, expected %v", expr.InnerExpr, out, allowed)))
			}
		}
	}
	return allErrs
}
