// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderer

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/openchoreo/openchoreo/internal/template"
)

// maxForEachItems bounds how many items a single forEach may expand to. Each item
// drives a full render, so an unbounded tenant-supplied collection amplifies work far
// beyond what the per-expression CEL cost limit can see.
//
// 100 covers what templates actually iterate over - a component's config files, mounts,
// routes, containers - and none of those collections is large. The cap is on the item
// count rather than the work per item because a static body charges nothing at all to the
// cost budget: renderString returns a span-free string without evaluating anything.
const maxForEachItems = 100

// maxForEachItemsPerRender bounds the sum of every forEach expansion in one pipeline
// render. The per-loop cap prevents one large fan-out; this cap prevents many legal
// 100-item loops with cheap or static bodies from recreating the same workload in total.
//
// The per-loop cap cannot stand in for it: the number of forEach sites is a product of
// nested arrays - a ComponentType's resources, and every trait's creates - so bounding
// each array on its own still leaves their product free to grow.
const maxForEachItemsPerRender = 10_000

type forEachBudget struct {
	mu    sync.Mutex
	spent int
}

type forEachBudgetKey struct{}

// WithForEachBudget returns a context carrying a render-scoped cumulative expansion
// budget. An existing budget is preserved so nested pipeline work shares the same total.
func WithForEachBudget(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(forEachBudgetKey{}).(*forEachBudget); ok {
		return ctx
	}
	return context.WithValue(ctx, forEachBudgetKey{}, &forEachBudget{})
}

// checkForEachLimit rejects a forEach result whose item count exceeds maxForEachItems.
// Callers must invoke it before copying the result or fanning out sub-renders over it, so
// an oversized collection costs no more than the length check.
//
// What it cannot save is the CEL-to-Go conversion: the engine has already materialized the
// result by the time a caller can count it. That step is bounded by the per-expression cost
// limit and the lists.range cap instead - this bound covers the per-item work that follows,
// which neither of those can see.
//
// The error wraps template.ErrExpansionLimitExceeded: the cap depends only on how many
// items the expression produced, so the same object trips it on every reconcile until a
// human edits the template or the data behind it.
func checkForEachLimit(ctx context.Context, n int) error {
	if n > maxForEachItems {
		return fmt.Errorf("%w: forEach expanded to %d items, exceeding the limit of %d",
			template.ErrExpansionLimitExceeded, n, maxForEachItems)
	}
	if ctx == nil {
		return nil
	}

	budget, ok := ctx.Value(forEachBudgetKey{}).(*forEachBudget)
	if !ok {
		return nil
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if budget.spent > maxForEachItemsPerRender-n {
		budget.spent = maxForEachItemsPerRender + 1
		return fmt.Errorf("%w: forEach expansions would exceed the cumulative limit of %d items in one render",
			template.ErrExpansionLimitExceeded, maxForEachItemsPerRender)
	}
	budget.spent += n
	return nil
}

// ToIterableItems converts a forEach evaluation result to a slice of items.
// Arrays are returned as-is. Maps are converted to sorted slice of {key, value} entries.
//
// Supported input types:
//   - []any - returned as-is
//   - []map[string]any - converted to []any
//   - map[string]any - converted to sorted slice of {"key": k, "value": v} entries
//
// For maps, the keys are sorted alphabetically to ensure deterministic iteration order.
// This is important for:
//   - Consistent resource generation across runs
//   - Predictable test results
//   - Reproducible deployments
//
// Results with more than maxForEachItems entries are rejected, and contexts created by
// WithForEachBudget also enforce the cumulative per-render cap.
func ToIterableItems(ctx context.Context, result any) ([]any, error) {
	switch v := result.(type) {
	case []any:
		if err := checkForEachLimit(ctx, len(v)); err != nil {
			return nil, err
		}
		return v, nil

	case []map[string]any:
		if err := checkForEachLimit(ctx, len(v)); err != nil {
			return nil, err
		}
		// Convert []map[string]any to []any
		items := make([]any, len(v))
		for i, m := range v {
			items[i] = m
		}
		return items, nil

	case map[string]any:
		if err := checkForEachLimit(ctx, len(v)); err != nil {
			return nil, err
		}
		// Convert map to sorted slice of {key, value} entries
		// Sort keys for deterministic iteration order
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Create items slice with {key, value} maps
		items := make([]any, 0, len(v))
		for _, key := range keys {
			items = append(items, map[string]any{
				"key":   key,
				"value": v[key],
			})
		}
		return items, nil

	default:
		return nil, fmt.Errorf("forEach must evaluate to array or map, got %T", result)
	}
}
