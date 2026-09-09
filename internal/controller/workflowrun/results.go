// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	workflowpipeline "github.com/openchoreo/openchoreo/internal/pipeline/workflow"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Result sizing. Results land in status, which every WorkflowRun watcher reads on every
// change, so a workflow that produces a large value must not be able to inflate the object
// for everyone. These are the defaults an unset ResultLimits field selects.
const (
	// defaultResultValueMaxBytes bounds a single recorded value.
	defaultResultValueMaxBytes = 4 * 1024

	// defaultResultsMaxBytes bounds every recorded value of one run put together. A run
	// that reaches it stops recording further results rather than truncating each one to
	// nothing, so the results that did fit stay usable.
	defaultResultsMaxBytes = 32 * 1024
)

// ResultLimits bounds what one WorkflowRun can write into status.results. A zero field
// means "unset" and selects the built-in default; neither means unlimited, because status
// is read by every watcher of the object and there is no supported way to opt out of
// bounding it.
type ResultLimits struct {
	// ValueMaxBytes bounds a single recorded value. A longer value is truncated and the
	// entry is marked truncated.
	ValueMaxBytes int

	// TotalMaxBytes bounds every value one run records put together. Once reached, the
	// remaining results are not recorded at all.
	TotalMaxBytes int
}

// Event reasons emitted while resolving results. Extraction is best-effort by design: a
// workflow whose results cannot be resolved has still run, and failing the reconcile would
// retry the whole thing forever over a value nobody is blocked on. Every failure below
// leaves the entry out and says why in an Event instead.
const (
	reasonResultExtractionFailed = "ResultExtractionFailed"
	reasonResultTruncated        = "ResultTruncated"
	reasonResultsBudgetExceeded  = "ResultsBudgetExceeded"
)

// TaskOutputs holds one task's outputs, keyed by result name.
type TaskOutputs map[string]string

// RunOutputs holds every task's outputs, keyed by the same task name that appears in
// WorkflowRunStatus.Tasks[].Name.
type RunOutputs map[string]TaskOutputs

// ResultExtractor turns one workflow engine's run resource into the vendor-neutral shapes
// result resolution works on. Argo is the only implementation today; a Tekton extractor
// maps TaskRun results to Outputs and TaskRun status to Phases the same way, and nothing
// below this seam knows which engine ran.
type ResultExtractor interface {
	// Outputs returns each completed task's outputs.
	Outputs() RunOutputs

	// Phases returns each task's phase, keyed by the same task names Outputs uses.
	Phases() map[string]string
}

// resolveResults evaluates the Workflow's declared results against a completed run and
// returns the entries to record in status, plus the test report projected from the reserved
// result when one is present and parses.
//
// It never returns an error. A declaration that cannot be resolved produces an Event and no
// entry: the run itself succeeded or failed on its own terms, and result extraction is not
// allowed to change that verdict or to wedge the reconcile.
func (r *Reconciler) resolveResults(
	ctx context.Context,
	workflowRun *openchoreodevv1alpha1.WorkflowRun,
	workflow *openchoreodevv1alpha1.Workflow,
	declarations []openchoreodevv1alpha1.WorkflowResult,
	extractor ResultExtractor,
) []openchoreodevv1alpha1.WorkflowRunResult {
	if len(declarations) == 0 {
		return nil
	}

	logger := log.FromContext(ctx)
	outputs := extractor.Outputs()
	phases := extractor.Phases()

	valueCap := r.resultValueMaxBytes()
	runCap := r.resultsMaxBytes()

	results := make([]openchoreodevv1alpha1.WorkflowRunResult, 0, len(declarations))
	// resolved feeds ${results['<name>']} so a later declaration can build on an earlier
	// one. It carries the value actually recorded, truncation included, so an expression
	// never sees bytes that status does not.
	resolved := make(map[string]string, len(declarations))
	var spent int

	for i := range declarations {
		decl := &declarations[i]

		value, err := r.resolveResultValue(ctx, workflowRun, workflow, decl, outputs, phases, resolved)
		if err != nil {
			// A declared result that produced nothing is a workflow authoring problem, not
			// a controller problem: surface it where the author will look and move on.
			r.eventf(workflowRun, corev1.EventTypeWarning, reasonResultExtractionFailed,
				"result %q was not recorded: %v", decl.Name, err)
			logger.V(1).Info("skipping unresolvable workflow result",
				"result", decl.Name, "workflowrun", workflowRun.Name, "reason", err.Error())
			continue
		}

		entry := openchoreodevv1alpha1.WorkflowRunResult{
			Name:        decl.Name,
			Description: decl.Description,
			Sensitive:   decl.Sensitive,
		}

		if decl.Sensitive {
			// The entry records that the run produced the result; the value is dropped
			// before it can reach status. It costs nothing against the run budget, and
			// nothing is put in resolved either - an expression must not be able to read a
			// value back out that status is withholding.
			results = append(results, entry)
			continue
		}

		recorded, truncated := truncateResultValue(value, valueCap)

		if spent+len(recorded) > runCap {
			// Out of room. Stop rather than shrink every remaining value to a stub: partial
			// results that are each complete are more useful than a full set of fragments.
			// Checked before the truncation event so a value that is dropped here is not
			// also reported as truncated - it was not recorded at all.
			r.eventf(workflowRun, corev1.EventTypeWarning, reasonResultsBudgetExceeded,
				"results from %q onward were not recorded: the run's total result size cap of %d bytes was reached",
				decl.Name, runCap)
			logger.V(1).Info("workflow results budget exhausted",
				"workflowrun", workflowRun.Name, "recorded", len(results), "declared", len(declarations))
			break
		}
		spent += len(recorded)

		if truncated {
			r.eventf(workflowRun, corev1.EventTypeNormal, reasonResultTruncated,
				"result %q was truncated to %d bytes (produced %d)", decl.Name, len(recorded), len(value))
		}

		entry.Value = recorded
		entry.Truncated = truncated
		results = append(results, entry)
		resolved[decl.Name] = recorded
	}

	if len(results) == 0 {
		return nil
	}

	return results
}

// resolveResultValue produces the raw value for one declaration, before any size cap is
// applied.
func (r *Reconciler) resolveResultValue(
	ctx context.Context,
	workflowRun *openchoreodevv1alpha1.WorkflowRun,
	workflow *openchoreodevv1alpha1.Workflow,
	decl *openchoreodevv1alpha1.WorkflowResult,
	outputs RunOutputs,
	phases map[string]string,
	resolved map[string]string,
) (string, error) {
	switch {
	case decl.ValueFrom.TaskResult != nil:
		return resolveTaskResult(decl.ValueFrom.TaskResult, outputs)
	case decl.ValueFrom.Expression != "":
		return r.evaluateResultExpression(ctx, workflowRun, workflow, decl.ValueFrom.Expression, outputs, phases, resolved)
	default:
		// Rejected by the CRD's XValidation, so reaching here means an object predates the
		// schema or was written past it. Treat it as unresolvable, not as a panic.
		return "", errors.New("no value source is set")
	}
}

// resolveTaskResult reads one task's output. The two failure modes are reported apart
// because they point at different mistakes: a task that never ran, versus a task that ran
// and did not emit what the author expected.
func resolveTaskResult(ref *openchoreodevv1alpha1.TaskResultRef, outputs RunOutputs) (string, error) {
	taskOutputs, ok := outputs[ref.Task]
	if !ok {
		return "", fmt.Errorf("task %q produced no outputs (known tasks: %s)", ref.Task, knownTasks(outputs))
	}
	value, ok := taskOutputs[ref.Result]
	if !ok {
		return "", fmt.Errorf("task %q has no output named %q (its outputs: %s)",
			ref.Task, ref.Result, sortedKeys(taskOutputs))
	}
	return value, nil
}

// evaluateResultExpression runs one expression on the reconciler's existing template engine,
// under the same cost budget and render deadline as every other render in this reconcile.
// There is deliberately no second expression dialect here: an author who can write a
// runTemplate can write a result.
func (r *Reconciler) evaluateResultExpression(
	ctx context.Context,
	workflowRun *openchoreodevv1alpha1.WorkflowRun,
	workflow *openchoreodevv1alpha1.Workflow,
	expression string,
	outputs RunOutputs,
	phases map[string]string,
	resolved map[string]string,
) (string, error) {
	inputs, err := r.buildResultCELContext(workflowRun, workflow, outputs, phases, resolved)
	if err != nil {
		return "", err
	}

	ctx, cancel := template.WithRenderTimeout(ctx, r.RenderTimeout)
	defer cancel()

	rendered, err := r.resultEngine().Render(ctx, expression, inputs)
	if err != nil {
		return "", fmt.Errorf("expression did not evaluate: %w", err)
	}

	return resultValueToString(rendered)
}

// buildResultCELContext assembles the inputs a result expression is evaluated against.
// buildResultCELContext assembles the inputs a result expression is evaluated against.
//
// The base context comes from the same pipeline call the runTemplate render uses, so
// ${parameters.*} carries the schema defaults and ${metadata.namespace} exists. Building it
// by hand here meant an expression copied from a runTemplate - the idiom every shipped
// sample uses - failed on a parameter the developer had left to its default.
func (r *Reconciler) buildResultCELContext(
	workflowRun *openchoreodevv1alpha1.WorkflowRun,
	workflow *openchoreodevv1alpha1.Workflow,
	outputs RunOutputs,
	phases map[string]string,
	resolved map[string]string,
) (map[string]any, error) {
	tasks := make(map[string]any, len(phases))
	for name, phase := range phases {
		results := map[string]string{}
		if taskOutputs, ok := outputs[name]; ok {
			results = taskOutputs
		}
		tasks[name] = map[string]any{
			"phase":   phase,
			"results": results,
		}
	}
	// A task with outputs but no recorded phase would otherwise be invisible to
	// expressions while being reachable through taskResult. Keep the two sources agreeing.
	for name, taskOutputs := range outputs {
		if _, ok := tasks[name]; ok {
			continue
		}
		tasks[name] = map[string]any{"phase": "", "results": taskOutputs}
	}

	base, err := r.Pipeline.BuildCELContext(&workflowpipeline.RenderInput{
		WorkflowRun: workflowRun,
		Workflow:    workflow,
		Context: workflowpipeline.WorkflowContext{
			NamespaceName:   workflowRun.Namespace,
			WorkflowRunName: workflowRun.Name,
			Labels:          workflowRun.Labels,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("the run's parameters could not be read: %w", err)
	}

	inputs := make(map[string]any, len(base)+2)
	for k, v := range base {
		inputs[k] = v
	}
	inputs["tasks"] = tasks
	inputs["results"] = resolved
	return inputs, nil
}

// resultValueToString renders an evaluated expression as the string status will hold. A
// string is taken as-is; anything else is JSON encoded, so an expression may legitimately
// assemble an object from several task outputs.
func resultValueToString(value any) (string, error) {
	if template.IsOmitted(value) {
		// RemoveOmittedFields prunes the sentinel from inside maps and slices, but a whole
		// expression can evaluate to it. Marshaling that writes "{}" into status, which
		// reads as an empty object rather than "the author asked for nothing here".
		return "", errors.New("expression evaluated to oc_omit()")
	}

	switch v := value.(type) {
	case nil:
		return "", errors.New("expression evaluated to null")
	case string:
		return v, nil
	}

	encoded, err := json.Marshal(template.RemoveOmittedFields(value))
	if err != nil {
		return "", fmt.Errorf("expression evaluated to a value that could not be encoded: %w", err)
	}
	return string(encoded), nil
}

// truncateResultValue caps one value, reporting whether anything was dropped. It cuts on a
// UTF-8 boundary so a truncated value is still valid to store and display.
func truncateResultValue(value string, maxBytes int) (string, bool) {
	if len(value) <= maxBytes {
		return value, false
	}
	return template.TruncateTo(value, maxBytes), true
}

// resultValueMaxBytes and resultsMaxBytes read the configured caps, falling back to the
// defaults.
func (r *Reconciler) resultValueMaxBytes() int {
	if r.ResultLimits.ValueMaxBytes > 0 {
		return r.ResultLimits.ValueMaxBytes
	}
	return defaultResultValueMaxBytes
}

func (r *Reconciler) resultsMaxBytes() int {
	if r.ResultLimits.TotalMaxBytes > 0 {
		return r.ResultLimits.TotalMaxBytes
	}
	return defaultResultsMaxBytes
}

// resultEngine returns the engine result expressions are evaluated on. The pipeline owns
// one for rendering whole templates; results need to evaluate a single expression against a
// different context, so they get an engine configured the same way rather than a different
// dialect.
func (r *Reconciler) resultEngine() *template.Engine {
	if r.ResultEngine != nil {
		return r.ResultEngine
	}
	// Deliberately not memoised onto the reconciler here. Reconciles run concurrently
	// (--max-concurrent-reconciles), so a lazy assignment to a shared field would be a data
	// race. SetupWithManager builds the long-lived engine, which is the path that wants the
	// CEL environment cache; this fallback only serves tests that construct a Reconciler
	// directly, where a per-call engine costs nothing that matters.
	return template.NewEngineWithOptions(template.WithCostLimit(r.CELCostLimit))
}

// eventf records an Event when a recorder is wired. The reconciler runs without one in unit
// tests, where the returned results are what is being asserted on.
func (r *Reconciler) eventf(
	workflowRun *openchoreodevv1alpha1.WorkflowRun,
	eventType, reason, messageFmt string,
	args ...any,
) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(workflowRun, eventType, reason, messageFmt, args...)
}

func knownTasks(outputs RunOutputs) string {
	if len(outputs) == 0 {
		return "none produced outputs"
	}
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func sortedKeys(outputs TaskOutputs) string {
	if len(outputs) == 0 {
		return "none"
	}
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
