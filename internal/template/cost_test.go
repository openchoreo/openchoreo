// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// hostileExpr is the denial-of-service proof of concept: 2,000 outer iterations each
// building a 2,000-element list, or 4M inner iterations. It runs for roughly a second
// unguarded, and every cost-limit test drives the guard with it.
const hostileExpr = "${size(lists.range(2000).map(x, lists.range(2000).map(y, x + y)))}"

func TestCELUnboundedEvaluation(t *testing.T) {
	e := NewEngine()
	start := time.Now()
	_, err := e.Render(t.Context(), hostileExpr, map[string]any{"spec": map[string]any{}})
	t.Logf("elapsed=%s err=%v", time.Since(start), err)
}

func TestCostLimitStopsHeavyExpression(t *testing.T) {
	e := NewEngine()
	start := time.Now()
	_, err := e.Render(t.Context(), hostileExpr, map[string]any{"spec": map[string]any{}})
	if err == nil {
		t.Fatalf("expected cost-limit error, got nil (took %s)", time.Since(start))
	}
	if !strings.Contains(err.Error(), "cost limit exceeded") {
		t.Fatalf("expected cost-limit error, got: %v", err)
	}
	if !errors.Is(err, ErrCostLimitExceeded) {
		t.Fatalf("expected error matching ErrCostLimitExceeded, got: %v", err)
	}
	if !strings.Contains(err.Error(), "lists.range(2000)") {
		t.Fatalf("expected error to name the offending expression, got: %v", err)
	}
}

func TestCostLimitAllowsNormalExpression(t *testing.T) {
	e := NewEngine()
	got, err := e.Render(t.Context(), "${spec.replicas}", map[string]any{"spec": map[string]any{"replicas": int64(3)}})
	if err != nil || got != int64(3) {
		t.Fatalf("normal expr broke: got=%v err=%v", got, err)
	}
}

// The cost limit is a property of the engine, so every construction path must carry it.
// A zero limit selects the default rather than disabling the guard.
func TestCostLimitDefaultsOnEveryConstructor(t *testing.T) {
	tests := []struct {
		name   string
		engine *Engine
	}{
		{name: "NewEngine", engine: NewEngine()},
		{name: "NewEngineWithOptions without WithCostLimit", engine: NewEngineWithOptions(DisableCache())},
		{name: "WithCostLimit(0) selects the default", engine: NewEngineWithOptions(WithCostLimit(0))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.engine.costLimit != defaultCELCostLimit {
				t.Fatalf("expected cost limit %d, got %d", defaultCELCostLimit, tt.engine.costLimit)
			}
		})
	}
}

// Pipelines construct engines through NewEngineWithOptions(WithCELExtensions(...)) without
// asking for a cost limit, so that path must end up bounded end-to-end and surface the
// typed sentinel rather than merely failing.
func TestNewEngineWithOptionsDefaultsCostLimit(t *testing.T) {
	e := NewEngineWithOptions(WithCELExtensions())
	if e.costLimit != defaultCELCostLimit {
		t.Fatalf("expected the default cost limit %d, got %d", defaultCELCostLimit, e.costLimit)
	}
	_, err := e.Render(t.Context(), hostileExpr, map[string]any{"spec": map[string]any{}})
	if !errors.Is(err, ErrCostLimitExceeded) {
		t.Fatalf("options-constructed engine must enforce the default limit, got: %v", err)
	}
}

func TestWithCostLimitOverridesDefault(t *testing.T) {
	e := NewEngineWithOptions(WithCostLimit(50))
	if e.costLimit != 50 {
		t.Fatalf("expected cost limit 50, got %d", e.costLimit)
	}
	_, err := e.Render(t.Context(), "${size(lists.range(100).map(x, x + 1))}", map[string]any{"spec": map[string]any{}})
	if !errors.Is(err, ErrCostLimitExceeded) {
		t.Fatalf("expected a low limit to be enforced, got: %v", err)
	}
}

// Cost must be charged on every evaluation path, including the two that return before a
// result is produced: oc_omit(), which short-circuits the walk, and an expression that
// breaches the per-expression limit. Work performed counts even when its result is
// discarded, or a template could spend the reconcile's whole allowance for free by making
// every expression fail.
func TestBudgetIsChargedOnEveryEvaluationPath(t *testing.T) {
	budget := NewCostBudget(0) // unbounded: used here as a meter, not a guard
	ctx := WithCostBudget(t.Context(), budget)
	e := NewEngine()

	inputs := map[string]any{"spec": map[string]any{"replicas": int64(3)}}
	if _, err := e.Render(ctx, "${spec.replicas}", inputs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	afterPlain := budget.Spent()
	if afterPlain == 0 {
		t.Fatal("expected an ordinary evaluation to charge the budget")
	}

	if _, err := e.Render(ctx, "${oc_omit()}", inputs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	afterOmit := budget.Spent()
	if afterOmit <= afterPlain {
		t.Fatalf("expected oc_omit() to charge the budget: %d -> %d", afterPlain, afterOmit)
	}

	if _, err := e.Render(ctx, hostileExpr, inputs); !errors.Is(err, ErrCostLimitExceeded) {
		t.Fatalf("expected the breaching expression to exceed the cost limit, got: %v", err)
	}
	// A breach must report the cost it actually burned rather than being skipped, so the
	// reconcile budget still sees the work an aborted evaluation did.
	if burned := budget.Spent() - afterOmit; burned <= defaultCELCostLimit {
		t.Fatalf("expected the breach to charge more than the limit %d, got %d", defaultCELCostLimit, burned)
	}
}
