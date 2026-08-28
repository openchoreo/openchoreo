# CEL Cost Enforcement

Templates are tenant-authored, and rendering runs in the controller. Without a bound, a
single expression such as

```cel
${size(lists.range(2000).map(x, lists.range(2000).map(y, x + y)))}
```

(4M inner iterations, roughly a second of CPU) runs per evaluation, per expression, per
reconcile. The guards below bound that work. They live in `internal/template`, the four
rendering pipelines under `internal/pipeline`, and `internal/controller`.

Three layers cover what the others cannot see, plus a wall-clock backstop:

| Guard | Scope | Default | Error |
|---|---|---|---|
| `cel.CostLimit` | One expression | 2,000,000 units | `ErrCostLimitExceeded` |
| `CostBudget` | Attached: one reconcile; fallback: one `Render` | cost limit × 1 | `ErrCostBudgetExceeded` |
| Custom-function estimator | `oc_*` data traversal | same tracker as the limit | (feeds the two above) |
| forEach caps | Per loop / cumulative per render | 100 / 10,000 | `ErrExpansionLimitExceeded` |
| Patch operations | Per component render | 10,000 | `ErrExpansionLimitExceeded` |
| `lists.range` size | Elements per call | 10,000 | plain evaluation error |
| Render timeout | Wall clock per rendering step | disabled (`0`) | `context.DeadlineExceeded` |

## Per-expression cost limit

Every CEL program is compiled and run with `cel.CostLimit`. cel-go meters each evaluation
and aborts once the accumulated cost passes the limit, so no single expression can run
away regardless of how the inputs are shaped.

The 2,000,000 default was derived once, by measuring the most expensive expression in the
shipped ComponentTypes, ProjectTypes, ResourceTypes and workflows (~36,000 units) and
allowing two orders of magnitude of headroom, capped by what fits a controller's memory
budget. No test re-derives it; if a legitimate template starts tripping the limit after an
upgrade, raise it with `--cel-cost-limit` (below) and file the template as a data point.

## Cumulative cost budget

A per-expression limit alone is bypassed by volume: a large CR of individually cheap
expressions costs the same as one expensive one. `CostBudget` is a per-reconcile pool that
every evaluation charges against, seeded on the reconcile context by each rendering
reconciler and shared by everything that renders during that reconcile — manifests, outputs
and each `readyWhen`.

Because the budget is the cost limit × 1, it is the *tighter* of the two bounds: a
reconcile gets, in total, what one expression is allowed. When rendering happens outside a
reconcile (tests, tools), the pipeline falls back to a budget of its own so the bound is
never simply absent.

Once the budget is exhausted, further evaluations are refused before they run rather than
after. A concrete failure is still named ahead of the breach: if an expression fails on its
own terms, the operator sees that error, and the breach surfaces on the next evaluation.

## Custom-function cost accounting

cel-go charges an unknown overload roughly O(1). The `oc_*` functions
(`oc_hash`, `oc_generate_name`, `oc_dns_label`, `oc_merge`) walk tenant data, so an
`interpreter.ActualCostEstimator` charges them by bytes walked, feeding the same tracker as
the limit and the budget.

Dispatch is by function name, because `DynType` arguments block overload resolution.
**A new `oc_*` function ships unmetered unless it is added to
`customFunctionCostEstimator.CallCost` in `internal/template/custom_functions.go`.**

Interpolation — the non-CEL string work that stitches `${...}` results back into a template
string — is done in a single pass with a byte builder. That work is invisible to cel-go's
meter, so it must stay linear in input and output size.

## Expansion guards

Fan-out happens *between* evaluations, where neither the limit nor the budget is metered:
one cheap `forEach` expression can drive thousands of sub-renders.

- **forEach item cap (100 per loop).** Checked before any copying or fan-out, so an
  oversized collection costs only the length check. It applies to every `forEach` —
  resource-level, trait create, trait patch and trait remove alike — because all of them
  funnel through `renderer.ToIterableItems`. 100 covers what templates actually iterate
  over: a component's config files, mounts, routes and containers.
- **Cumulative forEach cap (10,000 per render).** Many legal 100-item loops recreate the
  same workload in total; this bounds their sum. The per-loop cap cannot stand in for it,
  because the number of forEach sites is a product of nested arrays — a ComponentType's
  `resources`, and the `creates` of each trait bound to the component — and bounding each
  array on its own leaves their product free to grow.
- **Patch operation cap (10,000 per component render).** Counts targets × operations, so a
  trait patching many targets is bounded even when each patch is small. The error names the
  running total the batch would have reached, which is what tells an operator whether they
  are marginally or wildly over.
- **`lists.range()` size cap (10,000 elements).** cel-go's own metering charges
  `lists.range` only *after* it allocates the whole slice, so the cost limit cannot
  interrupt the allocation. The cap is checked before it, and is therefore load-bearing.
  cel-go raises it as an ordinary evaluation error, not a sentinel.

## Render timeout

A wall-clock backstop for work the cost model measures imperfectly (`split()` on very large
strings is nonlinear, and whole-collection references charge roughly constant). It is
**disabled by default**; the cost guards are the primary bound.

It applies to each rendering *step*, not to the reconcile: a reconcile that renders more
than once (manifests, outputs, each `readyWhen`) may spend it at each. The deadline is
derived inside each render entry point and released when it returns, so nothing accumulates
across a loop. Cluster reads are deliberately left outside it — a slow API server should not
surface as a template failure.

## Failure surface

A breach is reported on the object's conditions, not only in the logs: `Synced=False` /
`ReleaseSynced=False` for manifest rendering, `OutputsResolved=False` for outputs,
`ResourcesReady=False` for `readyWhen`, and `WorkflowCompleted=False` with reason
`WorkflowRenderingFailed` for a WorkflowRun. Retries then follow each reconciler's
pre-existing behavior — controller-runtime's per-key exponential backoff. Every retry is
itself bounded by the same guards, so a repeating breach costs one cost-limited render.

Two properties make that safe:

**Messages are deterministic.** The recursive render walks map keys in sorted order and
stops at the first failing expression, so the expression named in the error does not follow
Go's randomized map iteration, and the breach message carries no live spend. An unchanged
object therefore produces a byte-identical condition on every reconcile, the status write
no-ops, and no watch event is generated — a failing object does not self-trigger a loop.

**Messages are bounded.** A tenant-authored expression may be hundreds of KB, and an
oversized or invalid-UTF-8 condition is rejected by the API server, which would mean the
breach is never recorded at all. Fragments are truncated at 256 bytes on a rune boundary,
with the marker counted inside the bound. Every layer that re-quotes tenant input routes
through the same helper (`template.TruncateFragment`), aggregated post-render failures are
capped at 20 entries, and `controller.NewCondition` bounds whatever it is handed at 8192
bytes as a backstop.

```go
// internal/controller/conditions.go
const maxConditionMessageLen = 8192
Message: template.TruncateTo(message, maxConditionMessageLen),
```

## Configuration

Both settings are cluster-wide, applied to every rendering reconciler.

| Flag | Environment variable | Helm value | Flag default | Chart default |
|---|---|---|---|---|
| `--cel-cost-limit` | `CEL_COST_LIMIT` | `controllerManager.manager.celCostLimit` | `0` (built-in limit) | `2000000` |
| `--render-timeout` | `RENDER_TIMEOUT` | `controllerManager.manager.renderTimeout` | `0` (disabled) | `"0"` |

The two cost-limit defaults describe the same bound. `0` means "use the built-in limit",
and the chart states that limit — `defaultCELCostLimit` in `internal/template/engine_cache.go`
— outright, so an operator reading `values.yaml` sees the number rather than an indirection.
The two therefore have to be changed together. The chart omits the flag entirely when the
value is `0`, so an operator can still hand the decision back to the controller.

A malformed environment value fails startup rather than being silently replaced by the
default, and the Helm schema rejects an invalid duration at install time. Raising the cost
limit weakens the bound proportionally — the budget is derived from it — so treat it as an
escape hatch for a legitimate template, not a tuning knob.

## Known gaps

- Admission does not gate cost: a hostile ComponentType is accepted and fails at render
  time. The runtime guards are the only defence.
- The authorization CEL runtime (`internal/authz`) is not covered by these guards.
- Whole-collection identifier references (`${parameters.foo}`, `oc_merge` arguments) charge
  roughly constant regardless of the collection's size.
