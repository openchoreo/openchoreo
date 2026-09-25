// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/releasebinding/gate"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Deployment-hook gate on the ReleaseBinding. Pre-deploy bindings run before
// the RenderedRelease is created or updated; post-deploy bindings run once the
// release reports ResourcesReady. A binding into an Environment without
// spec.hooks has no gate and renders as before.

const (
	// hookRequeueInterval paces polling of running hooks; WorkflowRun status
	// changes also enqueue the binding through the Owns watch.
	hookRequeueInterval = 30 * time.Second
	// defaultHookTimeout bounds a Sync hook when the binding sets no timeout.
	defaultHookTimeout = 30 * time.Minute

	// Event reasons.
	eventHookStarted        = "HookStarted"
	eventHookSucceeded      = "HookSucceeded"
	eventHookFailed         = "HookFailed"
	eventHookTimedOut       = "HookTimedOut"
	eventHookDispatched     = "HookDispatched"
	eventHookDispatchFailed = "HookDispatchFailed"
	eventGatePassed         = "GatePassed"
	eventGateBlocked        = "GateBlocked"
	eventHookFailureIgnored = "HookFailureIgnored"

	// Hook status reasons not covered by the WorkflowRun reasons.
	hookReasonNotApplicable       = gate.SkipReasonNotApplicable
	hookReasonNotEnabled          = gate.SkipReasonNotEnabled
	hookReasonHookNotFound        = gate.ReasonHookNotFound
	hookReasonWorkflowNotFound    = "WorkflowNotFound"
	hookReasonPlaneUnavailable    = "PlaneUnavailable"
	hookReasonParameterResolution = "ParameterResolutionFailed"
	hookReasonDispatched          = "Dispatched"
	hookReasonDispatchFailed      = "DispatchFailed"
	hookReasonTimedOut            = "TimedOut"
	hookReasonRetrying            = "Retrying"
	hookReasonHookSetChanged      = "HookSetChanged"
)

// hookGate is the resolved state of the gate for one reconcile, computed by
// reconcilePreDeployHooks and reused by reconcilePostDeployHooks.
type hookGate struct {
	set     gate.EffectiveHookSet
	key     string
	hash    string
	trigger openchoreov1alpha1.DeploymentTrigger
	inputs  map[string]any
	input   gate.ContextInput
}

// gateOutcome tells reconcileRelease whether rendering may proceed.
type gateOutcome struct {
	blocked bool
	result  ctrl.Result
	gate    *hookGate
}

// hookEngine returns the shared engine for hook parameter expressions. It is
// built on first use, exactly once even under concurrent reconciles.
func (r *Reconciler) hookEngine() *template.Engine {
	r.hookEngineOnce.Do(func() {
		if r.hookTemplateEngine == nil {
			r.hookTemplateEngine = template.NewEngineWithOptions(template.WithCostLimit(r.CELCostLimit))
		}
	})
	return r.hookTemplateEngine
}

func (r *Reconciler) recordEvent(rb *openchoreov1alpha1.ReleaseBinding, eventType, reason, message string) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Event(rb, eventType, reason, message)
}

// clearGate removes every trace of the gate when no hook binds the environment.
func clearGate(rb *openchoreov1alpha1.ReleaseBinding) {
	rb.Status.Gate = nil
	meta.RemoveStatusCondition(&rb.Status.Conditions, string(ConditionPreDeployHooksPassed))
	meta.RemoveStatusCondition(&rb.Status.Conditions, string(ConditionPostDeployHooksPassed))
}

// reconcilePreDeployHooks evaluates the pre-deploy gate. It returns
// blocked=true when the RenderedRelease must not be created or updated yet.
//
// nolint:gocyclo // Gate state machine; the branches mirror the documented phases.
func (r *Reconciler) reconcilePreDeployHooks(ctx context.Context, rb *openchoreov1alpha1.ReleaseBinding,
	cr *openchoreov1alpha1.ComponentRelease, env *openchoreov1alpha1.Environment,
	component *openchoreov1alpha1.Component, project *openchoreov1alpha1.Project) (gateOutcome, error) {
	logger := log.FromContext(ctx)

	if env == nil || env.Spec.Hooks == nil {
		clearGate(rb)
		return gateOutcome{}, nil
	}

	subject := gate.Subject{
		ComponentTypeKind: cr.Spec.ComponentType.Kind,
		ComponentTypeName: cr.Spec.ComponentType.Name,
	}
	set, err := gate.Resolve(ctx, r.Client, env, subject)
	if err != nil {
		return gateOutcome{}, err
	}
	if set.Empty() {
		clearGate(rb)
		return gateOutcome{}, nil
	}

	if rb.Status.Gate == nil {
		rb.Status.Gate = &openchoreov1alpha1.DeploymentGateStatus{}
	}
	g := rb.Status.Gate
	key, hash, err := gate.Key(cr.Name, g.Sequence, set)
	if err != nil {
		return gateOutcome{}, fmt.Errorf("computing gate key: %w", err)
	}
	now := metav1.Now()

	if g.Key != key {
		carry := gate.CarryForward(g, cr.Name, hash)
		if !carry {
			// A new deployment attempt. Every release change runs the hooks,
			// including a return to a release that passed before, so the
			// attempt gets its own key and does not reuse that pass's runs.
			g.Sequence++
			if key, hash, err = gate.Key(cr.Name, g.Sequence, set); err != nil {
				return gateOutcome{}, fmt.Errorf("computing gate key: %w", err)
			}
		}
		g.Key = key
		g.HookSetHash = hash
		g.PreDeploy = nil
		g.PostDeploy = nil
		meta.RemoveStatusCondition(&rb.Status.Conditions, string(ConditionPostDeployHooksPassed))
		if carry {
			// Only the hook set changed: the release stays deployed and the
			// new set applies from the next release change.
			gate.RecordPass(g, key, cr.Name, hash, now)
			g.PostDeployKey = key
			// The release did not change, so its post-deploy hooks do not run
			// either. Record them as Skipped: with no entry, the post-deploy pass
			// would start each binding as new.
			for _, h := range set.PostDeploy {
				entry := findOrInitHookStatus(&g.PostDeploy, h)
				setHookPhase(entry, openchoreov1alpha1.HookPhaseSkipped, hookReasonHookSetChanged,
					"Hook set changed; the new set applies from the next release change", &now)
			}
			controller.MarkTrueCondition(rb, ConditionPreDeployHooksPassed, ReasonHooksPassed,
				"Hook set changed; the new set applies from the next release change")
		} else {
			g.PostDeployKey = ""
		}
	}
	trigger := gate.CurrentTrigger(g, key, cr.Name, hash)

	hg := &hookGate{set: set, key: key, hash: hash, trigger: trigger}
	hg.input = gate.ContextInput{Release: cr, Component: component, Project: project, Environment: env, Trigger: trigger}
	hg.inputs = gate.BuildContext(hg.input)

	if err := r.applyHookRetry(ctx, rb, hg); err != nil {
		return gateOutcome{}, err
	}

	if g.PassedKey == key {
		if applicable(set.PreDeploy) > 0 && meta.FindStatusCondition(rb.Status.Conditions, string(ConditionPreDeployHooksPassed)) == nil {
			r.markHooksPassed(rb, ConditionPreDeployHooksPassed, gate.PhasePreDeploy,
				ignoredFailures(set.PreDeploy, g.PreDeploy, gate.PhasePreDeploy))
		}
		return gateOutcome{gate: hg}, nil
	}

	// Run the pre-deploy bindings.
	var running, planeUnavailable bool
	var blockReason controller.ConditionReason
	var blockMsg string
	for _, h := range set.PreDeploy {
		entry := findOrInitHookStatus(&g.PreDeploy, h)
		if err := r.advanceHook(ctx, rb, hg, h, entry, now); err != nil {
			return gateOutcome{}, err
		}
		switch entry.Phase {
		case openchoreov1alpha1.HookPhasePending, openchoreov1alpha1.HookPhaseRunning:
			running = true
		case openchoreov1alpha1.HookPhaseFailed, openchoreov1alpha1.HookPhaseTimedOut, openchoreov1alpha1.HookPhasePlaneUnavailable:
			if entry.Phase == openchoreov1alpha1.HookPhasePlaneUnavailable && gate.Mode(h.Binding) == openchoreov1alpha1.HookModeSync {
				planeUnavailable = true
			}
			if gate.Mode(h.Binding) != openchoreov1alpha1.HookModeSync ||
				gate.OnFailure(h.Binding, gate.PhasePreDeploy) != openchoreov1alpha1.HookFailurePolicyBlock {
				continue
			}
			if blockReason == "" {
				blockReason = blockReasonFor(entry.Phase)
				blockMsg = fmt.Sprintf("Pre-deploy hook %q %s: %s", entry.Name, strings.ToLower(string(entry.Phase)), entry.Message)
			}
		}
	}

	if blockReason != "" {
		if controller.MarkFalseCondition(rb, ConditionPreDeployHooksPassed, blockReason, blockMsg) {
			r.recordEvent(rb, corev1.EventTypeWarning, eventGateBlocked, blockMsg)
		}
		logger.Info("Deployment blocked by pre-deploy hook", "reason", blockReason, "message", blockMsg)
		// Running hooks alongside a blocker still need polling, and so does a
		// missing plane: nothing watches planes, and the hook starts once it exists.
		if running || planeUnavailable {
			return gateOutcome{blocked: true, result: ctrl.Result{RequeueAfter: hookRequeueInterval}, gate: hg}, nil
		}
		return gateOutcome{blocked: true, gate: hg}, nil
	}
	if running {
		controller.MarkFalseCondition(rb, ConditionPreDeployHooksPassed, ReasonHooksRunning,
			fmt.Sprintf("Waiting for pre-deploy hooks: %s", strings.Join(runningNames(g.PreDeploy), ", ")))
		return gateOutcome{blocked: true, result: ctrl.Result{RequeueAfter: hookRequeueInterval}, gate: hg}, nil
	}

	gate.RecordPass(g, key, cr.Name, hash, now)
	if applicable(set.PreDeploy) > 0 {
		r.markHooksPassed(rb, ConditionPreDeployHooksPassed, gate.PhasePreDeploy,
			ignoredFailures(set.PreDeploy, g.PreDeploy, gate.PhasePreDeploy))
		r.recordEvent(rb, corev1.EventTypeNormal, eventGatePassed, fmt.Sprintf("Pre-deploy hooks passed for release %q", cr.Name))
	}
	return gateOutcome{gate: hg}, nil
}

// reconcilePostDeployHooks runs the post-deploy bindings once per key after
// ResourcesReady=True and keeps observing them on later reconciles.
func (r *Reconciler) reconcilePostDeployHooks(ctx context.Context, rb *openchoreov1alpha1.ReleaseBinding, hg *hookGate) (ctrl.Result, error) {
	if hg == nil || rb.Status.Gate == nil || len(hg.set.PostDeploy) == 0 {
		return ctrl.Result{}, nil
	}
	g := rb.Status.Gate
	resourcesReady := meta.IsStatusConditionTrue(rb.Status.Conditions, string(ConditionResourcesReady))
	if g.PostDeployKey != hg.key {
		if !resourcesReady {
			return ctrl.Result{}, nil
		}
		g.PostDeployKey = hg.key
		g.PostDeploy = nil
	}

	hg.input.Endpoints = rb.Status.Endpoints
	hg.inputs = gate.BuildContext(hg.input)
	now := metav1.Now()

	var running, planeUnavailable, alerted bool
	var alertMsg string
	for _, h := range hg.set.PostDeploy {
		entry := findOrInitHookStatus(&g.PostDeploy, h)
		if err := r.advanceHook(ctx, rb, hg, h, entry, now); err != nil {
			return ctrl.Result{}, err
		}
		switch entry.Phase {
		case openchoreov1alpha1.HookPhasePending, openchoreov1alpha1.HookPhaseRunning:
			running = true
		case openchoreov1alpha1.HookPhaseFailed, openchoreov1alpha1.HookPhaseTimedOut, openchoreov1alpha1.HookPhasePlaneUnavailable:
			if entry.Phase == openchoreov1alpha1.HookPhasePlaneUnavailable && gate.Mode(h.Binding) == openchoreov1alpha1.HookModeSync {
				planeUnavailable = true
			}
			if gate.Mode(h.Binding) == openchoreov1alpha1.HookModeSync &&
				gate.OnFailure(h.Binding, gate.PhasePostDeploy) == openchoreov1alpha1.HookFailurePolicyAlert && !alerted {
				alerted = true
				alertMsg = fmt.Sprintf("Post-deploy hook %q %s: %s", entry.Name, strings.ToLower(string(entry.Phase)), entry.Message)
			}
		}
	}

	switch {
	case alerted && rb.Annotations[labels.AnnotationKeyGateAcknowledged] != hg.key:
		controller.MarkFalseCondition(rb, ConditionPostDeployHooksPassed, ReasonPostDeployHookFailed, alertMsg)
	case alerted:
		controller.MarkTrueCondition(rb, ConditionPostDeployHooksPassed, ReasonPostDeployHooksAcknowledged,
			"Post-deploy hook failure acknowledged")
	case running:
		controller.MarkFalseCondition(rb, ConditionPostDeployHooksPassed, ReasonHooksRunning,
			fmt.Sprintf("Waiting for post-deploy hooks: %s", strings.Join(runningNames(g.PostDeploy), ", ")))
	default:
		r.markHooksPassed(rb, ConditionPostDeployHooksPassed, gate.PhasePostDeploy,
			ignoredFailures(hg.set.PostDeploy, g.PostDeploy, gate.PhasePostDeploy))
	}
	// As in pre-deploy: nothing watches planes, so a Sync hook waiting on a
	// missing plane is only retried by the requeue.
	if running || planeUnavailable {
		return ctrl.Result{RequeueAfter: hookRequeueInterval}, nil
	}
	return ctrl.Result{}, nil
}

// ignoredFailures names the Sync bindings, in binding order, that ended in a
// failed phase but whose onFailure policy is Ignore, so the deployment went on.
func ignoredFailures(hooks []gate.ResolvedHook, statuses []openchoreov1alpha1.DeploymentHookStatus, phase gate.Phase) []string {
	phases := make(map[string]openchoreov1alpha1.HookPhase, len(statuses))
	for _, st := range statuses {
		phases[st.Name] = st.Phase
	}
	var names []string
	for _, h := range hooks {
		if gate.Mode(h.Binding) != openchoreov1alpha1.HookModeSync ||
			gate.OnFailure(h.Binding, phase) != openchoreov1alpha1.HookFailurePolicyIgnore {
			continue
		}
		switch phases[h.Binding.Name] {
		case openchoreov1alpha1.HookPhaseFailed, openchoreov1alpha1.HookPhaseTimedOut, openchoreov1alpha1.HookPhasePlaneUnavailable:
			names = append(names, h.Binding.Name)
		}
	}
	return names
}

// markHooksPassed sets the phase's passed condition. When a failure was
// ignored the condition stays True (Ready is unaffected) but says so, and a
// HookFailureIgnored warning is recorded once per hook when the condition first
// takes that form.
func (r *Reconciler) markHooksPassed(rb *openchoreov1alpha1.ReleaseBinding, ct controller.ConditionType,
	phase gate.Phase, ignored []string) {
	label, all := "Pre-deploy", "All pre-deploy hooks passed"
	if phase == gate.PhasePostDeploy {
		label, all = "Post-deploy", "All post-deploy hooks completed"
	}
	if len(ignored) == 0 {
		controller.MarkTrueCondition(rb, ct, ReasonHooksPassed, all)
		return
	}
	msg := fmt.Sprintf("%s hooks passed; ignored failures: %s", label, strings.Join(ignored, ", "))
	if controller.MarkTrueCondition(rb, ct, ReasonHooksPassedWithIgnoredFailures, msg) {
		for _, name := range ignored {
			r.recordEvent(rb, corev1.EventTypeWarning, eventHookFailureIgnored,
				fmt.Sprintf("%s hook %q failed; its onFailure policy is Ignore, so the deployment proceeds", label, name))
		}
	}
}

// advanceHook moves one binding's status entry through its state machine for
// the current key: skip, dispatch (Async), or start/observe/retry (Sync).
//
// nolint:gocyclo // Per-hook state machine; the cases are the documented phases.
func (r *Reconciler) advanceHook(ctx context.Context, rb *openchoreov1alpha1.ReleaseBinding, hg *hookGate,
	h gate.ResolvedHook, entry *openchoreov1alpha1.DeploymentHookStatus, now metav1.Time) error {
	switch entry.Phase {
	case openchoreov1alpha1.HookPhaseSucceeded, openchoreov1alpha1.HookPhaseFailed, openchoreov1alpha1.HookPhaseTimedOut,
		openchoreov1alpha1.HookPhaseSkipped, openchoreov1alpha1.HookPhaseDispatched, openchoreov1alpha1.HookPhaseDispatchFailed:
		return nil
	}

	if !h.Applicable() {
		setHookPhase(entry, openchoreov1alpha1.HookPhaseSkipped, h.SkipReason, "Binding does not apply to this component", &now)
		return nil
	}
	if h.NotFound || h.Hook == nil {
		setHookPhase(entry, openchoreov1alpha1.HookPhaseFailed, hookReasonHookNotFound,
			fmt.Sprintf("%s %q not found", h.Binding.HookRef.Kind, h.Binding.HookRef.Name), &now)
		return nil
	}

	params, err := gate.ResolveParameters(ctx, r.hookEngine(), h.Hook, h.Binding, hg.inputs)
	if err != nil {
		setHookPhase(entry, openchoreov1alpha1.HookPhaseFailed, hookReasonParameterResolution, err.Error(), &now)
		r.recordEvent(rb, corev1.EventTypeWarning, eventHookFailed, fmt.Sprintf("Hook %q: %v", h.Binding.Name, err))
		return nil
	}

	if gate.Mode(h.Binding) == openchoreov1alpha1.HookModeAsync {
		// A retried Async hook carries its attempt on the entry, so it is
		// dispatched under a new run name rather than the retried run's.
		attempt := entry.Attempt
		if attempt < 1 {
			attempt = 1
		}
		run, _, err := r.ensureHookRun(ctx, rb, h, hg.key, attempt, params)
		if err != nil {
			if isTransientHookError(err) {
				return err
			}
			setHookPhase(entry, openchoreov1alpha1.HookPhaseDispatchFailed, hookReasonDispatchFailed, err.Error(), &now)
			r.recordEvent(rb, corev1.EventTypeWarning, eventHookDispatchFailed, fmt.Sprintf("Hook %q: %v", h.Binding.Name, err))
			return nil
		}
		entry.WorkflowRunRef = run.Name
		entry.Attempt = attempt
		entry.StartedAt = &now
		setHookPhase(entry, openchoreov1alpha1.HookPhaseDispatched, hookReasonDispatched, "WorkflowRun dispatched; not awaited", &now)
		r.recordEvent(rb, corev1.EventTypeNormal, eventHookDispatched, fmt.Sprintf("Hook %q dispatched as WorkflowRun %q", h.Binding.Name, run.Name))
		return nil
	}

	// Sync: start when nothing is running, then observe.
	if entry.Attempt == 0 {
		entry.Attempt = 1
	}
	if entry.WorkflowRunRef == "" {
		run, created, err := r.ensureHookRun(ctx, rb, h, hg.key, entry.Attempt, params)
		if err != nil {
			switch {
			case errors.Is(err, errHookPlaneUnavailable):
				setHookPhase(entry, openchoreov1alpha1.HookPhasePlaneUnavailable, hookReasonPlaneUnavailable, err.Error(), nil)
				return nil
			case errors.Is(err, errHookWorkflowNotFound):
				setHookPhase(entry, openchoreov1alpha1.HookPhaseFailed, hookReasonWorkflowNotFound, err.Error(), &now)
				return nil
			}
			return err
		}
		entry.WorkflowRunRef = run.Name
		entry.StartedAt = &now
		entry.FinishedAt = nil
		setHookPhase(entry, openchoreov1alpha1.HookPhaseRunning, "WorkflowRunning", "Workflow is starting", nil)
		if created {
			r.recordEvent(rb, corev1.EventTypeNormal, eventHookStarted,
				fmt.Sprintf("Hook %q started as WorkflowRun %q (attempt %d)", h.Binding.Name, run.Name, entry.Attempt))
		}
		return nil
	}

	run := &openchoreov1alpha1.WorkflowRun{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: rb.Namespace, Name: entry.WorkflowRunRef}, run); err != nil {
		if apierrors.IsNotFound(err) {
			// The run vanished (TTL, manual delete): start it again on the next pass.
			entry.WorkflowRunRef = ""
			setHookPhase(entry, openchoreov1alpha1.HookPhasePending, "WorkflowRunMissing", "WorkflowRun was deleted; restarting", nil)
			return nil
		}
		return fmt.Errorf("getting WorkflowRun %q: %w", entry.WorkflowRunRef, err)
	}

	phase, reason, message := observeHookRun(run)
	switch phase {
	case openchoreov1alpha1.HookPhaseSucceeded:
		setHookPhase(entry, phase, reason, message, &now)
		r.recordEvent(rb, corev1.EventTypeNormal, eventHookSucceeded, fmt.Sprintf("Hook %q succeeded", h.Binding.Name))
	case openchoreov1alpha1.HookPhaseFailed:
		if r.retryHook(rb, h, entry, message) {
			return nil
		}
		setHookPhase(entry, phase, reason, message, &now)
		r.recordEvent(rb, corev1.EventTypeWarning, eventHookFailed, fmt.Sprintf("Hook %q failed: %s", h.Binding.Name, message))
	case openchoreov1alpha1.HookPhasePlaneUnavailable:
		setHookPhase(entry, phase, reason, message, nil)
	default:
		if timedOut(entry, h.Binding, now.Time) {
			// Stop the run before retrying or giving up, so a timed-out hook does
			// not keep running in the plane next to its retry.
			if err := r.stopHookRun(ctx, rb, entry.WorkflowRunRef); err != nil {
				return err
			}
			msg := fmt.Sprintf("Timed out after %s", hookTimeout(h.Binding))
			if r.retryHook(rb, h, entry, msg) {
				return nil
			}
			setHookPhase(entry, openchoreov1alpha1.HookPhaseTimedOut, hookReasonTimedOut, msg, &now)
			r.recordEvent(rb, corev1.EventTypeWarning, eventHookTimedOut, fmt.Sprintf("Hook %q timed out after %s", h.Binding.Name, hookTimeout(h.Binding)))
			return nil
		}
		setHookPhase(entry, openchoreov1alpha1.HookPhaseRunning, reason, message, nil)
	}
	return nil
}

// stopHookRun stops a hook's WorkflowRun by deleting it; the workflowrun
// controller's finalizer then removes the workflow and its resources from the
// plane. A run that is already gone counts as stopped.
func (r *Reconciler) stopHookRun(ctx context.Context, rb *openchoreov1alpha1.ReleaseBinding, name string) error {
	if name == "" {
		return nil
	}
	run := &openchoreov1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Namespace: rb.Namespace, Name: name}}
	if err := client.IgnoreNotFound(r.Delete(ctx, run)); err != nil {
		return fmt.Errorf("stopping timed-out WorkflowRun %q: %w", name, err)
	}
	return nil
}

// retryHook resets the entry for another attempt when retries remain.
func (r *Reconciler) retryHook(rb *openchoreov1alpha1.ReleaseBinding, h gate.ResolvedHook,
	entry *openchoreov1alpha1.DeploymentHookStatus, why string) bool {
	if entry.Attempt > h.Binding.Retries {
		return false
	}
	r.recordEvent(rb, corev1.EventTypeWarning, eventHookFailed,
		fmt.Sprintf("Hook %q attempt %d failed (%s); retrying", h.Binding.Name, entry.Attempt, why))
	entry.Attempt++
	entry.WorkflowRunRef = ""
	entry.StartedAt = nil
	entry.FinishedAt = nil
	setHookPhase(entry, openchoreov1alpha1.HookPhasePending, hookReasonRetrying,
		fmt.Sprintf("Attempt %d failed: %s", entry.Attempt-1, why), nil)
	return true
}

// applyHookRetry honors the openchoreo.dev/hook-retry annotation in two steps,
// so a failed status write can never lose or half-apply a retry:
//
//  1. The named hook's status entry is reset to the next attempt and the
//     annotation is rewritten to "<phase>/<name>#<attempt>". Nothing else
//     changes; the reset reaches the API with the reconcile's status update.
//  2. Once the stored entry has reached that attempt, the reset is known to be
//     saved: the replaced attempt's run is deleted and the annotation removed.
//
// If the status write in step 1 fails, the stored entry is still below the
// target attempt, so the next reconcile applies the reset again. That is safe:
// the new attempt's run name is deterministic and an existing run is reused.
func (r *Reconciler) applyHookRetry(ctx context.Context, rb *openchoreov1alpha1.ReleaseBinding, hg *hookGate) error {
	value, ok := rb.Annotations[labels.AnnotationKeyHookRetry]
	if !ok {
		return nil
	}
	request, targetStr, marked := strings.Cut(value, "#")
	phase, name, _ := strings.Cut(request, "/")
	g := rb.Status.Gate
	var list *[]openchoreov1alpha1.DeploymentHookStatus
	var hooks []gate.ResolvedHook
	switch gate.Phase(phase) {
	case gate.PhasePreDeploy:
		list, hooks = &g.PreDeploy, hg.set.PreDeploy
	case gate.PhasePostDeploy:
		list, hooks = &g.PostDeploy, hg.set.PostDeploy
	}
	var e *openchoreov1alpha1.DeploymentHookStatus
	if list != nil {
		for i := range *list {
			if (*list)[i].Name == name {
				e = &(*list)[i]
				break
			}
		}
	}
	if e == nil {
		// Unknown phase or hook: nothing to retry.
		return r.setRetryAnnotation(ctx, rb, "")
	}

	target, err := strconv.ParseInt(targetStr, 10, 32)
	if marked && err == nil && int64(e.Attempt) >= target {
		// Step 2: the reset is stored. Delete the replaced attempt's run and
		// consume the annotation.
		if old := replacedRunName(hooks, name, hg.key, int32(target)-1); old != "" {
			run := &openchoreov1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Namespace: rb.Namespace, Name: old}}
			if err := client.IgnoreNotFound(r.Delete(ctx, run)); err != nil {
				return fmt.Errorf("deleting WorkflowRun %q for retry: %w", old, err)
			}
		}
		log.FromContext(ctx).Info("Hook retry applied", "phase", phase, "hook", name, "attempt", target)
		return r.setRetryAnnotation(ctx, rb, "")
	}

	// Step 1: reset the entry to the next attempt. A new attempt gets its own
	// run name, so it never collides with the run it replaces (which may still
	// be terminating). A re-applied reset keeps the attempt already chosen.
	next := e.Attempt + 1
	if marked && err == nil && target > int64(next) {
		next = int32(target)
	}
	if next < 2 {
		next = 2
	}
	if gate.Phase(phase) == gate.PhasePreDeploy && g.PassedKey == hg.key {
		// Re-open the gate so the hook actually re-runs.
		g.PassedKey = ""
		meta.RemoveStatusCondition(&rb.Status.Conditions, string(ConditionPreDeployHooksPassed))
	}
	*e = openchoreov1alpha1.DeploymentHookStatus{
		Name:    e.Name,
		HookRef: e.HookRef,
		Mode:    e.Mode,
		Phase:   openchoreov1alpha1.HookPhasePending,
		Reason:  hookReasonRetrying,
		Message: "Retry requested",
		Attempt: next,
	}
	log.FromContext(ctx).Info("Hook retry requested", "phase", phase, "hook", name, "attempt", next)
	return r.setRetryAnnotation(ctx, rb, fmt.Sprintf("%s#%d", request, next))
}

// replacedRunName is the run name of the given attempt of a bound hook, or ""
// when the hook's workflow cannot be resolved (then there is no run to delete).
func replacedRunName(hooks []gate.ResolvedHook, binding, key string, attempt int32) string {
	for _, h := range hooks {
		if h.Binding.Name != binding {
			continue
		}
		if h.Hook == nil || h.Hook.WorkflowRef == nil {
			return ""
		}
		return hookRunName(h.Binding.Name, h.Phase, h.Hook.WorkflowRef.Name, key, attempt)
	}
	return ""
}

// setRetryAnnotation sets (or, for "", removes) the hook-retry annotation with
// a merge patch. The response is decoded into rb, which refreshes its
// resourceVersion for the deferred status update but also overwrites the
// in-memory status; restore it (the Gate pointer, which callers hold, is kept).
func (r *Reconciler) setRetryAnnotation(ctx context.Context, rb *openchoreov1alpha1.ReleaseBinding, value string) error {
	base := rb.DeepCopy()
	if value == "" {
		delete(rb.Annotations, labels.AnnotationKeyHookRetry)
	} else {
		rb.Annotations[labels.AnnotationKeyHookRetry] = value
	}
	status := rb.Status
	if err := r.Patch(ctx, rb, client.MergeFrom(base)); err != nil {
		return fmt.Errorf("updating hook-retry annotation: %w", err)
	}
	rb.Status = status
	return nil
}

// ── status helpers ────────────────────────────────────────────────────────────

func findOrInitHookStatus(list *[]openchoreov1alpha1.DeploymentHookStatus, h gate.ResolvedHook) *openchoreov1alpha1.DeploymentHookStatus {
	for i := range *list {
		if (*list)[i].Name == h.Binding.Name {
			return &(*list)[i]
		}
	}
	*list = append(*list, openchoreov1alpha1.DeploymentHookStatus{
		Name:    h.Binding.Name,
		HookRef: h.Binding.HookRef,
		Mode:    gate.Mode(h.Binding),
		Phase:   openchoreov1alpha1.HookPhasePending,
	})
	return &(*list)[len(*list)-1]
}

func setHookPhase(entry *openchoreov1alpha1.DeploymentHookStatus, phase openchoreov1alpha1.HookPhase, reason, message string, finished *metav1.Time) {
	entry.Phase = phase
	entry.Reason = reason
	entry.Message = message
	if finished != nil && entry.FinishedAt == nil {
		entry.FinishedAt = finished
	}
}

func applicable(list []gate.ResolvedHook) int {
	n := 0
	for _, h := range list {
		if h.Applicable() {
			n++
		}
	}
	return n
}

func runningNames(list []openchoreov1alpha1.DeploymentHookStatus) []string {
	var names []string
	for _, e := range list {
		if e.Phase == openchoreov1alpha1.HookPhaseRunning || e.Phase == openchoreov1alpha1.HookPhasePending {
			names = append(names, e.Name)
		}
	}
	return names
}

func blockReasonFor(phase openchoreov1alpha1.HookPhase) controller.ConditionReason {
	switch phase {
	case openchoreov1alpha1.HookPhaseTimedOut:
		return ReasonHookTimedOut
	case openchoreov1alpha1.HookPhasePlaneUnavailable:
		return ReasonPlaneUnavailable
	default:
		return ReasonHookFailed
	}
}

func hookTimeout(b openchoreov1alpha1.HookBinding) time.Duration {
	if b.Timeout == "" {
		return defaultHookTimeout
	}
	d, err := time.ParseDuration(b.Timeout)
	if err != nil || d <= 0 {
		return defaultHookTimeout
	}
	return d
}

func timedOut(entry *openchoreov1alpha1.DeploymentHookStatus, b openchoreov1alpha1.HookBinding, now time.Time) bool {
	if entry.StartedAt == nil {
		return false
	}
	return now.Sub(entry.StartedAt.Time) > hookTimeout(b)
}

// isTransientHookError separates API errors worth returning (and retrying with
// backoff) from resolution failures that belong on the hook's status.
func isTransientHookError(err error) bool {
	return !errors.Is(err, errHookPlaneUnavailable) && !errors.Is(err, errHookWorkflowNotFound)
}

// ── watches ──────────────────────────────────────────────────────────────────

// releaseBindingsForEnvironment enqueues every ReleaseBinding that deploys into
// the environment, so a change to its hooks re-evaluates their gates.
func (r *Reconciler) releaseBindingsForEnvironment(ctx context.Context, obj client.Object) []reconcile.Request {
	env, ok := obj.(*openchoreov1alpha1.Environment)
	if !ok {
		return nil
	}
	return r.releaseBindingsForEnvironments(ctx, []openchoreov1alpha1.Environment{*env})
}

func (r *Reconciler) releaseBindingsForEnvironments(ctx context.Context, envs []openchoreov1alpha1.Environment) []reconcile.Request {
	logger := log.FromContext(ctx)
	byNamespace := map[string]map[string]struct{}{}
	for _, e := range envs {
		if byNamespace[e.Namespace] == nil {
			byNamespace[e.Namespace] = map[string]struct{}{}
		}
		byNamespace[e.Namespace][e.Name] = struct{}{}
	}
	var requests []reconcile.Request
	for ns, names := range byNamespace {
		bindings := &openchoreov1alpha1.ReleaseBindingList{}
		if err := r.List(ctx, bindings, client.InNamespace(ns)); err != nil {
			logger.Error(err, "listing ReleaseBindings for Environment", "namespace", ns)
			continue
		}
		for _, rb := range bindings.Items {
			if _, ok := names[rb.Spec.Environment]; ok {
				requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: rb.Namespace, Name: rb.Name}})
			}
		}
	}
	return requests
}

// releaseBindingsForHook enqueues the bindings of every environment that binds
// the Hook or ClusterHook. Environments are listed and filtered; the set is small.
func (r *Reconciler) releaseBindingsForHook(ctx context.Context, obj client.Object) []reconcile.Request {
	var ref openchoreov1alpha1.HookRef
	var opts []client.ListOption
	switch h := obj.(type) {
	case *openchoreov1alpha1.Hook:
		ref = openchoreov1alpha1.HookRef{Kind: openchoreov1alpha1.HookRefKindHook, Name: h.Name}
		opts = append(opts, client.InNamespace(h.Namespace))
	case *openchoreov1alpha1.ClusterHook:
		ref = openchoreov1alpha1.HookRef{Kind: openchoreov1alpha1.HookRefKindClusterHook, Name: h.Name}
	default:
		return nil
	}
	envs := &openchoreov1alpha1.EnvironmentList{}
	if err := r.List(ctx, envs, opts...); err != nil {
		log.FromContext(ctx).Error(err, "listing Environments for hook", "hook", ref.Name)
		return nil
	}
	var bound []openchoreov1alpha1.Environment
	for _, e := range envs.Items {
		if environmentBindsHook(&e, ref) {
			bound = append(bound, e)
		}
	}
	return r.releaseBindingsForEnvironments(ctx, bound)
}

func environmentBindsHook(e *openchoreov1alpha1.Environment, ref openchoreov1alpha1.HookRef) bool {
	if e.Spec.Hooks == nil {
		return false
	}
	for _, list := range [][]openchoreov1alpha1.HookBinding{e.Spec.Hooks.PreDeploy, e.Spec.Hooks.PostDeploy} {
		for _, b := range list {
			kind := b.HookRef.Kind
			if kind == "" {
				kind = openchoreov1alpha1.HookRefKindHook
			}
			if kind == ref.Kind && b.HookRef.Name == ref.Name {
				return true
			}
		}
	}
	return false
}
