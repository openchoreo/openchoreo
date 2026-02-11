// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderer

import (
	"fmt"
	"maps"
	"strings"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// ShouldInclude evaluates an includeWhen CEL expression to determine if a resource should be created.
//
// Returns:
//   - true if includeWhen is empty (default behavior - resource is always created)
//   - true if includeWhen evaluates to true
//   - false if includeWhen evaluates to false
//   - error for evaluation failures (including missing data)
func ShouldInclude(engine *template.Engine, includeWhen string, context map[string]any) (bool, error) {
	if includeWhen == "" {
		return true, nil
	}

	result, err := engine.Render(includeWhen, context)
	if err != nil {
		return false, err
	}

	boolResult, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("includeWhen must evaluate to boolean, got %T", result)
	}
	return boolResult, nil
}

// EvalForEach evaluates a forEach CEL expression and returns contexts for each item.
//
// The forEach expression must evaluate to an array or map:
//   - Arrays: each element becomes an item
//   - Maps: converted to sorted slice of {key, value} entries for deterministic order
//
// For each item, a shallow clone of the context is created with the loop variable added.
// Shallow cloning is safe because the template engine only reads from the context.
//
// If varName is empty, "item" is used as the default variable name.
// Returns error if forEach evaluation fails or conversion fails.
func EvalForEach(
	engine *template.Engine,
	forEach string,
	varName string,
	context map[string]any,
) ([]map[string]any, error) {
	result, err := engine.Render(forEach, context)
	if err != nil {
		return nil, err
	}

	items, err := ToIterableItems(result)
	if err != nil {
		return nil, err
	}

	if varName == "" {
		varName = "item"
	}

	contexts := make([]map[string]any, len(items))
	for i, item := range items {
		itemContext := maps.Clone(context)
		itemContext[varName] = item
		contexts[i] = itemContext
	}
	return contexts, nil
}

// EvaluateValidationRules evaluates a list of CEL-based validation rules against the given context.
// All rules are evaluated (no short-circuiting) and all failures are collected into a single error.
// Returns nil if there are no rules or all rules pass.
//
// Error messages include rule index and rule text for easy identification.
// The returned error does not include a "validation failed:" prefix — callers add their own context.
func EvaluateValidationRules(engine *template.Engine, rules []v1alpha1.ValidationRule, context map[string]any) error {
	if len(rules) == 0 {
		return nil
	}
	var errs []string
	for i, rule := range rules {
		ruleRef := truncateRule(rule.Rule, 120)
		result, err := engine.Render(rule.Rule, context)
		if err != nil {
			errs = append(errs, fmt.Sprintf("rule[%d] %q evaluation error: %v", i, ruleRef, err))
			continue
		}
		boolResult, ok := result.(bool)
		if !ok {
			errs = append(errs, fmt.Sprintf("rule[%d] %q must evaluate to boolean, got %T", i, ruleRef, result))
			continue
		}
		if !boolResult {
			errs = append(errs, fmt.Sprintf("rule[%d] %q evaluated to false: %s", i, ruleRef, rule.Message))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// truncateRule returns the rule string truncated to maxLen runes.
func truncateRule(rule string, maxLen int) string {
	runes := []rune(rule)
	if len(runes) <= maxLen {
		return rule
	}
	return string(runes[:maxLen]) + "..."
}
