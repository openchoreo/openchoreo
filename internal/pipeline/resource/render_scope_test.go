// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcepipeline

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// costlyExpr evaluates to a fixed integer but costs enough CEL budget to be
// measurable, so a budget can be sized just under two evaluations of it. It stays
// far below the default per-expression cost limit, which keeps these tests about
// the cumulative budget rather than the per-expression guard.
const costlyExpr = "${size(lists.range(999))}"

// costlyBoolExpr is costlyExpr shaped into the boolean a readyWhen must return, and
// costlyStringExpr into the string an output value must be.
const (
	costlyBoolExpr   = "${size(lists.range(999)) == 999}"
	costlyStringExpr = "${string(size(lists.range(999)))}"
)

// scopedInput builds a RenderInput exercising all three render entry points with
// the same costly expression: one manifest, one output, one readyWhen.
func scopedInput(t *testing.T) *RenderInput {
	t.Helper()
	return makeRenderInput(v1alpha1.ResourceTypeSpec{
		Resources: []v1alpha1.ResourceTypeManifest{{
			ID:        "claim",
			Template:  rawExt(t, map[string]any{"spec": map[string]any{"size": costlyExpr}}),
			ReadyWhen: costlyBoolExpr,
		}},
		Outputs: []v1alpha1.ResourceTypeOutput{{
			Name:  "size",
			Value: costlyStringExpr,
		}},
	})
}

// TestSeededBudgetIsSharedAcrossEntryPoints is the reconcile-scoped budget's whole
// point: RenderManifests, ResolveOutputs and EvaluateReadyWhen are three separate
// calls, so a per-call budget would let a template spend its full allowance again at
// each one. Seeding the budget on the context must make them draw from one pool.
func TestSeededBudgetIsSharedAcrossEntryPoints(t *testing.T) {
	p := NewPipeline()
	input := scopedInput(t)

	// Measure one manifest render against an unbounded budget.
	measured := template.NewCostBudget(0)
	_, err := p.RenderManifests(template.WithCostBudget(t.Context(), measured), input)
	require.NoError(t, err)
	manifestCost := measured.Spent()
	require.Positive(t, manifestCost, "the fixture must actually evaluate CEL")

	// A budget sized to exactly one manifest render leaves nothing for the calls that
	// follow it in a reconcile.
	shared := template.NewCostBudget(manifestCost)
	sharedCtx := template.WithCostBudget(t.Context(), shared)

	_, err = p.RenderManifests(sharedCtx, input)
	require.NoError(t, err, "the first entry point fits the budget exactly")

	_, err = p.ResolveOutputs(sharedCtx, input, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, template.ErrCostBudgetExceeded),
		"output resolution should exhaust the budget carried over from the manifest render, got %v", err)

	_, err = p.EvaluateReadyWhen(sharedCtx, input, nil, costlyBoolExpr)
	require.Error(t, err)
	assert.True(t, errors.Is(err, template.ErrCostBudgetExceeded),
		"readyWhen draws from the same exhausted budget, got %v", err)
	assert.False(t, errors.Is(err, template.ErrCostLimitExceeded),
		"the per-expression limit is untouched here; the budget is what ran out")
}

// TestUnseededEntryPointsGetIndependentBudgets is the control for the test above: with
// no budget on the context each call falls back to its own per-Render budget, so the
// second and third calls succeed. Without this, the shared-budget assertions could pass
// because the fixture is simply too expensive.
func TestUnseededEntryPointsGetIndependentBudgets(t *testing.T) {
	p := NewPipeline()
	input := scopedInput(t)

	_, err := p.RenderManifests(t.Context(), input)
	require.NoError(t, err)
	_, err = p.ResolveOutputs(t.Context(), input, nil)
	require.NoError(t, err)
	ready, err := p.EvaluateReadyWhen(t.Context(), input, nil, costlyBoolExpr)
	require.NoError(t, err)
	assert.True(t, ready)
}

// TestRenderDeadlineInterruptsEvaluation checks that the deadline configured with
// WithRenderTimeout reaches the engine from inside the entry point, and that the
// interrupt is classifiable as a deadline rather than as an opaque CEL failure.
func TestRenderDeadlineInterruptsEvaluation(t *testing.T) {
	// A comprehension long enough that the engine's periodic interrupt check runs, and
	// cheap enough to stay well under the per-expression cost limit.
	input := renderSingle(t, rawExt(t, map[string]any{
		"spec": map[string]any{"size": "${size(lists.range(5000).map(i, i + 1))}"},
	}))

	// Control: the same render on a pipeline with no deadline must succeed, so the
	// assertion below cannot pass because the expression is simply broken.
	_, err := NewPipeline().RenderManifests(t.Context(), input)
	require.NoError(t, err)

	// The caller passes an ordinary context; the deadline comes from the pipeline.
	_, err = NewPipeline(WithRenderTimeout(time.Nanosecond)).RenderManifests(t.Context(), input)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded),
		"an expired render deadline must surface as DeadlineExceeded, got %v", err)
	assert.False(t, template.IsTerminalRenderError(err),
		"a deadline is load-dependent and must not be classified as terminal")
}

// TestAPIStallDoesNotConsumeTheRenderDeadline reproduces the resourcereleasebinding
// shape: two render entry points with an API call between them. The render deadline
// must be derived per entry point, so a slow API call cannot expire the deadline of the
// render that follows it — otherwise a stalled API server is reported to the user as a
// runaway template.
func TestAPIStallDoesNotConsumeTheRenderDeadline(t *testing.T) {
	const (
		renderTimeout = 200 * time.Millisecond
		apiStall      = 400 * time.Millisecond
	)

	p := NewPipeline(WithRenderTimeout(renderTimeout))
	input := scopedInput(t)

	// The reconcile context: it carries the budget and no deadline. The caller never
	// derives one - each entry point does that for itself.
	renderCtx := template.WithReconcileBudget(t.Context(), 0)

	_, err := p.RenderManifests(renderCtx, input)
	require.NoError(t, err)

	// Stand in for the CreateOrUpdate the controller performs between the two render
	// entry points. It runs on renderCtx, which must be unaffected.
	time.Sleep(apiStall)
	require.NoError(t, renderCtx.Err(), "the reconcile context must not carry a render deadline")

	// A deadline spanning both entry points would already have expired by now.
	_, err = p.ResolveOutputs(renderCtx, input, nil)
	require.NoError(t, err, "the second entry point must get a fresh deadline")
}

// costlyFailingExpr burns real budget and then fails for a reason of its own: the
// comprehension runs to completion before the index lookup is refused. ResolveOutputs
// aggregates output errors and keeps going, so an expression of this shape is what would
// let a template spend its whole allowance many times over — each evaluation is paid for,
// but the error carries the index, not the overspend, so nothing stops the sweep.
const costlyFailingExpr = "${string(lists.range(999).map(i, i + 1)[99999])}"

// failingOutputsInput builds a RenderInput whose outputs are all costlyFailingExpr.
func failingOutputsInput(t *testing.T, n int) *RenderInput {
	t.Helper()
	outputs := make([]v1alpha1.ResourceTypeOutput, n)
	for i := range outputs {
		outputs[i] = v1alpha1.ResourceTypeOutput{
			Name:  fmt.Sprintf("out%d", i),
			Value: costlyFailingExpr,
		}
	}
	return makeRenderInput(v1alpha1.ResourceTypeSpec{Outputs: outputs})
}

// An exhausted budget must stop the output sweep even when every output fails on its own
// terms. The engine charges a budget only after an evaluation returns, so the expression
// that crosses the line reports its own failure and the breach is deferred; the refusal on
// the next evaluation is what turns that deferral into a bound. Without it the aggregator
// walks every remaining output at full per-expression cost and the budget bounds nothing.
func TestExhaustedBudgetStopsTheOutputSweep(t *testing.T) {
	p := NewPipeline()

	// Measure one failing output against an unbounded budget.
	measured := template.NewCostBudget(0)
	_, err := p.ResolveOutputs(template.WithCostBudget(t.Context(), measured), failingOutputsInput(t, 1), nil)
	require.Error(t, err, "the fixture output must fail on its own terms")
	assert.False(t, errors.Is(err, template.ErrCostBudgetExceeded),
		"the fixture must fail for its own reason, not by exhausting an unbounded budget")
	oneOutputCost := measured.Spent()
	require.Positive(t, oneOutputCost, "the fixture must actually evaluate CEL")

	// Ten identical outputs against a budget that one and a half of them exhaust.
	const outputs = 10
	shared := template.NewCostBudget(oneOutputCost * 3 / 2)
	_, err = p.ResolveOutputs(template.WithCostBudget(t.Context(), shared), failingOutputsInput(t, outputs), nil)

	require.Error(t, err)
	assert.True(t, errors.Is(err, template.ErrCostBudgetExceeded),
		"the sweep must surface the breach rather than ten index errors, got %v", err)
	// Two evaluations at most: the one that crossed the line, and none after it.
	assert.LessOrEqual(t, shared.Spent(), oneOutputCost*2,
		"the sweep must stop at the overspend, not evaluate all %d outputs", outputs)
}
