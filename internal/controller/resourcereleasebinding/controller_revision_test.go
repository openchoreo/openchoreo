// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	resourcepipeline "github.com/openchoreo/openchoreo/internal/pipeline/resource"
	"github.com/openchoreo/openchoreo/internal/revision"
)

var _ = Describe("ResourceReleaseBinding controller — revisions", func() {
	const (
		bindingName = "rev-db-rev-env"
		releaseA    = "rev-db-release-a"
		releaseB    = "rev-db-release-b"
	)

	var (
		reconciler *Reconciler
		envName    string
	)

	// revisionRelease returns a ResourceRelease for the suite's test resource
	// whose ResourceType accepts a "size" environment config.
	revisionRelease := func(name string) *openchoreov1alpha1.ResourceRelease {
		return &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ResourceReleaseSpec{
				Owner: openchoreov1alpha1.ResourceReleaseOwner{
					ProjectName:  "test-project",
					ResourceName: "test-resource",
				},
				ResourceType: openchoreov1alpha1.ResourceReleaseResourceType{
					Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
					Name: "mysql",
					Spec: openchoreov1alpha1.ResourceTypeSpec{
						EnvironmentConfigs: &openchoreov1alpha1.SchemaSection{
							OpenAPIV3Schema: &runtime.RawExtension{
								Raw: []byte(`{"type":"object","properties":{"size":{"type":"string","default":"small"}}}`),
							},
						},
						Resources: []openchoreov1alpha1.ResourceTypeManifest{{
							ID: "claim",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"` + name + `"}}`),
							},
						}},
					},
				},
			},
		}
	}

	fetchBinding := func() *openchoreov1alpha1.ResourceReleaseBinding {
		GinkgoHelper()
		b := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: bindingName}, b)).To(Succeed())
		return b
	}

	reconcileBinding := func() {
		GinkgoHelper()
		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: client.ObjectKey{Namespace: "default", Name: bindingName},
		})
		Expect(err).NotTo(HaveOccurred())
	}

	updateBinding := func(mutate func(b *openchoreov1alpha1.ResourceReleaseBinding)) {
		GinkgoHelper()
		b := fetchBinding()
		mutate(b)
		Expect(k8sClient.Update(ctx, b)).To(Succeed())
	}

	listRevisions := func(b *openchoreov1alpha1.ResourceReleaseBinding) []openchoreov1alpha1.ResourceReleaseBindingRevision {
		GinkgoHelper()
		list := &openchoreov1alpha1.ResourceReleaseBindingRevisionList{}
		Expect(k8sClient.List(ctx, list, client.InNamespace("default"),
			client.MatchingLabels{revision.LabelKeyBindingUID: string(b.UID)})).To(Succeed())
		sort.Slice(list.Items, func(i, j int) bool { return list.Items[i].Spec.Revision < list.Items[j].Spec.Revision })
		return list.Items
	}

	// expectRevisionInvariants asserts what the CRD cannot enforce: unique,
	// gap-free numbers from 1; names equal to revision.Name(binding, n); a
	// controller ownerReference to the binding on every revision (envtest has no
	// garbage collector); each spec recorded once; exactly one active revision,
	// recording the binding's current spec and named by status.currentRevision.
	expectRevisionInvariants := func() []openchoreov1alpha1.ResourceReleaseBindingRevision {
		GinkgoHelper()
		b := fetchBinding()
		revs := listRevisions(b)
		Expect(revs).NotTo(BeEmpty())

		active := 0
		for i := range revs {
			rev := &revs[i]
			Expect(rev.Spec.Revision).To(Equal(int64(i+1)), "revision numbers must be unique and gap-free")
			Expect(rev.Name).To(Equal(revision.Name(b.Name, rev.Spec.Revision)))

			owner := metav1.GetControllerOf(rev)
			Expect(owner).NotTo(BeNil(), "revision %q has no controller ownerReference", rev.Name)
			Expect(owner.Kind).To(Equal("ResourceReleaseBinding"))
			Expect(owner.Name).To(Equal(b.Name))
			Expect(owner.UID).To(Equal(b.UID))
			Expect(owner.BlockOwnerDeletion).To(HaveValue(BeTrue()))

			if rev.Status.Active {
				active++
			}
		}
		Expect(active).To(Equal(1), "exactly one revision must be active")

		hashes := map[string]string{}
		var current *openchoreov1alpha1.ResourceReleaseBindingRevision
		for i := range revs {
			Expect(hashes).NotTo(HaveKey(revs[i].Spec.SnapshotHash),
				"%s records the same spec as %s", revs[i].Name, hashes[revs[i].Spec.SnapshotHash])
			hashes[revs[i].Spec.SnapshotHash] = revs[i].Name
			if revs[i].Status.Active {
				current = &revs[i]
			}
		}
		wantHash, err := revision.ResourceReleaseBindingSpecHash(&b.Spec)
		Expect(err).NotTo(HaveOccurred())
		Expect(current.Spec.SnapshotHash).To(Equal(wantHash), "the active revision must record the current spec")
		Expect(current.Spec.ResourceReleaseName).To(Equal(b.Spec.ResourceRelease))
		Expect(b.Status.CurrentRevision).To(Equal(current.Name), "the binding must point at its active revision")
		return revs
	}

	sizeOverride := func(size string) *runtime.RawExtension {
		return &runtime.RawExtension{Raw: []byte(`{"size":"` + size + `"}`)}
	}

	BeforeEach(func() {
		reconciler = &Reconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Pipeline: resourcepipeline.NewPipeline(),
		}

		for _, name := range []string{releaseA, releaseB} {
			rel := revisionRelease(name)
			Expect(k8sClient.Create(ctx, rel)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rel) })
		}

		dp := &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "rev-dp", Namespace: "default"}}
		Expect(k8sClient.Create(ctx, dp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, dp) })

		env := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "rev-env", Namespace: "default"},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane, Name: dp.Name},
			},
		}
		Expect(k8sClient.Create(ctx, env)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, env) })
		envName = env.Name

		b := &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       bindingName,
				Namespace:  "default",
				Finalizers: []string{ResourceReleaseBindingFinalizer},
			},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner:           openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "test-project", ResourceName: "test-resource"},
				Environment:     envName,
				ResourceRelease: releaseA,
			},
		}
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			// envtest has no garbage collector: delete the RenderedReleases the
			// binding owns (the suite deletes its revisions), then strip its
			// finalizer and delete it.
			rrList := &openchoreov1alpha1.RenderedReleaseList{}
			_ = k8sClient.List(ctx, rrList, client.InNamespace("default"))
			for i := range rrList.Items {
				_ = k8sClient.Delete(ctx, &rrList.Items[i])
			}
			current := &openchoreov1alpha1.ResourceReleaseBinding{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(b), current); err == nil {
				current.Finalizers = nil
				_ = k8sClient.Update(ctx, current)
				_ = k8sClient.Delete(ctx, current)
			}
		})
	})

	It("records r1 with cause Deploy on the first bind, active and Progressing", func() {
		reconcileBinding()

		revs := expectRevisionInvariants()
		Expect(revs).To(HaveLen(1))
		r1 := revs[0]
		Expect(r1.Name).To(Equal(bindingName+"-r1"), "revision objects are named {binding}-r{N}")
		Expect(r1.Spec.Cause).To(Equal(openchoreov1alpha1.RevisionCauseDeploy))
		Expect(r1.Spec.ResourceReleaseName).To(Equal(releaseA))
		Expect(r1.Status.Health).To(Equal(openchoreov1alpha1.RevisionHealthProgressing))

		By("labeling the revision for kubectl selectors")
		Expect(r1.Labels).To(HaveKeyWithValue(revision.LabelKeyResourceReleaseBinding, bindingName))
		Expect(r1.Labels).To(HaveKeyWithValue("openchoreo.dev/resource", "test-resource"))
		Expect(r1.Labels).To(HaveKeyWithValue("openchoreo.dev/project", "test-project"))
		Expect(r1.Labels).To(HaveKeyWithValue("openchoreo.dev/environment", envName))
	})

	It("records r2 with cause Override when an environment config is overridden, deactivating r1 without touching its spec", func() {
		reconcileBinding()
		r1Before := listRevisions(fetchBinding())[0]

		updateBinding(func(b *openchoreov1alpha1.ResourceReleaseBinding) {
			b.Spec.ResourceTypeEnvironmentConfigs = sizeOverride("large")
		})
		reconcileBinding()

		revs := expectRevisionInvariants()
		Expect(revs).To(HaveLen(2))
		Expect(revs[1].Spec.Cause).To(Equal(openchoreov1alpha1.RevisionCauseOverride))
		Expect(revs[0].Status.Active).To(BeFalse())
		Expect(revs[0].Spec).To(Equal(r1Before.Spec), "r1's spec must not change")
	})

	It("records nothing when an identical spec is re-applied", func() {
		reconcileBinding()
		before := fetchBinding()

		updateBinding(func(b *openchoreov1alpha1.ResourceReleaseBinding) { b.Spec = *before.Spec.DeepCopy() })
		reconcileBinding()

		Expect(fetchBinding().Generation).To(Equal(before.Generation))
		Expect(expectRevisionInvariants()).To(HaveLen(1))
	})

	It("records nothing when only annotations or labels change", func() {
		reconcileBinding()

		updateBinding(func(b *openchoreov1alpha1.ResourceReleaseBinding) {
			b.Annotations = map[string]string{"example.com/note": "touched"}
			b.Labels = map[string]string{"example.com/team": "data"}
		})
		reconcileBinding()

		Expect(expectRevisionInvariants()).To(HaveLen(1))
	})

	It("makes r1 active again, creating nothing, when the spec returns to release A", func() {
		reconcileBinding()
		updateBinding(func(b *openchoreov1alpha1.ResourceReleaseBinding) { b.Spec.ResourceRelease = releaseB })
		reconcileBinding()
		updateBinding(func(b *openchoreov1alpha1.ResourceReleaseBinding) { b.Spec.ResourceRelease = releaseA })
		reconcileBinding()

		revs := expectRevisionInvariants()
		Expect(revs).To(HaveLen(2), "returning to release A reuses r1")
		Expect(revs[0].Status.Active).To(BeTrue())
		Expect(revs[1].Status.Active).To(BeFalse())
		Expect(fetchBinding().Status.CurrentRevision).To(Equal(bindingName + "-r1"))
	})

	It("keeps the status invariants through deploys, overrides and a rollback", func() {
		type step struct {
			name   string
			mutate func(b *openchoreov1alpha1.ResourceReleaseBinding)
			// wantActive is the revision active afterwards; wantCount the revisions
			// that exist. A new revision is revision wantCount, with cause wantCause.
			wantActive int64
			wantCount  int
			wantCause  openchoreov1alpha1.RevisionCause
		}
		steps := []step{
			{"override size", func(b *openchoreov1alpha1.ResourceReleaseBinding) {
				b.Spec.ResourceTypeEnvironmentConfigs = sizeOverride("medium")
			}, 2, 2, openchoreov1alpha1.RevisionCauseOverride},
			{"deploy release B", func(b *openchoreov1alpha1.ResourceReleaseBinding) {
				b.Spec.ResourceRelease = releaseB
			}, 3, 3, openchoreov1alpha1.RevisionCauseDeploy},
			{"change retain policy", func(b *openchoreov1alpha1.ResourceReleaseBinding) {
				b.Spec.RetainPolicy = openchoreov1alpha1.ResourceRetainPolicyRetain
			}, 4, 4, openchoreov1alpha1.RevisionCauseOverride},
			{"roll back to r2's spec, pinned", func(b *openchoreov1alpha1.ResourceReleaseBinding) {
				b.Spec.ResourceRelease = releaseA
				b.Spec.RetainPolicy = ""
			}, 2, 4, ""},
		}

		reconcileBinding()
		expectRevisionInvariants()

		for _, s := range steps {
			By(s.name)
			updateBinding(s.mutate)
			reconcileBinding()

			revs := expectRevisionInvariants()
			Expect(revs).To(HaveLen(s.wantCount))
			Expect(revs[s.wantActive-1].Status.Active).To(BeTrue(), "revision %d must be active", s.wantActive)
			if s.wantCause != "" {
				Expect(revs[s.wantCount-1].Spec.Cause).To(Equal(s.wantCause))
			}
		}
		Expect(fetchBinding().Status.CurrentRevision).To(Equal(bindingName + "-r2"))
	})
})
