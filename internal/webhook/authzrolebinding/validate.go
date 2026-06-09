// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrolebinding

import (
	"fmt"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"k8s.io/apimachinery/pkg/util/validation/field"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// validateRoleMappings validates conditions on each RoleMapping, collecting errors across all conditions.
func validateRoleMappings(mappings []openchoreodevv1alpha1.RoleMapping) field.ErrorList {
	var allErrs field.ErrorList
	basePath := field.NewPath("spec").Child("roleMappings")
	for i, m := range mappings {
		for j, cond := range m.Conditions {
			condPath := basePath.Index(i).Child("conditions").Index(j)
			allErrs = append(allErrs, ValidateCondition(cond, condPath)...)
		}
	}
	return allErrs
}

// extractDottedAccesses walks the CEL AST and collects all "<root>.<leaf>"
// accesses where root is one of the known CEL root variables.
func extractDottedAccesses(expr celast.Expr, roots map[string]bool) []string {
	var result []string
	seen := make(map[string]bool)
	visitor := celast.NewExprVisitor(func(e celast.Expr) {
		if e.Kind() != celast.SelectKind {
			return
		}
		sel := e.AsSelect()
		operand := sel.Operand()
		if operand.Kind() == celast.IdentKind && roots[operand.AsIdent()] {
			path := operand.AsIdent() + "." + sel.FieldName()
			// avoid adding duplicates if the same attribute is accessed multiple times in the expression
			if !seen[path] {
				seen[path] = true
				result = append(result, path)
			}
		}
	})
	celast.PostOrderVisit(expr, visitor)
	return result
}

// ValidateCondition checks that a single AuthzCondition is well-formed:
//  1. actions and expression are non-empty.
//  2. The CEL expression compiles and returns bool.
//  3. Every <root>.<leaf> attribute access in the expression is in the
//     intersection of allowed attributes for all listed actions.
//
// All validation errors are collected and returned together.
func ValidateCondition(cond openchoreodevv1alpha1.AuthzCondition, fieldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if len(cond.Actions) == 0 {
		allErrs = append(allErrs, field.Required(fieldPath.Child("actions"), "actions must be non-empty"))
	}
	if cond.Expression == "" {
		allErrs = append(allErrs, field.Required(fieldPath.Child("expression"), "expression must be non-empty"))
	}

	// Can't validate CEL without both fields present
	if len(allErrs) > 0 {
		return allErrs
	}

	env, err := authzcore.GetCELEnv()
	if err != nil {
		return append(allErrs, field.InternalError(fieldPath.Child("expression"), fmt.Errorf("CEL environment unavailable: %w", err)))
	}

	checkedAST, issues := env.Compile(cond.Expression)
	if issues != nil && issues.Err() != nil {
		return append(allErrs, field.Invalid(fieldPath.Child("expression"), cond.Expression, fmt.Sprintf("CEL compile error: %s", issues.Err())))
	}
	if !checkedAST.OutputType().IsExactType(cel.BoolType) {
		allErrs = append(allErrs, field.Invalid(fieldPath.Child("expression"), cond.Expression, fmt.Sprintf("expression must return bool, got %s", checkedAST.OutputType())))
		return allErrs
	}

	allowedSpecs := authzcore.IntersectConditionsForActions(cond.Actions)
	if len(allowedSpecs) == 0 {
		return append(allErrs, field.Invalid(fieldPath.Child("actions"), cond.Actions, fmt.Sprintf("conditions are not supported for the specified action set %v", cond.Actions)))
	}

	allowedSet := make(map[string]bool, len(allowedSpecs))
	allowedKeys := make([]string, 0, len(allowedSpecs))
	for _, spec := range allowedSpecs {
		allowedSet[spec.Key] = true
		allowedKeys = append(allowedKeys, spec.Key)
	}

	usedConditions := extractDottedAccesses(checkedAST.NativeRep().Expr(), authzcore.KnownCELRoots)
	if len(usedConditions) == 0 {
		return append(allErrs, field.Invalid(fieldPath.Child("expression"), cond.Expression,
			fmt.Sprintf("expression must reference at least one allowed attribute; allowed attributes for actions %v are: %v", cond.Actions, allowedKeys)))
	}
	for _, path := range usedConditions {
		if !allowedSet[path] {
			allErrs = append(allErrs, field.Invalid(fieldPath.Child("expression"), cond.Expression,
				fmt.Sprintf("attribute %q is not supported for actions %v; allowed: %v", path, cond.Actions, allowedKeys)))
		}
	}

	return allErrs
}
