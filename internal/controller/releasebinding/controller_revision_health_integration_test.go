// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
	"github.com/openchoreo/openchoreo/internal/revision"
)

// These specs drive revision health from the binding's ResourcesReady condition,
// set directly on the stored binding. Reconcile recomputes ResourcesReady from
// the RenderedRelease on every pass (setResourcesReadyStatus), so a directly set
// condition only survives until the next Reconcile: revisions are recorded with
// Reconcile, and health is then observed with observeRevisionHealth -- the
// function Reconcile itself calls -- against the stored binding. The last spec
// goes through Reconcile end to end instead.
var _ = Describe("ReleaseBinding Controller — revision health", func() {
	const (
		project  = "health-proj"
		compName = "health-comp"
		envName  = "health-env"
		dpName   = "health-dp"
		rbName   = "health-comp-health-env"
		crName   = "health-cr"
		rrName   = compName + "-" + envName // makeDataPlaneReleaseName

		window = 2 * time.Second
	)
	req := reconcileRequest(rbName)

	var r *Reconciler

	// reconcileRB records the binding's current spec as a revision.
	reconcileRB := func() {
		GinkgoHelper()
		mustReconcile(r, req)
		mustReconcile(r, req)
	}

	// revisionNamed returns the binding's revision numbered n.
	revisionNamed := func(n int64) *openchoreov1alpha1.ReleaseBindingRevision {
		GinkgoHelper()
		var list openchoreov1alpha1.ReleaseBindingRevisionList
		Expect(k8sClient.List(ctx, &list, client.InNamespace(ns),
			client.MatchingLabels{revision.LabelKeyReleaseBinding: rbName})).To(Succeed())
		for i := range list.Items {
			if list.Items[i].Spec.Revision == n {
				return &list.Items[i]
			}
		}
		Fail(fmt.Sprintf("binding %q has no revision %d", rbName, n))
		return nil
	}

	// markApplied reports the RenderedRelease's current generation as applied,
	// as the renderedrelease controller (not running in envtest) would, so its
	// resource health counts as current.
	markApplied := func() {
		GinkgoHelper()
		rr := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: rrName}, rr)).To(Succeed())
		apimeta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
			Type:               renderedrelease.ConditionResourcesApplied,
			Status:             metav1.ConditionTrue,
			Reason:             "Applied",
			ObservedGeneration: rr.Generation,
		})
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())
	}

	// setResourcesReady writes the ResourcesReady condition straight onto the
	// stored binding, for its current generation.
	setResourcesReady := func(status metav1.ConditionStatus, reason controller.ConditionReason) {
		GinkgoHelper()
		rb := fetchRB(rbName)
		apimeta.SetStatusCondition(&rb.Status.Conditions, metav1.Condition{
			Type:               string(ConditionResourcesReady),
			Status:             status,
			Reason:             string(reason),
			Message:            "set by test",
			ObservedGeneration: rb.Generation,
		})
		Expect(k8sClient.Status().Update(ctx, rb)).To(Succeed())
	}

	// observeWith runs the controller's health observation against the stored
	// binding, judging it from rendered (nil reads the stored RenderedRelease),
	// and returns the requeue it asks for.
	observeWith := func(rendered *openchoreov1alpha1.RenderedRelease) time.Duration {
		GinkgoHelper()
		cr := &openchoreov1alpha1.ComponentRelease{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: crName}, cr)).To(Succeed())
		requeueAfter, err := r.observeRevisionHealth(ctx, r.revisionSnapshotter(), fetchRB(rbName), cr, rendered)
		Expect(err).NotTo(HaveOccurred())
		return requeueAfter
	}
	// observe runs it as a reconcile that rendered would: judging health from
	// the stored RenderedRelease it read or wrote.
	observe := func() time.Duration {
		GinkgoHelper()
		stored := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: rrName}, stored)).To(Succeed())
		return observeWith(stored)
	}

	override := func(replicas int) {
		GinkgoHelper()
		rb := fetchRB(rbName)
		rb.Spec.ComponentTypeEnvironmentConfigs = replicasOverride(fmt.Sprint(replicas))
		Expect(k8sClient.Update(ctx, rb)).To(Succeed())
	}

	BeforeEach(func() {
		r = testReconcilerWithPipeline()
		r.RevisionKnownGoodWindow = window
		Expect(k8sClient.Create(ctx, revisionCRFixture(crName, project, compName))).To(Succeed())
		Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
		Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
		Expect(k8sClient.Create(ctx, componentFixture(compName, project))).To(Succeed())
		Expect(k8sClient.Create(ctx, projectFixture(project))).To(Succeed())
		Expect(k8sClient.Create(ctx, rbFixture(rbName, project, compName, envName, crName, true))).To(Succeed())
	})

	AfterEach(func() {
		forceDelete(rbName)
		forceDeleteRelease(rrName)
		for _, obj := range []client.Object{
			&openchoreov1alpha1.ComponentRelease{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName}},
			&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName}},
			&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName}},
			&openchoreov1alpha1.Component{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName}},
			&openchoreov1alpha1.Project{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: project}},
		} {
			_ = k8sClient.Delete(ctx, obj)
		}
	})

	It("goes Progressing to Healthy, and becomes known-good only after the window", func() {
		reconcileRB()
		markApplied()

		By("ResourcesReady=False/Progressing")
		setResourcesReady(metav1.ConditionFalse, ReasonResourcesProgressing)
		Expect(observe()).To(BeZero(), "nothing to wait for while Progressing")
		r1 := revisionNamed(1)
		Expect(r1.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthProgressing))
		Expect(r1.Status.HealthySince).To(BeNil())

		By("ResourcesReady=True/Ready")
		setResourcesReady(metav1.ConditionTrue, ReasonReady)
		healthyAt := time.Now()
		requeue := observe()
		Expect(requeue).To(BeNumerically(">", 0))
		// The window counts from healthySince, stored rounded up to a whole second.
		Expect(requeue).To(BeNumerically("<=", window+time.Second), "requeue for the rest of the window instead of sleeping")

		r1 = revisionNamed(1)
		Expect(r1.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthHealthy))
		Expect(r1.Status.HealthySince).NotTo(BeNil())
		Expect(r1.Status.ObservedAt).NotTo(BeNil())
		Expect(r1.Status.KnownGood).To(BeFalse(), "not known-good before the window has passed")

		By("staying Healthy until the window has passed")
		Eventually(func(g Gomega) {
			observe()
			g.Expect(revisionNamed(1).Status.KnownGood).To(BeTrue())
		}, 4*window, 200*time.Millisecond).Should(Succeed())
		// healthySince is stored with one-second precision, so allow that much.
		Expect(time.Since(healthyAt)).To(BeNumerically(">=", window-time.Second))
		Expect(revisionNamed(1).Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthHealthy))
	})

	// The reconcile has just written a new generation of the RenderedRelease
	// (a data plane change, with the binding's generation unchanged), so
	// ResourcesReady from the last pass still reads True and a read of the
	// RenderedRelease can still return the applied generation before it. Health
	// is judged from the object the reconcile wrote, and stays Progressing.
	It("judges health from the RenderedRelease the reconcile wrote, not another read", func() {
		r.RevisionKnownGoodWindow = 0
		reconcileRB()
		markApplied()
		setResourcesReady(metav1.ConditionTrue, ReasonReady)

		stored := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: rrName}, stored)).To(Succeed())
		written := stored.DeepCopy()
		written.Generation++ // its status still reports the previous generation

		observeWith(written)
		r1 := revisionNamed(1)
		Expect(r1.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthProgressing))
		Expect(r1.Status.KnownGood).To(BeFalse(), "not known-good before the new generation is applied")

		By("the stored RenderedRelease alone would have passed it as Healthy")
		observe()
		Expect(revisionNamed(1).Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthHealthy))
	})

	It("does not report Healthy from a reconcile that did not render", func() {
		r.RevisionKnownGoodWindow = 0
		reconcileRB()
		markApplied()
		setResourcesReady(metav1.ConditionTrue, ReasonReady)

		// The reconcile returned before rendering, so its ResourcesReady was not
		// evaluated against the RenderedRelease it would read now.
		observeWith(nil)
		r1 := revisionNamed(1)
		Expect(r1.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthProgressing))
		Expect(r1.Status.KnownGood).To(BeFalse())

		By("a failure is still recorded without rendering")
		setResourcesReady(metav1.ConditionFalse, ReasonResourcesNotReady)
		observeWith(nil)
		Expect(revisionNamed(1).Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthFailed))
	})

	It("stays Progressing while a connection holds the rollout, whatever ResourcesReady still says", func() {
		r.RevisionKnownGoodWindow = 0
		reconcileRB()
		markApplied()
		setResourcesReady(metav1.ConditionTrue, ReasonReady)

		// The connections guard returned before ResourcesReady was evaluated
		// again, so the True above can describe an earlier RenderedRelease.
		rb := fetchRB(rbName)
		apimeta.SetStatusCondition(&rb.Status.Conditions, metav1.Condition{
			Type:               string(ConditionConnectionsResolved),
			Status:             metav1.ConditionFalse,
			Reason:             "ConnectionsPending",
			Message:            "set by test",
			ObservedGeneration: rb.Generation,
		})
		Expect(k8sClient.Status().Update(ctx, rb)).To(Succeed())

		observe()
		r1 := revisionNamed(1)
		Expect(r1.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthProgressing))
		Expect(r1.Status.KnownGood).To(BeFalse())

		By("once the connections resolve, ResourcesReady decides")
		rb = fetchRB(rbName)
		apimeta.SetStatusCondition(&rb.Status.Conditions, metav1.Condition{
			Type:               string(ConditionConnectionsResolved),
			Status:             metav1.ConditionTrue,
			Reason:             "AllConnectionsResolved",
			ObservedGeneration: rb.Generation,
		})
		Expect(k8sClient.Status().Update(ctx, rb)).To(Succeed())
		observe()
		Expect(revisionNamed(1).Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthHealthy))
	})

	It("keeps an undeploy Progressing until the observability RenderedRelease is gone too", func() {
		reconcileRB()

		// An observability RenderedRelease that stays while it is deleted.
		obsName := rrName + "-observability"
		dp := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: rrName}, dp)).To(Succeed())
		obs := &openchoreov1alpha1.RenderedRelease{ObjectMeta: metav1.ObjectMeta{
			Namespace: ns, Name: obsName, Finalizers: []string{"openchoreo.dev/test-hold"}}}
		obs.Spec = dp.Spec
		Expect(k8sClient.Create(ctx, obs)).To(Succeed())
		DeferCleanup(func() { forceDeleteRelease(obsName) })

		rb := fetchRB(rbName)
		rb.Spec.State = openchoreov1alpha1.ReleaseStateUndeploy
		Expect(k8sClient.Update(ctx, rb)).To(Succeed())
		reconcileRB()
		forceDeleteRelease(rrName) // the data-plane release is gone

		observeWith(nil) // an undeploy does not render
		Expect(revisionNamed(2).Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthProgressing),
			"the observability RenderedRelease is still being removed")

		forceDeleteRelease(obsName)
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: obsName}, &openchoreov1alpha1.RenderedRelease{})
			return err != nil
		}, timeout, interval).Should(BeTrue())
		observeWith(nil) // an undeploy does not render
		Expect(revisionNamed(2).Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthHealthy))
	})

	It("stays not known-good when Ready turns Degraded within the window", func() {
		reconcileRB()
		markApplied()

		setResourcesReady(metav1.ConditionTrue, ReasonReady)
		Expect(observe()).To(BeNumerically(">", 0))

		setResourcesReady(metav1.ConditionFalse, ReasonResourcesDegraded)
		Expect(observe()).To(BeZero(), "a Degraded revision has no known-good window running")
		r1 := revisionNamed(1)
		Expect(r1.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthDegraded))
		Expect(r1.Status.HealthySince).To(BeNil())

		By("observing Degraded past the end of the original window")
		Consistently(func(g Gomega) {
			observe()
			g.Expect(revisionNamed(1).Status.KnownGood).To(BeFalse())
		}, window+time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("keeps known-good true when a known-good revision turns Degraded", func() {
		r.RevisionKnownGoodWindow = 0
		reconcileRB()
		markApplied()

		setResourcesReady(metav1.ConditionTrue, ReasonReady)
		observe()
		Expect(revisionNamed(1).Status.KnownGood).To(BeTrue())

		for _, reason := range []controller.ConditionReason{ReasonResourcesDegraded, ReasonResourceApplyFailed, ReasonResourcesUnknown} {
			By(string(reason))
			setResourcesReady(metav1.ConditionFalse, reason)
			observe()
			r1 := revisionNamed(1)
			Expect(r1.Status.Health).NotTo(Equal(openchoreov1alpha1.RevisionHealthHealthy))
			Expect(r1.Status.KnownGood).To(BeTrue(), "known-good must never be written back to false")
		}
		Expect(revisionNamed(1).Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthUnknown))
	})

	It("freezes the previous revision's health when a new revision becomes active", func() {
		r.RevisionKnownGoodWindow = 0
		reconcileRB()
		markApplied()
		setResourcesReady(metav1.ConditionTrue, ReasonReady)
		observe()
		r1Before := revisionNamed(1)
		Expect(r1Before.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthHealthy))

		override(3)
		reconcileRB()

		r1 := revisionNamed(1)
		Expect(r1.Status.Active).To(BeFalse())
		Expect(r1.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthHealthy), "r1 keeps its last observed health")
		Expect(r1.Status.KnownGood).To(BeTrue())
		Expect(r1.Status.HealthySince).To(Equal(r1Before.Status.HealthySince))
		Expect(r1.Status.ObservedAt).To(Equal(r1Before.Status.ObservedAt))

		r2 := revisionNamed(2)
		Expect(r2.Status.Active).To(BeTrue())
		Expect(r2.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthProgressing))

		By("health changes on the binding now reach only r2")
		markApplied()
		setResourcesReady(metav1.ConditionFalse, ReasonResourcesDegraded)
		observe()
		Expect(revisionNamed(2).Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthDegraded))
		Expect(revisionNamed(1).Status).To(Equal(r1.Status))
	})

	It("requeues Reconcile for the rest of the known-good window", func() {
		By("recording r1, then reporting a healthy primary Deployment on the RenderedRelease")
		reconcileRB()
		rr := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: rrName}, rr)).To(Succeed())
		rr.Status.Resources = []openchoreov1alpha1.RenderedManifestStatus{{
			ID: "deployment", Group: "apps", Version: "v1", Kind: "Deployment", Name: "rev-deployment",
			HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
		}}
		apimeta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
			Type: renderedrelease.ConditionResourcesApplied, Status: metav1.ConditionTrue,
			Reason: "Applied", ObservedGeneration: rr.Generation,
		})
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		result := mustReconcile(r, req)
		Expect(conditionFor(fetchRB(rbName), string(ConditionResourcesReady)).Status).To(Equal(metav1.ConditionTrue))
		Expect(revisionNamed(1).Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthHealthy))
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(result.RequeueAfter).To(BeNumerically("<=", window+time.Second))

		By("reconciling again once the window has passed")
		time.Sleep(result.RequeueAfter) // the test stands in for the workqueue's delayed requeue
		Eventually(func(g Gomega) {
			res := mustReconcile(r, req)
			g.Expect(revisionNamed(1).Status.KnownGood).To(BeTrue())
			g.Expect(res.RequeueAfter).To(BeZero())
		}, 2*window, 200*time.Millisecond).Should(Succeed())
	})
})
