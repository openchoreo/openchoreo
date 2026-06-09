// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
	resourcepipeline "github.com/openchoreo/openchoreo/internal/pipeline/resource"
)

// Resolve-and-validate covers the chain that runs before any rendering:
//   - resourceRelease pin set
//   - ResourceRelease exists
//   - owner alignment (binding ↔ release)
//   - Environment exists
//   - DataPlane resolves
//
// All failure modes surface on the Synced condition; the aggregate Ready
// inherits the failing reason. Schema validation of
// resourceTypeEnvironmentConfigs is intentionally not in the controller —
// it lives in a future binding webhook (mirrors the componentrelease
// webhook precedent for parameter validation).
var _ = Describe("ResourceReleaseBinding controller — resolve and validate", func() {
	var reconciler *Reconciler

	BeforeEach(func() {
		reconciler = &Reconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Pipeline: resourcepipeline.NewPipeline(),
		}
	})

	// newBinding builds a minimal ResourceReleaseBinding for tests. The
	// finalizer is pre-set so a single Reconcile call exercises the validation
	// chain instead of returning early after adding it. Tests focused on the
	// finalizer-add path build their own object without it.
	newBinding := func(name, releaseName, env string) *openchoreov1alpha1.ResourceReleaseBinding {
		return &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "default",
				Finalizers: []string{ResourceReleaseBindingFinalizer},
			},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "test-project",
					ResourceName: "test-resource",
				},
				Environment:     env,
				ResourceRelease: releaseName,
			},
		}
	}
	_ = newBinding

	// reconcileBinding runs the controller against the supplied binding and
	// returns the freshly-fetched live object so tests can assert on status.
	reconcileBinding := func(b *openchoreov1alpha1.ResourceReleaseBinding) *openchoreov1alpha1.ResourceReleaseBinding {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(b),
		})
		Expect(err).NotTo(HaveOccurred())

		updated := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), updated)).To(Succeed())
		return updated
	}
	_ = reconcileBinding

	// snapshotForOwner builds an immutable ResourceRelease pinned to the given
	// owner. Used by tests downstream of the resourceRelease lookup.
	snapshotForOwner := func(name, projectName, resourceName string) *openchoreov1alpha1.ResourceRelease {
		return &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: openchoreov1alpha1.ResourceReleaseSpec{
				Owner: openchoreov1alpha1.ResourceReleaseOwner{
					ProjectName:  projectName,
					ResourceName: resourceName,
				},
				ResourceType: openchoreov1alpha1.ResourceReleaseResourceType{
					Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
					Name: "mysql",
					Spec: openchoreov1alpha1.ResourceTypeSpec{
						Resources: []openchoreov1alpha1.ResourceTypeManifest{
							{
								ID: "claim",
								Template: &runtime.RawExtension{
									Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"x"}}`),
								},
							},
						},
					},
				},
			},
		}
	}
	_ = snapshotForOwner

	It("sets Synced=False, Reason=ResourceReleaseNotSet when spec.resourceRelease is empty", func() {
		b := newBinding("notset-pin", "", "dev")
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, b)
		})

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil(), "expected Synced condition to be set")
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonResourceReleaseNotSet)))

		By("aggregating Ready=False on every early-return path")
		ready := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionReady))
		Expect(ready).NotTo(BeNil(), "Ready must be set on every reconcile, including validation failures")
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(string(ReasonResourceReleaseNotSet)),
			"Ready inherits the failing sub-condition's reason")
	})

	It("sets Synced=False, Reason=ResourceReleaseNotFound when the referenced ResourceRelease does not exist", func() {
		b := newBinding("notfound-pin", "missing-release-abc123", "dev")
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, b)
		})

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil(), "expected Synced condition to be set")
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonResourceReleaseNotFound)))
	})

	It("sets Synced=False, Reason=InvalidReleaseConfiguration when the ResourceRelease owner does not match the binding", func() {
		release := snapshotForOwner("ownermismatch-release", "other-project", "other-resource")
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, release)
		})

		// newBinding's default owner is {test-project, test-resource}, which
		// disagrees with the snapshot above on both fields.
		b := newBinding("ownermismatch-binding", release.Name, "dev")
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, b)
		})

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil(), "expected Synced condition to be set")
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonInvalidReleaseConfiguration)))
	})

	It("sets Synced=False, Reason=EnvironmentNotFound when the referenced Environment does not exist", func() {
		release := snapshotForOwner("envmissing-release", "test-project", "test-resource")
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, release)
		})

		b := newBinding("envmissing-binding", release.Name, "missing-env")
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, b)
		})

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil(), "expected Synced condition to be set")
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonEnvironmentNotFound)))
	})

	It("sets Synced=False, Reason=ResourceNotFound when the owning Resource does not exist", func() {
		release := snapshotForOwner("resmissing-release", "test-project", "missing-resource")
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		dp := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "resmissing-dp", Namespace: "default"},
		}
		Expect(k8sClient.Create(ctx, dp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, dp) })

		env := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "resmissing-env", Namespace: "default"},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
					Name: dp.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, env)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, env) })

		// Binding owner deliberately points at a Resource that doesn't exist.
		b := newBinding("resmissing-binding", release.Name, env.Name)
		b.Spec.Owner.ResourceName = "missing-resource"
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, b) })

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonResourceNotFound)))
	})

	It("sets Synced=False, Reason=ProjectNotFound when the owning Project does not exist", func() {
		release := snapshotForOwner("projmissing-release", "missing-project", "projmissing-resource")
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		// Project doesn't exist; we still need a Resource that points back
		// at the same project the binding claims, so owner alignment passes.
		proxyResource := &openchoreov1alpha1.Resource{
			ObjectMeta: metav1.ObjectMeta{Name: "projmissing-resource", Namespace: "default"},
			Spec: openchoreov1alpha1.ResourceSpec{
				Owner: openchoreov1alpha1.ResourceOwner{ProjectName: "missing-project"},
				Type: openchoreov1alpha1.ResourceTypeRef{
					Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
					Name: "mysql",
				},
			},
		}
		Expect(k8sClient.Create(ctx, proxyResource)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, proxyResource) })

		dp := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "projmissing-dp", Namespace: "default"},
		}
		Expect(k8sClient.Create(ctx, dp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, dp) })

		env := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "projmissing-env", Namespace: "default"},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
					Name: dp.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, env)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, env) })

		b := newBinding("projmissing-binding", release.Name, env.Name)
		b.Spec.Owner.ProjectName = "missing-project"
		b.Spec.Owner.ResourceName = "projmissing-resource"
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, b) })

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonProjectNotFound)))
	})

	It("sets Synced=False, Reason=DataPlaneNotFound when the Environment's dataPlaneRef does not resolve", func() {
		release := snapshotForOwner("dpmissing-release", "test-project", "test-resource")
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, release)
		})

		env := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "dpmissing-env", Namespace: "default"},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
					Name: "missing-dp",
				},
			},
		}
		Expect(k8sClient.Create(ctx, env)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, env)
		})

		b := newBinding("dpmissing-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, b)
		})

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil(), "expected Synced condition to be set")
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonDataPlaneNotFound)))
	})

	It("adds the resourcereleasebinding-cleanup finalizer on first reconcile", func() {
		// Build the binding without the pre-set finalizer so the first
		// reconcile exercises the add path.
		b := &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fin-add-binding",
				Namespace: "default",
			},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "test-project",
					ResourceName: "test-resource",
				},
				Environment: "dev",
			},
		}
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, b)
		})

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(b),
		})
		Expect(err).NotTo(HaveOccurred())

		updated := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), updated)).To(Succeed())
		Expect(updated.Finalizers).To(ContainElement(ResourceReleaseBindingFinalizer))
	})

	It("returns without error when the binding does not exist (already-deleted)", func() {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: client.ObjectKey{Name: "ghost-binding", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())
	})

})

// Render-and-emit covers the second half of the reconcile chain: build the
// pipeline input from the snapshot, render manifests, write a RenderedRelease
// owned by the binding, and surface the result on the Synced condition.
var _ = Describe("ResourceReleaseBinding controller — render and emit", func() {
	var reconciler *Reconciler

	BeforeEach(func() {
		reconciler = &Reconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Pipeline: resourcepipeline.NewPipeline(),
		}
	})

	// configMapTemplate produces a ResourceTypeManifest body that the pipeline
	// can render against an empty CEL context. Tests use it as the default
	// payload for snapshots when the rendered shape doesn't matter.
	configMapTemplate := func(name string) *runtime.RawExtension {
		return &runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"` + name + `"}}`),
		}
	}

	// snapshotWithEntries builds a ResourceRelease pinned to the default
	// {test-project, test-resource} owner with the supplied resources[]
	// entries. Tests in this Describe never need to vary the owner; the
	// ownership-conflict test (2.5) creates a separate squatter directly.
	snapshotWithEntries := func(name string, entries []openchoreov1alpha1.ResourceTypeManifest) *openchoreov1alpha1.ResourceRelease {
		return &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: openchoreov1alpha1.ResourceReleaseSpec{
				Owner: openchoreov1alpha1.ResourceReleaseOwner{
					ProjectName:  "test-project",
					ResourceName: "test-resource",
				},
				ResourceType: openchoreov1alpha1.ResourceReleaseResourceType{
					Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
					Name: "mysql",
					Spec: openchoreov1alpha1.ResourceTypeSpec{
						Resources: entries,
					},
				},
			},
		}
	}

	// makeBinding builds a binding with the cleanup finalizer pre-set so a
	// single Reconcile call exercises the whole chain.
	makeBinding := func(name, releaseName, env string) *openchoreov1alpha1.ResourceReleaseBinding {
		return &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "default",
				Finalizers: []string{ResourceReleaseBindingFinalizer},
			},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "test-project",
					ResourceName: "test-resource",
				},
				Environment:     env,
				ResourceRelease: releaseName,
			},
		}
	}

	// makeEnvAndDP creates an Environment and DataPlane in the namespace; the
	// returned cleanup must be deferred by the caller.
	makeEnvAndDP := func(prefix string) *openchoreov1alpha1.Environment {
		dp := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: prefix + "-dp", Namespace: "default"},
		}
		Expect(k8sClient.Create(ctx, dp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, dp) })

		env := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: prefix + "-env", Namespace: "default"},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
					Name: dp.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, env)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, env) })
		return env
	}

	reconcileBinding := func(b *openchoreov1alpha1.ResourceReleaseBinding) *openchoreov1alpha1.ResourceReleaseBinding {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(b),
		})
		Expect(err).NotTo(HaveOccurred())
		updated := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), updated)).To(Succeed())
		return updated
	}

	It("creates a RenderedRelease owned by the binding and marks Synced=True, Reason=ReleaseCreated", func() {
		release := snapshotWithEntries("emit-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("emit-claim")},
			},
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("emit")

		b := makeBinding("emit-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			// Best-effort cleanup of the emitted RenderedRelease.
			rrList := &openchoreov1alpha1.RenderedReleaseList{}
			_ = k8sClient.List(ctx, rrList, client.InNamespace("default"))
			for i := range rrList.Items {
				_ = k8sClient.Delete(ctx, &rrList.Items[i])
			}
			_ = k8sClient.Delete(ctx, b)
		})

		updated := reconcileBinding(b)

		By("setting Synced=True, Reason=ReleaseCreated")
		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil(), "expected Synced condition to be set")
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(string(ReasonReleaseCreated)))

		By("creating a RenderedRelease with the right shape")
		rrList := &openchoreov1alpha1.RenderedReleaseList{}
		Expect(k8sClient.List(ctx, rrList, client.InNamespace("default"))).To(Succeed())
		Expect(rrList.Items).To(HaveLen(1), "expected one RenderedRelease emitted by this binding")

		rr := &rrList.Items[0]
		Expect(rr.Spec.Owner).To(Equal(openchoreov1alpha1.RenderedReleaseOwner{
			ProjectName:  "test-project",
			ResourceName: "test-resource",
		}))
		Expect(rr.Spec.EnvironmentName).To(Equal(env.Name))
		Expect(rr.Spec.TargetPlane).To(Equal(openchoreov1alpha1.TargetPlaneDataPlane))
		Expect(rr.Spec.Resources).To(HaveLen(1))
		Expect(rr.Spec.Resources[0].ID).To(Equal("claim"))

		By("setting an owner-ref pointing to the binding")
		hasOwner, err := controllerutil.HasOwnerReference(rr.GetOwnerReferences(), updated, k8sClient.Scheme())
		Expect(err).NotTo(HaveOccurred())
		Expect(hasOwner).To(BeTrue(), "RenderedRelease should be controller-owned by the binding")
	})

	It("filters entries whose includeWhen evaluates false and preserves order of the rest", func() {
		release := snapshotWithEntries("filter-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("filter-claim")},
				{ID: "tls-cert", IncludeWhen: "${1==2}", Template: configMapTemplate("filter-tls")},
				{ID: "alerts", Template: configMapTemplate("filter-alerts")},
			},
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("filter")

		b := makeBinding("filter-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			rrList := &openchoreov1alpha1.RenderedReleaseList{}
			_ = k8sClient.List(ctx, rrList, client.InNamespace("default"))
			for i := range rrList.Items {
				_ = k8sClient.Delete(ctx, &rrList.Items[i])
			}
			_ = k8sClient.Delete(ctx, b)
		})

		_ = reconcileBinding(b)

		rr := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: dpkubernetes.GenerateK8sName("r_test-resource", env.Name), Namespace: "default"}, rr)).To(Succeed())

		Expect(rr.Spec.Resources).To(HaveLen(2), "the tls-cert entry must be filtered out")
		ids := []string{rr.Spec.Resources[0].ID, rr.Spec.Resources[1].ID}
		Expect(ids).To(Equal([]string{"claim", "alerts"}),
			"remaining entries keep their original spec order")
	})

	It("re-renders the RenderedRelease when spec.resourceRelease is advanced to a new release", func() {
		oldRelease := snapshotWithEntries("pin-old-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("pin-old-claim")},
			},
		)
		Expect(k8sClient.Create(ctx, oldRelease)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, oldRelease) })

		newRelease := snapshotWithEntries("pin-new-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("pin-new-claim")},
			},
		)
		Expect(k8sClient.Create(ctx, newRelease)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, newRelease) })

		env := makeEnvAndDP("pin")

		b := makeBinding("pin-binding", oldRelease.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			rrList := &openchoreov1alpha1.RenderedReleaseList{}
			_ = k8sClient.List(ctx, rrList, client.InNamespace("default"))
			for i := range rrList.Items {
				_ = k8sClient.Delete(ctx, &rrList.Items[i])
			}
			_ = k8sClient.Delete(ctx, b)
		})

		current := reconcileBinding(b)

		rrAfterCreate := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: dpkubernetes.GenerateK8sName("r_test-resource", env.Name), Namespace: "default"}, rrAfterCreate)).To(Succeed())
		Expect(rrAfterCreate.Spec.Resources).To(HaveLen(1))
		Expect(string(rrAfterCreate.Spec.Resources[0].Object.Raw)).To(ContainSubstring("pin-old-claim"))

		By("advancing the pin to the new release")
		current.Spec.ResourceRelease = newRelease.Name
		Expect(k8sClient.Update(ctx, current)).To(Succeed())

		updated := reconcileBinding(current)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(string(ReasonReleaseCreated)),
			"create-or-update lumps Created and Updated under ReleaseCreated; ReleaseSynced is reserved for OperationResultNone")

		rrAfterUpdate := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rrAfterCreate), rrAfterUpdate)).To(Succeed())
		Expect(string(rrAfterUpdate.Spec.Resources[0].Object.Raw)).To(ContainSubstring("pin-new-claim"))
	})

	It("sets Synced=False, Reason=ReleaseOwnershipConflict when an unrelated RenderedRelease occupies the target name", func() {
		release := snapshotWithEntries("conflict-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("conflict-claim")},
			},
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("conflict")

		// Pre-create a RenderedRelease at the target name (test-resource-{env})
		// owned by an unrelated controller (a sibling ResourceReleaseBinding
		// here serves as a stand-in for "some other parent").
		squatter := &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "conflict-squatter",
				Namespace: "default",
			},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "other-project",
					ResourceName: "other-resource",
				},
				Environment: env.Name,
			},
		}
		Expect(k8sClient.Create(ctx, squatter)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, squatter) })

		preexisting := &openchoreov1alpha1.RenderedRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dpkubernetes.GenerateK8sName("r_test-resource", env.Name),
				Namespace: "default",
			},
			Spec: openchoreov1alpha1.RenderedReleaseSpec{
				Owner: openchoreov1alpha1.RenderedReleaseOwner{
					ProjectName:  "other-project",
					ResourceName: "other-resource",
				},
				EnvironmentName: env.Name,
				TargetPlane:     openchoreov1alpha1.TargetPlaneDataPlane,
			},
		}
		Expect(controllerutil.SetControllerReference(squatter, preexisting, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, preexisting)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, preexisting) })

		b := makeBinding("conflict-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, b) })

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonReleaseOwnershipConflict)))

		By("not modifying the pre-existing RenderedRelease")
		after := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(preexisting), after)).To(Succeed())
		Expect(after.Spec.Owner.ResourceName).To(Equal("other-resource"))
	})

	It("sets Synced=False, Reason=RenderingFailed when the pipeline rejects a template", func() {
		// Templates may not reference applied.<id> during rendering; the
		// pipeline aborts with a CEL evaluation error.
		release := snapshotWithEntries("renderfail-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{
					ID: "claim",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"x","annotations":{"forbidden":"${applied.claim.status.host}"}}}`),
					},
				},
			},
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("renderfail")

		b := makeBinding("renderfail-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, b) })

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionSynced))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonRenderingFailed)))

		By("not creating a RenderedRelease when render fails")
		rr := &openchoreov1alpha1.RenderedRelease{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: dpkubernetes.GenerateK8sName("r_test-resource", env.Name), Namespace: "default"}, rr)
		Expect(err).To(HaveOccurred(), "expected NotFound for the RenderedRelease")
	})

	It("is idempotent — second reconcile leaves the RenderedRelease untouched and reports Synced=ReleaseSynced", func() {
		release := snapshotWithEntries("idemp-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("idemp-claim")},
			},
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("idemp")

		b := makeBinding("idemp-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			rrList := &openchoreov1alpha1.RenderedReleaseList{}
			_ = k8sClient.List(ctx, rrList, client.InNamespace("default"))
			for i := range rrList.Items {
				_ = k8sClient.Delete(ctx, &rrList.Items[i])
			}
			_ = k8sClient.Delete(ctx, b)
		})

		// First reconcile creates the RenderedRelease.
		first := reconcileBinding(b)
		firstCond := meta.FindStatusCondition(first.Status.Conditions, string(ConditionSynced))
		Expect(firstCond).NotTo(BeNil())
		Expect(firstCond.Reason).To(Equal(string(ReasonReleaseCreated)))

		rrFirst := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: dpkubernetes.GenerateK8sName("r_test-resource", env.Name), Namespace: "default"}, rrFirst)).To(Succeed())
		firstResourceVersion := rrFirst.ResourceVersion

		// Second reconcile must not touch the RenderedRelease and must
		// transition Synced to Reason=ReleaseSynced.
		second := reconcileBinding(b)
		secondCond := meta.FindStatusCondition(second.Status.Conditions, string(ConditionSynced))
		Expect(secondCond).NotTo(BeNil())
		Expect(secondCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(secondCond.Reason).To(Equal(string(ReasonReleaseSynced)))

		rrSecond := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rrFirst), rrSecond)).To(Succeed())
		Expect(rrSecond.ResourceVersion).To(Equal(firstResourceVersion),
			"RenderedRelease must not be updated by an idempotent reconcile")
	})

	It("preserves ResourceType.spec.resources[].id verbatim and in spec order", func() {
		// Three entries with intentionally non-alphabetical IDs to lock in
		// the contract that the controller forwards what the pipeline yields
		// rather than regenerating from kind/name.
		release := snapshotWithEntries("ids-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("ids-claim")},
				{ID: "tls-cert", Template: configMapTemplate("ids-tls-cert")},
				{ID: "alerts", Template: configMapTemplate("ids-alerts")},
			},
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("ids")

		b := makeBinding("ids-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			rrList := &openchoreov1alpha1.RenderedReleaseList{}
			_ = k8sClient.List(ctx, rrList, client.InNamespace("default"))
			for i := range rrList.Items {
				_ = k8sClient.Delete(ctx, &rrList.Items[i])
			}
			_ = k8sClient.Delete(ctx, b)
		})

		_ = reconcileBinding(b)

		rr := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: dpkubernetes.GenerateK8sName("r_test-resource", env.Name), Namespace: "default"}, rr)).To(Succeed())

		Expect(rr.Spec.Resources).To(HaveLen(3))
		ids := []string{rr.Spec.Resources[0].ID, rr.Spec.Resources[1].ID, rr.Spec.Resources[2].ID}
		Expect(ids).To(Equal([]string{"claim", "tls-cert", "alerts"}),
			"IDs must come straight from ResourceType.spec.resources[].id in spec order")
	})
})

// Outputs-and-readiness covers the third reconcile step: read the
// RenderedRelease status the renderedrelease controller writes back, resolve
// declared outputs against observed applied status, aggregate per-entry
// readiness, and compute the Ready aggregate.
var _ = Describe("ResourceReleaseBinding controller — outputs and readiness", func() {
	var reconciler *Reconciler

	BeforeEach(func() {
		reconciler = &Reconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Pipeline: resourcepipeline.NewPipeline(),
		}
	})

	configMapTemplate := func(name string) *runtime.RawExtension {
		return &runtime.RawExtension{
			Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"` + name + `"}}`),
		}
	}

	snapshotWith := func(name string, entries []openchoreov1alpha1.ResourceTypeManifest, outputs []openchoreov1alpha1.ResourceTypeOutput) *openchoreov1alpha1.ResourceRelease {
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
						Resources: entries,
						Outputs:   outputs,
					},
				},
			},
		}
	}

	makeBinding := func(name, releaseName, env string) *openchoreov1alpha1.ResourceReleaseBinding {
		return &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "default",
				Finalizers: []string{ResourceReleaseBindingFinalizer},
			},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "test-project",
					ResourceName: "test-resource",
				},
				Environment:     env,
				ResourceRelease: releaseName,
			},
		}
	}

	makeEnvAndDP := func(prefix string) *openchoreov1alpha1.Environment {
		dp := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: prefix + "-dp", Namespace: "default"},
		}
		Expect(k8sClient.Create(ctx, dp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, dp) })

		env := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: prefix + "-env", Namespace: "default"},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
					Name: dp.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, env)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, env) })
		return env
	}

	reconcileBinding := func(b *openchoreov1alpha1.ResourceReleaseBinding) *openchoreov1alpha1.ResourceReleaseBinding {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(b),
		})
		Expect(err).NotTo(HaveOccurred())
		updated := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), updated)).To(Succeed())
		return updated
	}

	// fetchRenderedRelease returns the RenderedRelease emitted for the
	// supplied binding's resource/env pair.
	fetchRenderedRelease := func(b *openchoreov1alpha1.ResourceReleaseBinding) *openchoreov1alpha1.RenderedRelease {
		rr := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      makeRenderedReleaseName(b),
			Namespace: b.Namespace,
		}, rr)).To(Succeed())
		return rr
	}

	// setRenderedReleaseStatus simulates the renderedrelease controller's
	// status writeback.
	setRenderedReleaseStatus := func(rr *openchoreov1alpha1.RenderedRelease, resources []openchoreov1alpha1.RenderedManifestStatus) {
		rr.Status.Resources = resources
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())
	}

	cleanupReleases := func(b *openchoreov1alpha1.ResourceReleaseBinding) {
		rrList := &openchoreov1alpha1.RenderedReleaseList{}
		_ = k8sClient.List(ctx, rrList, client.InNamespace(b.Namespace))
		for i := range rrList.Items {
			_ = k8sClient.Delete(ctx, &rrList.Items[i])
		}
	}

	It("reports Ready=True when every resource is healthy and outputs resolve", func() {
		release := snapshotWith("ready-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("ready-claim")},
			},
			[]openchoreov1alpha1.ResourceTypeOutput{
				{Name: "host", Value: "${applied.claim.status.host}"},
			},
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("ready")

		b := makeBinding("ready-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		// First reconcile creates the RenderedRelease.
		_ = reconcileBinding(b)
		rr := fetchRenderedRelease(b)

		By("simulating the renderedrelease controller writing back observed status")
		setRenderedReleaseStatus(rr, []openchoreov1alpha1.RenderedManifestStatus{
			{
				ID:           "claim",
				Group:        "",
				Version:      "v1",
				Kind:         "ConfigMap",
				Name:         "ready-claim",
				HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
				Status:       &runtime.RawExtension{Raw: []byte(`{"host":"10.0.0.5"}`)},
			},
		})

		updated := reconcileBinding(b)

		By("setting ResourcesReady=True, OutputsResolved=True, and aggregate Ready=True")
		resReady := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionResourcesReady))
		Expect(resReady).NotTo(BeNil())
		Expect(resReady.Status).To(Equal(metav1.ConditionTrue))
		Expect(resReady.Reason).To(Equal(string(ReasonResourcesReady)))

		outputs := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionOutputsResolved))
		Expect(outputs).NotTo(BeNil())
		Expect(outputs.Status).To(Equal(metav1.ConditionTrue))
		Expect(outputs.Reason).To(Equal(string(ReasonOutputsResolved)))

		ready := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionReady))
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.Reason).To(Equal(string(ReasonReady)))

		By("populating status.outputs with the resolved value")
		Expect(updated.Status.Outputs).To(HaveLen(1))
		Expect(updated.Status.Outputs[0].Name).To(Equal("host"))
		Expect(updated.Status.Outputs[0].Value).To(Equal("10.0.0.5"))
	})

	It("returns ResourcesReady and OutputsResolved from Unknown to True when validation recovers", func() {
		env := makeEnvAndDP("recovery")

		// Create the binding first, pinned to a release that doesn't exist
		// yet — this is a real GitOps scenario (apply everything at once,
		// the binding races ahead of its release).
		b := makeBinding("recovery-binding", "recovery-release", env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		// First reconcile: release missing → Synced=False, deps Unknown.
		failed := reconcileBinding(b)
		failedSynced := meta.FindStatusCondition(failed.Status.Conditions, string(ConditionSynced))
		Expect(failedSynced).NotTo(BeNil())
		Expect(failedSynced.Status).To(Equal(metav1.ConditionFalse))
		Expect(failedSynced.Reason).To(Equal(string(ReasonResourceReleaseNotFound)))
		failedRR := meta.FindStatusCondition(failed.Status.Conditions, string(ConditionResourcesReady))
		Expect(failedRR).NotTo(BeNil())
		Expect(failedRR.Status).To(Equal(metav1.ConditionUnknown))
		failedOR := meta.FindStatusCondition(failed.Status.Conditions, string(ConditionOutputsResolved))
		Expect(failedOR).NotTo(BeNil())
		Expect(failedOR.Status).To(Equal(metav1.ConditionUnknown))

		By("creating the release so validation can succeed on the next reconcile")
		release := snapshotWith("recovery-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("recovery-claim")},
			},
			[]openchoreov1alpha1.ResourceTypeOutput{
				{Name: "host", Value: "${applied.claim.status.host}"},
			},
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		// Second reconcile: render succeeds; deps remain Unknown until the
		// renderedrelease writeback because nothing has been observed yet.
		_ = reconcileBinding(b)
		rr := fetchRenderedRelease(b)
		setRenderedReleaseStatus(rr, []openchoreov1alpha1.RenderedManifestStatus{
			{
				ID:           "claim",
				Version:      "v1",
				Kind:         "ConfigMap",
				Name:         "recovery-claim",
				HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
				Status:       &runtime.RawExtension{Raw: []byte(`{"host":"10.0.0.5"}`)},
			},
		})

		// Third reconcile: now everything resolves.
		recovered := reconcileBinding(b)

		recoveredSynced := meta.FindStatusCondition(recovered.Status.Conditions, string(ConditionSynced))
		Expect(recoveredSynced.Status).To(Equal(metav1.ConditionTrue))
		recoveredRR := meta.FindStatusCondition(recovered.Status.Conditions, string(ConditionResourcesReady))
		Expect(recoveredRR.Status).To(Equal(metav1.ConditionTrue),
			"ResourcesReady must leave Unknown and reflect the live evaluation")
		recoveredOR := meta.FindStatusCondition(recovered.Status.Conditions, string(ConditionOutputsResolved))
		Expect(recoveredOR.Status).To(Equal(metav1.ConditionTrue),
			"OutputsResolved must leave Unknown and reflect the live evaluation")
		recoveredReady := meta.FindStatusCondition(recovered.Status.Conditions, string(ConditionReady))
		Expect(recoveredReady.Status).To(Equal(metav1.ConditionTrue))
	})

	It("does not gate ResourcesReady on entries filtered out by IncludeWhen", func() {
		release := snapshotWith("filter-readiness-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("filter-readiness-claim")},
				{ID: "tls-cert", IncludeWhen: "${1==2}", Template: configMapTemplate("filter-readiness-tls")},
			},
			nil,
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("filter-readiness")

		b := makeBinding("filter-readiness-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		_ = reconcileBinding(b)
		rr := fetchRenderedRelease(b)

		// Only the rendered entry has observed status; the filtered entry
		// never produced a manifest and never will.
		setRenderedReleaseStatus(rr, []openchoreov1alpha1.RenderedManifestStatus{
			{
				ID:           "claim",
				Version:      "v1",
				Kind:         "ConfigMap",
				Name:         "filter-readiness-claim",
				HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
			},
		})

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionResourcesReady))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue),
			"filtered IncludeWhen entries must not gate readiness; only rendered entries count")
		Expect(cond.Reason).To(Equal(string(ReasonResourcesReady)))
	})

	It("forces ResourcesReady and OutputsResolved to Unknown when Synced flips to False after a successful render", func() {
		release := snapshotWith("transition-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("transition-claim")},
			},
			[]openchoreov1alpha1.ResourceTypeOutput{
				{Name: "host", Value: "${applied.claim.status.host}"},
			},
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		releaseDeleted := false
		DeferCleanup(func() {
			if !releaseDeleted {
				_ = k8sClient.Delete(ctx, release)
			}
		})

		env := makeEnvAndDP("transition")

		b := makeBinding("transition-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		// First reconcile: Synced=True, ResourcesReady evaluated, OutputsResolved evaluated.
		_ = reconcileBinding(b)
		rr := fetchRenderedRelease(b)
		setRenderedReleaseStatus(rr, []openchoreov1alpha1.RenderedManifestStatus{
			{
				ID:           "claim",
				Version:      "v1",
				Kind:         "ConfigMap",
				Name:         "transition-claim",
				HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
				Status:       &runtime.RawExtension{Raw: []byte(`{"host":"10.0.0.5"}`)},
			},
		})
		afterSuccess := reconcileBinding(b)

		Expect(meta.FindStatusCondition(afterSuccess.Status.Conditions, string(ConditionResourcesReady)).Status).
			To(Equal(metav1.ConditionTrue))
		Expect(meta.FindStatusCondition(afterSuccess.Status.Conditions, string(ConditionOutputsResolved)).Status).
			To(Equal(metav1.ConditionTrue))

		By("deleting the ResourceRelease so the next reconcile fails validation")
		Expect(k8sClient.Delete(ctx, release)).To(Succeed())
		releaseDeleted = true

		afterFailure := reconcileBinding(b)

		synced := meta.FindStatusCondition(afterFailure.Status.Conditions, string(ConditionSynced))
		Expect(synced).NotTo(BeNil())
		Expect(synced.Status).To(Equal(metav1.ConditionFalse))
		Expect(synced.Reason).To(Equal(string(ReasonResourceReleaseNotFound)))

		By("forcing dependent sub-conditions to Unknown so per-axis status is coherent")
		resReady := meta.FindStatusCondition(afterFailure.Status.Conditions, string(ConditionResourcesReady))
		Expect(resReady).NotTo(BeNil())
		Expect(resReady.Status).To(Equal(metav1.ConditionUnknown),
			"ResourcesReady=True is stale once Synced=False; must reset to Unknown")

		outputs := meta.FindStatusCondition(afterFailure.Status.Conditions, string(ConditionOutputsResolved))
		Expect(outputs).NotTo(BeNil())
		Expect(outputs.Status).To(Equal(metav1.ConditionUnknown))
	})

	It("aggregates Ready from sub-conditions: ResourcesReady=False (Progressing) → Ready=False with the same reason and message", func() {
		release := snapshotWith("aggregate-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("aggregate-claim")},
			},
			nil,
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("aggregate")

		b := makeBinding("aggregate-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		// Single reconcile: Synced=True (Created), OutputsResolved=True
		// (no outputs declared), ResourcesReady=False (no observed status).
		// Ready aggregate must surface the failing sub-condition.
		updated := reconcileBinding(b)

		ready := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionReady))
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(string(ReasonResourcesProgressing)))

		resReady := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionResourcesReady))
		Expect(resReady).NotTo(BeNil())
		Expect(ready.Message).To(Equal(resReady.Message), "Ready inherits the failing sub-condition's message")
	})

	It("reports OutputsResolved=True with no entries when the snapshot declares no outputs", func() {
		release := snapshotWith("no-outputs-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("no-outputs-claim")},
			},
			nil,
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("no-outputs")

		b := makeBinding("no-outputs-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		_ = reconcileBinding(b)
		rr := fetchRenderedRelease(b)

		setRenderedReleaseStatus(rr, []openchoreov1alpha1.RenderedManifestStatus{
			{
				ID:           "claim",
				Version:      "v1",
				Kind:         "ConfigMap",
				Name:         "no-outputs-claim",
				HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
			},
		})

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionOutputsResolved))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(string(ReasonOutputsResolved)))
		Expect(updated.Status.Outputs).To(BeEmpty())
	})

	It("decouples ResourcesReady and OutputsResolved: a healthy claim with a misspelled output ref reports ResourcesReady=True, OutputsResolved=False", func() {
		release := snapshotWith("decoupled-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("decoupled-claim")},
			},
			[]openchoreov1alpha1.ResourceTypeOutput{
				// Misspelled — observed status only has "host".
				{Name: "host", Value: "${applied.claim.status.hsot}"},
			},
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("decoupled")

		b := makeBinding("decoupled-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		_ = reconcileBinding(b)
		rr := fetchRenderedRelease(b)

		setRenderedReleaseStatus(rr, []openchoreov1alpha1.RenderedManifestStatus{
			{
				ID:           "claim",
				Version:      "v1",
				Kind:         "ConfigMap",
				Name:         "decoupled-claim",
				HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
				Status:       &runtime.RawExtension{Raw: []byte(`{"host":"db.example.com"}`)},
			},
		})

		updated := reconcileBinding(b)

		resReady := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionResourcesReady))
		Expect(resReady).NotTo(BeNil())
		Expect(resReady.Status).To(Equal(metav1.ConditionTrue), "ResourcesReady is independent of output resolution")

		outputs := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionOutputsResolved))
		Expect(outputs).NotTo(BeNil())
		Expect(outputs.Status).To(Equal(metav1.ConditionFalse))
		Expect(outputs.Reason).To(Equal(string(ReasonOutputResolutionFailed)))
	})

	It("ignores a stale ResourcesApplied=False (older ObservedGeneration) and evaluates per-entry health normally", func() {
		release := snapshotWith("stale-apply-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("stale-apply-claim")},
			},
			nil,
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("stale-apply")

		b := makeBinding("stale-apply-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		_ = reconcileBinding(b)
		rr := fetchRenderedRelease(b)

		// Pre-current-generation apply failure (ObservedGeneration=0 against
		// rr.Generation=1) plus a healthy entry observed for the current
		// generation. The stale apply condition must be ignored; readiness
		// flows through the per-Kind health path.
		rr.Status.Resources = []openchoreov1alpha1.RenderedManifestStatus{
			{
				ID:           "claim",
				Version:      "v1",
				Kind:         "ConfigMap",
				Name:         "stale-apply-claim",
				HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
			},
		}
		rr.Status.Conditions = []metav1.Condition{
			{
				Type:               renderedrelease.ConditionResourcesApplied,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: rr.Generation - 1,
				Reason:             "StaleApplyError",
				Message:            "stale message that must not surface on the binding",
				LastTransitionTime: metav1.Now(),
			},
		}
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionResourcesReady))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(string(ReasonResourcesReady)))
	})

	It("reports ResourcesReady=False, Reason=ResourceApplyFailed when ResourcesApplied=False matches the current RR generation", func() {
		release := snapshotWith("apply-failed-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("apply-failed-claim")},
			},
			nil,
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("apply-failed")

		b := makeBinding("apply-failed-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		_ = reconcileBinding(b)
		rr := fetchRenderedRelease(b)

		// Simulate the renderedrelease controller reporting an apply failure
		// for the current generation.
		rr.Status.Conditions = []metav1.Condition{
			{
				Type:               renderedrelease.ConditionResourcesApplied,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: rr.Generation,
				Reason:             "ApplyError",
				Message:            "kube-apiserver rejected ConfigMap apply-failed-claim",
				LastTransitionTime: metav1.Now(),
			},
		}
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionResourcesReady))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonResourceApplyFailed)))
		Expect(cond.Message).To(ContainSubstring("kube-apiserver rejected"))
	})

	It("populates status.outputs with all three output kinds (value, secretKeyRef, configMapKeyRef)", func() {
		release := snapshotWith("kinds-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("kinds-claim")},
			},
			[]openchoreov1alpha1.ResourceTypeOutput{
				{Name: "host", Value: "${applied.claim.status.host}"},
				{Name: "password", SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{
					Name: "${metadata.resourceName}-conn",
					Key:  "password",
				}},
				{Name: "caCert", ConfigMapKeyRef: &openchoreov1alpha1.ConfigMapKeyRef{
					Name: "${metadata.resourceName}-tls",
					Key:  "ca.crt",
				}},
			},
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("kinds")

		b := makeBinding("kinds-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		_ = reconcileBinding(b)
		rr := fetchRenderedRelease(b)

		setRenderedReleaseStatus(rr, []openchoreov1alpha1.RenderedManifestStatus{
			{
				ID:           "claim",
				Version:      "v1",
				Kind:         "ConfigMap",
				Name:         "kinds-claim",
				HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
				Status:       &runtime.RawExtension{Raw: []byte(`{"host":"db.example.com"}`)},
			},
		})

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionOutputsResolved))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))

		Expect(updated.Status.Outputs).To(HaveLen(3))

		byName := map[string]openchoreov1alpha1.ResolvedResourceOutput{}
		for _, out := range updated.Status.Outputs {
			byName[out.Name] = out
		}

		hostOut := byName["host"]
		Expect(hostOut.Value).To(Equal("db.example.com"))
		Expect(hostOut.SecretKeyRef).To(BeNil())
		Expect(hostOut.ConfigMapKeyRef).To(BeNil())

		passwordOut := byName["password"]
		Expect(passwordOut.Value).To(BeEmpty())
		Expect(passwordOut.SecretKeyRef).NotTo(BeNil())
		Expect(passwordOut.SecretKeyRef.Name).To(Equal("test-resource-conn"))
		Expect(passwordOut.SecretKeyRef.Key).To(Equal("password"))
		Expect(passwordOut.ConfigMapKeyRef).To(BeNil())

		caCertOut := byName["caCert"]
		Expect(caCertOut.Value).To(BeEmpty())
		Expect(caCertOut.SecretKeyRef).To(BeNil())
		Expect(caCertOut.ConfigMapKeyRef).NotTo(BeNil())
		Expect(caCertOut.ConfigMapKeyRef.Name).To(Equal("test-resource-tls"))
		Expect(caCertOut.ConfigMapKeyRef.Key).To(Equal("ca.crt"))
	})

	It("reports OutputsResolved=False, Reason=OutputResolutionFailed when an output's CEL references a missing applied path; partial outputs are preserved", func() {
		release := snapshotWith("partial-outputs-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("partial-claim")},
			},
			[]openchoreov1alpha1.ResourceTypeOutput{
				{Name: "host", Value: "${applied.claim.status.host}"},
				{Name: "missing", Value: "${applied.claim.status.notThere}"},
			},
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("partial-outputs")

		b := makeBinding("partial-outputs-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		_ = reconcileBinding(b)
		rr := fetchRenderedRelease(b)

		setRenderedReleaseStatus(rr, []openchoreov1alpha1.RenderedManifestStatus{
			{
				ID:           "claim",
				Version:      "v1",
				Kind:         "ConfigMap",
				Name:         "partial-claim",
				HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
				Status:       &runtime.RawExtension{Raw: []byte(`{"host":"10.0.0.5"}`)},
			},
		})

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionOutputsResolved))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonOutputResolutionFailed)))

		By("preserving the successfully-resolved output entry in status.outputs")
		Expect(updated.Status.Outputs).To(HaveLen(1))
		Expect(updated.Status.Outputs[0].Name).To(Equal("host"))
		Expect(updated.Status.Outputs[0].Value).To(Equal("10.0.0.5"))
	})

	It("reports ResourcesReady=False, Reason=ResourcesDegraded when an entry's healthStatus is Degraded", func() {
		release := snapshotWith("degraded-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("degraded-claim")},
			},
			nil,
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("degraded")

		b := makeBinding("degraded-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		_ = reconcileBinding(b)
		rr := fetchRenderedRelease(b)

		setRenderedReleaseStatus(rr, []openchoreov1alpha1.RenderedManifestStatus{
			{
				ID:           "claim",
				Version:      "v1",
				Kind:         "ConfigMap",
				Name:         "degraded-claim",
				HealthStatus: openchoreov1alpha1.HealthStatusDegraded,
			},
		})

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionResourcesReady))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonResourcesDegraded)))
	})

	It("reports ResourcesReady=False, Reason=ResourcesProgressing when the applied object has not yet been observed", func() {
		release := snapshotWith("missing-applied-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: configMapTemplate("missing-applied-claim")},
			},
			nil,
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("missing-applied")

		b := makeBinding("missing-applied-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		// Reconcile creates the RenderedRelease, but no observed status is
		// written back — the renderedrelease controller hasn't run yet.
		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionResourcesReady))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonResourcesProgressing)))
	})

	It("reports ResourcesReady=False, Reason=ResourcesProgressing when an entry's readyWhen returns false", func() {
		release := snapshotWith("readywhen-false-release",
			[]openchoreov1alpha1.ResourceTypeManifest{
				{
					ID:        "claim",
					Template:  configMapTemplate("readywhen-claim"),
					ReadyWhen: "${applied.claim.status.phase == 'Done'}",
				},
			},
			nil,
		)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		env := makeEnvAndDP("readywhen-false")

		b := makeBinding("readywhen-false-binding", release.Name, env.Name)
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			cleanupReleases(b)
			_ = k8sClient.Delete(ctx, b)
		})

		_ = reconcileBinding(b)
		rr := fetchRenderedRelease(b)

		// Phase is "Pending"; the readyWhen requires "Done".
		setRenderedReleaseStatus(rr, []openchoreov1alpha1.RenderedManifestStatus{
			{
				ID:           "claim",
				Version:      "v1",
				Kind:         "ConfigMap",
				Name:         "readywhen-claim",
				HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
				Status:       &runtime.RawExtension{Raw: []byte(`{"phase":"Pending"}`)},
			},
		})

		updated := reconcileBinding(b)

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionResourcesReady))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(ReasonResourcesProgressing)))
	})
})

// metadata-name covers the platform-computed base name and label set the
// pipeline exposes to PE templates. The Component side and Resource side
// share a DP namespace, so the name shape needs to keep Resource-emitted
// objects from clashing with Component-emitted ones.
var _ = Describe("ResourceReleaseBinding controller — metadata.name and labels", func() {
	makeBinding := func(resourceName, env string) *openchoreov1alpha1.ResourceReleaseBinding {
		return &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName + "-" + env,
				Namespace: "default",
			},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "p",
					ResourceName: resourceName,
				},
				Environment: env,
			},
		}
	}
	envObj := func(name string) *openchoreov1alpha1.Environment {
		return &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: "env-uid"},
		}
	}
	dpObj := func() *openchoreov1alpha1.DataPlane {
		return &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "default-dp", UID: "dp-uid"},
		}
	}
	resourceObj := func(name string) *openchoreov1alpha1.Resource {
		return &openchoreov1alpha1.Resource{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: "res-uid"},
		}
	}
	projectObj := func() *openchoreov1alpha1.Project {
		return &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default", UID: "proj-uid"},
		}
	}

	It("adopts an r- prefix and uses a hidden discriminator so a Component named r-{resource} cannot collide", func() {
		// Resource "foo" → visible name r-foo-dev-{hash(r_foo-dev)}
		// Component "r-foo" → visible name r-foo-dev-{hash(r-foo-dev)}
		// Same visible base, different hash, different K8s name.
		resMeta := buildMetadataContext(makeBinding("foo", "dev"), envObj("dev"), dpObj(), resourceObj("foo"), projectObj())
		componentName := dpkubernetes.GenerateK8sName("r-foo", "dev")

		Expect(resMeta.Name).To(HavePrefix("r-foo-dev-"),
			"Resource-emitted name should carry the r- prefix")
		Expect(resMeta.Name).NotTo(Equal(componentName),
			"hash discriminator must keep the Resource name distinct from a Component named r-foo")
	})

	It("populates the per-Resource standard labels including UIDs", func() {
		meta := buildMetadataContext(makeBinding("mysql", "prod"), envObj("prod"), dpObj(), resourceObj("mysql"), projectObj())

		Expect(meta.Labels).To(HaveKeyWithValue(labels.LabelKeyResourceName, "mysql"))
		Expect(meta.Labels).To(HaveKeyWithValue(labels.LabelKeyResourceUID, "res-uid"))
		Expect(meta.Labels).To(HaveKeyWithValue(labels.LabelKeyProjectName, "p"))
		Expect(meta.Labels).To(HaveKeyWithValue(labels.LabelKeyProjectUID, "proj-uid"))
		Expect(meta.Labels).To(HaveKeyWithValue(labels.LabelKeyEnvironmentName, "prod"))
		Expect(meta.Labels).To(HaveKeyWithValue(labels.LabelKeyEnvironmentUID, "env-uid"))
	})
})
