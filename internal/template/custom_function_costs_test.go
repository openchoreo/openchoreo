// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// observedCost renders one expression with an effectively unlimited cost limit and reports
// what the engine charged for it. A single expression under a fresh unbounded budget spends
// exactly its own cost, so the budget doubles as the meter.
func observedCost(t *testing.T, expr string, inputs map[string]any) uint64 {
	t.Helper()
	budget := NewCostBudget(0)
	ctx := WithCostBudget(t.Context(), budget)
	e := NewEngineWithOptions(WithCostLimit(1 << 40))
	if _, err := e.Render(ctx, expr, inputs); err != nil {
		t.Fatalf("render of %q failed: %v", expr, err)
	}
	return budget.Spent()
}

// Without a runtime cost tracker, cel-go charges one unit for any overload it does not
// recognize, so an oc_* call over a megabyte of tenant data spends the same as one over an
// empty string. These are the overloads that traverse their input; each must charge for what
// it walked, or the per-expression limit and the reconcile budget are blind to it.
func TestCustomFunctionCostGrowsWithInputSize(t *testing.T) {
	tests := []struct {
		name  string
		expr  string
		build func(size int) map[string]any
	}{
		{
			name:  "oc_hash",
			expr:  "${oc_hash(payload)}",
			build: func(size int) map[string]any { return map[string]any{"payload": strings.Repeat("a", size)} },
		},
		{
			name:  "oc_generate_name string",
			expr:  "${oc_generate_name(payload)}",
			build: func(size int) map[string]any { return map[string]any{"payload": strings.Repeat("a", size)} },
		},
		{
			name:  "oc_dns_label string",
			expr:  "${oc_dns_label(payload)}",
			build: func(size int) map[string]any { return map[string]any{"payload": strings.Repeat("a", size)} },
		},
		{
			name: "oc_generate_name list",
			expr: "${oc_generate_name([payload, payload])}",
			build: func(size int) map[string]any {
				return map[string]any{"payload": strings.Repeat("a", size/2)}
			},
		},
		{
			name: "oc_generate_name list charges empty elements",
			expr: "${oc_generate_name(payload)}",
			build: func(size int) map[string]any {
				return map[string]any{"payload": emptyStrings(size)}
			},
		},
		{
			name: "oc_dns_label list charges empty elements",
			expr: "${oc_dns_label(payload)}",
			build: func(size int) map[string]any {
				return map[string]any{"payload": emptyStrings(size)}
			},
		},
		{
			name: "oc_merge",
			expr: "${oc_merge(left, right)}",
			build: func(size int) map[string]any {
				left := make(map[string]any, size)
				for i := range size {
					left[fmt.Sprintf("k%d", i)] = "v"
				}
				return map[string]any{"left": left, "right": map[string]any{"other": "v"}}
			},
		},
	}

	// The small and large inputs differ by 100x, so a cost model that still charges O(1)
	// cannot pass: it would report the same number twice.
	const (
		small = 100
		large = 10_000
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			smallCost := observedCost(t, tt.expr, tt.build(small))
			largeCost := observedCost(t, tt.expr, tt.build(large))
			t.Logf("%s: cost(%d)=%d cost(%d)=%d", tt.name, small, smallCost, large, largeCost)

			if largeCost <= smallCost {
				t.Fatalf("cost did not grow with input size: cost(%d)=%d, cost(%d)=%d — "+
					"the overload is still metered as O(1)", small, smallCost, large, largeCost)
			}

			// Growth must track the input, not merely be non-zero: a 100x larger input has to
			// cost at least an order of magnitude more.
			if largeCost < smallCost*10 {
				t.Errorf("cost grew sublinearly for a 100x larger input: cost(%d)=%d, cost(%d)=%d",
					small, smallCost, large, largeCost)
			}
		})
	}
}

func emptyStrings(size int) []any {
	items := make([]any, size)
	for i := range items {
		items[i] = ""
	}
	return items
}

// oc_hash's charge is proportional to the bytes it hashes, so a large enough argument trips
// the ordinary per-expression limit — the outcome that was unreachable while the overload
// was metered as a single unit.
func TestHugeCustomFunctionInputBreachesDefaultLimit(t *testing.T) {
	// At StringTraversalCostFactor (0.1 per byte) the default limit of 2,000,000 units
	// corresponds to ~20MB of traversal; go comfortably past it.
	payload := strings.Repeat("a", 32<<20)

	e := NewEngine()
	_, err := e.Render(t.Context(), "${oc_hash(payload)}", map[string]any{"payload": payload})
	if err == nil {
		t.Fatal("expected a cost breach for a 32MB oc_hash argument, got nil")
	}
	if !errors.Is(err, ErrCostLimitExceeded) && !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("expected a cost limit or budget breach, got: %v", err)
	}
	if !IsTerminalRenderError(err) {
		t.Fatalf("a cost breach must classify as terminal, got: %v", err)
	}
}

// oc_omit does no size-proportional work and is deliberately left on cel-go's default cost.
// It is the one custom overload without a tracker, so pin that it still evaluates.
func TestOmitStaysCheap(t *testing.T) {
	cost := observedCost(t, `${{"a": oc_omit()}}`, emptyInputs())
	if cost == 0 {
		t.Fatal("expected oc_omit to be charged cel-go's default cost, got 0")
	}
	t.Logf("oc_omit cost = %d", cost)
}
