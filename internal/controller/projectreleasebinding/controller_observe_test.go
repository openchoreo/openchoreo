// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
)

var _ = Describe("ProjectReleaseBinding controller — Observe mode", func() {
	const (
		projectName       = "observe-project"
		observedNamespace = "payments"
	)

	var reconciler *Reconciler

	BeforeEach(func() {
		reconciler = &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	})

	makeEnvironment := func(prefix string, allowed ...string) *openchoreov1alpha1.Environment {
		project := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: projectName, Namespace: "default"},
			Spec: openchoreov1alpha1.ProjectSpec{
				DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
				Type:                  openchoreov1alpha1.ProjectTypeRef{Name: "default"},
			},
		}
		if err := k8sClient.Create(ctx, project); err != nil {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(project), project)).To(Succeed())
		}

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
		if len(allowed) > 0 {
			env.Spec.Observe = &openchoreov1alpha1.EnvironmentObserve{}
			for _, ns := range allowed {
				env.Spec.Observe.AllowedNamespaces = append(env.Spec.Observe.AllowedNamespaces,
					openchoreov1alpha1.AllowedNamespace{Name: ns})
			}
		}
		Expect(k8sClient.Create(ctx, env)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, env) })
		return env
	}

	observeBinding := func(name, env string, claims ...openchoreov1alpha1.ObservedResourceClaim) *openchoreov1alpha1.ProjectReleaseBinding {
		return &openchoreov1alpha1.ProjectReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "default",
				Finalizers: []string{ProjectReleaseBindingFinalizer},
			},
			Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: projectName},
				Environment: env,
				Mode:        openchoreov1alpha1.BindingModeObserve,
				Observe: &openchoreov1alpha1.ProjectReleaseBindingObserve{
					Namespace: observedNamespace,
					Resources: claims,
				},
			},
		}
	}

	create := func(b *openchoreov1alpha1.ProjectReleaseBinding) *openchoreov1alpha1.ProjectReleaseBinding {
		Expect(k8sClient.Create(ctx, b)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(b), b)
			b.Finalizers = nil
			_ = k8sClient.Update(ctx, b)
			_ = k8sClient.Delete(ctx, b)
		})
		return b
	}

	reconcileBinding := func(b *openchoreov1alpha1.ProjectReleaseBinding) *openchoreov1alpha1.ProjectReleaseBinding {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)})
		Expect(err).NotTo(HaveOccurred())
		updated := &openchoreov1alpha1.ProjectReleaseBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(b), updated)).To(Succeed())
		return updated
	}

	releaseKey := func(b *openchoreov1alpha1.ProjectReleaseBinding) types.NamespacedName {
		return types.NamespacedName{Name: makeRenderedReleaseName(b), Namespace: b.Namespace}
	}

	condition := func(b *openchoreov1alpha1.ProjectReleaseBinding, t controller.ConditionType) *metav1.Condition {
		return meta.FindStatusCondition(b.Status.Conditions, string(t))
	}

	Context("validation", func() {
		It("defaults mode to Manage", func() {
			b := &openchoreov1alpha1.ProjectReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "observe-default-mode", Namespace: "default"},
				Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
					Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: projectName},
					Environment: "dev",
				},
			}
			create(b)
			Expect(b.Spec.Mode).To(Equal(openchoreov1alpha1.BindingModeManage))
		})

		It("requires observe in Observe mode", func() {
			b := observeBinding("observe-missing", "dev")
			b.Spec.Observe = nil
			Expect(k8sClient.Create(ctx, b)).To(MatchError(ContainSubstring("spec.observe is required")))
		})

		It("rejects observe in Manage mode", func() {
			b := observeBinding("observe-in-manage", "dev")
			b.Spec.Mode = openchoreov1alpha1.BindingModeManage
			Expect(k8sClient.Create(ctx, b)).To(MatchError(ContainSubstring("not allowed otherwise")))
		})

		It("keeps the mode and the namespace immutable", func() {
			b := create(observeBinding("observe-immutable", "dev"))

			changed := b.DeepCopy()
			changed.Spec.Observe.Namespace = "orders"
			Expect(k8sClient.Update(ctx, changed)).To(MatchError(ContainSubstring("spec.observe.namespace is immutable")))

			changed = b.DeepCopy()
			changed.Spec.Mode = openchoreov1alpha1.BindingModeManage
			changed.Spec.Observe = nil
			Expect(k8sClient.Update(ctx, changed)).To(MatchError(ContainSubstring("spec.mode is immutable")))
		})

		It("rejects an allowed namespace that is not a namespace name", func() {
			env := &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Name: "observe-bad-allowlist", Namespace: "default"},
				Spec: openchoreov1alpha1.EnvironmentSpec{
					Observe: &openchoreov1alpha1.EnvironmentObserve{
						AllowedNamespaces: []openchoreov1alpha1.AllowedNamespace{{Name: "Payments_Prod"}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, env)).To(MatchError(ContainSubstring("allowedNamespaces[0].name")))
		})
	})

	It("observes an allowed namespace without a pinned ProjectRelease", func() {
		env := makeEnvironment("observe-allowed", "payments")
		b := create(observeBinding("observe-allowed", env.Name,
			openchoreov1alpha1.ObservedResourceClaim{APIVersion: "networking.k8s.io/v1", Kind: "NetworkPolicy", Name: "default-deny"},
			openchoreov1alpha1.ObservedResourceClaim{APIVersion: "autoscaling/v1", Kind: "HorizontalPodAutoscaler", Name: "api"},
			openchoreov1alpha1.ObservedResourceClaim{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler", Name: "api"},
		))

		updated := reconcileBinding(b)

		Expect(updated.Status.Namespace).To(Equal("payments"))
		synced := condition(updated, ConditionSynced)
		Expect(synced).NotTo(BeNil())
		Expect(synced.Status).To(Equal(metav1.ConditionTrue))
		Expect(synced.Message).To(ContainSubstring("autoscaling/v2 HorizontalPodAutoscaler api"))

		rr := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, releaseKey(b), rr)).To(Succeed())
		Expect(metav1.IsControlledBy(rr, updated)).To(BeTrue())
		Expect(rr.Spec.Mode).To(Equal(openchoreov1alpha1.RenderedReleaseModeObserve))
		Expect(rr.Spec.Resources).To(BeEmpty())
		Expect(rr.Spec.ObservedResources).To(Equal([]openchoreov1alpha1.ObservedResource{
			{ID: controller.ObservedResourceID("v1", "Namespace", "payments"), APIVersion: "v1", Kind: "Namespace", Name: "payments"},
			{ID: controller.ObservedResourceID("networking.k8s.io/v1", "NetworkPolicy", "default-deny"), APIVersion: "networking.k8s.io/v1", Kind: "NetworkPolicy", Name: "default-deny", Namespace: "payments"},
			{ID: controller.ObservedResourceID("autoscaling/v1", "HorizontalPodAutoscaler", "api"), APIVersion: "autoscaling/v1", Kind: "HorizontalPodAutoscaler", Name: "api", Namespace: "payments"},
		}))
	})

	It("reports readiness from the observed resources", func() {
		env := makeEnvironment("observe-ready", "payments")
		b := create(observeBinding("observe-ready", env.Name,
			openchoreov1alpha1.ObservedResourceClaim{APIVersion: "v1", Kind: "ResourceQuota", Name: "payments"},
		))
		reconcileBinding(b)

		setObserved := func(namespaceHealth openchoreov1alpha1.HealthStatus) {
			rr := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, releaseKey(b), rr)).To(Succeed())
			rr.Status.ObservedResources = []openchoreov1alpha1.RenderedManifestStatus{
				{ID: rr.Spec.ObservedResources[0].ID, Version: "v1", Kind: "Namespace", Name: "payments", HealthStatus: namespaceHealth},
				{ID: rr.Spec.ObservedResources[1].ID, Version: "v1", Kind: "ResourceQuota", Name: "payments", Namespace: "payments", HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
			}
			meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
				Type:               renderedrelease.ConditionResourcesApplied,
				Status:             metav1.ConditionFalse,
				Reason:             renderedrelease.ReasonObserveMode,
				Message:            "Observe mode",
				ObservedGeneration: rr.Generation,
			})
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())
		}

		setObserved(openchoreov1alpha1.HealthStatusHealthy)
		updated := reconcileBinding(b)
		Expect(meta.IsStatusConditionTrue(updated.Status.Conditions, string(ConditionNamespaceReady))).To(BeTrue())
		Expect(meta.IsStatusConditionTrue(updated.Status.Conditions, string(ConditionResourcesReady))).To(BeTrue())
		Expect(meta.IsStatusConditionTrue(updated.Status.Conditions, string(ConditionReady))).To(BeTrue())

		setObserved(openchoreov1alpha1.HealthStatusUnknown)
		updated = reconcileBinding(b)
		Expect(condition(updated, ConditionNamespaceReady).Reason).To(Equal(string(ReasonNamespaceNotFound)))
		ready := condition(updated, ConditionReady)
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(string(ReasonNamespaceNotFound)))
	})

	It("does not observe a namespace the environment does not allow", func() {
		env := makeEnvironment("observe-denied", "orders")
		b := create(observeBinding("observe-denied", env.Name))

		updated := reconcileBinding(b)

		Expect(updated.Status.Namespace).To(BeEmpty())
		Expect(condition(updated, ConditionSynced).Reason).To(Equal(string(ReasonNamespaceNotAllowed)))
		Expect(condition(updated, ConditionReady).Reason).To(Equal(string(ReasonNamespaceNotAllowed)))
		err := k8sClient.Get(ctx, releaseKey(b), &openchoreov1alpha1.RenderedRelease{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("stops observing when the Environment is gone", func() {
		env := makeEnvironment("observe-envgone", "payments")
		b := create(observeBinding("observe-envgone", env.Name))
		reconcileBinding(b)
		Expect(k8sClient.Get(ctx, releaseKey(b), &openchoreov1alpha1.RenderedRelease{})).To(Succeed())

		Expect(k8sClient.Delete(ctx, env)).To(Succeed())
		updated := reconcileBinding(b)

		Expect(updated.Status.Namespace).To(BeEmpty())
		Expect(condition(updated, ConditionSynced).Reason).To(Equal(string(ReasonEnvironmentNotFound)))
		err := k8sClient.Get(ctx, releaseKey(b), &openchoreov1alpha1.RenderedRelease{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("stops observing when the environment no longer allows the namespace", func() {
		env := makeEnvironment("observe-revoked", "payments")
		b := create(observeBinding("observe-revoked", env.Name))
		reconcileBinding(b)
		Expect(k8sClient.Get(ctx, releaseKey(b), &openchoreov1alpha1.RenderedRelease{})).To(Succeed())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(env), env)).To(Succeed())
		env.Spec.Observe.AllowedNamespaces = nil
		Expect(k8sClient.Update(ctx, env)).To(Succeed())

		updated := reconcileBinding(b)

		Expect(updated.Status.Namespace).To(BeEmpty())
		Expect(condition(updated, ConditionSynced).Reason).To(Equal(string(ReasonNamespaceNotAllowed)))
		err := k8sClient.Get(ctx, releaseKey(b), &openchoreov1alpha1.RenderedRelease{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
})
