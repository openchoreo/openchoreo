// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/workflowrun"
	"github.com/openchoreo/openchoreo/internal/labels"
	componentpipeline "github.com/openchoreo/openchoreo/internal/pipeline/component"
)

// ─── Hook fixtures ────────────────────────────────────────────────────────────

const (
	hookWorkflowName = "hook-wf"
	hookName         = "trivy"
)

func clusterWorkflowPlaneFixture() *openchoreov1alpha1.ClusterWorkflowPlane {
	return &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "hooks-test"},
	}
}

func clusterWorkflowFixture() *openchoreov1alpha1.ClusterWorkflow {
	return &openchoreov1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: hookWorkflowName},
		Spec: openchoreov1alpha1.ClusterWorkflowSpec{
			RunTemplate: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"x"}}`)},
		},
	}
}

func clusterHookFixture() *openchoreov1alpha1.ClusterHook {
	sev := "CRITICAL"
	return &openchoreov1alpha1.ClusterHook{
		ObjectMeta: metav1.ObjectMeta{Name: hookName},
		Spec: openchoreov1alpha1.HookSpec{
			Type:        openchoreov1alpha1.HookTypeWorkflow,
			WorkflowRef: &openchoreov1alpha1.WorkflowRef{Kind: openchoreov1alpha1.WorkflowRefKindClusterWorkflow, Name: hookWorkflowName},
			Parameters: []openchoreov1alpha1.HookParameter{
				{Name: "image", From: "${deployment.workload.image}"},
				{Name: "severity", Default: &sev},
			},
		},
	}
}

// envWithHooks is envFixture with hooks bound on the environment.
func envWithHooks(name, dpName string, hooks *openchoreov1alpha1.HookSet) *openchoreov1alpha1.Environment {
	env := envFixture(name, dpName)
	env.Spec.Hooks = hooks
	return env
}

func hookBinding(name string, mode openchoreov1alpha1.HookMode, onFailure openchoreov1alpha1.HookFailurePolicy) openchoreov1alpha1.HookBinding {
	return openchoreov1alpha1.HookBinding{
		Name:      name,
		HookRef:   openchoreov1alpha1.HookRef{Kind: openchoreov1alpha1.HookRefKindClusterHook, Name: hookName},
		Mode:      mode,
		OnFailure: onFailure,
	}
}

func hookReconciler() *Reconciler {
	return &Reconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Pipeline: componentpipeline.NewPipeline(),
	}
}

// hookRuns lists the deployment-hook WorkflowRuns in the test namespace.
func hookRuns() []openchoreov1alpha1.WorkflowRun {
	GinkgoHelper()
	list := &openchoreov1alpha1.WorkflowRunList{}
	Expect(k8sClient.List(ctx, list, client.InNamespace(ns),
		client.MatchingLabels{labels.LabelKeyWorkflowPurpose: labels.LabelValueWorkflowPurposeDeploymentHook})).To(Succeed())
	return list.Items
}

// finishRun marks a hook WorkflowRun succeeded or failed the way the workflowrun
// controller would.
func finishRun(name string, succeeded bool, tasks ...openchoreov1alpha1.WorkflowTask) {
	GinkgoHelper()
	run := &openchoreov1alpha1.WorkflowRun{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, run)).To(Succeed())
	typ, reason := workflowrun.ConditionWorkflowSucceeded, workflowrun.ReasonWorkflowSucceeded
	if !succeeded {
		typ, reason = workflowrun.ConditionWorkflowFailed, workflowrun.ReasonWorkflowFailed
	}
	apimeta.SetStatusCondition(&run.Status.Conditions, metav1.Condition{
		Type: string(typ), Status: metav1.ConditionTrue, Reason: string(reason), Message: "done",
	})
	apimeta.SetStatusCondition(&run.Status.Conditions, metav1.Condition{
		Type: string(workflowrun.ConditionWorkflowCompleted), Status: metav1.ConditionTrue, Reason: string(reason), Message: "done",
	})
	run.Status.Tasks = tasks
	Expect(k8sClient.Status().Update(ctx, run)).To(Succeed())
}

func releaseExists(name string) bool {
	GinkgoHelper()
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &openchoreov1alpha1.RenderedRelease{})
	return err == nil
}

// ─── Specs ────────────────────────────────────────────────────────────────────

var _ = Describe("ReleaseBinding deployment hooks", func() {
	const (
		project  = "hooks-proj"
		compName = "hooks-comp"
		envName  = "hooks-prod"
		dpName   = "hooks-dp"
		rbName   = "rb-hooks"
		crName   = "cr-hooks"
	)
	req := reconcileRequest(rbName)
	releaseName := compName + "-" + envName

	// createDeps creates the component-side dependencies and the environment
	// carrying the hooks; the workflow-side objects (plane, workflow, hook) are
	// created separately so a spec can leave the plane out.
	createDeps := func(hooks *openchoreov1alpha1.HookSet, withPlane bool) {
		GinkgoHelper()
		Expect(k8sClient.Create(ctx, crFixture(crName, project, compName))).To(Succeed())
		Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
		Expect(k8sClient.Create(ctx, envWithHooks(envName, dpName, hooks))).To(Succeed())
		Expect(k8sClient.Create(ctx, componentFixture(compName, project))).To(Succeed())
		Expect(k8sClient.Create(ctx, projectFixture(project))).To(Succeed())
		Expect(k8sClient.Create(ctx, clusterWorkflowFixture())).To(Succeed())
		Expect(k8sClient.Create(ctx, clusterHookFixture())).To(Succeed())
		if withPlane {
			Expect(k8sClient.Create(ctx, clusterWorkflowPlaneFixture())).To(Succeed())
		}
		Expect(k8sClient.Create(ctx, rbFixture(rbName, project, compName, envName, crName, true))).To(Succeed())
	}

	AfterEach(func() {
		forceDelete(rbName)
		forceDeleteRelease(releaseName)
		_ = k8sClient.DeleteAllOf(ctx, &openchoreov1alpha1.WorkflowRun{}, client.InNamespace(ns),
			client.MatchingLabels{labels.LabelKeyWorkflowPurpose: labels.LabelValueWorkflowPurposeDeploymentHook})
		for _, obj := range []client.Object{
			&openchoreov1alpha1.ComponentRelease{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName}},
			&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName}},
			&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName}},
			&openchoreov1alpha1.Component{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName}},
			&openchoreov1alpha1.Project{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: project}},
			&openchoreov1alpha1.ClusterWorkflow{ObjectMeta: metav1.ObjectMeta{Name: hookWorkflowName}},
			&openchoreov1alpha1.ClusterHook{ObjectMeta: metav1.ObjectMeta{Name: hookName}},
			&openchoreov1alpha1.ClusterWorkflowPlane{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		} {
			_ = k8sClient.Delete(ctx, obj)
		}
	})

	// Hooks are always on, so an Environment without spec.hooks is the safety net:
	// it must be completely inert, so existing installs upgrade without behavior change.
	Context("when the environment binds no hooks", func() {
		It("renders immediately and records no gate", func() {
			createDeps(nil, true)

			result := mustReconcile(hookReconciler(), req)
			Expect(result.Requeue).To(BeTrue())
			Expect(releaseExists(releaseName)).To(BeTrue())
			rb := fetchRB(rbName)
			Expect(rb.Status.Gate).To(BeNil())
			Expect(conditionFor(rb, string(ConditionPreDeployHooksPassed))).To(BeNil())
			Expect(hookRuns()).To(BeEmpty())
		})
	})

	// The core promise: a Sync pre-deploy hook holds the RenderedRelease back until
	// its WorkflowRun succeeds, and the binding tells the developer exactly why.
	Context("with a Sync pre-deploy hook that succeeds", func() {
		It("blocks rendering, then renders and records the pass", func() {
			createDeps(&openchoreov1alpha1.HookSet{PreDeploy: []openchoreov1alpha1.HookBinding{
				hookBinding("scan", openchoreov1alpha1.HookModeSync, openchoreov1alpha1.HookFailurePolicyBlock)}}, true)
			r := hookReconciler()

			By("first reconcile: hook starts, nothing is rendered")
			result := mustReconcile(r, req)
			Expect(result.RequeueAfter).To(Equal(hookRequeueInterval))
			Expect(releaseExists(releaseName)).To(BeFalse())

			rb := fetchRB(rbName)
			Expect(rb.Status.Gate).NotTo(BeNil())
			Expect(rb.Status.Gate.Key).NotTo(BeEmpty())
			Expect(rb.Status.Gate.PassedKey).To(BeEmpty())
			Expect(rb.Status.Gate.PreDeploy).To(HaveLen(1))
			entry := rb.Status.Gate.PreDeploy[0]
			Expect(entry.Phase).To(Equal(openchoreov1alpha1.HookPhaseRunning))
			Expect(entry.Attempt).To(Equal(int32(1)))
			Expect(entry.StartedAt).NotTo(BeNil())
			cond := conditionFor(rb, string(ConditionPreDeployHooksPassed))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonHooksRunning)))
			Expect(conditionFor(rb, string(ConditionReady)).Reason).To(Equal(string(ReasonHooksRunning)))

			By("the WorkflowRun carries the hook labels, owner reference and resolved parameters")
			runs := hookRuns()
			Expect(runs).To(HaveLen(1))
			run := runs[0]
			Expect(run.Name).To(Equal(entry.WorkflowRunRef))
			Expect(run.Name).To(HavePrefix("scan-pre-" + hookWorkflowName + "-"))
			Expect(run.Labels).To(HaveKeyWithValue(labels.LabelKeyHook, "scan"))
			Expect(run.Labels).To(HaveKeyWithValue(labels.LabelKeyHookPhase, "preDeploy"))
			Expect(run.Labels).To(HaveKeyWithValue(labels.LabelKeyReleaseBinding, rbName))
			Expect(run.OwnerReferences).To(HaveLen(1))
			Expect(run.OwnerReferences[0].Kind).To(Equal("ReleaseBinding"))
			Expect(run.Spec.Workflow.Name).To(Equal(hookWorkflowName))
			var params map[string]string
			Expect(json.Unmarshal(run.Spec.Workflow.Parameters.Raw, &params)).To(Succeed())
			Expect(params).To(Equal(map[string]string{"image": "nginx:latest", "severity": "CRITICAL"}))

			By("the hook succeeds: the release renders and the key is recorded as passed")
			finishRun(run.Name, true)
			result = mustReconcile(r, req)
			Expect(result.Requeue).To(BeTrue(), "release creation requests a requeue")
			Expect(releaseExists(releaseName)).To(BeTrue())
			rb = fetchRB(rbName)
			Expect(rb.Status.Gate.PassedKey).To(Equal(rb.Status.Gate.Key))
			Expect(rb.Status.Gate.LastPassedRelease).To(Equal(crName))
			Expect(rb.Status.Gate.History).To(HaveLen(1))
			Expect(rb.Status.Gate.PreDeploy[0].Phase).To(Equal(openchoreov1alpha1.HookPhaseSucceeded))
			Expect(rb.Status.Gate.PreDeploy[0].FinishedAt).NotTo(BeNil())
			Expect(conditionFor(rb, string(ConditionPreDeployHooksPassed)).Status).To(Equal(metav1.ConditionTrue))
			Expect(conditionFor(rb, string(ConditionPreDeployHooksPassed)).Reason).To(Equal(string(ReasonHooksPassed)))

			By("a further reconcile does not start the hook again")
			mustReconcile(r, req)
			Expect(hookRuns()).To(HaveLen(1))
		})
	})

	// Every release change is a deployment the hooks must see, including a pin
	// moved back to a release that already passed: that release may be deployed
	// into a different world than when it last passed (a new CVE, changed data).
	Context("when the pin moves to another release and back", func() {
		It("runs the pre-deploy hook again for each release change", func() {
			const otherCR = "cr-hooks-other"
			createDeps(&openchoreov1alpha1.HookSet{PreDeploy: []openchoreov1alpha1.HookBinding{
				hookBinding("scan", openchoreov1alpha1.HookModeSync, openchoreov1alpha1.HookFailurePolicyBlock)}}, true)
			Expect(k8sClient.Create(ctx, crFixture(otherCR, project, compName))).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: otherCR}})
			})
			r := hookReconciler()

			pinAndPass := func(release string, wantRuns int) string {
				GinkgoHelper()
				rb := fetchRB(rbName)
				if rb.Spec.ReleaseName != release {
					rb.Spec.ReleaseName = release
					Expect(k8sClient.Update(ctx, rb)).To(Succeed())
				}
				mustReconcile(r, req)
				rb = fetchRB(rbName)
				Expect(rb.Status.Gate.PassedKey).NotTo(Equal(rb.Status.Gate.Key), "release %s must be gated, not waved through", release)
				Expect(hookRuns()).To(HaveLen(wantRuns))
				run := rb.Status.Gate.PreDeploy[0].WorkflowRunRef
				finishRun(run, true)
				mustReconcile(r, req)
				Expect(fetchRB(rbName).Status.Gate.PassedKey).To(Equal(fetchRB(rbName).Status.Gate.Key))
				return run
			}

			first := pinAndPass(crName, 1)
			pinAndPass(otherCR, 2)
			again := pinAndPass(crName, 3)
			Expect(again).NotTo(Equal(first), "the return to a passed release must not reuse its earlier run")
		})
	})

	// Failure must block with the workflow's own words, and an operator must be
	// able to re-run a single hook through the retry annotation without touching
	// the environment or the release.
	Context("with a Sync pre-deploy hook that fails", func() {
		It("blocks with the task message and re-runs on the retry annotation", func() {
			createDeps(&openchoreov1alpha1.HookSet{PreDeploy: []openchoreov1alpha1.HookBinding{
				hookBinding("scan", openchoreov1alpha1.HookModeSync, openchoreov1alpha1.HookFailurePolicyBlock)}}, true)
			r := hookReconciler()
			mustReconcile(r, req)
			first := hookRuns()
			Expect(first).To(HaveLen(1))

			finishRun(first[0].Name, false, openchoreov1alpha1.WorkflowTask{Name: "scan", Phase: "Failed", Message: "3 CRITICAL vulnerabilities"})
			result := mustReconcile(r, req)
			Expect(result.RequeueAfter).To(BeZero(), "a blocked gate waits for an operator, not a timer")
			Expect(releaseExists(releaseName)).To(BeFalse())
			rb := fetchRB(rbName)
			Expect(rb.Status.Gate.PreDeploy[0].Phase).To(Equal(openchoreov1alpha1.HookPhaseFailed))
			cond := conditionFor(rb, string(ConditionPreDeployHooksPassed))
			Expect(cond.Reason).To(Equal(string(ReasonHookFailed)))
			Expect(cond.Message).To(ContainSubstring(`task "scan" failed: 3 CRITICAL vulnerabilities`))
			Expect(conditionFor(rb, string(ConditionReady)).Reason).To(Equal(string(ReasonHookFailed)))

			By("retry annotation: the reset is applied and the gate re-enters HooksRunning")
			firstUID := first[0].UID
			rb.Annotations = map[string]string{labels.AnnotationKeyHookRetry: "preDeploy/scan"}
			Expect(k8sClient.Update(ctx, rb)).To(Succeed())
			result = mustReconcile(r, req)
			Expect(result.RequeueAfter).To(Equal(hookRequeueInterval))
			rb = fetchRB(rbName)
			Expect(rb.Annotations).To(HaveKeyWithValue(labels.AnnotationKeyHookRetry, "preDeploy/scan#2"),
				"the annotation stays until the reset is stored")
			Expect(rb.Status.Gate.PreDeploy[0].Attempt).To(Equal(int32(2)))
			Expect(rb.Status.Gate.PreDeploy[0].Phase).To(Equal(openchoreov1alpha1.HookPhaseRunning))
			Expect(conditionFor(rb, string(ConditionPreDeployHooksPassed)).Reason).To(Equal(string(ReasonHooksRunning)))
			Expect(hookRuns()).To(HaveLen(2), "the replaced run is kept until the reset is stored")

			By("next reconcile: the stored reset lets the old run go and consumes the annotation")
			mustReconcile(r, req)
			rb = fetchRB(rbName)
			Expect(rb.Annotations).NotTo(HaveKey(labels.AnnotationKeyHookRetry))
			second := hookRuns()
			Expect(second).To(HaveLen(1))
			Expect(second[0].UID).NotTo(Equal(firstUID), "retry must create a fresh WorkflowRun")
		})
	})

	// An ignored failure must not block, but it must not read as a clean pass
	// either: the condition names the hook so nobody mistakes a failed scan for
	// a green one.
	Context("with a Sync pre-deploy hook that fails with onFailure Ignore", func() {
		It("renders the release and reports the ignored failure by name", func() {
			createDeps(&openchoreov1alpha1.HookSet{PreDeploy: []openchoreov1alpha1.HookBinding{
				hookBinding("scan", openchoreov1alpha1.HookModeSync, openchoreov1alpha1.HookFailurePolicyIgnore)}}, true)
			r := hookReconciler()
			mustReconcile(r, req)
			runs := hookRuns()
			Expect(runs).To(HaveLen(1))
			Expect(releaseExists(releaseName)).To(BeFalse(), "a Sync hook is awaited even when its failure is ignored")

			finishRun(runs[0].Name, false, openchoreov1alpha1.WorkflowTask{Name: "scan", Phase: "Failed", Message: "1 CRITICAL vulnerability"})
			mustReconcile(r, req)
			Expect(releaseExists(releaseName)).To(BeTrue())
			rb := fetchRB(rbName)
			Expect(rb.Status.Gate.PreDeploy[0].Phase).To(Equal(openchoreov1alpha1.HookPhaseFailed))
			Expect(rb.Status.Gate.PassedKey).To(Equal(rb.Status.Gate.Key))
			cond := conditionFor(rb, string(ConditionPreDeployHooksPassed))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonHooksPassedWithIgnoredFailures)))
			Expect(cond.Message).To(Equal("Pre-deploy hooks passed; ignored failures: scan"))
		})
	})

	// Async hooks are fire-and-forget: they must never hold the release, so a
	// notification hook cannot become an outage.
	Context("with an Async pre-deploy hook", func() {
		It("dispatches the run and renders in the same reconcile", func() {
			createDeps(&openchoreov1alpha1.HookSet{PreDeploy: []openchoreov1alpha1.HookBinding{
				hookBinding("notify", openchoreov1alpha1.HookModeAsync, "")}}, true)
			result := mustReconcile(hookReconciler(), req)
			Expect(result.Requeue).To(BeTrue())
			Expect(releaseExists(releaseName)).To(BeTrue())
			rb := fetchRB(rbName)
			Expect(rb.Status.Gate.PassedKey).To(Equal(rb.Status.Gate.Key))
			Expect(rb.Status.Gate.PreDeploy[0].Phase).To(Equal(openchoreov1alpha1.HookPhaseDispatched))
			Expect(hookRuns()).To(HaveLen(1))
		})
	})

	// Fail closed: a Sync hook whose plane is missing cannot be silently skipped,
	// or a required scan would be bypassed by a misconfigured plane.
	Context("when the workflow plane is missing", func() {
		It("blocks with PlaneUnavailable, creates no run, and starts the hook once the plane exists", func() {
			createDeps(&openchoreov1alpha1.HookSet{PreDeploy: []openchoreov1alpha1.HookBinding{
				hookBinding("scan", openchoreov1alpha1.HookModeSync, openchoreov1alpha1.HookFailurePolicyBlock)}}, false)
			r := hookReconciler()
			result := mustReconcile(r, req)
			Expect(releaseExists(releaseName)).To(BeFalse())
			Expect(hookRuns()).To(BeEmpty())
			rb := fetchRB(rbName)
			Expect(rb.Status.Gate.PreDeploy[0].Phase).To(Equal(openchoreov1alpha1.HookPhasePlaneUnavailable))
			Expect(conditionFor(rb, string(ConditionReady)).Reason).To(Equal(string(ReasonPlaneUnavailable)))
			// No watch covers workflow planes, so only the requeue brings the
			// binding back once the plane is created.
			Expect(result.RequeueAfter).To(Equal(hookRequeueInterval))

			By("the plane appears: the next reconcile starts the hook")
			Expect(k8sClient.Create(ctx, clusterWorkflowPlaneFixture())).To(Succeed())
			mustReconcile(r, req)
			Expect(hookRuns()).To(HaveLen(1))
			Expect(fetchRB(rbName).Status.Gate.PreDeploy[0].Phase).To(Equal(openchoreov1alpha1.HookPhaseRunning))
		})
	})

	// Post-deploy Alert: the release stays up, Ready degrades until an operator
	// acknowledges that exact attempt (the key), so a stale acknowledgement never
	// silences a later failure.
	Context("with a Sync post-deploy hook using onFailure=Alert", func() {
		It("runs after ResourcesReady, degrades Ready on failure, and clears on acknowledge", func() {
			createDeps(&openchoreov1alpha1.HookSet{PostDeploy: []openchoreov1alpha1.HookBinding{
				hookBinding("smoke", openchoreov1alpha1.HookModeSync, openchoreov1alpha1.HookFailurePolicyAlert)}}, true)
			r := hookReconciler()

			By("no pre-deploy hooks: the release renders at once")
			result := mustReconcile(r, req)
			Expect(result.Requeue).To(BeTrue())
			Expect(releaseExists(releaseName)).To(BeTrue())
			mustReconcile(r, req)
			Expect(hookRuns()).To(BeEmpty(), "post-deploy hooks wait for ResourcesReady")

			By("the data plane reports the workload healthy")
			rel := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: releaseName}, rel)).To(Succeed())
			rel.Status.Resources = []openchoreov1alpha1.RenderedManifestStatus{{
				ID: "deployment", Group: "apps", Version: "v1", Kind: "Deployment", Name: "test-deployment",
				HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
			}}
			Expect(k8sClient.Status().Update(ctx, rel)).To(Succeed())

			result = mustReconcile(r, req)
			Expect(result.RequeueAfter).To(Equal(hookRequeueInterval))
			rb := fetchRB(rbName)
			Expect(conditionFor(rb, string(ConditionResourcesReady)).Status).To(Equal(metav1.ConditionTrue))
			Expect(rb.Status.Gate.PostDeployKey).To(Equal(rb.Status.Gate.Key))
			Expect(rb.Status.Gate.PostDeploy).To(HaveLen(1))
			Expect(rb.Status.Gate.PostDeploy[0].Phase).To(Equal(openchoreov1alpha1.HookPhaseRunning))
			runs := hookRuns()
			Expect(runs).To(HaveLen(1))
			Expect(runs[0].Labels).To(HaveKeyWithValue(labels.LabelKeyHookPhase, "postDeploy"))

			By("the smoke test fails: Ready degrades but the release stays")
			finishRun(runs[0].Name, false)
			mustReconcile(r, req)
			Expect(releaseExists(releaseName)).To(BeTrue())
			rb = fetchRB(rbName)
			cond := conditionFor(rb, string(ConditionPostDeployHooksPassed))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonPostDeployHookFailed)))
			Expect(conditionFor(rb, string(ConditionReady)).Reason).To(Equal(string(ReasonPostDeployHookFailed)))

			By("a stale acknowledgement is ignored")
			rb.Annotations = map[string]string{labels.AnnotationKeyGateAcknowledged: "not-this-key"}
			Expect(k8sClient.Update(ctx, rb)).To(Succeed())
			mustReconcile(r, req)
			rb = fetchRB(rbName)
			Expect(conditionFor(rb, string(ConditionReady)).Reason).To(Equal(string(ReasonPostDeployHookFailed)))

			By("acknowledging the current key restores Ready")
			rb.Annotations[labels.AnnotationKeyGateAcknowledged] = rb.Status.Gate.Key
			Expect(k8sClient.Update(ctx, rb)).To(Succeed())
			mustReconcile(r, req)
			rb = fetchRB(rbName)
			Expect(conditionFor(rb, string(ConditionPostDeployHooksPassed)).Reason).To(Equal(string(ReasonPostDeployHooksAcknowledged)))
			Expect(conditionFor(rb, string(ConditionReady)).Status).To(Equal(metav1.ConditionTrue))
		})
	})

	// Timeouts are enforced from the hook's own StartedAt so a hung approval
	// cannot hold a release forever; the bound is the binding's timeout.
	Context("with a Sync pre-deploy hook past its timeout", func() {
		It("marks the hook TimedOut and blocks", func() {
			b := hookBinding("scan", openchoreov1alpha1.HookModeSync, openchoreov1alpha1.HookFailurePolicyBlock)
			b.Timeout = "1s"
			createDeps(&openchoreov1alpha1.HookSet{PreDeploy: []openchoreov1alpha1.HookBinding{b}}, true)
			r := hookReconciler()
			mustReconcile(r, req)

			rb := fetchRB(rbName)
			past := metav1.NewTime(time.Now().Add(-time.Minute))
			rb.Status.Gate.PreDeploy[0].StartedAt = &past
			Expect(k8sClient.Status().Update(ctx, rb)).To(Succeed())

			mustReconcile(r, req)
			rb = fetchRB(rbName)
			Expect(rb.Status.Gate.PreDeploy[0].Phase).To(Equal(openchoreov1alpha1.HookPhaseTimedOut))
			Expect(conditionFor(rb, string(ConditionReady)).Reason).To(Equal(string(ReasonHookTimedOut)))
			Expect(releaseExists(releaseName)).To(BeFalse())
		})
	})
})
