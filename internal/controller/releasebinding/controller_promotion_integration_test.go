// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("ReleaseBinding promotion paths", func() {
	const (
		pipelineName = "promo-pipeline"
		projName     = "promo-proj"
		compName     = "promo-comp"
		dpName       = "promo-dp"
		devEnv       = "promo-development"
		stagingEnv   = "promo-staging"
		prodEnv      = "promo-production"
		releaseA     = "promo-release-a"
		releaseB     = "promo-release-b"
		stagingRB    = "promo-comp-staging"
		prodRB       = "promo-comp-production"
	)
	prodRendered := compName + "-" + prodEnv

	syncedCondition := func(rb *openchoreov1alpha1.ReleaseBinding) *metav1.Condition {
		return conditionFor(rb, string(ConditionReleaseSynced))
	}
	isSynced := func(rb *openchoreov1alpha1.ReleaseBinding) bool {
		cond := syncedCondition(rb)
		return cond != nil && cond.Status == metav1.ConditionTrue
	}
	isBlocked := func(rb *openchoreov1alpha1.ReleaseBinding) bool {
		cond := syncedCondition(rb)
		return cond != nil && cond.Reason == string(ReasonPromotionPathNotSatisfied)
	}

	BeforeEach(func() {
		pipeline := &openchoreov1alpha1.DeploymentPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: pipelineName, Namespace: ns},
			Spec: openchoreov1alpha1.DeploymentPipelineSpec{
				PromotionPaths: []openchoreov1alpha1.PromotionPath{
					promotionPath(devEnv, stagingEnv),
					promotionPath(stagingEnv, prodEnv),
				},
			},
		}
		project := projectFixture(projName)
		project.Spec.DeploymentPipelineRef.Name = pipelineName

		for _, obj := range []client.Object{
			pipeline,
			project,
			componentFixture(compName, projName),
			dpFixture(dpName),
			envFixture(devEnv, dpName),
			envFixture(stagingEnv, dpName),
			envFixture(prodEnv, dpName),
			crFixture(releaseA, projName, compName),
			crFixture(releaseB, projName, compName),
		} {
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		}
	})

	AfterEach(func() {
		forceDelete(stagingRB)
		forceDelete(prodRB)
		forceDeleteRelease(prodRendered)
		for _, obj := range []client.Object{
			&openchoreov1alpha1.DeploymentPipeline{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: pipelineName}},
			&openchoreov1alpha1.Project{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName}},
			&openchoreov1alpha1.Component{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName}},
			&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName}},
			&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: devEnv}},
			&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: stagingEnv}},
			&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: prodEnv}},
			&openchoreov1alpha1.ComponentRelease{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: releaseA}},
			&openchoreov1alpha1.ComponentRelease{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: releaseB}},
		} {
			_ = k8sClient.Delete(ctx, obj)
		}
	})

	It("holds a release that has not reached the source environment", func() {
		r := testReconcilerWithCachedClient()
		Expect(k8sClient.Create(ctx, rbFixture(prodRB, projName, compName, prodEnv, releaseA, true))).To(Succeed())

		rb := reconcileUntil(r, prodRB, isBlocked)
		cond := syncedCondition(rb)
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Message).To(ContainSubstring("must be referenced by a ReleaseBinding in " + stagingEnv))

		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: prodRendered}, &openchoreov1alpha1.RenderedRelease{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "a blocked binding must not render a release")
	})

	It("deploys the release once the source environment has it", func() {
		r := testReconcilerWithCachedClient()
		Expect(k8sClient.Create(ctx, rbFixture(stagingRB, projName, compName, stagingEnv, releaseA, true))).To(Succeed())
		Expect(k8sClient.Create(ctx, rbFixture(prodRB, projName, compName, prodEnv, releaseA, true))).To(Succeed())

		reconcileUntil(r, prodRB, isSynced)

		rendered := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: prodRendered}, rendered)).To(Succeed())
		Expect(rendered.Labels).To(HaveKeyWithValue(labels.LabelKeyComponentReleaseName, releaseA))
	})

	It("allows promotion when an undeployed source binding references the release", func() {
		r := testReconcilerWithCachedClient()
		staging := rbFixture(stagingRB, projName, compName, stagingEnv, releaseA, true)
		staging.Spec.State = openchoreov1alpha1.ReleaseStateUndeploy
		Expect(k8sClient.Create(ctx, staging)).To(Succeed())
		Expect(k8sClient.Create(ctx, rbFixture(prodRB, projName, compName, prodEnv, releaseA, true))).To(Succeed())

		reconcileUntil(r, prodRB, isSynced)
		rendered := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: prodRendered}, rendered)).To(Succeed())
		Expect(rendered.Labels).To(HaveKeyWithValue(labels.LabelKeyComponentReleaseName, releaseA))
	})

	It("keeps a deployed release after the source environment moves on", func() {
		r := testReconcilerWithCachedClient()
		Expect(k8sClient.Create(ctx, rbFixture(stagingRB, projName, compName, stagingEnv, releaseA, true))).To(Succeed())
		Expect(k8sClient.Create(ctx, rbFixture(prodRB, projName, compName, prodEnv, releaseA, true))).To(Succeed())
		reconcileUntil(r, prodRB, isSynced)

		By("Moving staging to a newer release")
		staging := fetchRB(stagingRB)
		staging.Spec.ReleaseName = releaseB
		Expect(k8sClient.Update(ctx, staging)).To(Succeed())
		Eventually(func(g Gomega) {
			cached := &openchoreov1alpha1.ReleaseBinding{}
			g.Expect(k8sCachedClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: stagingRB}, cached)).To(Succeed())
			g.Expect(cached.Spec.ReleaseName).To(Equal(releaseB))
		}, timeout, interval).Should(Succeed())

		By("Reconciling production, which still runs the release staging had")
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: prodRB}})
		Expect(err).NotTo(HaveOccurred())
		Expect(isSynced(fetchRB(prodRB))).To(BeTrue())
	})

	It("allows undeploying a release that never reached the source environment", func() {
		r := testReconcilerWithCachedClient()
		rb := rbFixture(prodRB, projName, compName, prodEnv, releaseA, true)
		rb.Spec.State = openchoreov1alpha1.ReleaseStateUndeploy
		Expect(k8sClient.Create(ctx, rb)).To(Succeed())

		rb = reconcileUntil(r, prodRB, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
			return syncedCondition(rb) != nil
		})
		Expect(syncedCondition(rb).Reason).To(Equal(string(ReasonResourcesUndeployed)))
	})

	It("undeploys an existing release after its pipeline is deleted", func() {
		r := testReconcilerWithCachedClient()
		Expect(k8sClient.Create(ctx, rbFixture(stagingRB, projName, compName, stagingEnv, releaseA, true))).To(Succeed())
		Expect(k8sClient.Create(ctx, rbFixture(prodRB, projName, compName, prodEnv, releaseA, true))).To(Succeed())
		reconcileUntil(r, prodRB, isSynced)

		pipeline := &openchoreov1alpha1.DeploymentPipeline{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: pipelineName}}
		Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sCachedClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: pipelineName}, pipeline))
		}, timeout, interval).Should(BeTrue())

		rb := fetchRB(prodRB)
		rb.Spec.State = openchoreov1alpha1.ReleaseStateUndeploy
		Expect(k8sClient.Update(ctx, rb)).To(Succeed())
		Eventually(func(g Gomega) {
			cached := &openchoreov1alpha1.ReleaseBinding{}
			g.Expect(k8sCachedClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: prodRB}, cached)).To(Succeed())
			g.Expect(cached.Spec.State).To(Equal(openchoreov1alpha1.ReleaseStateUndeploy))
		}, timeout, interval).Should(Succeed())

		reconcileUntil(r, prodRB, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
			cond := syncedCondition(rb)
			return cond != nil && cond.Reason == string(ReasonResourcesUndeployed)
		})
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: prodRendered}, &openchoreov1alpha1.RenderedRelease{}))
		}, timeout, interval).Should(BeTrue())
	})

	It("re-queues sibling bindings and the bindings of a changed pipeline", func() {
		r := testReconcilerWithCachedClient()
		Expect(k8sClient.Create(ctx, rbFixture(stagingRB, projName, compName, stagingEnv, releaseA, true))).To(Succeed())
		Expect(k8sClient.Create(ctx, rbFixture(prodRB, projName, compName, prodEnv, releaseA, true))).To(Succeed())
		stagingReq := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: stagingRB}}
		prodReq := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: prodRB}}

		Eventually(func() []reconcile.Request {
			return r.findSiblingReleaseBindings(ctx, fetchRB(stagingRB))
		}, timeout, interval).Should(ConsistOf(prodReq))

		pipeline := &openchoreov1alpha1.DeploymentPipeline{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: pipelineName}, pipeline)).To(Succeed())
		Eventually(func() []reconcile.Request {
			return r.findReleaseBindingsForDeploymentPipeline(ctx, pipeline)
		}, timeout, interval).Should(ConsistOf(stagingReq, prodReq))
	})
})
