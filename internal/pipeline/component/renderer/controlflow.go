// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderer

import (
	"context"
	"errors"
	"fmt"
	"maps"

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
func ShouldInclude(ctx context.Context, engine *template.Engine, includeWhen string, celContext map[string]any) (bool, error) {
	if includeWhen == "" {
		return true, nil
	}

	result, err := engine.Render(ctx, includeWhen, celContext)
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
	ctx context.Context,
	engine *template.Engine,
	forEach string,
	varName string,
	celContext map[string]any,
) ([]map[string]any, error) {
	result, err := engine.Render(ctx, forEach, celContext)
	if err != nil {
		return nil, err
	}

	items, err := ToIterableItems(ctx, result)
	if err != nil {
		return nil, err
	}

	if varName == "" {
		varName = "item"
	}

	contexts := make([]map[string]any, len(items))
	for i, item := range items {
		itemContext := maps.Clone(celContext)
		itemContext[varName] = item
		contexts[i] = itemContext
	}
	return contexts, nil
}

// EvaluateValidationRules evaluates a list of CEL-based validation rules against the given context.
// Every rule is evaluated and all failures are collected into a single joined error; the sole
// exception is an aborted render — an exhausted cost budget or a cancelled context — which stops
// the loop, because every remaining rule would fail the same way.
// Returns nil if there are no rules or all rules pass.
//
// Error messages include rule index and rule text for easy identification. Evaluation failures
// are wrapped rather than stringified, so a caller can classify a cost or deadline breach with
// errors.Is through the aggregate.
// The returned error does not include a "validation failed:" prefix — callers add their own context.
func EvaluateValidationRules(
	ctx context.Context,
	engine *template.Engine,
	rules []v1alpha1.ValidationRule,
	celContext map[string]any,
) error {
	if len(rules) == 0 {
		return nil
	}
	var errs []error
	for i, rule := range rules {
		ruleRef := template.TruncateTo(rule.Rule, 120)
		result, err := engine.Render(ctx, rule.Rule, celContext)
		if err != nil {
			errs = append(errs, fmt.Errorf("rule[%d] %q evaluation error: %w", i, ruleRef, err))
			if template.IsRenderAborted(err) {
				break
			}
			continue
		}
		boolResult, ok := result.(bool)
		if !ok {
			errs = append(errs, fmt.Errorf("rule[%d] %q must evaluate to boolean, got %T", i, ruleRef, result))
			continue
		}
		if !boolResult {
			errs = append(errs, fmt.Errorf("rule[%d] %q evaluated to false: %s", i, ruleRef, rule.Message))
		}
	}
	return errors.Join(errs...)
}
