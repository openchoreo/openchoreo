// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/releasebinding/gate"
	"github.com/openchoreo/openchoreo/internal/controller/workflowrun"
)

// The run name is how an operator (and the retry path) finds a hook's WorkflowRun:
// it encodes binding, phase, hook, attempt and the key prefix, and it must be a
// valid DNS label however long the inputs are — otherwise the create fails and
// the gate blocks with an opaque API error.
func TestHookRunName(t *testing.T) {
	key := "865f1323d0770138798ea1584477a8e6da669d2e79ea539e52fe96ea703d0ac6"
	if got := hookRunName("scan", gate.PhasePreDeploy, "trivy", key, 1); got != "scan-pre-trivy-865f1323" {
		t.Fatalf("got %q", got)
	}
	if got := hookRunName("smoke", gate.PhasePostDeploy, "smoke-test", key, 3); got != "smoke-post-smoke-test-865f1323-a3" {
		t.Fatalf("got %q", got)
	}
	long := hookRunName(strings.Repeat("b", 40), gate.PhasePreDeploy, strings.Repeat("h", 40), key, 2)
	if errs := validation.IsDNS1123Label(long); len(errs) > 0 {
		t.Fatalf("%q is not a valid DNS label: %v", long, errs)
	}
	if long == hookRunName(strings.Repeat("b", 40), gate.PhasePreDeploy, strings.Repeat("h", 40), key, 3) {
		t.Fatal("attempts must produce distinct names even when shortened")
	}
}

// observeHookRun is the only place WorkflowRun conditions become hook phases. A
// wrong mapping either blocks a passing deployment or lets a failed scan through,
// so every terminal shape the workflowrun controller produces is pinned here.
func TestObserveHookRun(t *testing.T) {
	cond := func(typ string, status metav1.ConditionStatus, reason, msg string) metav1.Condition {
		return metav1.Condition{Type: typ, Status: status, Reason: reason, Message: msg}
	}
	run := func(conds ...metav1.Condition) *openchoreov1alpha1.WorkflowRun {
		return &openchoreov1alpha1.WorkflowRun{Status: openchoreov1alpha1.WorkflowRunStatus{Conditions: conds}}
	}
	cases := []struct {
		name      string
		run       *openchoreov1alpha1.WorkflowRun
		wantPhase openchoreov1alpha1.HookPhase
		wantMsg   string
	}{
		{"no conditions yet", run(), openchoreov1alpha1.HookPhaseRunning, "Workflow is running"},
		{"pending", run(cond(string(workflowrun.ConditionWorkflowCompleted), metav1.ConditionFalse, string(workflowrun.ReasonWorkflowPending), "")),
			openchoreov1alpha1.HookPhaseRunning, "Workflow is running"},
		{"succeeded", run(cond(string(workflowrun.ConditionWorkflowSucceeded), metav1.ConditionTrue, string(workflowrun.ReasonWorkflowSucceeded), "")),
			openchoreov1alpha1.HookPhaseSucceeded, "Workflow completed successfully"},
		{"failed without tasks", run(cond(string(workflowrun.ConditionWorkflowFailed), metav1.ConditionTrue, string(workflowrun.ReasonWorkflowFailed), "Workflow execution failed")),
			openchoreov1alpha1.HookPhaseFailed, "Workflow execution failed"},
		{"plane not found", run(cond(string(workflowrun.ConditionWorkflowCompleted), metav1.ConditionFalse, string(workflowrun.ReasonWorkflowPlaneNotFound), "no plane")),
			openchoreov1alpha1.HookPhasePlaneUnavailable, "no plane"},
		{"workflow not found is terminal", run(cond(string(workflowrun.ConditionWorkflowCompleted), metav1.ConditionTrue, "WorkflowNotFound", "gone")),
			openchoreov1alpha1.HookPhaseFailed, "gone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			phase, _, msg := observeHookRun(tc.run)
			if phase != tc.wantPhase || msg != tc.wantMsg {
				t.Fatalf("got (%s, %q), want (%s, %q)", phase, msg, tc.wantPhase, tc.wantMsg)
			}
		})
	}

	t.Run("failed with a failing task copies its name and message", func(t *testing.T) {
		r := run(cond(string(workflowrun.ConditionWorkflowFailed), metav1.ConditionTrue, string(workflowrun.ReasonWorkflowFailed), ""))
		r.Status.Tasks = []openchoreov1alpha1.WorkflowTask{
			{Name: "fetch", Phase: "Succeeded"},
			{Name: "scan", Phase: "Failed", Message: "3 CRITICAL vulnerabilities"},
		}
		phase, _, msg := observeHookRun(r)
		if phase != openchoreov1alpha1.HookPhaseFailed || msg != `task "scan" failed: 3 CRITICAL vulnerabilities` {
			t.Fatalf("got (%s, %q)", phase, msg)
		}
	})
	t.Run("running task named while running", func(t *testing.T) {
		r := run()
		r.Status.Tasks = []openchoreov1alpha1.WorkflowTask{{Name: "approval", Phase: "Running"}}
		_, _, msg := observeHookRun(r)
		if msg != `task "approval" is running` {
			t.Fatalf("got %q", msg)
		}
	})
}

// The Ready aggregate must surface the gate above everything else: a blocked
// first deployment has no ReleaseSynced at all, and a blocked redeploy still has
// ReleaseSynced=True from the previous release. Post-deploy Alert failures
// degrade Ready while ResourcesReady is True.
func TestSetReadyConditionHookPriority(t *testing.T) {
	r := newTestReconciler()
	rb := &openchoreov1alpha1.ReleaseBinding{}
	rb.Status.Conditions = []metav1.Condition{
		{Type: string(ConditionReleaseSynced), Status: metav1.ConditionTrue, Reason: "ReleaseSynced"},
		{Type: string(ConditionResourcesReady), Status: metav1.ConditionTrue, Reason: "ResourcesReady"},
		{Type: string(ConditionPreDeployHooksPassed), Status: metav1.ConditionFalse, Reason: string(ReasonHookFailed), Message: "scan failed"},
	}
	r.setReadyCondition(rb)
	ready := readyCond(rb)
	if ready.Status != metav1.ConditionFalse || ready.Reason != string(ReasonHookFailed) || ready.Message != "scan failed" {
		t.Fatalf("blocked gate must drive Ready, got %+v", ready)
	}

	rb.Status.Conditions = []metav1.Condition{
		{Type: string(ConditionPreDeployHooksPassed), Status: metav1.ConditionFalse, Reason: string(ReasonHooksRunning), Message: "waiting"},
	}
	r.setReadyCondition(rb)
	if ready = readyCond(rb); ready.Reason != string(ReasonHooksRunning) {
		t.Fatalf("first deploy without ReleaseSynced must still report the gate, got %+v", ready)
	}

	rb.Status.Conditions = []metav1.Condition{
		{Type: string(ConditionReleaseSynced), Status: metav1.ConditionTrue, Reason: "ReleaseSynced"},
		{Type: string(ConditionResourcesReady), Status: metav1.ConditionTrue, Reason: "ResourcesReady"},
		{Type: string(ConditionPreDeployHooksPassed), Status: metav1.ConditionTrue, Reason: string(ReasonHooksPassed)},
		{Type: string(ConditionPostDeployHooksPassed), Status: metav1.ConditionFalse, Reason: string(ReasonPostDeployHookFailed), Message: "smoke failed"},
	}
	r.setReadyCondition(rb)
	if ready = readyCond(rb); ready.Status != metav1.ConditionFalse || ready.Reason != string(ReasonPostDeployHookFailed) {
		t.Fatalf("unacknowledged Alert must degrade Ready, got %+v", ready)
	}

	rb.Status.Conditions[3].Status = metav1.ConditionTrue
	r.setReadyCondition(rb)
	if ready = readyCond(rb); ready.Status != metav1.ConditionTrue {
		t.Fatalf("all hook conditions True must yield Ready, got %+v", ready)
	}
}

func readyCond(rb *openchoreov1alpha1.ReleaseBinding) *metav1.Condition {
	for i := range rb.Status.Conditions {
		if rb.Status.Conditions[i].Type == string(ConditionReady) {
			return &rb.Status.Conditions[i]
		}
	}
	return nil
}
