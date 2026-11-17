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

	// observe runs the controller's health observation against the stored
	// binding and returns the requeue it asks for.
	observe := func() time.Duration {
		GinkgoHelper()
		cr := &openchoreov1alpha1.ComponentRelease{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: crName}, cr)).To(Succeed())
		requeueAfter, err := r.observeRevisionHealth(ctx, r.revisionSnapshotter(nil), fetchRB(rbName), cr)
		Expect(err).NotTo(HaveOccurred())
		return requeueAfter
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
		Expect(requeue).To(BeNumerically("<=", window), "requeue for the rest of the window instead of sleeping")

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

	It("keeps the only known-good revision when 12 revisions exceed a limit of 9 old revisions", func() {
		r.RevisionKnownGoodWindow = 0
		// Nine old revisions and the active one: ten in all.
		r.RevisionHistoryLimit = 9

		By("r1, then r2 which becomes the only known-good revision")
		reconcileRB()
		override(2)
		reconcileRB()
		markApplied()
		setResourcesReady(metav1.ConditionTrue, ReasonReady)
		observe()
		Expect(revisionNamed(2).Status.KnownGood).To(BeTrue())

		By("r3 to r12, none of which is known-good")
		for replicas := 3; replicas <= 12; replicas++ {
			override(replicas)
			reconcileRB()
		}

		revs := listRevisions(fetchRB(rbName))
		numbers := make([]int64, 0, len(revs))
		for _, rev := range revs {
			numbers = append(numbers, rev.Spec.Revision)
		}
		Expect(numbers).To(Equal([]int64{2, 4, 5, 6, 7, 8, 9, 10, 11, 12}),
			"r1 and r3 are evicted; r2 survives as the only known-good revision")
		Expect(revs[0].Status.KnownGood).To(BeTrue())
		Expect(revs[len(revs)-1].Status.Active).To(BeTrue())
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
		Expect(result.RequeueAfter).To(BeNumerically("<=", window))

		By("reconciling again once the window has passed")
		time.Sleep(result.RequeueAfter) // the test stands in for the workqueue's delayed requeue
		Eventually(func(g Gomega) {
			res := mustReconcile(r, req)
			g.Expect(revisionNamed(1).Status.KnownGood).To(BeTrue())
			g.Expect(res.RequeueAfter).To(BeZero())
		}, 2*window, 200*time.Millisecond).Should(Succeed())
	})
})
