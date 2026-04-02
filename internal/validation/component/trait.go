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
func ValidateTraitCreatesAndPatchesWithSchema(
	trait *v1alpha1.Trait,
	parametersSchema *apiextschema.Structural,
	environmentConfigsSchema *apiextschema.Structural,
) field.ErrorList {
	return ValidateTraitSpec(trait.Spec, parametersSchema, environmentConfigsSchema, field.NewPath("spec"))
}

// ValidateClusterTraitCreatesAndPatchesWithSchema validates all creates, patches, and validations in a ClusterTrait with schema-aware type checking.
func ValidateClusterTraitCreatesAndPatchesWithSchema(
	ct *v1alpha1.ClusterTrait,
	parametersSchema *apiextschema.Structural,
	environmentConfigsSchema *apiextschema.Structural,
) field.ErrorList {
	// ClusterTraitSpec has the same fields as TraitSpec but is a distinct type.
	// Convert by extracting the relevant slices.
	spec := v1alpha1.TraitSpec{
		Parameters:         ct.Spec.Parameters,
		EnvironmentConfigs: ct.Spec.EnvironmentConfigs,
		Validations:        ct.Spec.Validations,
		Creates:            ct.Spec.Creates,
		Patches:            ct.Spec.Patches,
	}
	return ValidateTraitSpec(spec, parametersSchema, environmentConfigsSchema, field.NewPath("spec"))
}

// ValidateTraitSpec validates all creates, patches, and validations in a TraitSpec with schema-aware type checking.
// basePath is the field path prefix for error reporting (e.g., field.NewPath("spec") for top-level CRDs,
// or a nested path for embedded trait specs in ComponentRelease).
//
// If schemas are nil, DynType will be used for those variables (no static type checking).
func ValidateTraitSpec(
	spec v1alpha1.TraitSpec,
	parametersSchema *apiextschema.Structural,
	environmentConfigsSchema *apiextschema.Structural,
	basePath *field.Path,
) field.ErrorList {
	allErrs := field.ErrorList{}

	// Create schema-aware validator for trait context
	validator, err := NewCELValidator(TraitResource, SchemaOptions{
		ParametersSchema:         parametersSchema,
		EnvironmentConfigsSchema: environmentConfigsSchema,
	})
	if err != nil {
		allErrs = append(allErrs, field.InternalError(
			basePath,
			fmt.Errorf("failed to create CEL validator: %w", err)))
		return allErrs
	}

	// Validate validation rules
	for i, rule := range spec.Validations {
		rulePath := basePath.Child("validations").Index(i)
		errs := validateValidationRule(rule, validator, rulePath)
		allErrs = append(allErrs, errs...)
	}

	// Validate creates
	for i, create := range spec.Creates {
		createPath := basePath.Child("creates").Index(i)
		errs := validateTraitCreate(create, validator, createPath)
		allErrs = append(allErrs, errs...)
	}

	// Validate patches
	for i, patch := range spec.Patches {
		patchPath := basePath.Child("patches").Index(i)
		errs := validateTraitPatch(patch, validator, patchPath)
		allErrs = append(allErrs, errs...)
	}

	return allErrs
}

// ValidateTraitCreate validates a single trait create operation.
// It validates includeWhen (must return boolean), forEach (must return iterable),
// and the template body with schema-aware type checking.
func validateTraitCreate(
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
			forEachInfo, err := analyzeForEachExpression(
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
	}

	// Validate the create template
	if create.Template != nil {
		bodyErrs := validateTemplateBody(*create.Template, validator, env, basePath.Child("template"))
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
func validateTraitPatch(
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
			forEachInfo, err := analyzeForEachExpression(
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
	}

	// Validate target
	targetErrs := validatePatchTarget(patch.Target, validator, env, basePath.Child("target"))
	allErrs = append(allErrs, targetErrs...)

	// Validate patch operations
	for i, op := range patch.Operations {
		opPath := basePath.Child("operations").Index(i)
		errs := validatePatchOperation(op, validator, env, opPath)
		allErrs = append(allErrs, errs...)
	}

	return allErrs
}

const (
	patchOpAdd     = "add"
	patchOpReplace = "replace"
	patchOpRemove  = "remove"
)

// validatePatchOperation validates a single patch operation.
func validatePatchOperation(
	op v1alpha1.JSONPatchOperation,
	validator *CELValidator,
	env *cel.Env,
	basePath *field.Path,
) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate the operation type
	switch op.Op {
	case patchOpAdd, patchOpReplace, patchOpRemove:
		// valid
	default:
		allErrs = append(allErrs, field.Invalid(
			basePath.Child("op"),
			op.Op,
			"invalid patch operation (valid: add, replace, remove)"))
	}

	// Validate path is present and looks valid
	if op.Path == "" {
		allErrs = append(allErrs, field.Required(
			basePath.Child("path"),
			"patch path is required"))
	}

	// Validate value field if present
	if op.Value != nil {
		if op.Op == patchOpAdd || op.Op == patchOpReplace {
			valueErrs := validateTemplateBody(*op.Value, validator, env, basePath.Child("value"))
			allErrs = append(allErrs, valueErrs...)
		} else if op.Op == patchOpRemove {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("value"),
				"<value>",
				fmt.Sprintf("value should not be specified for '%s' operation", op.Op)))
		}
	} else if op.Op == patchOpAdd || op.Op == patchOpReplace {
		allErrs = append(allErrs, field.Required(
			basePath.Child("value"),
			fmt.Sprintf("value is required for '%s' operation", op.Op)))
	}

	return allErrs
}

// ValidatePatchTarget validates a patch target specification
func validatePatchTarget(
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
