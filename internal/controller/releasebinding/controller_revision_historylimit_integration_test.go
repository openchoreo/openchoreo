// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// The revision history limit is resolved from the binding's
// spec.revisionHistoryLimit, then its Environment's
// spec.componentRevisionHistoryLimit, then the controller's. It
// counts old revisions: the active one is always kept on top of it.
var _ = Describe("ReleaseBinding Controller — revision history limit", func() {
	const (
		project  = "limit-proj"
		compName = "limit-comp"
		envName  = "limit-env"
		dpName   = "limit-dp"
		rbName   = "limit-comp-limit-env"
		crA      = "limit-cr-a"
		crB      = "limit-cr-b"
	)
	req := reconcileRequest(rbName)

	var r *Reconciler

	reconcileRB := func() {
		GinkgoHelper()
		mustReconcile(r, req)
		mustReconcile(r, req)
	}

	// override changes the binding's replicas override, a spec change that
	// records a new revision.
	override := func(replicas int) {
		GinkgoHelper()
		rb := fetchRB(rbName)
		rb.Spec.ComponentTypeEnvironmentConfigs = replicasOverride(strconv.Itoa(replicas))
		Expect(k8sClient.Update(ctx, rb)).To(Succeed())
		reconcileRB()
	}

	setBindingLimit := func(limit *int32) {
		GinkgoHelper()
		rb := fetchRB(rbName)
		rb.Spec.RevisionHistoryLimit = limit
		Expect(k8sClient.Update(ctx, rb)).To(Succeed())
	}

	setEnvironmentLimit := func(limit *int32) {
		GinkgoHelper()
		env := &openchoreov1alpha1.Environment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: envName}, env)).To(Succeed())
		env.Spec.ComponentRevisionHistoryLimit = limit
		Expect(k8sClient.Update(ctx, env)).To(Succeed())
	}

	revisionNumbers := func() []int64 {
		GinkgoHelper()
		revs := listRevisions(fetchRB(rbName))
		numbers := make([]int64, 0, len(revs))
		for _, rev := range revs {
			numbers = append(numbers, rev.Spec.Revision)
		}
		return numbers
	}

	BeforeEach(func() {
		r = testReconcilerWithPipeline()
		Expect(k8sClient.Create(ctx, revisionCRFixture(crA, project, compName))).To(Succeed())
		Expect(k8sClient.Create(ctx, revisionCRFixture(crB, project, compName))).To(Succeed())
		Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
		Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
		Expect(k8sClient.Create(ctx, componentFixture(compName, project))).To(Succeed())
		Expect(k8sClient.Create(ctx, projectFixture(project))).To(Succeed())
		Expect(k8sClient.Create(ctx, rbFixture(rbName, project, compName, envName, crA, true))).To(Succeed())
	})

	AfterEach(func() {
		forceDelete(rbName)
		forceDeleteRelease(compName + "-" + envName)
		for _, obj := range []client.Object{
			&openchoreov1alpha1.ComponentRelease{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crA}},
			&openchoreov1alpha1.ComponentRelease{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crB}},
			&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName}},
			&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName}},
			&openchoreov1alpha1.Component{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName}},
			&openchoreov1alpha1.Project{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: project}},
		} {
			_ = k8sClient.Delete(ctx, obj)
		}
	})

	It("keeps only the active revision under a controller limit of 0", func() {
		r.RevisionHistoryLimit = 0
		reconcileRB()
		override(2)
		override(3)

		Expect(revisionNumbers()).To(Equal([]int64{3}))
		Expect(fetchRB(rbName).Status.CurrentRevision).To(Equal(rbName + "-r3"))
		Expect(fetchRB(rbName).Status.HighestRevision).To(Equal(int64(3)), "numbers are not reused")
	})

	It("uses the Environment's limit when the binding sets none, and trims when it is lowered", func() {
		r.RevisionHistoryLimit = 0
		setEnvironmentLimit(ptr.To(int32(2)))
		reconcileRB()
		for replicas := 2; replicas <= 5; replicas++ {
			override(replicas)
		}
		Expect(revisionNumbers()).To(Equal([]int64{3, 4, 5}), "two old revisions and the active one")

		By("lowering the Environment's limit, which changes no binding")
		generation := fetchRB(rbName).Generation
		setEnvironmentLimit(ptr.To(int32(0)))
		reconcileRB()

		Expect(revisionNumbers()).To(Equal([]int64{5}))
		Expect(fetchRB(rbName).Generation).To(Equal(generation))
		Expect(fetchRB(rbName).Status.CurrentRevision).To(Equal(rbName + "-r5"))
	})

	It("lets the Environment's limit override the controller's", func() {
		r.RevisionHistoryLimit = 0
		setEnvironmentLimit(ptr.To(int32(10)))
		reconcileRB()
		override(2)
		override(3)

		Expect(revisionNumbers()).To(Equal([]int64{1, 2, 3}))
	})

	It("lets the binding's limit override the Environment's", func() {
		r.RevisionHistoryLimit = 0
		setEnvironmentLimit(ptr.To(int32(0)))
		setBindingLimit(ptr.To(int32(10)))
		reconcileRB()
		override(2)
		override(3)
		Expect(revisionNumbers()).To(Equal([]int64{1, 2, 3}), "the Environment's 0 does not apply")

		By("an explicit 0 on the binding overriding a higher Environment limit")
		setEnvironmentLimit(ptr.To(int32(10)))
		setBindingLimit(ptr.To(int32(0)))
		reconcileRB()
		Expect(revisionNumbers()).To(Equal([]int64{3}))
	})

	It("records no revision when only the limit changes, and no revision stores it", func() {
		setBindingLimit(ptr.To(int32(5)))
		reconcileRB()
		override(2)
		before := listRevisions(fetchRB(rbName))
		Expect(before).To(HaveLen(2))

		setBindingLimit(ptr.To(int32(7)))
		reconcileRB()
		setBindingLimit(nil)
		reconcileRB()

		after := listRevisions(fetchRB(rbName))
		Expect(after).To(HaveLen(2), "changing the limit is not a change to what is deployed")
		for i := range after {
			Expect(after[i].Spec).To(Equal(before[i].Spec))
			Expect(after[i].Spec.BindingSpec.RevisionHistoryLimit).To(BeNil())
		}
		Expect(after[1].Status.Active).To(BeTrue())
		Expect(fetchRB(rbName).Status.CurrentRevision).To(Equal(rbName + "-r2"))
	})

	It("keeps the binding's current limit through a rollback", func() {
		setBindingLimit(ptr.To(int32(5)))
		reconcileRB() // r1, release A
		rb := fetchRB(rbName)
		rb.Spec.ReleaseName = crB
		Expect(k8sClient.Update(ctx, rb)).To(Succeed())
		reconcileRB() // r2, release B

		By("changing the limit after r1 was recorded")
		setBindingLimit(ptr.To(int32(3)))
		reconcileRB()

		svc := releasebindingsvc.NewService(k8sClient, testutil.TestLogger())
		preview, err := svc.RollbackReleaseBinding(ctx, ns, rbName, bindingrevision.RollbackRequest{Revision: 1, DryRun: true})
		Expect(err).NotTo(HaveOccurred())
		for _, change := range preview.Changes {
			Expect(change.Field).NotTo(ContainSubstring("revisionHistoryLimit"), "the limit is not part of a rollback")
		}

		_, err = svc.RollbackReleaseBinding(ctx, ns, rbName, bindingrevision.RollbackRequest{Revision: 1})
		Expect(err).NotTo(HaveOccurred())

		rb = fetchRB(rbName)
		Expect(rb.Spec.ReleaseName).To(Equal(crA))
		Expect(rb.Spec.RevisionHistoryLimit).To(Equal(ptr.To(int32(3))), "the rollback leaves the limit as it is")

		reconcileRB()
		revs := listRevisions(fetchRB(rbName))
		Expect(revs).To(HaveLen(2), "no revision is created: r1 becomes active again")
		Expect(revs[0].Status.Active).To(BeTrue())
	})

	It("rejects a negative limit on a binding and on an Environment", func() {
		rb := fetchRB(rbName)
		rb.Spec.RevisionHistoryLimit = ptr.To(int32(-1))
		Expect(k8sClient.Update(ctx, rb)).To(MatchError(ContainSubstring("revisionHistoryLimit")))

		env := &openchoreov1alpha1.Environment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: envName}, env)).To(Succeed())
		env.Spec.ComponentRevisionHistoryLimit = ptr.To(int32(-1))
		Expect(k8sClient.Update(ctx, env)).To(MatchError(ContainSubstring("componentRevisionHistoryLimit")))
	})

	It("enqueues an Environment's bindings, and no others, when the Environment changes", func() {
		other := rbFixture("limit-comp-other-env", project, compName, "other-env", crA, false)
		Expect(k8sClient.Create(ctx, other)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, other) })

		env := &openchoreov1alpha1.Environment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: envName}, env)).To(Succeed())

		Expect(r.findReleaseBindingsForEnvironment(ctx, env)).To(ConsistOf(
			reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: rbName}}))
		Expect(r.findReleaseBindingsForEnvironment(ctx, &openchoreov1alpha1.Project{})).To(BeEmpty())
	})
})
