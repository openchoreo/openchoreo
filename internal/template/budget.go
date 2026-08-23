// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"context"
	"fmt"
	"math"
	"sync"
)

// CostBudget accumulates the CEL evaluation cost spent across every Render that shares it.
// The per-expression cost limit bounds one expression; the budget bounds the whole unit of
// work, so a template cannot escape the guard by splitting hostile work across many cheap
// expressions. A reconciler seeds one budget per reconcile and carries it on the context;
// callers that do not, get a per-Render fallback budget.
//
// The mutex makes a budget safe to share, but sharing one across renders that run at the
// same time would break the bound on how far past the budget a render can go. A budget can
// only be charged once cel-go reports what an evaluation cost, so admission is a pre-check
// (see exceeded) followed by a separate charge (see add): concurrent renders could all pass
// the check and each spend up to a full per-expression limit before any charge lands. The
// bound holds because rendering is sequential — the pipeline is CPU bound and starts no
// goroutines. Rendering in parallel under one budget would first need the allowance reserved
// under the lock before evaluation and reconciled against the actual cost afterwards.
type CostBudget struct {
	mu     sync.Mutex
	spent  uint64
	budget uint64
}

// NewCostBudget creates a budget allowing the given total cost. A zero budget is unbounded,
// which is only appropriate for tests that use it as a meter rather than a guard.
func NewCostBudget(budget uint64) *CostBudget {
	return &CostBudget{budget: budget}
}

// Spent reports the cost charged to the budget so far.
func (b *CostBudget) Spent() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent
}

// Budget reports the total cost this budget allows. Zero means unbounded.
func (b *CostBudget) Budget() uint64 {
	return b.budget
}

// add charges cost against the budget and reports whether that exhausted it. The cost is
// always recorded, even when the caller goes on to surface a different error, so that a
// failed evaluation still counts against the unit of work that provoked it.
//
// The breach message names the configured budget but deliberately NOT the amount spent.
// Reconcilers put this text in a status condition, so it has to be identical on every
// reconcile of an unchanged object: a message that drifts rewrites the condition, and each
// rewrite is a watch event that re-triggers the reconcile, defeating the slow requeue meant
// to pace a terminal failure. The budget is a constant, but the spend at the moment of the
// breach is not — one budget funds every render of a whole reconcile, so the total at the
// instant it tips over depends on cluster state the object's own spec does not fix. It is
// therefore not surfaced at all; a caller that wants it can read Spent() off its own budget.
func (b *CostBudget) add(cost uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Saturate rather than wrap. A single charge can already be near MaxUint64 - the
	// custom-function estimator combines its own costs with saturatingAdd - and a wrapped
	// total would read as a small spend, reopening an allowance that is in fact exhausted.
	if cost > math.MaxUint64-b.spent {
		b.spent = math.MaxUint64
		// Overflow proves the true spend exceeds any representable budget, so report the
		// breach directly: on a budget of exactly MaxUint64 the saturated value would
		// otherwise read as an exact fit.
		if b.budget > 0 {
			return b.breachError()
		}
		return nil
	}
	b.spent += cost
	return b.breachLocked()
}

// exceeded reports the breach error when the budget is already spent, charging nothing.
// The budget can only be charged after an evaluation returns, so a caller that goes on to
// evaluate again needs a way to ask first; see the check at the top of evaluateCEL.
//
// It refuses on an exactly-spent budget, where add does not, and the asymmetry is the point.
// add answers "did this charge overspend the allowance?", and a render that lands exactly on
// the budget has stayed within it - failing it would turn a legitimate render into an error
// over the last unit it was entitled to. exceeded answers a different question: "is there
// anything left to fund the next evaluation?" On an exactly-spent budget there is not, and
// letting it start anyway hands it up to a full per-expression cost limit before the charge
// that follows can stop it - the unbounded overshoot this pre-check exists to prevent.
func (b *CostBudget) exceeded() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.budget > 0 && b.spent >= b.budget {
		return b.breachError()
	}
	return nil
}

// breachLocked reports the breach error if the allowance is overspent. The caller holds b.mu.
func (b *CostBudget) breachLocked() error {
	if b.budget > 0 && b.spent > b.budget {
		return b.breachError()
	}
	return nil
}

func (b *CostBudget) breachError() error {
	return fmt.Errorf("%w (budget %d)", ErrCostBudgetExceeded, b.budget)
}

// reconcileBudget returns the cumulative render budget for one reconcile, given the
// per-expression cost limit the reconciler was configured with. A zero limit resolves to
// the same built-in default the engine applies, so a reconciler that leaves the limit
// unset still gets a budget consistent with the engine's own bound.
//
// It is unexported so the sizing arithmetic stays beside renderBudgetFactor: callers seed
// a budget through WithReconcileBudget and never handle the number themselves.
func reconcileBudget(costLimit uint64) uint64 {
	if costLimit == 0 {
		costLimit = defaultCELCostLimit
	}
	return costLimit * renderBudgetFactor
}

type budgetKey struct{}

// WithCostBudget attaches a cost budget to the context so every Render performed under it
// draws from the same pool.
func WithCostBudget(ctx context.Context, b *CostBudget) context.Context {
	return context.WithValue(ctx, budgetKey{}, b)
}

// WithReconcileBudget returns a context carrying a fresh cost budget sized for one
// reconcile, derived from the reconciler's configured per-expression cost limit (zero
// selects the built-in default). Every Render performed under the returned context draws
// from the same pool, so work split across many expressions is bounded in total.
//
// The budget is a plain context value with no deadline: attaching it does not affect the
// API calls a reconcile interleaves with rendering. Deadlines come from WithRenderTimeout,
// which each render entry point applies to itself.
func WithReconcileBudget(ctx context.Context, costLimit uint64) context.Context {
	return WithCostBudget(ctx, NewCostBudget(reconcileBudget(costLimit)))
}

// CostBudgetFrom returns the budget carried by the context, or nil when there is none.
// Reconcilers use it to report the spend behind a render failure without threading the
// budget through every call: the context already carries it to the same places.
func CostBudgetFrom(ctx context.Context) *CostBudget {
	b, _ := ctx.Value(budgetKey{}).(*CostBudget)
	return b
}
