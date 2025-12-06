// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

// FieldStrategy defines the interface for rendering different field types.
// Each strategy handles a specific field type (primitive, object, array, map).
type FieldStrategy interface {
	// CanHandle returns true if this strategy can handle the given field context.
	CanHandle(ctx *FieldContext) bool

	// Render generates YAML for the field using the provided builder and context.
	Render(b *YAMLBuilder, ctx *FieldContext)
}

// StrategyDispatcher selects the appropriate strategy for a field and renders it.
type StrategyDispatcher struct {
	strategies []FieldStrategy
}

// NewStrategyDispatcher creates a new dispatcher with all field strategies.
func NewStrategyDispatcher(renderer *FieldRenderer) *StrategyDispatcher {
	nestedRenderer := NewNestedTypeRenderer(renderer)

	return &StrategyDispatcher{
		strategies: []FieldStrategy{
			// Order matters - more specific strategies first
			NewMapFieldStrategy(renderer, nestedRenderer),
			NewObjectFieldStrategy(renderer),
			NewArrayFieldStrategy(renderer, nestedRenderer),
			NewPrimitiveFieldStrategy(renderer),
		},
	}
}

// Dispatch finds the appropriate strategy and renders the field.
// Returns true if a strategy handled the field, false otherwise.
func (d *StrategyDispatcher) Dispatch(b *YAMLBuilder, ctx *FieldContext) bool {
	for _, strategy := range d.strategies {
		if strategy.CanHandle(ctx) {
			strategy.Render(b, ctx)
			return true
		}
	}
	return false
}
