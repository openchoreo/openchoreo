// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// ValidateComponentTypeResourcesWithSchema validates all resources in a ComponentType with schema-aware type checking.
func ValidateComponentTypeResourcesWithSchema(
	ct *v1alpha1.ComponentType,
	parametersSchema *apiextschema.Structural,
	environmentConfigsSchema *apiextschema.Structural,
) field.ErrorList {
	return ValidateResourcesWithSchema(ct.Spec.Resources, ct.Spec.Validations, parametersSchema, environmentConfigsSchema, field.NewPath("spec"))
}

// ValidateClusterComponentTypeResourcesWithSchema validates all resources in a ClusterComponentType with schema-aware type checking.
func ValidateClusterComponentTypeResourcesWithSchema(
	cct *v1alpha1.ClusterComponentType,
	parametersSchema *apiextschema.Structural,
	environmentConfigsSchema *apiextschema.Structural,
) field.ErrorList {
	return ValidateResourcesWithSchema(cct.Spec.Resources, cct.Spec.Validations, parametersSchema, environmentConfigsSchema, field.NewPath("spec"))
}

// ValidateResourcesWithSchema validates resource templates and validation rules with schema-aware CEL type checking.
// basePath is the field path prefix for error reporting (e.g., field.NewPath("spec") for top-level CRDs,
// or field.NewPath("spec", "componentType", "spec") for embedded resources in ComponentRelease).
func ValidateResourcesWithSchema(
	resources []v1alpha1.ResourceTemplate,
	validations []v1alpha1.ValidationRule,
	parametersSchema *apiextschema.Structural,
	environmentConfigsSchema *apiextschema.Structural,
	basePath *field.Path,
) field.ErrorList {
	allErrs := field.ErrorList{}

	// Create schema-aware validator for component context
	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{
		ParametersSchema:         parametersSchema,
		EnvironmentConfigsSchema: environmentConfigsSchema,
	})
	if err != nil {
		allErrs = append(allErrs, field.InternalError(
			basePath,
			fmt.Errorf("failed to create CEL validator: %w", err)))
		return allErrs
	}

	for i, resource := range resources {
		resourcePath := basePath.Child("resources").Index(i)
		errs := validateResourceTemplate(resource, validator, resourcePath)
		allErrs = append(allErrs, errs...)
	}

	for i, rule := range validations {
		rulePath := basePath.Child("validations").Index(i)
		errs := validateValidationRule(rule, validator, rulePath)
		allErrs = append(allErrs, errs...)
	}

	return allErrs
}

// validateResourceTemplate validates a single resource template including forEach handling.
func validateResourceTemplate(
	tmpl v1alpha1.ResourceTemplate,
	validator *CELValidator,
	basePath *field.Path,
) field.ErrorList {
	allErrs := field.ErrorList{}
	env := validator.GetBaseEnv()

	// Validate includeWhen first with the base env (before forEach extends it).
	// At runtime, includeWhen is evaluated before forEach — the loop variable
	// is not in scope, so validation must match that behavior.
	if tmpl.IncludeWhen != "" {
		includeWhenCEL, ok := extractCELFromTemplate(tmpl.IncludeWhen)
		if !ok {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("includeWhen"),
				tmpl.IncludeWhen,
				"includeWhen must be wrapped in ${...}"))
		} else if err := validator.ValidateBooleanExpression(includeWhenCEL, env); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("includeWhen"),
				tmpl.IncludeWhen,
				fmt.Sprintf("includeWhen must return boolean: %v", err)))
		}
	}

	// Handle forEach - analyze and extend environment with loop variable
	if tmpl.ForEach != "" {
		forEachCEL, ok := extractCELFromTemplate(tmpl.ForEach)
		if !ok {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("forEach"),
				tmpl.ForEach,
				"forEach must be wrapped in ${...}"))
			return allErrs
		}

		if err := validator.ValidateIterableExpression(forEachCEL, env); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("forEach"),
				tmpl.ForEach,
				fmt.Sprintf("parse error: %v", err)))
		}

		forEachInfo, err := analyzeForEachExpression(forEachCEL, tmpl.Var, env)
		if err != nil {
			if !strings.Contains(err.Error(), "type check") {
				allErrs = append(allErrs, field.Invalid(
					basePath.Child("forEach"),
					tmpl.ForEach,
					fmt.Sprintf("failed to analyze forEach: %v", err)))
			}
		}

		if forEachInfo != nil {
			extendedEnv, err := extendEnvWithForEach(env, forEachInfo, validator.GetTypeProvider())
			if err != nil {
				allErrs = append(allErrs, field.InternalError(
					basePath.Child("forEach"),
					fmt.Errorf("failed to extend environment: %w", err)))
			} else {
				env = extendedEnv
			}
		}
	}

	// Validate the resource template (uses extended env if forEach was present)
	if tmpl.Template != nil {
		allErrs = append(allErrs, validateTemplateBody(*tmpl.Template, validator, env, basePath.Child("template"))...)
	} else {
		allErrs = append(allErrs, field.Required(
			basePath.Child("template"),
			"template is required"))
	}

	return allErrs
}

// validateTemplateBody walks through a template body and validates all CEL expressions.
func validateTemplateBody(
	body runtime.RawExtension,
	validator *CELValidator,
	env *cel.Env,
	basePath *field.Path,
) field.ErrorList {
	if len(body.Raw) == 0 {
		return nil
	}

	var data any
	if err := json.Unmarshal(body.Raw, &data); err != nil {
		return field.ErrorList{field.Invalid(basePath, string(body.Raw),
			fmt.Sprintf("invalid JSON: %v", err))}
	}

	return walkAndValidateCEL(data, basePath, validator, env)
}

// walkAndValidateCEL recursively walks a JSON structure and validates CEL expressions.
func walkAndValidateCEL(data any, path *field.Path, validator *CELValidator, env *cel.Env) field.ErrorList {
	var errs field.ErrorList
	switch v := data.(type) {
	case string:
		expressions, err := template.FindCELExpressions(v)
		if err != nil {
			return append(errs, field.Invalid(path, v,
				fmt.Sprintf("failed to parse CEL expressions: %v", err)))
		}
		for _, expr := range expressions {
			if err := validator.ValidateWithEnv(expr.InnerExpr, env); err != nil {
				errs = append(errs, field.Invalid(path, v,
					fmt.Sprintf("invalid CEL expression '%s': %v", expr.InnerExpr, err)))
			}
		}

	case map[string]any:
		for key, value := range v {
			if containsCELExpression(key) {
				expressions, err := template.FindCELExpressions(key)
				if err != nil {
					errs = append(errs, field.Invalid(path.Key(key), key,
						fmt.Sprintf("invalid CEL in map key: %v", err)))
					continue
				}
				for _, expr := range expressions {
					if err := validator.ValidateWithEnv(expr.InnerExpr, env); err != nil {
						errs = append(errs, field.Invalid(path.Key(key), key,
							fmt.Sprintf("invalid CEL in map key '%s': %v", expr.InnerExpr, err)))
					}
				}
			}
			errs = append(errs, walkAndValidateCEL(value, path.Key(key), validator, env)...)
		}

	case []any:
		for i, item := range v {
			errs = append(errs, walkAndValidateCEL(item, path.Index(i), validator, env)...)
		}
	}
	return errs
}

// validateValidationRule validates a single validation rule's CEL expression.
func validateValidationRule(
	rule v1alpha1.ValidationRule,
	validator *CELValidator,
	basePath *field.Path,
) field.ErrorList {
	allErrs := field.ErrorList{}

	ruleCEL, ok := extractCELFromTemplate(rule.Rule)
	if !ok {
		allErrs = append(allErrs, field.Invalid(
			basePath.Child("rule"), rule.Rule,
			"rule must be wrapped in ${...}"))
		return allErrs
	}
	if err := validator.ValidateBooleanExpression(ruleCEL, validator.GetBaseEnv()); err != nil {
		allErrs = append(allErrs, field.Invalid(
			basePath.Child("rule"), rule.Rule,
			fmt.Sprintf("rule must return boolean: %v", err)))
	}

	return allErrs
}

// containsCELExpression checks if a string contains any CEL expressions.
func containsCELExpression(str string) bool {
	return strings.Contains(str, "${")
}
