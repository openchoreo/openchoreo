// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"
)

// heavyExpr costs ~10,011 units: cheap enough to pass any realistic per-expression limit,
// expensive enough that a handful of them exhaust a budget. lists.range is capped at
// maxListsRangeSize, so the size is derived from that constant rather than hard-coded.
var heavyExpr = fmt.Sprintf("${size(lists.range(%d))}", maxListsRangeSize-1)

// heavyExprCost is the measured cost of heavyExpr. Tests size their budgets from it so a
// change to the CEL cost model surfaces as a clear failure here rather than a flaky one.
const heavyExprCost uint64 = 10_011

// comprehensionExpr runs 5,000 fold iterations - well above interruptCheckFrequency, so a
// cancelled context interrupts it, and well under the default cost limit, so the interrupt
// is the only thing that can stop it. A bare builtin like lists.range is atomic and would
// never observe the cancellation.
const comprehensionExpr = "${size(lists.range(5000).map(i, i + 1))}"

// expensiveFailureExpr burns real cost and then fails for a reason of its own: the
// comprehension runs to completion before the index lookup is refused. That combination is
// what makes the budget's post-hoc charging insufficient on its own — the evaluation is
// paid for, but the error the caller sees names the index, not the overspend.
const expensiveFailureExpr = "${lists.range(9000).map(i, i + 1)[999999]}"

// expensiveFailureCost is the measured cost of expensiveFailureExpr, used to size a budget
// that one such evaluation overshoots.
const expensiveFailureCost uint64 = 135_024

func emptyInputs() map[string]any {
	return map[string]any{"spec": map[string]any{}}
}

// watchdogTimeout is generous enough to survive the ~10x slowdown of a -race build while
// still failing long before the go test timeout.
const watchdogTimeout = 30 * time.Second

// withWatchdog fails the test rather than hanging CI when a guard stops working and an
// evaluation runs away.
func withWatchdog(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(watchdogTimeout):
		t.Fatalf("render did not return within %s: the cost/cancellation guard is not stopping evaluation", watchdogTimeout)
	}
}

// A budget on the context is shared by every Render made under it, so a template cannot
// evade the per-expression limit by spreading hostile work over many cheap renders.
func TestCostBudgetAccumulatesAcrossRenders(t *testing.T) {
	// Room for two heavy expressions but not three.
	budget := NewCostBudget(heavyExprCost * 5 / 2)
	ctx := WithCostBudget(t.Context(), budget)
	e := NewEngine()

	if _, err := e.Render(ctx, heavyExpr, emptyInputs()); err != nil {
		t.Fatalf("first render must succeed: %v", err)
	}
	if _, err := e.Render(ctx, heavyExpr, emptyInputs()); err != nil {
		t.Fatalf("second render must succeed: %v", err)
	}

	_, err := e.Render(ctx, heavyExpr, emptyInputs())
	if !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("third render must exhaust the shared budget, got: %v", err)
	}
	// The per-expression limit is untouched: only the accumulation trips.
	if errors.Is(err, ErrCostLimitExceeded) {
		t.Fatalf("expected a budget breach, not a per-expression cost-limit breach: %v", err)
	}
	if spent := budget.Spent(); spent <= budget.budget {
		t.Fatalf("expected the budget to record an overspend, got spent=%d budget=%d", spent, budget.budget)
	}
}

// A fresh context means a fresh budget: budgets are scoped to their carrier, not global.
func TestCostBudgetIsScopedToItsContext(t *testing.T) {
	e := NewEngine()
	exhausted := WithCostBudget(t.Context(), NewCostBudget(1))
	if _, err := e.Render(exhausted, heavyExpr, emptyInputs()); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("expected the tiny budget to trip, got: %v", err)
	}
	if _, err := e.Render(t.Context(), heavyExpr, emptyInputs()); err != nil {
		t.Fatalf("a render on a budget-free context must not inherit the exhausted budget: %v", err)
	}
}

// Callers that supply no budget still get one: a single Render is bounded by
// costLimit x renderBudgetFactor, so many individually-legal expressions in one template
// cannot add up to unbounded work.
func TestRenderFallbackBudgetBoundsASingleRender(t *testing.T) {
	const limit uint64 = 50_000
	e := NewEngineWithOptions(WithCostLimit(limit))
	if got := e.renderBudgetDefault(); got != limit*renderBudgetFactor {
		t.Fatalf("expected fallback budget %d, got %d", limit*renderBudgetFactor, got)
	}

	// Six heavy values total ~60k against a 50k budget, while each one stays under the
	// 50k per-expression limit - only the accumulation can fail this render.
	data := make(map[string]any, 6)
	for i := range 6 {
		data[fmt.Sprintf("field%d", i)] = heavyExpr
	}

	var err error
	withWatchdog(t, func() {
		_, err = e.Render(t.Context(), data, emptyInputs())
	})
	if !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("expected the per-Render fallback budget to bound the render, got: %v", err)
	}
	if errors.Is(err, ErrCostLimitExceeded) {
		t.Fatalf("no single expression exceeds the limit; expected a budget breach, got: %v", err)
	}
}

// A budget breach message must be byte-identical across renders of the same template.
// Reconcilers put it in a status condition, so a message that varies rewrites the condition
// every reconcile, and each rewrite is a watch event that re-triggers the reconcile —
// defeating the slow requeue that paces a terminal failure.
//
// Two things used to make it vary, and the fixture is built to catch both: the spend at the
// moment of the breach (dropped from the message), and which expression is named, which
// followed Go's randomized map iteration until the render walk was ordered. The expressions
// here are DISTINCT and each cheap enough to pass the per-expression limit, so only their
// accumulation trips the budget and any one of them could plausibly be the one named. A
// fixture using one repeated expression would pass no matter how the walk is ordered.
func TestBudgetBreachMessageIsDeterministic(t *testing.T) {
	render := func() string {
		t.Helper()
		e := NewEngine()
		ctx := WithCostBudget(t.Context(), NewCostBudget(heavyExprCost*5/2))
		data := make(map[string]any, 6)
		for i := range 6 {
			data[fmt.Sprintf("field%d", i)] = fmt.Sprintf("${size(lists.range(%d))}", maxListsRangeSize-1-int64(i))
		}
		_, err := e.Render(ctx, data, emptyInputs())
		if !errors.Is(err, ErrCostBudgetExceeded) {
			t.Fatalf("expected the accumulated cost to exhaust the budget, got: %v", err)
		}
		return err.Error()
	}

	first := render()
	for range 30 {
		if got := render(); got != first {
			t.Fatalf("budget breach message is not deterministic:\n first: %s\n  then: %s", first, got)
		}
	}
}

// Dynamic map keys are evaluated CEL like any other expression, so they must be charged too.
// A key-only charge point is easy to miss when threading the budget through the walk.
func TestDynamicMapKeysChargeTheBudget(t *testing.T) {
	e := NewEngine()
	ctx := WithCostBudget(t.Context(), NewCostBudget(100))
	data := map[string]any{
		fmt.Sprintf("${'key' + string(size(lists.range(%d)))}", maxListsRangeSize-1): "value",
	}

	_, err := e.Render(ctx, data, emptyInputs())
	if !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("expected a dynamic map key to be charged against the budget, got: %v", err)
	}
}

// An exhausted budget must not be masked when the expression also fails: the underlying
// failure is reported, and the spend is still recorded so the next expression trips.
func TestCostLimitBreachTakesPrecedenceOverBudget(t *testing.T) {
	// A limit and a budget of the same size is the worst case for masking: with
	// renderBudgetFactor at 1 every per-expression breach is also a budget breach, so the
	// two errors compete on exactly the expression an operator most needs named.
	const limit uint64 = 50_000
	e := NewEngineWithOptions(WithCostLimit(limit))
	budget := NewCostBudget(limit)
	ctx := WithCostBudget(t.Context(), budget)

	var err error
	withWatchdog(t, func() {
		_, err = e.Render(ctx, comprehensionExpr, emptyInputs())
	})
	if !errors.Is(err, ErrCostLimitExceeded) {
		t.Fatalf("a per-expression breach must surface as ErrCostLimitExceeded, got: %v", err)
	}
	if spent := budget.Spent(); spent == 0 {
		t.Fatal("the cost burned by a failed evaluation must still be charged to the budget")
	}
	// The spend was recorded, so the next expression on the same context trips the budget.
	if _, err := e.Render(ctx, "${spec}", emptyInputs()); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("expected the recorded overspend to trip the next render, got: %v", err)
	}
}

// oc_omit() returns the sentinel through the error path, so it is the one success path that
// could swallow a budget breach and let a template keep rendering past its allowance.
func TestOmitDoesNotSwallowABudgetBreach(t *testing.T) {
	e := NewEngine()
	ctx := WithCostBudget(t.Context(), NewCostBudget(1))

	if _, err := e.Render(ctx, heavyExpr, emptyInputs()); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("expected the heavy expression to exhaust the budget, got: %v", err)
	}

	got, err := e.Render(ctx, "${oc_omit()}", emptyInputs())
	if !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("omit must surface the exhausted budget rather than the sentinel, got=%v err=%v", got, err)
	}
}

// A cancelled reconcile must stop evaluation rather than run the comprehension to completion,
// and the error must classify as context.Canceled for the caller.
func TestRenderStopsOnCancelledContext(t *testing.T) {
	e := NewEngine()

	// Control: the same expression completes normally, so a failure below is the
	// cancellation and not a broken expression.
	if _, err := e.Render(t.Context(), comprehensionExpr, emptyInputs()); err != nil {
		t.Fatalf("control render must succeed under a live context: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var err error
	withWatchdog(t, func() {
		_, err = e.Render(ctx, comprehensionExpr, emptyInputs())
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the error to wrap context.Canceled, got: %v", err)
	}
}

func TestRenderStopsOnExpiredDeadline(t *testing.T) {
	e := NewEngine()
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()

	var err error
	withWatchdog(t, func() {
		_, err = e.Render(ctx, comprehensionExpr, emptyInputs())
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the error to wrap context.DeadlineExceeded, got: %v", err)
	}
}

// Render is called with a nil context by no production path, but a zero value must not panic.
func TestRenderAcceptsNilContext(t *testing.T) {
	e := NewEngine()
	//nolint:staticcheck // deliberately passing a nil context to prove it is normalized
	got, err := e.Render(nil, "${spec.replicas}", map[string]any{"spec": map[string]any{"replicas": int64(3)}})
	if err != nil || got != int64(3) {
		t.Fatalf("nil context must be treated as context.Background(): got=%v err=%v", got, err)
	}
}

// The measured cost of heavyExpr underpins the budget sizes above; if the CEL cost model
// shifts, fail here with the new number rather than confusing the budget tests.
func TestHeavyExprCostIsStable(t *testing.T) {
	budget := NewCostBudget(0)
	ctx := WithCostBudget(t.Context(), budget)
	e := NewEngineWithOptions(WithCostLimit(1 << 40))
	if _, err := e.Render(ctx, heavyExpr, emptyInputs()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cost := budget.Spent(); cost != heavyExprCost {
		t.Fatalf("heavyExpr cost changed: expected %d, got %d - resize the budget tests", heavyExprCost, cost)
	}
}

// A render walking a large, entirely static structure never reaches a CEL evaluation, so
// ContextEval cannot interrupt it. renderString checks the context itself so an enabled
// render deadline still stops the traversal.
func TestStaticRenderObservesCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	e := NewEngine()
	_, err := e.Render(ctx, map[string]any{"a": "no expressions here"}, emptyInputs())
	if err == nil {
		t.Fatal("expected an expression-free render under a cancelled context to fail, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the error to wrap context.Canceled, got: %v", err)
	}
	if IsTerminalRenderError(err) {
		t.Fatalf("a cancelled context is transient, not terminal: %v", err)
	}
	if !IsRenderAborted(err) {
		t.Fatalf("a cancelled context must abort the render: %v", err)
	}
}

// An expression can burn most of a reconcile's allowance and still fail for its own reason,
// in which case that reason is what surfaces and the breach is deferred (see
// TestPerExpressionBreachIsNamedAheadOfTheBudget). Deferring is only safe if the next
// request to evaluate is refused outright: the pipelines aggregate render errors and keep
// going, so without the refusal every remaining expression would be evaluated in full and
// the budget would bound nothing at all.
func TestExhaustedBudgetRefusesFurtherEvaluation(t *testing.T) {
	budget := NewCostBudget(expensiveFailureCost / 2)
	ctx := WithCostBudget(t.Context(), budget)
	e := NewEngine()

	// The first evaluation overshoots the allowance but reports its own failure.
	_, err := e.Render(ctx, expensiveFailureExpr, emptyInputs())
	if err == nil {
		t.Fatal("expected the expression to fail on its own terms")
	}
	if errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("the concrete failure must be named ahead of the breach, got: %v", err)
	}
	spent := budget.Spent()
	if spent <= budget.Budget() {
		t.Fatalf("expected the failed evaluation to overshoot the budget, got spent=%d budget=%d",
			spent, budget.Budget())
	}

	// The second is refused before any work is done - the breach surfaces, and the spend
	// does not move. A charge here would mean the expression ran anyway.
	if _, err := e.Render(ctx, expensiveFailureExpr, emptyInputs()); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("an exhausted budget must refuse the next evaluation, got: %v", err)
	}
	if after := budget.Spent(); after != spent {
		t.Fatalf("a refused evaluation must charge nothing: spent %d -> %d", spent, after)
	}
}

// The refusal must not depend on the expression being the expensive one: any evaluation
// under an exhausted budget is refused, including one that would have cost almost nothing.
func TestExhaustedBudgetRefusesEvenACheapExpression(t *testing.T) {
	budget := NewCostBudget(expensiveFailureCost / 2)
	ctx := WithCostBudget(t.Context(), budget)
	e := NewEngine()

	if _, err := e.Render(ctx, expensiveFailureExpr, emptyInputs()); err == nil {
		t.Fatal("expected the expression to fail on its own terms")
	}
	spent := budget.Spent()

	if _, err := e.Render(ctx, "${1 + 1}", emptyInputs()); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("expected the exhausted budget to refuse a cheap expression, got: %v", err)
	}
	if after := budget.Spent(); after != spent {
		t.Fatalf("a refused evaluation must charge nothing: spent %d -> %d", spent, after)
	}
}

// A charge can arrive near MaxUint64 (the custom-function estimator combines its own costs
// with saturatingAdd), so the meter must saturate. A wrapped total would read as a small
// spend and reopen an allowance that is in fact long exhausted.
// Overflow at a budget of MaxUint64 is the one case the saturated value cannot report:
// it lands exactly on the budget, which breachLocked would read as an exact fit. The true
// spend exceeds any representable budget, so the overflowing charge itself must breach.
func TestOverflowBreachesAMaxUint64Budget(t *testing.T) {
	budget := NewCostBudget(math.MaxUint64)
	if err := budget.add(math.MaxUint64 - 1); err != nil {
		t.Fatalf("a charge within the budget must not breach it, got: %v", err)
	}
	if err := budget.add(2); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("an overflowing charge is an overspend, not an exact fit, got: %v", err)
	}
	if spent := budget.Spent(); spent != math.MaxUint64 {
		t.Fatalf("overflowing spend must saturate, got %d", spent)
	}
}

func TestCostBudgetSaturatesOnOverflow(t *testing.T) {
	budget := NewCostBudget(1000)
	if err := budget.add(math.MaxUint64 - 1); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("a charge far past the budget must breach it, got: %v", err)
	}
	if err := budget.add(2); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("overflowing charge must keep the budget breached, got: %v", err)
	}
	if spent := budget.Spent(); spent != math.MaxUint64 {
		t.Fatalf("overflowing spend wrapped instead of saturating: got %d", spent)
	}
	if err := budget.exceeded(); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("a saturated meter must stay breached, got: %v", err)
	}
}

// A render that lands exactly on its budget has stayed within it, so it must succeed - and
// then nothing may be evaluated under that budget again. The two halves pull in opposite
// directions, which is why add and exceeded ask different questions of the same counter:
// without the second half, an exactly-spent budget funds one more expression at up to the
// full per-expression limit, which is the unbounded overshoot the pre-check exists to stop.
func TestExactlySpentBudgetCompletesTheRenderThenRefusesTheNext(t *testing.T) {
	budget := NewCostBudget(heavyExprCost)
	ctx := WithCostBudget(t.Context(), budget)
	e := NewEngine()

	if _, err := e.Render(ctx, heavyExpr, emptyInputs()); err != nil {
		t.Fatalf("a render costing exactly its budget is within it and must succeed, got: %v", err)
	}
	if spent := budget.Spent(); spent != budget.Budget() {
		t.Fatalf("expected the render to spend the budget exactly, got spent=%d budget=%d",
			spent, budget.Budget())
	}

	spent := budget.Spent()
	if _, err := e.Render(ctx, "${1 + 1}", emptyInputs()); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("an exactly-spent budget has nothing left to fund the next evaluation, got: %v", err)
	}
	if after := budget.Spent(); after != spent {
		t.Fatalf("a refused evaluation must charge nothing: spent %d -> %d", spent, after)
	}
}

// add reports only that a charge overspent the allowance. An exact fit has not, and failing
// it would turn a legitimate render into an error over the last unit it was entitled to.
func TestExactFitChargeIsNotABreach(t *testing.T) {
	budget := NewCostBudget(1000)
	if err := budget.add(1000); err != nil {
		t.Fatalf("a charge landing exactly on the budget is within it, got: %v", err)
	}
	if err := budget.exceeded(); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("but nothing may be evaluated afterwards, got: %v", err)
	}
}

// A cancelled render must stop on data that holds no strings at all. Numbers and booleans
// reach neither the expression paths (which ContextEval interrupts) nor renderString (which
// checks the context itself), so before the walk checked for itself, a cancelled render over
// a large primitive list ran to completion and returned a result as though nothing had
// happened. Bounding the deadline is only worth anything if the walk observes it.
func TestCancelledRenderStopsOnPrimitiveOnlyData(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	items := make([]any, 50_000)
	for i := range items {
		items[i] = i%2 == 0
	}

	e := NewEngine()
	_, err := e.Render(ctx, items, emptyInputs())
	if err == nil {
		t.Fatal("expected a cancelled render over primitive-only data to fail, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the error to wrap context.Canceled, got: %v", err)
	}
	if !IsRenderAborted(err) {
		t.Fatalf("a cancelled context must abort the render: %v", err)
	}
}
