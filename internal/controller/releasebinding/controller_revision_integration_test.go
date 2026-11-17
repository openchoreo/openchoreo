// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/revision"
)

// ─── Revision Helpers ─────────────────────────────────────────────────────────

// replicasDeployment renders spec.replicas from environmentConfigs.replicas, so a
// ReleaseBinding can override it per environment.
var replicasDeployment = &runtime.RawExtension{
	Raw: []byte(`{` +
		`"apiVersion":"apps/v1",` +
		`"kind":"Deployment",` +
		`"metadata":{"name":"rev-deployment"},` +
		`"spec":{` +
		`"replicas":"${environmentConfigs.replicas}",` +
		`"selector":{"matchLabels":{"app":"rev"}},` +
		`"template":{` +
		`"metadata":{"labels":{"app":"rev"}},` +
		`"spec":{"containers":[{"name":"app","image":"nginx:latest"}]}` +
		`}}}`,
	),
}

// revisionCRFixture returns a ComponentRelease whose ComponentType accepts a
// replicas environment config.
func revisionCRFixture(name, project, component string) *openchoreov1alpha1.ComponentRelease {
	cr := crFixture(name, project, component)
	cr.Spec.ComponentType.Spec.Resources = []openchoreov1alpha1.ResourceTemplate{
		{ID: "deployment", Template: replicasDeployment},
	}
	cr.Spec.ComponentType.Spec.EnvironmentConfigs = &openchoreov1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{
			Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer","default":1}}}`),
		},
	}
	return cr
}

// replicasOverride returns componentTypeEnvironmentConfigs setting replicas.
func replicasOverride(n string) *runtime.RawExtension {
	return &runtime.RawExtension{Raw: []byte(`{"replicas":` + n + `}`)}
}

// listRevisions returns the binding's ReleaseBindingRevisions in revision order.
func listRevisions(rb *openchoreov1alpha1.ReleaseBinding) []openchoreov1alpha1.ReleaseBindingRevision {
	GinkgoHelper()
	list := &openchoreov1alpha1.ReleaseBindingRevisionList{}
	Expect(k8sClient.List(ctx, list, client.InNamespace(ns),
		client.MatchingLabels{revision.LabelKeyBindingUID: string(rb.UID)})).To(Succeed())
	sort.Slice(list.Items, func(i, j int) bool { return list.Items[i].Spec.Revision < list.Items[j].Spec.Revision })
	return list.Items
}

// ─── Tests ────────────────────────────────────────────────────────────────────

var _ = Describe("ReleaseBinding Controller — revisions", func() {
	const (
		project  = "rev-proj"
		compName = "rev-comp"
		envName  = "rev-env"
		dpName   = "rev-dp"
		rbName   = "rev-comp-rev-env"
		crA      = "rev-cr-a"
		crB      = "rev-cr-b"
	)
	req := reconcileRequest(rbName)

	var r *Reconciler

	// expectRevisionInvariants asserts the status invariants the CRD cannot enforce
	// and returns the binding's revisions in order:
	//   - revision numbers are unique and gap-free, starting at 1;
	//   - each name equals revision.Name(binding, n), {binding}-r{n};
	//   - each revision has a controller ownerReference to the binding (envtest runs
	//     no garbage collector, so the reference is asserted rather than cascade deletion);
	//   - exactly one revision is active, and it is the latest;
	//   - the latest revision records the binding's current spec.
	expectRevisionInvariants := func() []openchoreov1alpha1.ReleaseBindingRevision {
		GinkgoHelper()
		rb := fetchRB(rbName)
		revs := listRevisions(rb)
		Expect(revs).NotTo(BeEmpty(), "binding %q has no revisions", rbName)

		active := 0
		for i := range revs {
			rev := &revs[i]
			Expect(rev.Spec.Revision).To(Equal(int64(i+1)), "revision numbers must be unique and gap-free")
			Expect(rev.Name).To(Equal(revision.Name(rb.Name, rev.Spec.Revision)))

			owner := metav1.GetControllerOf(rev)
			Expect(owner).NotTo(BeNil(), "revision %q has no controller ownerReference", rev.Name)
			Expect(owner.Kind).To(Equal("ReleaseBinding"))
			Expect(owner.Name).To(Equal(rb.Name))
			Expect(owner.UID).To(Equal(rb.UID))
			Expect(owner.BlockOwnerDeletion).To(HaveValue(BeTrue()))

			if rev.Status.Active {
				active++
			}
		}
		Expect(active).To(Equal(1), "exactly one revision must be active")

		// Each distinct spec is recorded once, and the active revision records
		// the current one.
		hashes := map[string]string{}
		var current *openchoreov1alpha1.ReleaseBindingRevision
		for i := range revs {
			Expect(hashes).NotTo(HaveKey(revs[i].Spec.SnapshotHash),
				"%s records the same spec as %s", revs[i].Name, hashes[revs[i].Spec.SnapshotHash])
			hashes[revs[i].Spec.SnapshotHash] = revs[i].Name
			if revs[i].Status.Active {
				current = &revs[i]
			}
		}
		wantHash, err := revision.ReleaseBindingSpecHash(&rb.Spec)
		Expect(err).NotTo(HaveOccurred())
		Expect(current.Spec.SnapshotHash).To(Equal(wantHash), "the active revision must record the current spec")
		Expect(current.Spec.ComponentReleaseName).To(Equal(rb.Spec.ReleaseName))
		Expect(rb.Status.CurrentRevision).To(Equal(current.Name), "the binding must point at its active revision")
		return revs
	}

	// updateRB applies mutate to the stored ReleaseBinding and writes it back.
	updateRB := func(mutate func(rb *openchoreov1alpha1.ReleaseBinding)) {
		GinkgoHelper()
		rb := fetchRB(rbName)
		mutate(rb)
		Expect(k8sClient.Update(ctx, rb)).To(Succeed())
	}

	// reconcileRB runs the reconcile twice: the first pass creates the dataplane
	// Release and requests a requeue, the second settles.
	reconcileRB := func() {
		GinkgoHelper()
		mustReconcile(r, req)
		mustReconcile(r, req)
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

	It("records r1 with cause Deploy on the first bind, active and Progressing", func() {
		reconcileRB()

		revs := expectRevisionInvariants()
		Expect(revs).To(HaveLen(1))
		r1 := revs[0]
		Expect(r1.Name).To(Equal(rbName+"-r1"), "revision objects are named {binding}-r{N}")
		Expect(r1.Spec.Cause).To(Equal(openchoreov1alpha1.RevisionCauseDeploy))
		Expect(r1.Spec.ComponentReleaseName).To(Equal(crA))
		Expect(r1.Spec.SnapshotHash).To(HaveLen(64))
		Expect(r1.Status.Active).To(BeTrue())
		Expect(r1.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthProgressing))

		By("labeling the revision for kubectl selectors")
		Expect(r1.Labels).To(HaveKeyWithValue(revision.LabelKeyReleaseBinding, rbName))
		Expect(r1.Labels).To(HaveKeyWithValue("openchoreo.dev/component", compName))
		Expect(r1.Labels).To(HaveKeyWithValue("openchoreo.dev/project", project))
		Expect(r1.Labels).To(HaveKeyWithValue("openchoreo.dev/environment", envName))
	})

	It("records r2 with cause Override when replicas are overridden, deactivating r1 without touching its spec", func() {
		reconcileRB()
		r1Before := listRevisions(fetchRB(rbName))[0]

		updateRB(func(rb *openchoreov1alpha1.ReleaseBinding) {
			rb.Spec.ComponentTypeEnvironmentConfigs = replicasOverride("3")
		})
		reconcileRB()

		revs := expectRevisionInvariants()
		Expect(revs).To(HaveLen(2))
		Expect(revs[1].Spec.Cause).To(Equal(openchoreov1alpha1.RevisionCauseOverride))
		Expect(revs[1].Spec.ComponentReleaseName).To(Equal(crA))
		Expect(string(revs[1].Spec.BindingSpec.ComponentTypeEnvironmentConfigs.Raw)).To(MatchJSON(`{"replicas":3}`))

		Expect(revs[0].Status.Active).To(BeFalse())
		Expect(revs[0].Spec).To(Equal(r1Before.Spec), "r1's spec must not change")
		Expect(revs[0].Generation).To(Equal(r1Before.Generation))
	})

	It("records nothing when an identical spec is re-applied", func() {
		reconcileRB()
		before := fetchRB(rbName)

		updateRB(func(rb *openchoreov1alpha1.ReleaseBinding) {
			rb.Spec = *before.Spec.DeepCopy()
		})
		reconcileRB()

		Expect(fetchRB(rbName).Generation).To(Equal(before.Generation))
		Expect(expectRevisionInvariants()).To(HaveLen(1))
	})

	It("records nothing when only annotations or labels change", func() {
		reconcileRB()

		updateRB(func(rb *openchoreov1alpha1.ReleaseBinding) {
			rb.Annotations = map[string]string{"example.com/note": "touched"}
			rb.Labels = map[string]string{"example.com/team": "web"}
		})
		reconcileRB()

		Expect(expectRevisionInvariants()).To(HaveLen(1))
	})

	It("makes r1 active again, creating nothing, when the spec returns to release A", func() {
		reconcileRB()
		updateRB(func(rb *openchoreov1alpha1.ReleaseBinding) { rb.Spec.ReleaseName = crB })
		reconcileRB()
		Expect(fetchRB(rbName).Status.CurrentRevision).To(Equal(rbName + "-r2"))

		updateRB(func(rb *openchoreov1alpha1.ReleaseBinding) { rb.Spec.ReleaseName = crA })
		reconcileRB()

		revs := expectRevisionInvariants()
		Expect(revs).To(HaveLen(2), "returning to release A reuses r1")
		Expect(revs[0].Status.Active).To(BeTrue())
		Expect(revs[1].Status.Active).To(BeFalse())
		Expect(fetchRB(rbName).Status.CurrentRevision).To(Equal(rbName + "-r1"))
	})

	It("keeps the status invariants through deploys, overrides, a rollback, undeploy and redeploy", func() {
		type step struct {
			name   string
			mutate func(rb *openchoreov1alpha1.ReleaseBinding)
			// wantActive is the revision active afterwards; wantCount the revisions
			// that exist. A step that creates one sees it as revision wantCount,
			// with cause wantCause; a step back to a recorded spec creates none.
			wantActive int64
			wantCount  int
			wantCause  openchoreov1alpha1.RevisionCause
		}
		steps := []step{
			{"override replicas", func(rb *openchoreov1alpha1.ReleaseBinding) {
				rb.Spec.ComponentTypeEnvironmentConfigs = replicasOverride("2")
			}, 2, 2, openchoreov1alpha1.RevisionCauseOverride},
			{"deploy release B", func(rb *openchoreov1alpha1.ReleaseBinding) {
				rb.Spec.ReleaseName = crB
			}, 3, 3, openchoreov1alpha1.RevisionCauseDeploy},
			{"override replicas again", func(rb *openchoreov1alpha1.ReleaseBinding) {
				rb.Spec.ComponentTypeEnvironmentConfigs = replicasOverride("4")
			}, 4, 4, openchoreov1alpha1.RevisionCauseOverride},
			{"roll back to r2's spec, pinned", func(rb *openchoreov1alpha1.ReleaseBinding) {
				rb.Spec.ReleaseName = crA
				rb.Spec.ComponentTypeEnvironmentConfigs = replicasOverride("2")
			}, 2, 4, ""},
			{"undeploy", func(rb *openchoreov1alpha1.ReleaseBinding) {
				rb.Spec.State = openchoreov1alpha1.ReleaseStateUndeploy
			}, 5, 5, openchoreov1alpha1.RevisionCauseUndeploy},
			{"redeploy, back to r2's spec", func(rb *openchoreov1alpha1.ReleaseBinding) {
				rb.Spec.State = openchoreov1alpha1.ReleaseStateActive
			}, 2, 5, ""},
		}

		reconcileRB()
		expectRevisionInvariants()

		for _, s := range steps {
			By(s.name)
			updateRB(s.mutate)
			reconcileRB()

			revs := expectRevisionInvariants()
			Expect(revs).To(HaveLen(s.wantCount))
			Expect(revs[s.wantActive-1].Status.Active).To(BeTrue(), "revision %d must be active", s.wantActive)
			if s.wantCause != "" {
				Expect(revs[s.wantCount-1].Spec.Cause).To(Equal(s.wantCause))
			}
		}
	})
})
