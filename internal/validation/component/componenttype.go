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
	envOverridesSchema *apiextschema.Structural,
) field.ErrorList {
	return validateResourcesWithSchema(ct.Spec.Resources, ct.Spec.Validations, parametersSchema, envOverridesSchema)
}

// ValidateResourceTemplate validates a single resource template including forEach handling
func ValidateResourceTemplate(
	tmpl v1alpha1.ResourceTemplate,
	validator *CELValidator,
	basePath *field.Path,
) field.ErrorList {
	allErrs := field.ErrorList{}
	env := validator.GetBaseEnv()

	// Handle forEach - analyze and extend environment with loop variable
	if tmpl.ForEach != "" {
		// Extract CEL expression from template syntax ${...}
		forEachCEL, ok := extractCELFromTemplate(tmpl.ForEach)
		if !ok {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("forEach"),
				tmpl.ForEach,
				"forEach must be wrapped in ${...}"))
			return allErrs
		}

		// First validate that forEach returns an iterable
		if err := validator.ValidateIterableExpression(forEachCEL, env); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("forEach"),
				tmpl.ForEach,
				fmt.Sprintf("parse error: %v", err)))
			// Continue validation even if forEach is invalid to find more errors
		}

		// Analyze forEach to determine loop variable type
		forEachInfo, err := AnalyzeForEachExpression(
			forEachCEL,
			tmpl.Var,
			env,
		)

		if err != nil {
			// Only add error if it's not about type checking
			// (type check errors are OK for dynamic expressions)
			if !strings.Contains(err.Error(), "type check") {
				allErrs = append(allErrs, field.Invalid(
					basePath.Child("forEach"),
					tmpl.ForEach,
					fmt.Sprintf("failed to analyze forEach: %v", err)))
			}
		}

		// Extend environment with the loop variable
		if forEachInfo != nil {
			extendedEnv, err := ExtendEnvWithForEach(env, forEachInfo)
			if err != nil {
				allErrs = append(allErrs, field.InternalError(
					basePath.Child("forEach"),
					fmt.Errorf("failed to extend environment: %w", err)))
			} else {
				env = extendedEnv
			}
		}
	}

	// Validate includeWhen if present (must return boolean)
	if tmpl.IncludeWhen != "" {
		// Extract CEL expression from template syntax ${...}
		includeWhenCEL, ok := extractCELFromTemplate(tmpl.IncludeWhen)
		if !ok {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("includeWhen"),
				tmpl.IncludeWhen,
				"includeWhen must be wrapped in ${...}"))
			// Continue to validate other parts even if includeWhen is invalid
		} else if err := validator.ValidateBooleanExpression(includeWhenCEL, env); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("includeWhen"),
				tmpl.IncludeWhen,
				fmt.Sprintf("parse error: %v", err)))
		}
	}

	// Validate the resource template
	if tmpl.Template != nil {
		bodyErrs := ValidateTemplateBody(*tmpl.Template, validator, env, basePath.Child("template"))
		allErrs = append(allErrs, bodyErrs...)
	} else {
		allErrs = append(allErrs, field.Required(
			basePath.Child("template"),
			"template is required"))
	}

	return allErrs
}

// ValidateTemplateBody walks through a template body and validates all CEL expressions
func ValidateTemplateBody(
	body runtime.RawExtension,
	validator *CELValidator,
	env *cel.Env,
	basePath *field.Path,
) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(body.Raw) == 0 {
		// Empty body is valid
		return allErrs
	}

	// Parse the JSON body
	var data interface{}
	if err := json.Unmarshal(body.Raw, &data); err != nil {
		allErrs = append(allErrs, field.Invalid(
			basePath,
			string(body.Raw),
			fmt.Sprintf("invalid JSON: %v", err)))
		return allErrs
	}

	// Walk the structure and validate CEL expressions
	walker := NewExpressionWalker(validator, env)
	walkErrs := walker.Walk(data, basePath)
	allErrs = append(allErrs, walkErrs...)

	return allErrs
}

// ExpressionWalker walks through data structures to find and validate CEL expressions
type ExpressionWalker struct {
	validator *CELValidator
	env       *cel.Env
	errors    field.ErrorList
}

// NewExpressionWalker creates a walker for finding and validating CEL expressions
func NewExpressionWalker(validator *CELValidator, env *cel.Env) *ExpressionWalker {
	return &ExpressionWalker{
		validator: validator,
		env:       env,
		errors:    field.ErrorList{},
	}
}

// Walk recursively walks through the data structure and validates CEL expressions
func (w *ExpressionWalker) Walk(data interface{}, basePath *field.Path) field.ErrorList {
	w.errors = field.ErrorList{}
	w.walkValue(data, basePath)
	return w.errors
}

func (w *ExpressionWalker) walkValue(data interface{}, path *field.Path) {
	switch v := data.(type) {
	case string:
		w.validateString(v, path)

	case map[string]interface{}:
		for key, value := range v {
			// Check if the key contains CEL expressions (dynamic keys)
			if containsCELExpression(key) {
				expressions, err := template.FindCELExpressions(key)
				if err != nil {
					w.errors = append(w.errors, field.Invalid(
						path.Key(key),
						key,
						fmt.Sprintf("invalid CEL in map key: %v", err)))
					continue
				}
				for _, expr := range expressions {
					if err := w.validator.ValidateWithEnv(expr.InnerExpr, w.env); err != nil {
						w.errors = append(w.errors, field.Invalid(
							path.Key(key),
							key,
							fmt.Sprintf("invalid CEL in map key '%s': %v", expr.InnerExpr, err)))
					}
				}
			}

			// Walk the value
			w.walkValue(value, path.Key(key))
		}

	case []interface{}:
		for i, item := range v {
			w.walkValue(item, path.Index(i))
		}

	// Primitive types don't need validation
	case nil, bool, float64, int, int64:
		return
	}
}

func (w *ExpressionWalker) validateString(str string, path *field.Path) {
	expressions, err := template.FindCELExpressions(str)
	if err != nil {
		w.errors = append(w.errors, field.Invalid(
			path,
			str,
			fmt.Sprintf("failed to parse CEL expressions: %v", err)))
		return
	}

	for _, expr := range expressions {
		if err := w.validator.ValidateWithEnv(expr.InnerExpr, w.env); err != nil {
			w.errors = append(w.errors, field.Invalid(
				path,
				str,
				fmt.Sprintf("invalid CEL expression '%s': %v", expr.InnerExpr, err)))
		}
	}
}

// ValidateValidationRule validates a single validation rule's CEL expression.
// The rule must be wrapped in ${...} and must compile to a boolean expression.
func ValidateValidationRule(
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

// containsCELExpression checks if a string contains any CEL expressions
func containsCELExpression(str string) bool {
	return strings.Contains(str, "${")
}

// ValidateClusterComponentTypeResourcesWithSchema validates all resources in a ClusterComponentType with schema-aware type checking.
func ValidateClusterComponentTypeResourcesWithSchema(
	cct *v1alpha1.ClusterComponentType,
	parametersSchema *apiextschema.Structural,
	envOverridesSchema *apiextschema.Structural,
) field.ErrorList {
	return validateResourcesWithSchema(cct.Spec.Resources, cct.Spec.Validations, parametersSchema, envOverridesSchema)
}

// validateResourcesWithSchema validates resource templates and validation rules with schema-aware CEL type checking.
//
// Parameters:
//   - resources: Resource templates to validate
//   - validations: CEL validation rules to validate (may be nil)
//   - parametersSchema: Structural schema for parameters (nil uses DynType)
//   - envOverridesSchema: Structural schema for envOverrides (nil uses DynType)
func validateResourcesWithSchema(
	resources []v1alpha1.ResourceTemplate,
	validations []v1alpha1.ValidationRule,
	parametersSchema *apiextschema.Structural,
	envOverridesSchema *apiextschema.Structural,
) field.ErrorList {
	allErrs := field.ErrorList{}

	// Create schema-aware validator for component context
	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{
		ParametersSchema:   parametersSchema,
		EnvOverridesSchema: envOverridesSchema,
	})
	if err != nil {
		allErrs = append(allErrs, field.InternalError(
			field.NewPath("spec"),
			fmt.Errorf("failed to create CEL validator: %w", err)))
		return allErrs
	}

	// Validate each resource template
	for i, resource := range resources {
		resourcePath := field.NewPath("spec", "resources").Index(i)
		errs := ValidateResourceTemplate(resource, validator, resourcePath)
		allErrs = append(allErrs, errs...)
	}

	// Validate validation rules
	for i, rule := range validations {
		rulePath := field.NewPath("spec", "validations").Index(i)
		errs := ValidateValidationRule(rule, validator, rulePath)
		allErrs = append(allErrs, errs...)
	}

	return allErrs
}
