// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderer

import (
	"fmt"
	"maps"

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
