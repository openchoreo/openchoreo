// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// These specs run the openchoreo-api rollback service against the apiserver
// and then the ReleaseBinding controller, to show what the rollback leaves
// behind once the controller has reconciled it.
var _ = Describe("ReleaseBinding rollback of a GitOps-managed binding", func() {
	const (
		project  = "gitops-proj"
		compName = "gitops-comp"
		envName  = "gitops-env"
		dpName   = "gitops-dp"
		rbName   = "gitops-comp-gitops-env"
		crA      = "gitops-cr-a"
		crB      = "gitops-cr-b"
	)
	req := reconcileRequest(rbName)

	var r *Reconciler

	reconcileRB := func() {
		GinkgoHelper()
		mustReconcile(r, req)
		mustReconcile(r, req)
	}

	// revisionState is what the binding's revisions record and which is active,
	// with each revision's resourceVersion: equal states mean nothing was written.
	revisionState := func() map[string]string {
		GinkgoHelper()
		state := map[string]string{}
		for _, rev := range listRevisions(fetchRB(rbName)) {
			state[rev.Name] = rev.ResourceVersion
			if rev.Status.Active {
				state["active"] = rev.Name
			}
		}
		return state
	}

	// deployAThenB records r1 (release A) and r2 (release B, active).
	deployAThenB := func() {
		GinkgoHelper()
		reconcileRB()
		rb := fetchRB(rbName)
		rb.Spec.ReleaseName = crB
		Expect(k8sClient.Update(ctx, rb)).To(Succeed())
		reconcileRB()
		Expect(fetchRB(rbName).Status.CurrentRevision).To(Equal(rbName + "-r2"))
	}

	rollback := func(revision int64) error {
		_, err := releasebindingsvc.NewService(k8sClient, testutil.TestLogger()).RollbackReleaseBinding(ctx, ns, rbName,
			bindingrevision.RollbackRequest{Revision: revision})
		return err
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

	It("refuses the rollback of a binding Flux applies, and nothing changes after reconciling", func() {
		deployAThenB()
		rb := fetchRB(rbName)
		rb.Labels = map[string]string{
			"kustomize.toolkit.fluxcd.io/name":      "apps",
			"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
		}
		Expect(k8sClient.Update(ctx, rb)).To(Succeed())
		reconcileRB()
		before := fetchRB(rbName)
		revsBefore := revisionState()

		for _, target := range []int64{1, 0} {
			err := rollback(target)
			Expect(err).To(MatchError(bindingrevision.ErrGitOpsManaged), "revision %d", target)
			Expect(err.Error()).To(ContainSubstring("Revert the binding's configuration in Git and let GitOps reconcile the change"))
		}

		after := fetchRB(rbName)
		Expect(after.ResourceVersion).To(Equal(before.ResourceVersion), "the binding is not written")
		Expect(after.Generation).To(Equal(before.Generation))
		Expect(revisionState()).To(Equal(revsBefore), "no revision is written")

		reconcileRB()
		after = fetchRB(rbName)
		Expect(after.Spec).To(Equal(before.Spec), "the spec is still release B")
		Expect(after.Status.CurrentRevision).To(Equal(rbName + "-r2"))
		Expect(revisionState()).To(Equal(revsBefore), "the controller records and activates nothing new")
	})

	It("rolls back a binding not managed by GitOps, and the controller makes r1 active again", func() {
		deployAThenB()
		rb := fetchRB(rbName)
		rb.Labels = map[string]string{"team": "payments"}
		Expect(k8sClient.Update(ctx, rb)).To(Succeed())

		Expect(rollback(1)).To(Succeed())
		Expect(fetchRB(rbName).Spec.ReleaseName).To(Equal(crA))

		reconcileRB()
		revs := listRevisions(fetchRB(rbName))
		Expect(revs).To(HaveLen(2), "no revision is created: r1 becomes active again")
		Expect(revs[0].Status.Active).To(BeTrue())
		Expect(revs[1].Status.Active).To(BeFalse())
		Expect(fetchRB(rbName).Status.CurrentRevision).To(Equal(rbName + "-r1"))

		By("rolling back without a revision returns to r2")
		Expect(rollback(0)).To(Succeed())
		reconcileRB()
		Expect(fetchRB(rbName).Spec.ReleaseName).To(Equal(crB))
		Expect(fetchRB(rbName).Status.CurrentRevision).To(Equal(rbName + "-r2"))
		Expect(listRevisions(fetchRB(rbName))).To(HaveLen(2))
	})
})
