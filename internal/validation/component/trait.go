// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ValidateTraitCreatesAndPatchesWithSchema validates all creates and patches in a Trait with schema-aware type checking.
// It checks CEL expressions, forEach loops, and ensures proper variable usage.
//
// Parameters:
//   - trait: The Trait to validate
//   - parametersSchema: Structural schema for parameters (from Trait.Schema.Parameters)
//   - envOverridesSchema: Structural schema for envOverrides (from Trait.Schema.EnvOverrides)
//
// If schemas are nil, DynType will be used for those variables (no static type checking).
// This provides better error messages by catching type errors at validation time.
func ValidateTraitCreatesAndPatchesWithSchema(
	trait *v1alpha1.Trait,
	parametersSchema *apiextschema.Structural,
	envOverridesSchema *apiextschema.Structural,
) field.ErrorList {
	allErrs := field.ErrorList{}

	// Create schema-aware validator for trait context
	validator, err := NewCELValidator(TraitResource, SchemaOptions{
		ParametersSchema:   parametersSchema,
		EnvOverridesSchema: envOverridesSchema,
	})
	if err != nil {
		allErrs = append(allErrs, field.InternalError(
			field.NewPath("spec"),
			fmt.Errorf("failed to create CEL validator: %w", err)))
		return allErrs
	}

	basePath := field.NewPath("spec")

	// Validate validation rules
	for i, rule := range trait.Spec.Validations {
		rulePath := basePath.Child("validations").Index(i)
		errs := ValidateValidationRule(rule, validator, rulePath)
		allErrs = append(allErrs, errs...)
	}

	// Validate creates
	for i, create := range trait.Spec.Creates {
		createPath := basePath.Child("creates").Index(i)
		errs := ValidateTraitCreate(create, validator, createPath)
		allErrs = append(allErrs, errs...)
	}

	// Validate patches
	for i, patch := range trait.Spec.Patches {
		patchPath := basePath.Child("patches").Index(i)
		errs := ValidateTraitPatch(patch, validator, patchPath)
		allErrs = append(allErrs, errs...)
	}

	return allErrs
}

// ValidateClusterTraitCreatesAndPatchesWithSchema validates all creates and patches in a ClusterTrait with schema-aware type checking.
// ClusterTraitSpec does not have Validations, so only creates and patches are validated.
func ValidateClusterTraitCreatesAndPatchesWithSchema(
	ct *v1alpha1.ClusterTrait,
	parametersSchema *apiextschema.Structural,
	envOverridesSchema *apiextschema.Structural,
) field.ErrorList {
	allErrs := field.ErrorList{}

	// Create schema-aware validator for trait context
	validator, err := NewCELValidator(TraitResource, SchemaOptions{
		ParametersSchema:   parametersSchema,
		EnvOverridesSchema: envOverridesSchema,
	})
	if err != nil {
		allErrs = append(allErrs, field.InternalError(
			field.NewPath("spec"),
			fmt.Errorf("failed to create CEL validator: %w", err)))
		return allErrs
	}

	basePath := field.NewPath("spec")

	// Validate creates
	for i, create := range ct.Spec.Creates {
		createPath := basePath.Child("creates").Index(i)
		errs := ValidateTraitCreate(create, validator, createPath)
		allErrs = append(allErrs, errs...)
	}

	// Validate patches
	for i, patch := range ct.Spec.Patches {
		patchPath := basePath.Child("patches").Index(i)
		errs := ValidateTraitPatch(patch, validator, patchPath)
		allErrs = append(allErrs, errs...)
	}

	return allErrs
}

// ValidateTraitCreate validates a single trait create operation.
// It validates includeWhen (must return boolean), forEach (must return iterable),
// and the template body with schema-aware type checking.
func ValidateTraitCreate(
	create v1alpha1.TraitCreate,
	validator *CELValidator,
	basePath *field.Path,
) field.ErrorList {
	allErrs := field.ErrorList{}
	env := validator.GetBaseEnv()

	// Validate includeWhen if specified (must return boolean)
	if create.IncludeWhen != "" {
		includeWhenCEL, ok := extractCELFromTemplate(create.IncludeWhen)
		if !ok {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("includeWhen"),
				create.IncludeWhen,
				"includeWhen must be a template expression wrapped with ${...}"))
		} else {
			if err := validator.ValidateBooleanExpression(includeWhenCEL, env); err != nil {
				allErrs = append(allErrs, field.Invalid(
					basePath.Child("includeWhen"),
					create.IncludeWhen,
					fmt.Sprintf("includeWhen must return boolean: %v", err)))
			}
		}
	}

	// Handle forEach - analyze and extend environment with loop variable
	if create.ForEach != "" {
		forEachCEL, ok := extractCELFromTemplate(create.ForEach)
		if !ok {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("forEach"),
				create.ForEach,
				"forEach must be a template expression wrapped with ${...}"))
		} else {
			// Validate that forEach returns an iterable
			if err := validator.ValidateIterableExpression(forEachCEL, env); err != nil {
				allErrs = append(allErrs, field.Invalid(
					basePath.Child("forEach"),
					create.ForEach,
					err.Error()))
			}

			// Analyze forEach to determine loop variable type
			forEachInfo, err := AnalyzeForEachExpression(
				forEachCEL,
				create.Var,
				env,
			)

			if err != nil {
				// Only add error if it's not about type checking
				if !strings.Contains(err.Error(), "type check") {
					allErrs = append(allErrs, field.Invalid(
						basePath.Child("forEach"),
						create.ForEach,
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
	}

	// Validate the create template
	if create.Template != nil {
		bodyErrs := ValidateTemplateBody(*create.Template, validator, env, basePath.Child("template"))
		allErrs = append(allErrs, bodyErrs...)
	} else {
		allErrs = append(allErrs, field.Required(
			basePath.Child("template"),
			"template is required"))
	}

	return allErrs
}

// extractCELFromTemplate extracts CEL expression from template syntax ${...}
// Returns the inner CEL expression and whether extraction was successful
func extractCELFromTemplate(templateExpr string) (string, bool) {
	trimmed := strings.TrimSpace(templateExpr)
	if strings.HasPrefix(trimmed, "${") && strings.HasSuffix(trimmed, "}") {
		return strings.TrimSpace(trimmed[2 : len(trimmed)-1]), true
	}
	return templateExpr, false
}

// ValidateTraitPatch validates a single trait patch operation
func ValidateTraitPatch(
	patch v1alpha1.TraitPatch,
	validator *CELValidator,
	basePath *field.Path,
) field.ErrorList {
	allErrs := field.ErrorList{}
	env := validator.GetBaseEnv()

	// Handle forEach - analyze and extend environment with loop variable
	if patch.ForEach != "" {
		// Extract CEL expression from template syntax ${...}
		forEachCEL, ok := extractCELFromTemplate(patch.ForEach)
		if !ok {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("forEach"),
				patch.ForEach,
				"forEach must be a template expression wrapped with ${...}"))
		} else {
			// Validate that forEach returns an iterable
			if err := validator.ValidateIterableExpression(forEachCEL, env); err != nil {
				allErrs = append(allErrs, field.Invalid(
					basePath.Child("forEach"),
					patch.ForEach,
					err.Error()))
			}

			// Analyze forEach to determine loop variable type
			forEachInfo, err := AnalyzeForEachExpression(
				forEachCEL,
				patch.Var,
				env,
			)

			if err != nil {
				// Only add error if it's not about type checking
				if !strings.Contains(err.Error(), "type check") {
					allErrs = append(allErrs, field.Invalid(
						basePath.Child("forEach"),
						patch.ForEach,
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
	}

	// Validate target
	targetErrs := ValidatePatchTarget(patch.Target, validator, env, basePath.Child("target"))
	allErrs = append(allErrs, targetErrs...)

	// Validate patch operations
	for i, op := range patch.Operations {
		opPath := basePath.Child("operations").Index(i)
		errs := ValidatePatchOperation(op, validator, env, opPath)
		allErrs = append(allErrs, errs...)
	}

	return allErrs
}

// ValidatePatchOperation validates a single patch operation
func ValidatePatchOperation(
	op v1alpha1.JSONPatchOperation,
	validator *CELValidator,
	env *cel.Env,
	basePath *field.Path,
) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate the operation type
	validOps := map[string]bool{
		"add":          true,
		"replace":      true,
		"remove":       true,
		"mergeShallow": false,
	}

	if !validOps[op.Op] {
		allErrs = append(allErrs, field.Invalid(
			basePath.Child("op"),
			op.Op,
			fmt.Sprintf("invalid patch operation '%s' (valid: add, replace, remove, mergeShallow)", op.Op)))
	}

	// Validate path is present and looks valid
	if op.Path == "" {
		allErrs = append(allErrs, field.Required(
			basePath.Child("path"),
			"patch path is required"))
	}

	// Validate value field if present
	if op.Value != nil {
		// For add/replace/mergeShallow operations, value is expected
		if op.Op == "add" || op.Op == "replace" || op.Op == "mergeShallow" {
			valueErrs := ValidateTemplateBody(*op.Value, validator, env, basePath.Child("value"))
			allErrs = append(allErrs, valueErrs...)
		} else if op.Op == "remove" {
			// Remove operation shouldn't have a value
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("value"),
				"<value>",
				fmt.Sprintf("value should not be specified for '%s' operation", op.Op)))
		}
	} else {
		// Value is required for add/replace/mergeShallow operations
		if op.Op == "add" || op.Op == "replace" || op.Op == "mergeShallow" {
			allErrs = append(allErrs, field.Required(
				basePath.Child("value"),
				fmt.Sprintf("value is required for '%s' operation", op.Op)))
		}
	}

	return allErrs
}

// ValidatePatchTarget validates a patch target specification
func ValidatePatchTarget(
	target v1alpha1.PatchTarget,
	validator *CELValidator,
	env *cel.Env,
	basePath *field.Path,
) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate required fields
	if target.Version == "" {
		allErrs = append(allErrs, field.Required(
			basePath.Child("version"),
			"version is required"))
	}

	if target.Kind == "" {
		allErrs = append(allErrs, field.Required(
			basePath.Child("kind"),
			"kind is required"))
	}

	// Validate where filter if present (must return boolean)
	if target.Where != "" {
		// Extract CEL expression from template syntax ${...} if present
		whereCEL, ok := extractCELFromTemplate(target.Where)
		if !ok {
			// where might not use template syntax, accept as-is
			whereCEL = target.Where
		}

		// Extend environment with 'resource' variable for where clause validation.
		// At runtime, 'resource' is bound to the target resource being filtered.
		whereEnv, err := env.Extend(cel.Variable("resource", cel.DynType))
		if err != nil {
			allErrs = append(allErrs, field.InternalError(
				basePath.Child("where"),
				fmt.Errorf("failed to extend environment with resource variable: %w", err)))
		} else if err := validator.ValidateBooleanExpression(whereCEL, whereEnv); err != nil {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("where"),
				target.Where,
				fmt.Sprintf("where filter must return boolean: %v", err)))
		}
	}

	return allErrs
}
