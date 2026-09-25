// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/releasebinding/gate"
	"github.com/openchoreo/openchoreo/internal/controller/workflowrun"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Fake-client tests for the hook gate state machine. The envtest specs in
// controller_hooks_integration_test.go cover the happy paths end to end; these
// pin the branches that are awkward to reach against a real API server:
// API failures, carry-forward, retries, the retry annotation and the watches.

const (
	unitNS  = "ns"
	unitEnv = "prod"
	unitRB  = "rb"
	unitCR  = "cr-1"
)

var errBoom = errors.New("boom")

type hookHarness struct {
	t   *testing.T
	r   *Reconciler
	c   client.Client
	rec *record.FakeRecorder
	rb  *openchoreov1alpha1.ReleaseBinding
	cr  *openchoreov1alpha1.ComponentRelease
	env *openchoreov1alpha1.Environment
}

func unitClusterHook(name string, params ...openchoreov1alpha1.HookParameter) *openchoreov1alpha1.ClusterHook {
	return &openchoreov1alpha1.ClusterHook{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openchoreov1alpha1.HookSpec{
			Type:        openchoreov1alpha1.HookTypeWorkflow,
			WorkflowRef: &openchoreov1alpha1.WorkflowRef{Kind: openchoreov1alpha1.WorkflowRefKindClusterWorkflow, Name: hookWorkflowName},
			Parameters:  params,
		},
	}
}

// newHookHarness builds a reconciler over a fake client holding the binding,
// the environment carrying hooks, the "trivy" ClusterHook, its ClusterWorkflow
// and the default ClusterWorkflowPlane. extra objects are added; funcs
// intercept client calls.
func newHookHarness(t *testing.T, hooks *openchoreov1alpha1.HookSet, funcs *interceptor.Funcs, extra ...client.Object) *hookHarness {
	t.Helper()
	s := watchTestScheme(t)
	rb := &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Namespace: unitNS, Name: unitRB, UID: "rb-uid"},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "proj", ComponentName: "comp"},
			Environment: unitEnv,
			ReleaseName: unitCR,
		},
	}
	env := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Namespace: unitNS, Name: unitEnv},
		Spec:       openchoreov1alpha1.EnvironmentSpec{Hooks: hooks},
	}
	cr := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{Namespace: unitNS, Name: unitCR},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{ProjectName: "proj", ComponentName: "comp"},
			ComponentType: openchoreov1alpha1.ComponentReleaseComponentType{
				Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType, Name: "deployment/service",
			},
		},
	}
	objs := append([]client.Object{rb.DeepCopy(), env.DeepCopy(), unitClusterHook(hookName),
		clusterWorkflowFixture(), clusterWorkflowPlaneFixture()}, extra...)
	b := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...)
	if funcs != nil {
		b = b.WithInterceptorFuncs(*funcs)
	}
	c := b.Build()
	rec := record.NewFakeRecorder(100)
	return &hookHarness{
		t:   t,
		r:   &Reconciler{Client: c, Scheme: s, Recorder: rec},
		c:   c,
		rec: rec,
		rb:  rb,
		cr:  cr,
		env: env,
	}
}

func (h *hookHarness) pre() gateOutcome {
	h.t.Helper()
	out, err := h.r.reconcilePreDeployHooks(context.Background(), h.rb, h.cr, h.env, nil, nil)
	require.NoError(h.t, err)
	return out
}

// events drains the recorder; each element is "<type> <reason> <message>".
func (h *hookHarness) events() []string {
	var out []string
	for {
		select {
		case e := <-h.rec.Events:
			out = append(out, e)
		default:
			return out
		}
	}
}

func (h *hookHarness) runs() []openchoreov1alpha1.WorkflowRun {
	h.t.Helper()
	list := &openchoreov1alpha1.WorkflowRunList{}
	require.NoError(h.t, h.c.List(context.Background(), list, client.InNamespace(unitNS)))
	return list.Items
}

// setRunConditions updates a hook run's status the way the workflowrun
// controller would.
func (h *hookHarness) setRunConditions(name string, conds ...metav1.Condition) {
	h.t.Helper()
	run := &openchoreov1alpha1.WorkflowRun{}
	require.NoError(h.t, h.c.Get(context.Background(), types.NamespacedName{Namespace: unitNS, Name: name}, run))
	for _, c := range conds {
		apimeta.SetStatusCondition(&run.Status.Conditions, c)
	}
	require.NoError(h.t, h.c.Update(context.Background(), run))
}

func (h *hookHarness) failRun(name string) {
	h.t.Helper()
	h.setRunConditions(name, metav1.Condition{
		Type: string(workflowrun.ConditionWorkflowFailed), Status: metav1.ConditionTrue,
		Reason: string(workflowrun.ReasonWorkflowFailed), Message: "scan failed",
	})
}

func (h *hookHarness) succeedRun(name string) {
	h.t.Helper()
	h.setRunConditions(name, metav1.Condition{
		Type: string(workflowrun.ConditionWorkflowSucceeded), Status: metav1.ConditionTrue,
		Reason: string(workflowrun.ReasonWorkflowSucceeded), Message: "ok",
	})
}

// setRetryAnnotation writes the hook-retry annotation on the stored binding and
// mirrors it in memory with the stored resourceVersion, as the reconciler would
// see it.
func (h *hookHarness) setRetryAnnotation(value string) {
	h.t.Helper()
	stored := &openchoreov1alpha1.ReleaseBinding{}
	require.NoError(h.t, h.c.Get(context.Background(), types.NamespacedName{Namespace: unitNS, Name: unitRB}, stored))
	if stored.Annotations == nil {
		stored.Annotations = map[string]string{}
	}
	stored.Annotations[labels.AnnotationKeyHookRetry] = value
	require.NoError(h.t, h.c.Update(context.Background(), stored))
	h.rb.Annotations = stored.Annotations
	h.rb.ResourceVersion = stored.ResourceVersion
}

func preHooks(bindings ...openchoreov1alpha1.HookBinding) *openchoreov1alpha1.HookSet {
	return &openchoreov1alpha1.HookSet{PreDeploy: bindings}
}

func syncBlock(name string) openchoreov1alpha1.HookBinding {
	return hookBinding(name, openchoreov1alpha1.HookModeSync, openchoreov1alpha1.HookFailurePolicyBlock)
}

func preCond(rb *openchoreov1alpha1.ReleaseBinding) *metav1.Condition {
	return apimeta.FindStatusCondition(rb.Status.Conditions, string(ConditionPreDeployHooksPassed))
}

func postCond(rb *openchoreov1alpha1.ReleaseBinding) *metav1.Condition {
	return apimeta.FindStatusCondition(rb.Status.Conditions, string(ConditionPostDeployHooksPassed))
}

func countEvents(events []string, reason string) int {
	n := 0
	for _, e := range events {
		if strings.Contains(e, " "+reason+" ") {
			n++
		}
	}
	return n
}

func getErrFor(match func(client.Object) bool) *interceptor.Funcs {
	return &interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if match(obj) {
				return errBoom
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}
}

func isRun(obj client.Object) bool {
	_, ok := obj.(*openchoreov1alpha1.WorkflowRun)
	return ok
}

// ─── Gate entry: no hooks, resolve failures ──────────────────────────────────

// Removing every hook from the environment (or losing the environment) must
// wipe the gate: a stale Failed condition would otherwise keep Ready False
// forever for a binding that no longer has anything to wait for.
func TestPreDeployHooks_NoBoundHooksClearsStaleGate(t *testing.T) {
	cases := []struct {
		name string
		env  func(*openchoreov1alpha1.Environment) *openchoreov1alpha1.Environment
	}{
		{"environment not found", func(*openchoreov1alpha1.Environment) *openchoreov1alpha1.Environment { return nil }},
		{"hooks removed", func(e *openchoreov1alpha1.Environment) *openchoreov1alpha1.Environment {
			e.Spec.Hooks = nil
			return e
		}},
		{"empty hook set", func(e *openchoreov1alpha1.Environment) *openchoreov1alpha1.Environment {
			e.Spec.Hooks = &openchoreov1alpha1.HookSet{}
			return e
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHookHarness(t, preHooks(syncBlock("scan")), nil)
			h.env = tc.env(h.env)
			h.rb.Status.Gate = &openchoreov1alpha1.DeploymentGateStatus{Key: "old"}
			h.rb.Status.Conditions = []metav1.Condition{
				{Type: string(ConditionPreDeployHooksPassed), Status: metav1.ConditionFalse, Reason: string(ReasonHookFailed)},
				{Type: string(ConditionPostDeployHooksPassed), Status: metav1.ConditionFalse, Reason: string(ReasonPostDeployHookFailed)},
			}
			out := h.pre()
			assert.False(t, out.blocked)
			assert.Nil(t, out.gate)
			assert.Nil(t, h.rb.Status.Gate)
			assert.Nil(t, preCond(h.rb))
			assert.Nil(t, postCond(h.rb))
			assert.Empty(t, h.runs())
		})
	}
}

// A transient API failure while resolving hooks is not "no hooks": the gate
// must return the error (requeue with backoff) and leave the recorded gate
// intact, otherwise an apiserver blip would wave a release past its scan.
func TestPreDeployHooks_ResolveErrorKeepsGate(t *testing.T) {
	h := newHookHarness(t, preHooks(syncBlock("scan")), getErrFor(func(o client.Object) bool {
		_, ok := o.(*openchoreov1alpha1.ClusterHook)
		return ok
	}))
	h.rb.Status.Gate = &openchoreov1alpha1.DeploymentGateStatus{Key: "old"}
	_, err := h.r.reconcilePreDeployHooks(context.Background(), h.rb, h.cr, h.env, nil, nil)
	require.ErrorIs(t, err, errBoom)
	require.NotNil(t, h.rb.Status.Gate)
	assert.Equal(t, "old", h.rb.Status.Gate.Key)
}

// ─── Pass bookkeeping ────────────────────────────────────────────────────────

// Editing the bound hook set while the current release is deployed must not
// re-open the gate: the new set applies from the next release change. The gate
// key moves (so the next release sees the new set) but no new hook runs.
func TestPreDeployHooks_HookSetEditCarriesPassForward(t *testing.T) {
	h := newHookHarness(t, &openchoreov1alpha1.HookSet{
		PreDeploy:  []openchoreov1alpha1.HookBinding{hookBinding("notify", openchoreov1alpha1.HookModeAsync, "")},
		PostDeploy: []openchoreov1alpha1.HookBinding{hookBinding("smoke", openchoreov1alpha1.HookModeSync, "")},
	}, nil)
	out := h.pre()
	require.False(t, out.blocked, "an Async hook never gates")
	g := h.rb.Status.Gate
	require.Equal(t, g.Key, g.PassedKey)
	firstKey, seq := g.Key, g.Sequence
	apimeta.SetStatusCondition(&h.rb.Status.Conditions, metav1.Condition{
		Type: string(ConditionResourcesReady), Status: metav1.ConditionTrue, Reason: "Ready",
	})
	_, err := h.r.reconcilePostDeployHooks(context.Background(), h.rb, out.gate)
	require.NoError(t, err)
	require.Len(t, h.runs(), 2, "the pre-deploy and post-deploy hook each ran once for the release")

	h.env.Spec.Hooks.PreDeploy = append(h.env.Spec.Hooks.PreDeploy, syncBlock("scan"))
	out = h.pre()
	assert.False(t, out.blocked, "a hook edit must not block the deployed release")
	assert.NotEqual(t, firstKey, g.Key)
	assert.Equal(t, g.Key, g.PassedKey)
	assert.Equal(t, g.Key, g.PostDeployKey, "post-deploy hooks must not re-run for a hook edit either")
	assert.Equal(t, seq, g.Sequence, "a carried pass is not a new deployment attempt")
	assert.Len(t, h.runs(), 2, "the newly bound Sync hook must not run until the next release")
	cond := preCond(h.rb)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Contains(t, cond.Message, "Hook set changed")

	// The post-deploy pass for the unchanged release must not start its hooks
	// again: they are recorded as Skipped for the carried key.
	_, err = h.r.reconcilePostDeployHooks(context.Background(), h.rb, out.gate)
	require.NoError(t, err)
	assert.Len(t, h.runs(), 2, "post-deploy hooks must not re-run for a hook edit")
	require.Len(t, g.PostDeploy, 1)
	assert.Equal(t, openchoreov1alpha1.HookPhaseSkipped, g.PostDeploy[0].Phase)
	assert.Equal(t, hookReasonHookSetChanged, g.PostDeploy[0].Reason)
}

// When the gate already passed for this key but the condition was lost (a
// status rewrite), the condition is restored without running the hooks again.
func TestPreDeployHooks_PassedKeyRestoresMissingCondition(t *testing.T) {
	h := newHookHarness(t, preHooks(hookBinding("notify", openchoreov1alpha1.HookModeAsync, "")), nil)
	h.pre()
	require.Len(t, h.runs(), 1)
	apimeta.RemoveStatusCondition(&h.rb.Status.Conditions, string(ConditionPreDeployHooksPassed))
	h.events()

	out := h.pre()
	assert.False(t, out.blocked)
	cond := preCond(h.rb)
	require.NotNil(t, cond)
	assert.Equal(t, string(ReasonHooksPassed), cond.Reason)
	assert.Len(t, h.runs(), 1)
	assert.Zero(t, countEvents(h.events(), eventHookDispatched), "restoring the condition must not re-dispatch")
}

// A binding that does not apply to this component is recorded as Skipped and
// never runs; with nothing applicable there is no condition to report.
func TestPreDeployHooks_NotApplicableBindingIsSkipped(t *testing.T) {
	b := syncBlock("scan")
	b.AppliesTo = []openchoreov1alpha1.HookSubjectSelector{{
		Kind: openchoreov1alpha1.HookSubjectSelectorKindClusterComponentType, Name: "deployment/other",
	}}
	h := newHookHarness(t, preHooks(b), nil)
	out := h.pre()
	assert.False(t, out.blocked)
	require.Len(t, h.rb.Status.Gate.PreDeploy, 1)
	entry := h.rb.Status.Gate.PreDeploy[0]
	assert.Equal(t, openchoreov1alpha1.HookPhaseSkipped, entry.Phase)
	assert.Equal(t, hookReasonNotApplicable, entry.Reason)
	assert.Empty(t, h.runs())
	assert.Nil(t, preCond(h.rb))
	assert.Equal(t, h.rb.Status.Gate.Key, h.rb.Status.Gate.PassedKey)
}

// ─── Blocking ────────────────────────────────────────────────────────────────

// A Block failure stops the release even while other hooks are still running,
// the first failure is named in the condition, the gate keeps polling the
// running hook, and GateBlocked fires once, not on every reconcile.
func TestPreDeployHooks_BlockerAlongsideRunningHookKeepsPolling(t *testing.T) {
	missing := syncBlock("a-missing")
	missing.HookRef.Name = "nope"
	h := newHookHarness(t, preHooks(missing, syncBlock("scan")), nil)

	out := h.pre()
	assert.True(t, out.blocked)
	assert.Equal(t, hookRequeueInterval, out.result.RequeueAfter)
	g := h.rb.Status.Gate
	require.Len(t, g.PreDeploy, 2)
	assert.Equal(t, openchoreov1alpha1.HookPhaseFailed, g.PreDeploy[0].Phase)
	assert.Equal(t, hookReasonHookNotFound, g.PreDeploy[0].Reason)
	assert.Equal(t, openchoreov1alpha1.HookPhaseRunning, g.PreDeploy[1].Phase)
	cond := preCond(h.rb)
	require.NotNil(t, cond)
	assert.Equal(t, string(ReasonHookFailed), cond.Reason)
	assert.Contains(t, cond.Message, `"a-missing"`)
	assert.Equal(t, 1, countEvents(h.events(), eventGateBlocked))

	out = h.pre()
	assert.True(t, out.blocked)
	assert.Zero(t, countEvents(h.events(), eventGateBlocked), "GateBlocked must not repeat for an unchanged block")
}

// A hook whose parameters cannot be resolved has failed: under Block it stops
// the release rather than running the workflow with missing inputs.
func TestPreDeployHooks_ParameterResolutionFailureBlocks(t *testing.T) {
	required := unitClusterHook("strict", openchoreov1alpha1.HookParameter{Name: "token", Required: true})
	b := syncBlock("scan")
	b.HookRef.Name = "strict"
	h := newHookHarness(t, preHooks(b), nil, required)

	out := h.pre()
	assert.True(t, out.blocked)
	entry := h.rb.Status.Gate.PreDeploy[0]
	assert.Equal(t, openchoreov1alpha1.HookPhaseFailed, entry.Phase)
	assert.Equal(t, hookReasonParameterResolution, entry.Reason)
	assert.Contains(t, entry.Message, `"token" is required`)
	assert.Empty(t, h.runs())
	assert.Equal(t, 1, countEvents(h.events(), eventHookFailed))
}

// A Sync hook whose workflow does not exist fails closed with WorkflowNotFound;
// a hook with no workflowRef at all is the same failure.
func TestPreDeployHooks_MissingWorkflowBlocks(t *testing.T) {
	noRef := unitClusterHook("noref")
	noRef.Spec.WorkflowRef = nil
	cases := []struct {
		name  string
		hook  string
		extra []client.Object
		setup func(*hookHarness)
	}{
		{"workflow deleted", hookName, nil, func(h *hookHarness) {
			require.NoError(t, h.c.Delete(context.Background(), clusterWorkflowFixture()))
		}},
		{"no workflowRef", "noref", []client.Object{noRef}, func(*hookHarness) {}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := syncBlock("scan")
			b.HookRef.Name = tc.hook
			h := newHookHarness(t, preHooks(b), nil, tc.extra...)
			tc.setup(h)
			out := h.pre()
			assert.True(t, out.blocked)
			assert.Zero(t, out.result.RequeueAfter, "a terminal failure waits for an operator")
			entry := h.rb.Status.Gate.PreDeploy[0]
			assert.Equal(t, openchoreov1alpha1.HookPhaseFailed, entry.Phase)
			assert.Equal(t, hookReasonWorkflowNotFound, entry.Reason)
			assert.Empty(t, h.runs())
		})
	}
}

// A run whose plane cannot be resolved (reported by the workflowrun controller)
// blocks with PlaneUnavailable rather than reading as a hook failure.
func TestPreDeployHooks_RunReportsPlaneUnavailable(t *testing.T) {
	h := newHookHarness(t, preHooks(syncBlock("scan")), nil)
	h.pre()
	run := h.rb.Status.Gate.PreDeploy[0].WorkflowRunRef
	h.setRunConditions(run, metav1.Condition{
		Type: string(workflowrun.ConditionWorkflowCompleted), Status: metav1.ConditionFalse,
		Reason: string(workflowrun.ReasonWorkflowPlaneNotFound), Message: "no plane",
	})
	out := h.pre()
	assert.True(t, out.blocked)
	assert.Equal(t, hookRequeueInterval, out.result.RequeueAfter, "nothing watches planes; only the requeue notices it return")
	assert.Equal(t, openchoreov1alpha1.HookPhasePlaneUnavailable, h.rb.Status.Gate.PreDeploy[0].Phase)
	assert.Equal(t, string(ReasonPlaneUnavailable), preCond(h.rb).Reason)
}

// ─── Sync observation, retries, timeouts ─────────────────────────────────────

// While the run is in progress the hook stays Running, the gate keeps polling,
// and no second run is created.
func TestPreDeployHooks_RunningHookIsObservedNotRestarted(t *testing.T) {
	h := newHookHarness(t, preHooks(syncBlock("scan")), nil)
	h.pre()
	h.setRunConditions(h.rb.Status.Gate.PreDeploy[0].WorkflowRunRef, metav1.Condition{
		Type: string(workflowrun.ConditionWorkflowCompleted), Status: metav1.ConditionFalse,
		Reason: string(workflowrun.ReasonWorkflowRunning), Message: "",
	})
	out := h.pre()
	assert.True(t, out.blocked)
	assert.Equal(t, hookRequeueInterval, out.result.RequeueAfter)
	assert.Equal(t, openchoreov1alpha1.HookPhaseRunning, h.rb.Status.Gate.PreDeploy[0].Phase)
	assert.Contains(t, preCond(h.rb).Message, "scan")
	assert.Len(t, h.runs(), 1)
}

// A run deleted out from under the gate (TTL, manual cleanup) is restarted
// rather than leaving the hook stuck Running forever.
func TestPreDeployHooks_DeletedRunIsRestarted(t *testing.T) {
	h := newHookHarness(t, preHooks(syncBlock("scan")), nil)
	h.pre()
	ref := h.rb.Status.Gate.PreDeploy[0].WorkflowRunRef
	require.NoError(t, h.c.Delete(context.Background(), &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{Namespace: unitNS, Name: ref}}))

	out := h.pre()
	assert.True(t, out.blocked)
	entry := h.rb.Status.Gate.PreDeploy[0]
	assert.Equal(t, openchoreov1alpha1.HookPhasePending, entry.Phase)
	assert.Equal(t, "WorkflowRunMissing", entry.Reason)
	assert.Empty(t, entry.WorkflowRunRef)

	h.pre()
	assert.Equal(t, ref, h.rb.Status.Gate.PreDeploy[0].WorkflowRunRef, "the same attempt is recreated")
	assert.Len(t, h.runs(), 1)
}

// Retries re-run a failed Sync hook as a new attempt (a distinct run name) and
// keep the gate closed meanwhile; only when retries are exhausted does the
// hook fail and block.
func TestPreDeployHooks_FailureRetriesThenBlocks(t *testing.T) {
	b := syncBlock("scan")
	b.Retries = 1
	h := newHookHarness(t, preHooks(b), nil)
	h.pre()
	first := h.rb.Status.Gate.PreDeploy[0].WorkflowRunRef
	h.failRun(first)
	h.events()

	out := h.pre()
	assert.True(t, out.blocked)
	assert.Equal(t, hookRequeueInterval, out.result.RequeueAfter, "a retrying hook is still running")
	entry := h.rb.Status.Gate.PreDeploy[0]
	assert.Equal(t, openchoreov1alpha1.HookPhasePending, entry.Phase)
	assert.Equal(t, hookReasonRetrying, entry.Reason)
	assert.Equal(t, int32(2), entry.Attempt)
	assert.Equal(t, string(ReasonHooksRunning), preCond(h.rb).Reason)
	evs := h.events()
	require.Equal(t, 1, countEvents(evs, eventHookFailed))
	assert.Contains(t, evs[0], "retrying")

	h.pre()
	second := h.rb.Status.Gate.PreDeploy[0].WorkflowRunRef
	assert.NotEqual(t, first, second)
	assert.True(t, strings.HasSuffix(second, "-a2"), "attempt 2 gets its own run: %s", second)

	h.failRun(second)
	out = h.pre()
	assert.True(t, out.blocked)
	assert.Zero(t, out.result.RequeueAfter)
	assert.Equal(t, openchoreov1alpha1.HookPhaseFailed, h.rb.Status.Gate.PreDeploy[0].Phase)
	assert.Equal(t, string(ReasonHookFailed), preCond(h.rb).Reason)
}

// A timeout counts as a failed attempt: with retries left the hook restarts
// instead of being marked TimedOut.
func TestPreDeployHooks_TimeoutRetries(t *testing.T) {
	b := syncBlock("scan")
	b.Retries = 1
	b.Timeout = "1s"
	h := newHookHarness(t, preHooks(b), nil)
	h.pre()
	timedOutRun := h.rb.Status.Gate.PreDeploy[0].WorkflowRunRef
	past := metav1.NewTime(time.Now().Add(-time.Minute))
	h.rb.Status.Gate.PreDeploy[0].StartedAt = &past

	h.pre()
	entry := h.rb.Status.Gate.PreDeploy[0]
	for _, run := range h.runs() {
		assert.NotEqual(t, timedOutRun, run.Name, "the timed-out run must be stopped before the retry")
	}
	assert.Equal(t, openchoreov1alpha1.HookPhasePending, entry.Phase)
	assert.Equal(t, hookReasonRetrying, entry.Reason)
	assert.Contains(t, entry.Message, "Timed out after 1s")
	assert.Equal(t, int32(2), entry.Attempt)
}

// A hook that times out with no retries left is stopped, so it cannot keep
// running in the plane after the gate gave up on it. A run that is already
// gone counts as stopped; failing to stop it is returned for backoff, and the
// hook is not marked TimedOut while its run may still be active.
func TestPreDeployHooks_TimeoutStopsRun(t *testing.T) {
	notFound := apierrors.NewNotFound(openchoreov1alpha1.GroupVersion.WithResource("workflowruns").GroupResource(), "run")
	cases := []struct {
		name      string
		deleteErr error
		wantErr   bool
	}{
		{name: "stops the run and gives up"},
		{name: "run already gone", deleteErr: notFound},
		{name: "stop fails", deleteErr: errBoom, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := syncBlock("scan")
			b.Timeout = "1s"
			var deletes int
			h := newHookHarness(t, preHooks(b), &interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if isRun(obj) {
						deletes++
						if tc.deleteErr != nil {
							return tc.deleteErr
						}
					}
					return c.Delete(ctx, obj, opts...)
				},
			})
			h.pre()
			run := h.rb.Status.Gate.PreDeploy[0].WorkflowRunRef
			past := metav1.NewTime(time.Now().Add(-time.Minute))
			h.rb.Status.Gate.PreDeploy[0].StartedAt = &past

			_, err := h.r.reconcilePreDeployHooks(context.Background(), h.rb, h.cr, h.env, nil, nil)
			entry := h.rb.Status.Gate.PreDeploy[0]
			assert.Equal(t, 1, deletes, "the timed-out run is stopped exactly once")
			if tc.wantErr {
				require.ErrorIs(t, err, errBoom)
				assert.NotEqual(t, openchoreov1alpha1.HookPhaseTimedOut, entry.Phase,
					"a hook whose run may still be active is not given up on")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, openchoreov1alpha1.HookPhaseTimedOut, entry.Phase)
			if tc.deleteErr == nil {
				for _, r := range h.runs() {
					assert.NotEqual(t, run, r.Name, "the timed-out run must be removed")
				}
			}
		})
	}
}

// ─── Async ───────────────────────────────────────────────────────────────────

// An Async hook that cannot be dispatched is recorded as DispatchFailed but
// never blocks: a broken notification must not become an outage.
func TestPreDeployHooks_AsyncDispatchFailureDoesNotGate(t *testing.T) {
	noRef := unitClusterHook("noref")
	noRef.Spec.WorkflowRef = nil
	b := hookBinding("notify", openchoreov1alpha1.HookModeAsync, "")
	b.HookRef.Name = "noref"
	h := newHookHarness(t, preHooks(b), nil, noRef)

	out := h.pre()
	assert.False(t, out.blocked)
	entry := h.rb.Status.Gate.PreDeploy[0]
	assert.Equal(t, openchoreov1alpha1.HookPhaseDispatchFailed, entry.Phase)
	assert.Equal(t, hookReasonDispatchFailed, entry.Reason)
	assert.Equal(t, 1, countEvents(h.events(), eventHookDispatchFailed))
	assert.Equal(t, h.rb.Status.Gate.Key, h.rb.Status.Gate.PassedKey)
}

// ─── Transient API errors ────────────────────────────────────────────────────

// API errors while starting or observing a hook are returned for backoff, not
// written onto the hook as a failure: a blip must neither fail a scan nor pass it.
func TestPreDeployHooks_TransientErrorsAreReturned(t *testing.T) {
	cases := []struct {
		name string
		mode openchoreov1alpha1.HookMode
		prep bool // run one clean reconcile first so a run exists
	}{
		{"sync: getting the run to start", openchoreov1alpha1.HookModeSync, false},
		{"async: getting the run to dispatch", openchoreov1alpha1.HookModeAsync, false},
		{"sync: observing the running run", openchoreov1alpha1.HookModeSync, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := hookBinding("scan", tc.mode, "")
			var fail bool
			funcs := &interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if fail && isRun(obj) {
						return errBoom
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}
			h := newHookHarness(t, preHooks(b), funcs)
			if tc.prep {
				h.pre()
				require.NotEmpty(t, h.rb.Status.Gate.PreDeploy[0].WorkflowRunRef)
			}
			fail = true
			before := h.rb.Status.Gate.DeepCopy()
			_, err := h.r.reconcilePreDeployHooks(context.Background(), h.rb, h.cr, h.env, nil, nil)
			require.ErrorIs(t, err, errBoom)
			if before != nil {
				assert.Equal(t, before.PreDeploy[0].Phase, h.rb.Status.Gate.PreDeploy[0].Phase)
			} else {
				assert.Equal(t, openchoreov1alpha1.HookPhasePending, h.rb.Status.Gate.PreDeploy[0].Phase)
			}
		})
	}
}

// ─── Retry annotation ────────────────────────────────────────────────────────

// Retrying a hook of a gate that already passed re-opens the gate so the hook
// really runs again. The retry lands in two steps: first the entry is reset and
// the annotation marked with the target attempt; only once that reset is stored
// is the old run deleted and the annotation consumed, so a lost status write
// cannot lose the retry.
func TestApplyHookRetry_ReopensPassedGate(t *testing.T) {
	h := newHookHarness(t, preHooks(syncBlock("scan")), nil)
	h.pre()
	first := h.runs()[0]
	h.succeedRun(first.Name)
	require.False(t, h.pre().blocked)
	require.Equal(t, h.rb.Status.Gate.Key, h.rb.Status.Gate.PassedKey)

	h.setRetryAnnotation("preDeploy/scan")
	out := h.pre()
	assert.True(t, out.blocked, "the gate must re-open for the retried hook")
	assert.Equal(t, string(ReasonHooksRunning), preCond(h.rb).Reason)
	assert.Equal(t, "preDeploy/scan#2", h.rb.Annotations[labels.AnnotationKeyHookRetry],
		"the annotation stays, marked with the target attempt, until the reset is stored")
	assert.Equal(t, "preDeploy/scan#2", storedRetryAnnotation(t, h))
	require.Len(t, h.runs(), 2, "the old run is kept until the reset is stored")
	retried := h.rb.Status.Gate.PreDeploy[0].WorkflowRunRef
	assert.True(t, strings.HasSuffix(retried, "-a2"), "the retry runs as attempt 2: %s", retried)

	// Next reconcile: the stored entry has reached attempt 2.
	h.pre()
	assert.NotContains(t, h.rb.Annotations, labels.AnnotationKeyHookRetry)
	assert.Empty(t, storedRetryAnnotation(t, h))
	runs := h.runs()
	require.Len(t, runs, 1, "the replaced run is deleted once the reset is stored")
	assert.Equal(t, retried, runs[0].Name)
	assert.Empty(t, runs[0].Status.Conditions, "the retried hook got a fresh run, not the old succeeded one")
}

// If the status write carrying the reset is lost, the marked annotation makes
// the next reconcile apply the same reset again. It reuses the new attempt's
// run (same deterministic name) instead of creating another, and does not
// delete the old run until the reset is actually stored.
func TestApplyHookRetry_ReappliedAfterFailedStatusWrite(t *testing.T) {
	h := newHookHarness(t, preHooks(syncBlock("scan")), nil)
	h.pre()
	first := h.runs()[0]
	h.failRun(first.Name)
	require.True(t, h.pre().blocked)
	stored := h.rb.Status.DeepCopy()

	h.setRetryAnnotation("preDeploy/scan")
	h.pre()
	require.Equal(t, "preDeploy/scan#2", storedRetryAnnotation(t, h))
	require.Len(t, h.runs(), 2)

	// The status update of that reconcile failed: the API still has the old
	// status. The annotation patch went through.
	h.rb.Status = *stored
	h.pre()
	entry := h.rb.Status.Gate.PreDeploy[0]
	assert.Equal(t, int32(2), entry.Attempt, "the reset is applied again")
	assert.Equal(t, openchoreov1alpha1.HookPhaseRunning, entry.Phase)
	assert.Len(t, h.runs(), 2, "the new attempt's run is reused, not created twice")
	assert.Equal(t, "preDeploy/scan#2", storedRetryAnnotation(t, h), "not consumed before the reset is stored")
	oldKept := false
	for _, r := range h.runs() {
		if r.Name == first.Name {
			oldKept = true
		}
	}
	assert.True(t, oldKept, "the old run is not deleted before the reset is stored")

	h.pre()
	assert.Empty(t, storedRetryAnnotation(t, h))
	require.Len(t, h.runs(), 1)
	assert.NotEqual(t, first.Name, h.runs()[0].Name)
}

func storedRetryAnnotation(t *testing.T, h *hookHarness) string {
	t.Helper()
	stored := &openchoreov1alpha1.ReleaseBinding{}
	require.NoError(t, h.c.Get(context.Background(), types.NamespacedName{Namespace: unitNS, Name: unitRB}, stored))
	return stored.Annotations[labels.AnnotationKeyHookRetry]
}

// The retry annotation names one hook: only that entry is reset, and nothing
// is deleted before the reset is stored. Post-deploy entries can be retried the
// same way; an unknown phase or hook is consumed without touching anything.
func TestApplyHookRetry_TargetsOneEntry(t *testing.T) {
	statusWith := func() *openchoreov1alpha1.DeploymentGateStatus {
		return &openchoreov1alpha1.DeploymentGateStatus{
			Key:        "k",
			PreDeploy:  []openchoreov1alpha1.DeploymentHookStatus{{Name: "a", WorkflowRunRef: "run-a", Attempt: 1}, {Name: "b", WorkflowRunRef: "run-b", Attempt: 1}},
			PostDeploy: []openchoreov1alpha1.DeploymentHookStatus{{Name: "smoke", WorkflowRunRef: "run-smoke", Attempt: 1}},
		}
	}
	run := func(name string) client.Object {
		return &openchoreov1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Namespace: unitNS, Name: name}}
	}
	cases := []struct {
		value          string
		wantReset      string
		wantAnnotation string
	}{
		{"preDeploy/b", "b", "preDeploy/b#2"},
		{"postDeploy/smoke", "smoke", "postDeploy/smoke#2"},
		{"bogus/a", "", ""},
		{"preDeploy/missing", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			h := newHookHarness(t, nil, nil, run("run-a"), run("run-b"), run("run-smoke"))
			h.rb.Status.Gate = statusWith()
			h.setRetryAnnotation(tc.value)
			require.NoError(t, h.r.applyHookRetry(context.Background(), h.rb, &hookGate{key: "other"}))

			assert.Equal(t, tc.wantAnnotation, h.rb.Annotations[labels.AnnotationKeyHookRetry])
			assert.Equal(t, tc.wantAnnotation, storedRetryAnnotation(t, h))
			assert.NotNil(t, h.rb.Status.Gate, "the patch response must not wipe the in-memory status")
			for _, e := range append(append([]openchoreov1alpha1.DeploymentHookStatus{},
				h.rb.Status.Gate.PreDeploy...), h.rb.Status.Gate.PostDeploy...) {
				if e.Name == tc.wantReset {
					assert.Empty(t, e.WorkflowRunRef, "%s must lose its old run", e.Name)
					assert.Equal(t, int32(2), e.Attempt, "%s must move to the next attempt", e.Name)
					assert.Equal(t, openchoreov1alpha1.HookPhasePending, e.Phase)
				} else {
					assert.Equal(t, "run-"+e.Name, e.WorkflowRunRef, "%s must be untouched", e.Name)
				}
			}
			for _, n := range []string{"run-a", "run-b", "run-smoke"} {
				assert.NoError(t, h.c.Get(context.Background(), types.NamespacedName{Namespace: unitNS, Name: n}, &openchoreov1alpha1.WorkflowRun{}),
					"%s must not be deleted before the reset is stored", n)
			}
		})
	}
}

// Deleting a run is not instant (finalizers keep it around). A retry must still
// get a fresh run: it runs under a new attempt-specific name instead of picking
// up the run that is being torn down under the old name.
func TestApplyHookRetry_DoesNotReuseTerminatingRun(t *testing.T) {
	h := newHookHarness(t, preHooks(syncBlock("scan")), nil)
	h.pre()
	first := h.runs()[0]
	h.failRun(first.Name)
	require.True(t, h.pre().blocked)

	// Hold the old run in Terminating: the retry's Delete only sets its
	// deletionTimestamp.
	held := &openchoreov1alpha1.WorkflowRun{}
	require.NoError(t, h.c.Get(context.Background(), types.NamespacedName{Namespace: unitNS, Name: first.Name}, held))
	held.Finalizers = append(held.Finalizers, "test.openchoreo.dev/hold")
	require.NoError(t, h.c.Update(context.Background(), held))

	h.setRetryAnnotation("preDeploy/scan")
	out := h.pre()
	assert.True(t, out.blocked)
	entry := h.rb.Status.Gate.PreDeploy[0]
	assert.Equal(t, int32(2), entry.Attempt)
	assert.Equal(t, openchoreov1alpha1.HookPhaseRunning, entry.Phase, "the new attempt runs, not the old failure")
	require.NotEmpty(t, entry.WorkflowRunRef)
	assert.NotEqual(t, first.Name, entry.WorkflowRunRef, "the retry must not reuse the terminating run")
	assert.True(t, strings.HasSuffix(entry.WorkflowRunRef, "-a2"), "the run is named for its attempt: %s", entry.WorkflowRunRef)
}

// An Async hook is retried the same way: it is dispatched again under the new
// attempt's run name, not the retried run's.
func TestApplyHookRetry_AsyncUsesNewAttempt(t *testing.T) {
	h := newHookHarness(t, preHooks(hookBinding("notify", openchoreov1alpha1.HookModeAsync, "")), nil)
	h.pre()
	first := h.rb.Status.Gate.PreDeploy[0]
	require.Equal(t, openchoreov1alpha1.HookPhaseDispatched, first.Phase)
	require.Equal(t, int32(1), first.Attempt)

	h.setRetryAnnotation("preDeploy/notify")
	h.pre()
	entry := h.rb.Status.Gate.PreDeploy[0]
	assert.Equal(t, openchoreov1alpha1.HookPhaseDispatched, entry.Phase)
	assert.Equal(t, int32(2), entry.Attempt)
	assert.NotEqual(t, first.WorkflowRunRef, entry.WorkflowRunRef)
	assert.True(t, strings.HasSuffix(entry.WorkflowRunRef, "-a2"), "the run is named for its attempt: %s", entry.WorkflowRunRef)
}

// If the old run cannot be deleted or the annotation cannot be removed, the
// retry fails with an error and is attempted again, rather than silently
// resetting status while the annotation lingers.
func TestApplyHookRetry_APIErrors(t *testing.T) {
	t.Run("marking the annotation fails", func(t *testing.T) {
		h := newHookHarness(t, nil, &interceptor.Funcs{
			Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
				return errBoom
			},
		})
		h.rb.Status.Gate = &openchoreov1alpha1.DeploymentGateStatus{
			PreDeploy: []openchoreov1alpha1.DeploymentHookStatus{{Name: "scan", WorkflowRunRef: "run-scan"}},
		}
		h.rb.Annotations = map[string]string{labels.AnnotationKeyHookRetry: "preDeploy/scan"}
		err := h.r.applyHookRetry(context.Background(), h.rb, &hookGate{key: "k"})
		require.ErrorIs(t, err, errBoom)
	})

	// Deleting the replaced run happens after the reset is stored; if it fails
	// the annotation stays so the next reconcile tries again.
	t.Run("deleting the replaced run fails", func(t *testing.T) {
		var failDelete bool
		h := newHookHarness(t, preHooks(syncBlock("scan")), &interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if failDelete && isRun(obj) {
					return errBoom
				}
				return c.Delete(ctx, obj, opts...)
			},
		})
		h.pre()
		h.failRun(h.runs()[0].Name)
		h.pre()
		h.setRetryAnnotation("preDeploy/scan")
		h.pre()
		failDelete = true
		_, err := h.r.reconcilePreDeployHooks(context.Background(), h.rb, h.cr, h.env, nil, nil)
		require.ErrorIs(t, err, errBoom)
		assert.Equal(t, "preDeploy/scan#2", storedRetryAnnotation(t, h), "kept for the next attempt")
	})

	t.Run("error surfaces from the pre-deploy gate", func(t *testing.T) {
		h := newHookHarness(t, preHooks(syncBlock("scan")), &interceptor.Funcs{
			Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
				return errBoom
			},
		})
		h.rb.Annotations = map[string]string{labels.AnnotationKeyHookRetry: "preDeploy/scan"}
		h.rb.Status.Gate = &openchoreov1alpha1.DeploymentGateStatus{
			PreDeploy: []openchoreov1alpha1.DeploymentHookStatus{{Name: "scan"}},
		}
		_, err := h.r.reconcilePreDeployHooks(context.Background(), h.rb, h.cr, h.env, nil, nil)
		require.ErrorIs(t, err, errBoom)
		assert.Empty(t, h.runs(), "nothing runs when the retry could not be applied")
	})
}

// ─── Post-deploy ─────────────────────────────────────────────────────────────

func postSetup(t *testing.T, b openchoreov1alpha1.HookBinding, funcs *interceptor.Funcs) (*hookHarness, *hookGate) {
	t.Helper()
	h := newHookHarness(t, &openchoreov1alpha1.HookSet{PostDeploy: []openchoreov1alpha1.HookBinding{b}}, funcs)
	out := h.pre()
	require.False(t, out.blocked)
	require.NotNil(t, out.gate)
	apimeta.SetStatusCondition(&h.rb.Status.Conditions, metav1.Condition{
		Type: string(ConditionResourcesReady), Status: metav1.ConditionTrue, Reason: "Ready",
	})
	return h, out.gate
}

// Post-deploy hooks that all succeed report a clean pass with the post-deploy
// wording, so the pre and post conditions are distinguishable.
func TestPostDeployHooks_SuccessMarksPassed(t *testing.T) {
	h, hg := postSetup(t, syncBlock("smoke"), nil)
	res, err := h.r.reconcilePostDeployHooks(context.Background(), h.rb, hg)
	require.NoError(t, err)
	assert.Equal(t, hookRequeueInterval, res.RequeueAfter)
	h.succeedRun(h.rb.Status.Gate.PostDeploy[0].WorkflowRunRef)

	res, err = h.r.reconcilePostDeployHooks(context.Background(), h.rb, hg)
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter)
	cond := postCond(h.rb)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, string(ReasonHooksPassed), cond.Reason)
	assert.Equal(t, "All post-deploy hooks completed", cond.Message)
}

// Post-deploy failures default to Ignore: Ready stays True, but the condition
// names the failed hook and a HookFailureIgnored warning is recorded exactly
// once, not on every reconcile.
func TestPostDeployHooks_IgnoredFailureWarnsOnce(t *testing.T) {
	h, hg := postSetup(t, hookBinding("smoke", openchoreov1alpha1.HookModeSync, ""), nil)
	_, err := h.r.reconcilePostDeployHooks(context.Background(), h.rb, hg)
	require.NoError(t, err)
	h.failRun(h.rb.Status.Gate.PostDeploy[0].WorkflowRunRef)
	h.events()

	_, err = h.r.reconcilePostDeployHooks(context.Background(), h.rb, hg)
	require.NoError(t, err)
	cond := postCond(h.rb)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, string(ReasonHooksPassedWithIgnoredFailures), cond.Reason)
	assert.Equal(t, "Post-deploy hooks passed; ignored failures: smoke", cond.Message)
	evs := h.events()
	require.Equal(t, 1, countEvents(evs, eventHookFailureIgnored))
	assert.Contains(t, evs[len(evs)-1], `Post-deploy hook "smoke" failed`)

	_, err = h.r.reconcilePostDeployHooks(context.Background(), h.rb, hg)
	require.NoError(t, err)
	assert.Zero(t, countEvents(h.events(), eventHookFailureIgnored))
}

// A post-deploy Sync hook whose plane is missing must keep being retried: no
// watch fires when the plane appears, so without the requeue the hook would
// never run. A plain failure, by contrast, is final and must not poll.
func TestPostDeployHooks_PlaneUnavailableRequeues(t *testing.T) {
	h, hg := postSetup(t, syncBlock("smoke"), nil)
	_, err := h.r.reconcilePostDeployHooks(context.Background(), h.rb, hg)
	require.NoError(t, err)
	run := h.rb.Status.Gate.PostDeploy[0].WorkflowRunRef
	h.setRunConditions(run, metav1.Condition{
		Type: string(workflowrun.ConditionWorkflowCompleted), Status: metav1.ConditionFalse,
		Reason: string(workflowrun.ReasonWorkflowPlaneNotFound), Message: "no plane",
	})

	res, err := h.r.reconcilePostDeployHooks(context.Background(), h.rb, hg)
	require.NoError(t, err)
	assert.Equal(t, openchoreov1alpha1.HookPhasePlaneUnavailable, h.rb.Status.Gate.PostDeploy[0].Phase)
	assert.Equal(t, hookRequeueInterval, res.RequeueAfter, "nothing watches planes; only the requeue retries the hook")

	h2, hg2 := postSetup(t, hookBinding("smoke", openchoreov1alpha1.HookModeSync, ""), nil)
	_, err = h2.r.reconcilePostDeployHooks(context.Background(), h2.rb, hg2)
	require.NoError(t, err)
	h2.failRun(h2.rb.Status.Gate.PostDeploy[0].WorkflowRunRef)
	res, err = h2.r.reconcilePostDeployHooks(context.Background(), h2.rb, hg2)
	require.NoError(t, err)
	assert.Equal(t, openchoreov1alpha1.HookPhaseFailed, h2.rb.Status.Gate.PostDeploy[0].Phase)
	assert.Zero(t, res.RequeueAfter, "a finished failure is final and must not poll")
}

// Post-deploy hooks wait for ResourcesReady; before that nothing starts. With
// no gate at all the phase is a no-op.
func TestPostDeployHooks_WaitsForResourcesReady(t *testing.T) {
	h, hg := postSetup(t, syncBlock("smoke"), nil)
	apimeta.RemoveStatusCondition(&h.rb.Status.Conditions, string(ConditionResourcesReady))
	res, err := h.r.reconcilePostDeployHooks(context.Background(), h.rb, hg)
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter)
	assert.Empty(t, h.runs())
	assert.Nil(t, postCond(h.rb))

	res, err = h.r.reconcilePostDeployHooks(context.Background(), h.rb, nil)
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter)
}

// A transient error while starting a post-deploy hook is returned for backoff.
func TestPostDeployHooks_TransientErrorIsReturned(t *testing.T) {
	var fail bool
	h, hg := postSetup(t, syncBlock("smoke"), &interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if fail && isRun(obj) {
				return errBoom
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
	fail = true
	_, err := h.r.reconcilePostDeployHooks(context.Background(), h.rb, hg)
	require.ErrorIs(t, err, errBoom)
	assert.Empty(t, h.runs())
}

// ─── ensureHookRun ───────────────────────────────────────────────────────────

// ensureHookRun separates "the hook is misconfigured" (sentinel errors the gate
// records on the hook) from API failures (returned for backoff), and treats a
// concurrent create of the same run as success so a hook never starts twice.
func TestEnsureHookRun_Errors(t *testing.T) {
	cases := []struct {
		name      string
		funcs     *interceptor.Funcs
		noPlane   bool
		wantSent  error
		wantBoom  bool
		wantExist bool
		existing  bool // the run already exists before the call
		noScheme  bool // the reconciler's scheme cannot resolve the owner
	}{
		// A reconcile whose status write was lost comes back with no
		// WorkflowRunRef; the attempt's run must be found, not duplicated.
		{name: "run already exists", existing: true, wantExist: true},
		{name: "owner reference cannot be set", noScheme: true, wantBoom: true},
		{name: "workflow get fails", funcs: getErrFor(func(o client.Object) bool { _, ok := o.(*openchoreov1alpha1.ClusterWorkflow); return ok }), wantBoom: true},
		{name: "plane get fails", funcs: getErrFor(func(o client.Object) bool { _, ok := o.(*openchoreov1alpha1.ClusterWorkflowPlane); return ok }), wantBoom: true},
		{name: "plane missing", noPlane: true, wantSent: errHookPlaneUnavailable},
		{name: "create fails", funcs: &interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error { return errBoom },
		}, wantBoom: true},
		{name: "create races an existing run", funcs: &interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if err := c.Create(ctx, obj.DeepCopyObject().(client.Object), opts...); err != nil {
					return err
				}
				return apierrors.NewAlreadyExists(schema.GroupResource{Resource: "workflowruns"}, obj.GetName())
			},
		}, wantExist: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var extra []client.Object
			if tc.existing {
				extra = append(extra, &openchoreov1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{
					Namespace: unitNS, Name: "scan-pre-" + hookWorkflowName + "-01234567"}})
			}
			h := newHookHarness(t, nil, tc.funcs, extra...)
			if tc.noScheme {
				h.r.Scheme = runtime.NewScheme()
			}
			if tc.noPlane {
				require.NoError(t, h.c.Delete(context.Background(), clusterWorkflowPlaneFixture()))
			}
			rh := gate.ResolvedHook{Binding: syncBlock("scan"), Phase: gate.PhasePreDeploy, Hook: &unitClusterHook(hookName).Spec}
			run, created, err := h.r.ensureHookRun(context.Background(), h.rb, rh, "0123456789", 1, map[string]string{})
			switch {
			case tc.wantSent != nil:
				require.ErrorIs(t, err, tc.wantSent)
				assert.False(t, isTransientHookError(err))
			case tc.wantBoom && tc.noScheme:
				require.Error(t, err)
				assert.True(t, isTransientHookError(err))
				assert.Empty(t, h.runs(), "an ownerless run would outlive its binding")
			case tc.wantBoom:
				require.ErrorIs(t, err, errBoom)
				assert.True(t, isTransientHookError(err))
			case tc.wantExist:
				require.NoError(t, err)
				assert.False(t, created, "a run created by someone else must not be reported as started by us")
				assert.Equal(t, "scan-pre-"+hookWorkflowName+"-01234567", run.Name)
			}
		})
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// An unset, malformed or non-positive timeout falls back to the default, so a
// bad value can never disable the bound on a Sync hook.
// Reconciles run concurrently (--max-concurrent-reconciles), so the lazily
// built parameter engine must be created once and shared, without a data race
// (this test is meaningful under -race).
func TestHookEngine_SharedUnderConcurrency(t *testing.T) {
	r := &Reconciler{}
	const callers = 16
	engines := make([]*template.Engine, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			engines[i] = r.hookEngine()
		}(i)
	}
	wg.Wait()
	require.NotNil(t, engines[0])
	for i, e := range engines {
		assert.Same(t, engines[0], e, "caller %d got a different engine", i)
	}
	assert.Same(t, engines[0], r.hookEngine(), "later calls return the cached engine")
}

func TestHookTimeout(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"": defaultHookTimeout, "bogus": defaultHookTimeout, "0s": defaultHookTimeout, "5m": 5 * time.Minute,
	} {
		assert.Equal(t, want, hookTimeout(openchoreov1alpha1.HookBinding{Timeout: in}), "timeout %q", in)
	}
	assert.False(t, timedOut(&openchoreov1alpha1.DeploymentHookStatus{}, openchoreov1alpha1.HookBinding{Timeout: "1s"}, time.Now()),
		"a hook that never started cannot time out")
}

// ─── watches ─────────────────────────────────────────────────────────────────

// Editing an environment's hooks re-queues exactly the bindings deploying into
// that environment in its namespace, and no others.
func TestReleaseBindingsForEnvironment(t *testing.T) {
	r := newReconcilerWith(t,
		bindingFor("ns1", "rb-prod", "prod"),
		bindingFor("ns1", "rb-dev", "dev"),
		bindingFor("ns2", "rb-prod-other-ns", "prod"),
	)
	env := &openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "prod"}}
	assert.Equal(t, []string{"rb-prod"}, requestNames(r.releaseBindingsForEnvironment(context.Background(), env)))
	assert.Nil(t, r.releaseBindingsForEnvironment(context.Background(), &openchoreov1alpha1.Hook{}))
}

// Editing a Hook re-queues the bindings of every environment in its namespace
// that binds it (pre or post, with the Kind defaulted to Hook); a same-named
// Hook in another namespace is a different hook. A ClusterHook reaches every
// namespace.
func TestReleaseBindingsForHook(t *testing.T) {
	bind := func(kind openchoreov1alpha1.HookRefKind, name string) openchoreov1alpha1.HookBinding {
		return openchoreov1alpha1.HookBinding{Name: "b", HookRef: openchoreov1alpha1.HookRef{Kind: kind, Name: name}}
	}
	env := func(ns, name string, hooks *openchoreov1alpha1.HookSet) *openchoreov1alpha1.Environment {
		return &openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name}, Spec: openchoreov1alpha1.EnvironmentSpec{Hooks: hooks}}
	}
	r := newReconcilerWith(t,
		env("ns1", "pre-hook", preHooks(bind("", "scan"))),
		env("ns1", "post-cluster", &openchoreov1alpha1.HookSet{PostDeploy: []openchoreov1alpha1.HookBinding{bind(openchoreov1alpha1.HookRefKindClusterHook, "scan")}}),
		env("ns1", "no-hooks", nil),
		env("ns1", "other-hook", preHooks(bind(openchoreov1alpha1.HookRefKindHook, "lint"))),
		env("ns2", "pre-hook", preHooks(bind(openchoreov1alpha1.HookRefKindHook, "scan"))),
		env("ns2", "cluster", preHooks(bind(openchoreov1alpha1.HookRefKindClusterHook, "scan"))),
		bindingFor("ns1", "rb-pre-hook", "pre-hook"),
		bindingFor("ns1", "rb-post-cluster", "post-cluster"),
		bindingFor("ns1", "rb-no-hooks", "no-hooks"),
		bindingFor("ns1", "rb-other-hook", "other-hook"),
		bindingFor("ns2", "rb-ns2-pre-hook", "pre-hook"),
		bindingFor("ns2", "rb-ns2-cluster", "cluster"),
	)
	ctx := context.Background()

	hook := &openchoreov1alpha1.Hook{ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "scan"}}
	assert.ElementsMatch(t, []string{"rb-pre-hook"}, requestNames(r.releaseBindingsForHook(ctx, hook)))

	cluster := &openchoreov1alpha1.ClusterHook{ObjectMeta: metav1.ObjectMeta{Name: "scan"}}
	assert.ElementsMatch(t, []string{"rb-post-cluster", "rb-ns2-cluster"}, requestNames(r.releaseBindingsForHook(ctx, cluster)))

	assert.Nil(t, r.releaseBindingsForHook(ctx, &openchoreov1alpha1.Environment{}))
}

// A List failure in a watch mapper is logged and yields no requests rather
// than panicking the controller; the periodic resync picks the change up.
func TestHookWatchMappers_ListErrors(t *testing.T) {
	failing := func(match func(client.ObjectList) bool) *Reconciler {
		c := fake.NewClientBuilder().WithScheme(watchTestScheme(t)).WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if match(list) {
					return errBoom
				}
				return c.List(ctx, list, opts...)
			},
		}).WithObjects(
			&openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "prod"},
				Spec: openchoreov1alpha1.EnvironmentSpec{Hooks: preHooks(openchoreov1alpha1.HookBinding{
					Name: "b", HookRef: openchoreov1alpha1.HookRef{Kind: openchoreov1alpha1.HookRefKindClusterHook, Name: "scan"}})},
			},
			bindingFor("ns1", "rb", "prod"),
		).Build()
		return &Reconciler{Client: c}
	}
	ctx := context.Background()
	cluster := &openchoreov1alpha1.ClusterHook{ObjectMeta: metav1.ObjectMeta{Name: "scan"}}

	r := failing(func(l client.ObjectList) bool { _, ok := l.(*openchoreov1alpha1.EnvironmentList); return ok })
	assert.Empty(t, r.releaseBindingsForHook(ctx, cluster))

	r = failing(func(l client.ObjectList) bool { _, ok := l.(*openchoreov1alpha1.ReleaseBindingList); return ok })
	assert.Empty(t, r.releaseBindingsForHook(ctx, cluster))
	assert.Empty(t, r.releaseBindingsForEnvironment(ctx, &openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "prod"}}))
}
