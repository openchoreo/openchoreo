// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/projectreleasebinding"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
	"github.com/openchoreo/openchoreo/internal/labels"
	resourcepipeline "github.com/openchoreo/openchoreo/internal/pipeline/resource"
)

var _ = Describe("ResourceReleaseBinding — Observe mode", func() {
	// The suite creates Project test-project and Resource test-resource in
	// default. Each test uses its own environment, so releases don't collide.
	const (
		project   = "test-project"
		namespace = "boutique-prod"
	)

	deployment := openchoreov1alpha1.ObservedResourceClaim{APIVersion: "apps/v1", Kind: "Deployment", Name: "redis-cart"}
	service := openchoreov1alpha1.ObservedResourceClaim{APIVersion: "v1", Kind: "Service", Name: "redis-cart"}

	var reconciler *Reconciler
	BeforeEach(func() {
		reconciler = &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Pipeline: resourcepipeline.NewPipeline()}
	})

	newBinding := func(name string, mode openchoreov1alpha1.BindingMode,
		claims ...openchoreov1alpha1.ObservedResourceClaim) *openchoreov1alpha1.ResourceReleaseBinding {
		b := &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: project, ResourceName: "test-resource"},
				Environment: name,
				Mode:        mode,
			},
		}
		if len(claims) > 0 {
			b.Spec.Observe = &openchoreov1alpha1.ResourceReleaseBindingObserve{Resources: claims}
		}
		return b
	}

	create := func(b *openchoreov1alpha1.ResourceReleaseBinding) *openchoreov1alpha1.ResourceReleaseBinding {
		GinkgoHelper()
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, b) })
		return b
	}

	releaseKey := func(b *openchoreov1alpha1.ResourceReleaseBinding) types.NamespacedName {
		return types.NamespacedName{Name: makeRenderedReleaseName(b), Namespace: "default"}
	}

	// createWithFinalizer creates b with the cleanup finalizer, so one reconcile
	// runs the whole path, and removes it and its release after the test.
	createWithFinalizer := func(b *openchoreov1alpha1.ResourceReleaseBinding) *openchoreov1alpha1.ResourceReleaseBinding {
		GinkgoHelper()
		b.Finalizers = []string{ResourceReleaseBindingFinalizer}
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			rr := &openchoreov1alpha1.RenderedRelease{}
			if err := k8sClient.Get(ctx, releaseKey(b), rr); err == nil {
				_ = k8sClient.Delete(ctx, rr)
			}
			current := &openchoreov1alpha1.ResourceReleaseBinding{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(b), current); err == nil {
				current.Finalizers = nil
				_ = k8sClient.Update(ctx, current)
				_ = k8sClient.Delete(ctx, current)
			}
		})
		return b
	}

	// observed creates a binding in Observe mode for test-resource in env.
	observed := func(env string, claims ...openchoreov1alpha1.ObservedResourceClaim) *openchoreov1alpha1.ResourceReleaseBinding {
		GinkgoHelper()
		return createWithFinalizer(newBinding(env, openchoreov1alpha1.BindingModeObserve, claims...))
	}

	makePRB := func(name, env string, mode openchoreov1alpha1.BindingMode, reported string) *openchoreov1alpha1.ProjectReleaseBinding {
		GinkgoHelper()
		prb := &openchoreov1alpha1.ProjectReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: project},
				Environment: env,
				Mode:        mode,
			},
		}
		if mode == openchoreov1alpha1.BindingModeObserve {
			prb.Spec.Observe = &openchoreov1alpha1.ProjectReleaseBindingObserve{Namespace: namespace}
		}
		Expect(k8sClient.Create(ctx, prb)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, prb) })
		if reported != "" {
			prb.Status.Namespace = reported
			Expect(k8sClient.Status().Update(ctx, prb)).To(Succeed())
		}
		return prb
	}

	reconcileBinding := func(b *openchoreov1alpha1.ResourceReleaseBinding) *openchoreov1alpha1.ResourceReleaseBinding {
		GinkgoHelper()
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)})
		Expect(err).NotTo(HaveOccurred())
		updated := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), updated)).To(Succeed())
		return updated
	}

	releaseOf := func(b *openchoreov1alpha1.ResourceReleaseBinding) (*openchoreov1alpha1.RenderedRelease, error) {
		rr := &openchoreov1alpha1.RenderedRelease{}
		return rr, k8sClient.Get(ctx, releaseKey(b), rr)
	}

	condition := func(b *openchoreov1alpha1.ResourceReleaseBinding, t controller.ConditionType) *metav1.Condition {
		return meta.FindStatusCondition(b.Status.Conditions, string(t))
	}

	// readObserved writes what the RenderedRelease controller reports after
	// reading the claimed objects at the release's current generation.
	readObserved := func(b *openchoreov1alpha1.ResourceReleaseBinding, health ...openchoreov1alpha1.HealthStatus) {
		GinkgoHelper()
		rr, err := releaseOf(b)
		Expect(err).NotTo(HaveOccurred())
		rr.Status.ObservedResources = nil
		for i, entry := range rr.Spec.ObservedResources {
			rr.Status.ObservedResources = append(rr.Status.ObservedResources, openchoreov1alpha1.RenderedManifestStatus{
				ID: entry.ID, Version: "v1", Kind: entry.Kind, Name: entry.Name, Namespace: entry.Namespace,
				HealthStatus: health[i],
			})
		}
		meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
			Type:               string(renderedrelease.ConditionObservedResourcesRead),
			Status:             metav1.ConditionTrue,
			Reason:             string(renderedrelease.ReasonObservedReadSucceeded),
			Message:            "Read",
			ObservedGeneration: rr.Generation,
		})
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())
	}

	Context("validation", func() {
		It("defaults mode to Manage", func() {
			b := create(newBinding("rrb-default-mode", ""))
			Expect(b.Spec.Mode).To(Equal(openchoreov1alpha1.BindingModeManage))
		})

		It("rejects an unknown mode", func() {
			b := newBinding("rrb-unknown-mode", "Watch")
			Expect(k8sClient.Create(ctx, b)).To(MatchError(ContainSubstring("spec.mode")))
		})

		It("rejects the same claim twice", func() {
			b := newBinding("rrb-duplicate-claim", openchoreov1alpha1.BindingModeObserve, deployment, deployment)
			Expect(k8sClient.Create(ctx, b)).To(MatchError(ContainSubstring("Duplicate value")))
		})

		It("takes the same name under another kind or version", func() {
			hpaV1 := openchoreov1alpha1.ObservedResourceClaim{APIVersion: "autoscaling/v1", Kind: "HorizontalPodAutoscaler", Name: "redis-cart"}
			hpaV2 := openchoreov1alpha1.ObservedResourceClaim{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler", Name: "redis-cart"}
			b := create(newBinding("rrb-same-name", openchoreov1alpha1.BindingModeObserve, deployment, service, hpaV1, hpaV2))
			Expect(b.Spec.Observe.Resources).To(HaveLen(4))
		})

		It("lets the mode change", func() {
			b := create(newBinding("rrb-mode-change", openchoreov1alpha1.BindingModeManage))
			current := &openchoreov1alpha1.ResourceReleaseBinding{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), current)).To(Succeed())
			current.Spec.Mode = openchoreov1alpha1.BindingModeObserve
			current.Spec.Observe = &openchoreov1alpha1.ResourceReleaseBindingObserve{
				Resources: []openchoreov1alpha1.ObservedResourceClaim{deployment},
			}
			Expect(k8sClient.Update(ctx, current)).To(Succeed())
			Expect(current.Spec.IsObserve()).To(BeTrue())
		})
	})

	It("observes the claimed objects in the ProjectReleaseBinding's namespace without a pinned release", func() {
		makePRB("obs-claims", "obs-claims", openchoreov1alpha1.BindingModeObserve, namespace)
		b := reconcileBinding(observed("obs-claims", deployment, service))

		rr, err := releaseOf(b)
		Expect(err).NotTo(HaveOccurred())
		Expect(rr.Spec.Mode).To(Equal(openchoreov1alpha1.RenderedReleaseModeObserve))
		Expect(rr.Spec.Resources).To(BeEmpty(), "nothing is rendered")
		Expect(rr.Spec.Owner).To(Equal(openchoreov1alpha1.RenderedReleaseOwner{ProjectName: project, ResourceName: "test-resource"}))
		Expect(rr.Labels).To(HaveKeyWithValue(labels.LabelKeyResourceName, "test-resource"))
		Expect(metav1.IsControlledBy(rr, b)).To(BeTrue())
		Expect(rr.Spec.ObservedResources).To(ConsistOf(
			openchoreov1alpha1.ObservedResource{
				ID:         controller.ObservedResourceID("apps/v1", "Deployment", namespace, "redis-cart"),
				APIVersion: "apps/v1", Kind: "Deployment", Name: "redis-cart", Namespace: namespace,
			},
			openchoreov1alpha1.ObservedResource{
				ID:         controller.ObservedResourceID("v1", "Service", namespace, "redis-cart"),
				APIVersion: "v1", Kind: "Service", Name: "redis-cart", Namespace: namespace,
			},
		))

		Expect(condition(b, ConditionSynced).Reason).To(Equal(string(ReasonReleaseCreated)))
		Expect(condition(b, ConditionResourcesReady).Reason).To(Equal(string(ReasonResourcesProgressing)),
			"waits for the release to read the claims")
		Expect(condition(b, ConditionOutputsResolved)).To(BeNil())
		Expect(condition(b, ConditionReady).Status).To(Equal(metav1.ConditionFalse))
	})

	It("reports health from the observed resources", func() {
		makePRB("obs-health", "obs-health", openchoreov1alpha1.BindingModeObserve, namespace)
		b := reconcileBinding(observed("obs-health", deployment, service))

		readObserved(b, openchoreov1alpha1.HealthStatusSuspended, openchoreov1alpha1.HealthStatusHealthy)
		b = reconcileBinding(b)
		Expect(condition(b, ConditionResourcesReady).Status).To(Equal(metav1.ConditionTrue))
		Expect(condition(b, ConditionReady).Status).To(Equal(metav1.ConditionTrue), "no outputs are needed")
		Expect(b.Status.Outputs).To(BeEmpty())

		readObserved(b, openchoreov1alpha1.HealthStatusUnknown, openchoreov1alpha1.HealthStatusDegraded)
		b = reconcileBinding(b)
		Expect(condition(b, ConditionResourcesReady).Reason).To(Equal(string(ReasonResourcesDegraded)),
			"Degraded wins over Unknown whatever the order")
		Expect(condition(b, ConditionReady).Status).To(Equal(metav1.ConditionFalse))

		readObserved(b, openchoreov1alpha1.HealthStatusUnknown, openchoreov1alpha1.HealthStatusHealthy)
		b = reconcileBinding(b)
		Expect(condition(b, ConditionResourcesReady).Reason).To(Equal(string(ReasonResourcesUnknown)))
		Expect(condition(b, ConditionResourcesReady).Message).To(ContainSubstring(`Deployment "redis-cart" could not be read`))

		By("A new claim waits for the release to read it")
		b.Spec.Observe.Resources = append(b.Spec.Observe.Resources,
			openchoreov1alpha1.ObservedResourceClaim{APIVersion: "v1", Kind: "ConfigMap", Name: "boutique-config"})
		Expect(k8sClient.Update(ctx, b)).To(Succeed())
		b = reconcileBinding(b)
		Expect(condition(b, ConditionResourcesReady).Reason).To(Equal(string(ReasonResourcesProgressing)))
	})

	DescribeTable("needs one observed ProjectReleaseBinding with a namespace",
		func(env string, setup func(env string), reason controller.ConditionReason) {
			setup(env)
			b := reconcileBinding(observed(env, deployment))
			Expect(condition(b, ConditionSynced).Reason).To(Equal(string(reason)))
			Expect(condition(b, ConditionReady).Status).To(Equal(metav1.ConditionFalse))
			_, err := releaseOf(b)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "no release is created")
		},
		Entry("none", "obs-none", func(string) {}, projectreleasebinding.ReasonProjectReleaseBindingNotFound),
		Entry("a managed one", "obs-managed", func(env string) {
			makePRB(env, env, openchoreov1alpha1.BindingModeManage, "dp-somewhere")
		}, projectreleasebinding.ReasonProjectNotObserved),
		Entry("an observed one without a namespace", "obs-not-allowed", func(env string) {
			makePRB(env, env, openchoreov1alpha1.BindingModeObserve, "")
		}, projectreleasebinding.ReasonNamespaceNotObserved),
		Entry("several", "obs-several", func(env string) {
			makePRB(env+"-a", env, openchoreov1alpha1.BindingModeObserve, namespace)
			makePRB(env+"-b", env, openchoreov1alpha1.BindingModeObserve, namespace)
		}, projectreleasebinding.ReasonProjectReleaseBindingConflict),
	)

	It("stops observing when the namespace is revoked", func() {
		prb := makePRB("obs-revoke", "obs-revoke", openchoreov1alpha1.BindingModeObserve, namespace)
		b := reconcileBinding(observed("obs-revoke", deployment))
		_, err := releaseOf(b)
		Expect(err).NotTo(HaveOccurred())

		prb.Status.Namespace = ""
		Expect(k8sClient.Status().Update(ctx, prb)).To(Succeed())
		b = reconcileBinding(b)
		Expect(condition(b, ConditionSynced).Reason).To(Equal(string(projectreleasebinding.ReasonNamespaceNotObserved)))
		_, err = releaseOf(b)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("reports nothing observed without claims", func() {
		makePRB("obs-empty", "obs-empty", openchoreov1alpha1.BindingModeObserve, namespace)
		b := reconcileBinding(observed("obs-empty"))
		Expect(condition(b, ConditionSynced).Reason).To(Equal(string(ReasonNothingObserved)))
		_, err := releaseOf(b)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("needs the Resource of its own project", func() {
		makePRB("obs-owner", "obs-owner", openchoreov1alpha1.BindingModeObserve, namespace)

		missing := newBinding("obs-owner", openchoreov1alpha1.BindingModeObserve, deployment)
		missing.Name = "obs-owner-missing"
		missing.Spec.Owner.ResourceName = "missing-resource"
		createWithFinalizer(missing)
		Expect(condition(reconcileBinding(missing), ConditionSynced).Reason).To(Equal(string(ReasonResourceNotFound)))

		other := &openchoreov1alpha1.Resource{
			ObjectMeta: metav1.ObjectMeta{Name: "other-project-cache", Namespace: "default"},
			Spec: openchoreov1alpha1.ResourceSpec{
				Owner: openchoreov1alpha1.ResourceOwner{ProjectName: "other-project"},
				Type:  openchoreov1alpha1.ResourceTypeRef{Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType, Name: "valkey"},
			},
		}
		Expect(k8sClient.Create(ctx, other)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, other) })
		foreign := newBinding("obs-owner", openchoreov1alpha1.BindingModeObserve, deployment)
		foreign.Name = "obs-owner-foreign"
		foreign.Spec.Owner.ResourceName = other.Name
		createWithFinalizer(foreign)
		b := reconcileBinding(foreign)
		Expect(condition(b, ConditionSynced).Reason).To(Equal(string(ReasonResourceProjectMismatch)))
		_, err := releaseOf(b)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("clears the outputs Manage mode reported once it observes, and not before", func() {
		prb := makePRB("obs-outputs", "obs-outputs", openchoreov1alpha1.BindingModeManage, "dp-somewhere")
		b := observed("obs-outputs", deployment)
		b.Status.Outputs = []openchoreov1alpha1.ResolvedResourceOutput{{Name: "host", Value: "redis-cart.boutique-prod"}}
		meta.SetStatusCondition(&b.Status.Conditions, metav1.Condition{
			Type: string(ConditionOutputsResolved), Status: metav1.ConditionTrue, Reason: string(ReasonOutputsResolved),
		})
		Expect(k8sClient.Status().Update(ctx, b)).To(Succeed())

		By("Under a managed ProjectReleaseBinding it can't observe, so the outputs stay")
		b = reconcileBinding(b)
		Expect(b.Status.Outputs).To(HaveLen(1))

		By("Once the release is in Observe mode, they go")
		Expect(k8sClient.Delete(ctx, prb)).To(Succeed())
		makePRB("obs-outputs-observed", "obs-outputs", openchoreov1alpha1.BindingModeObserve, namespace)
		b = reconcileBinding(b)
		Expect(b.Status.Outputs).To(BeEmpty())
		Expect(condition(b, ConditionOutputsResolved)).To(BeNil())
	})

	It("keeps an Observe-mode release after a switch to Manage while allowed, and deletes it once revoked", func() {
		prb := makePRB("obs-switch", "obs-switch", openchoreov1alpha1.BindingModeObserve, namespace)
		b := reconcileBinding(observed("obs-switch", deployment))

		b.Spec.Mode = openchoreov1alpha1.BindingModeManage
		Expect(k8sClient.Update(ctx, b)).To(Succeed())
		b = reconcileBinding(b)
		rr, err := releaseOf(b)
		Expect(err).NotTo(HaveOccurred())
		Expect(rr.Spec.Mode).To(Equal(openchoreov1alpha1.RenderedReleaseModeObserve))
		Expect(condition(b, ConditionSynced).Reason).To(Equal(string(projectreleasebinding.ReasonProjectObserved)))

		prb.Status.Namespace = ""
		Expect(k8sClient.Status().Update(ctx, prb)).To(Succeed())
		reconcileBinding(b)
		_, err = releaseOf(b)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	Context("deletion", func() {
		// finalize reconciles a deleted binding until it's gone or n passes ran,
		// and returns the reason Finalizing last showed.
		finalize := func(b *openchoreov1alpha1.ResourceReleaseBinding, n int) (gone bool, reason string) {
			GinkgoHelper()
			Expect(k8sClient.Delete(ctx, b)).To(Succeed())
			for range n {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)})
				Expect(err).NotTo(HaveOccurred())
				current := &openchoreov1alpha1.ResourceReleaseBinding{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(b), current); apierrors.IsNotFound(err) {
					return true, reason
				}
				if c := condition(current, ConditionFinalizing); c != nil {
					reason = c.Reason
				}
			}
			return false, reason
		}

		It("doesn't hold an observed binding for retainPolicy Retain", func() {
			makePRB("obs-retain", "obs-retain", openchoreov1alpha1.BindingModeObserve, namespace)
			b := observed("obs-retain", deployment)
			b.Spec.RetainPolicy = openchoreov1alpha1.ResourceRetainPolicyRetain
			Expect(k8sClient.Update(ctx, b)).To(Succeed())
			b = reconcileBinding(b)
			_, err := releaseOf(b)
			Expect(err).NotTo(HaveOccurred())

			gone, reason := finalize(b, 4)
			Expect(gone).To(BeTrue())
			Expect(reason).NotTo(Equal(string(ReasonRetainHold)))
			_, err = releaseOf(b)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("holds for Retain while its release in Manage mode has applied objects", func() {
			makePRB("obs-retain-applied", "obs-retain-applied", openchoreov1alpha1.BindingModeManage, "dp-somewhere")
			b := observed("obs-retain-applied", deployment)
			b.Spec.RetainPolicy = openchoreov1alpha1.ResourceRetainPolicyRetain
			Expect(k8sClient.Update(ctx, b)).To(Succeed())

			rr := &openchoreov1alpha1.RenderedRelease{
				ObjectMeta: metav1.ObjectMeta{Name: releaseKey(b).Name, Namespace: "default"},
				Spec: openchoreov1alpha1.RenderedReleaseSpec{
					Owner:           openchoreov1alpha1.RenderedReleaseOwner{ProjectName: project, ResourceName: "test-resource"},
					EnvironmentName: "obs-retain-applied",
					TargetPlane:     openchoreov1alpha1.TargetPlaneDataPlane,
					Mode:            openchoreov1alpha1.RenderedReleaseModeManage,
				},
			}
			Expect(controllerutil.SetControllerReference(b, rr, k8sClient.Scheme())).To(Succeed())
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			rr.Status.Resources = []openchoreov1alpha1.RenderedManifestStatus{
				{ID: "statefulset", Group: "apps", Version: "v1", Kind: "StatefulSet", Name: "cache", Namespace: "dp-somewhere"},
			}
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			// Two passes, as in the Manage-mode Retain test: the first marks the
			// binding Finalizing, the second holds it.
			gone, reason := finalize(b, 2)
			Expect(gone).To(BeFalse())
			Expect(reason).To(Equal(string(ReasonRetainHold)))
			_, err := releaseOf(b)
			Expect(err).NotTo(HaveOccurred(), "what Manage mode applied stays")
		})

		It("never deletes a release another object controls", func() {
			squatter := create(newBinding("obs-squatter", openchoreov1alpha1.BindingModeManage))
			b := observed("obs-squatter-victim", deployment)
			rr := &openchoreov1alpha1.RenderedRelease{
				ObjectMeta: metav1.ObjectMeta{Name: releaseKey(b).Name, Namespace: "default"},
				Spec: openchoreov1alpha1.RenderedReleaseSpec{
					Owner:           openchoreov1alpha1.RenderedReleaseOwner{ProjectName: project, ResourceName: "test-resource"},
					EnvironmentName: "obs-squatter-victim",
					TargetPlane:     openchoreov1alpha1.TargetPlaneDataPlane,
				},
			}
			Expect(controllerutil.SetControllerReference(squatter, rr, k8sClient.Scheme())).To(Succeed())
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			gone, _ := finalize(b, 3)
			Expect(gone).To(BeTrue())
			_, err := releaseOf(b)
			Expect(err).NotTo(HaveOccurred(), "the other object's release stays")
		})
	})
})
