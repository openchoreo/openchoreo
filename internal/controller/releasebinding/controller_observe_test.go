// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/releaseline"
)

var _ = Describe("ReleaseBinding controller — Observe mode", func() {
	const (
		project   = "obs-project"
		env       = "obs-env"
		namespace = "payments"
	)

	deployment := openchoreov1alpha1.ObservedResourceClaim{APIVersion: "apps/v1", Kind: "Deployment", Name: "api"}
	service := openchoreov1alpha1.ObservedResourceClaim{APIVersion: "v1", Kind: "Service", Name: "api"}

	makePRB := func(mode openchoreov1alpha1.BindingMode, reported string) *openchoreov1alpha1.ProjectReleaseBinding {
		prb := &openchoreov1alpha1.ProjectReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: project + "-" + env, Namespace: ns},
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

	makeComponent := func(name string) {
		comp := componentFixture(name, project)
		Expect(k8sClient.Create(ctx, comp)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, comp) })
	}

	makeBinding := func(name string, claims ...openchoreov1alpha1.ObservedResourceClaim) *openchoreov1alpha1.ReleaseBinding {
		rb := rbFixture(name, project, name, env, "", true)
		rb.Spec.Mode = openchoreov1alpha1.BindingModeObserve
		rb.Spec.Observe = &openchoreov1alpha1.ReleaseBindingObserve{Resources: claims}
		Expect(k8sClient.Create(ctx, rb)).To(Succeed())
		DeferCleanup(func() {
			forceDelete(name)
			forceDeleteRelease(releaseline.RenderedReleaseName(name, "", env))
		})
		return rb
	}

	releaseOf := func(rb *openchoreov1alpha1.ReleaseBinding) (*openchoreov1alpha1.RenderedRelease, error) {
		rr := &openchoreov1alpha1.RenderedRelease{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      releaseline.RenderedReleaseName(rb.Spec.Owner.ComponentName, "", env),
		}, rr)
		return rr, err
	}

	It("observes the claimed objects in the ProjectReleaseBinding's namespace", func() {
		makePRB(openchoreov1alpha1.BindingModeObserve, namespace)
		makeComponent("obs-api")
		rb := makeBinding("obs-api", deployment, service,
			openchoreov1alpha1.ObservedResourceClaim{APIVersion: "autoscaling/v1", Kind: "HorizontalPodAutoscaler", Name: "api"},
			openchoreov1alpha1.ObservedResourceClaim{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler", Name: "api"},
		)
		rb.Spec.ReleaseName = "not-a-release"
		rb.Spec.State = openchoreov1alpha1.ReleaseStateUndeploy
		Expect(k8sClient.Update(ctx, rb)).To(Succeed())

		mustReconcile(testReconciler(), reconcileRequest(rb.Name))

		updated := fetchRB(rb.Name)
		synced := conditionFor(updated, string(ConditionReleaseSynced))
		Expect(synced.Status).To(Equal(metav1.ConditionTrue))
		Expect(synced.Message).To(ContainSubstring("autoscaling/v2 HorizontalPodAutoscaler api"))
		Expect(synced.Message).To(ContainSubstring("Undeploy has no effect"))

		rr, err := releaseOf(rb)
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.IsControlledBy(rr, updated)).To(BeTrue())
		Expect(rr.Spec.Mode).To(Equal(openchoreov1alpha1.RenderedReleaseModeObserve))
		Expect(rr.Spec.Resources).To(BeEmpty())
		Expect(rr.Labels).To(HaveKeyWithValue(labels.LabelKeyComponentName, "obs-api"))
		Expect(rr.Labels).NotTo(HaveKey(labels.LabelKeyComponentReleaseName))
		Expect(rr.Spec.ObservedResources).To(Equal([]openchoreov1alpha1.ObservedResource{
			{ID: controller.ObservedResourceID("apps/v1", "Deployment", "api"), APIVersion: "apps/v1", Kind: "Deployment", Name: "api", Namespace: namespace},
			{ID: controller.ObservedResourceID("v1", "Service", "api"), APIVersion: "v1", Kind: "Service", Name: "api", Namespace: namespace},
			{ID: controller.ObservedResourceID("autoscaling/v1", "HorizontalPodAutoscaler", "api"), APIVersion: "autoscaling/v1", Kind: "HorizontalPodAutoscaler", Name: "api", Namespace: namespace},
		}))
	})

	It("reports health from the observed resources", func() {
		makePRB(openchoreov1alpha1.BindingModeObserve, namespace)
		makeComponent("obs-health")
		rb := makeBinding("obs-health", deployment, service)
		mustReconcile(testReconciler(), reconcileRequest(rb.Name))

		rr, err := releaseOf(rb)
		Expect(err).NotTo(HaveOccurred())
		rr.Status.ObservedResources = []openchoreov1alpha1.RenderedManifestStatus{
			{ID: rr.Spec.ObservedResources[0].ID, Group: "apps", Version: "v1", Kind: "Deployment", Name: "api", Namespace: namespace, HealthStatus: openchoreov1alpha1.HealthStatusSuspended},
			{ID: rr.Spec.ObservedResources[1].ID, Version: "v1", Kind: "Service", Name: "api", Namespace: namespace, HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
		}
		apimeta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
			Type:               renderedrelease.ConditionResourcesApplied,
			Status:             metav1.ConditionFalse,
			Reason:             renderedrelease.ReasonObserveMode,
			Message:            "Observe mode",
			ObservedGeneration: rr.Generation,
		})
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		mustReconcile(testReconciler(), reconcileRequest(rb.Name))

		updated := fetchRB(rb.Name)
		resources := conditionFor(updated, string(ConditionResourcesReady))
		Expect(resources.Status).To(Equal(metav1.ConditionTrue))
		Expect(resources.Reason).To(Equal(string(ReasonReadyWithSuspendedResources)))
		Expect(conditionFor(updated, string(ConditionReady)).Status).To(Equal(metav1.ConditionTrue))
	})

	It("needs an observed ProjectReleaseBinding", func() {
		makeComponent("obs-noprb")
		rb := makeBinding("obs-noprb", deployment)

		mustReconcile(testReconciler(), reconcileRequest(rb.Name))
		Expect(conditionFor(fetchRB(rb.Name), string(ConditionReady)).Reason).
			To(Equal(string(ReasonProjectReleaseBindingNotFound)))

		makePRB(openchoreov1alpha1.BindingModeManage, "")
		mustReconcile(testReconciler(), reconcileRequest(rb.Name))
		Expect(conditionFor(fetchRB(rb.Name), string(ConditionReady)).Reason).
			To(Equal(string(ReasonProjectNotObserved)))

		_, err := releaseOf(rb)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("stops observing when the ProjectReleaseBinding stops observing the namespace", func() {
		prb := makePRB(openchoreov1alpha1.BindingModeObserve, namespace)
		makeComponent("obs-revoked")
		rb := makeBinding("obs-revoked", deployment)
		mustReconcile(testReconciler(), reconcileRequest(rb.Name))
		_, err := releaseOf(rb)
		Expect(err).NotTo(HaveOccurred())

		prb.Status.Namespace = ""
		apimeta.SetStatusCondition(&prb.Status.Conditions, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "NamespaceNotAllowed",
			Message: `Namespace "payments" is not in Environment "obs-env" spec.observe.allowedNamespaces`,
		})
		Expect(k8sClient.Status().Update(ctx, prb)).To(Succeed())

		mustReconcile(testReconciler(), reconcileRequest(rb.Name))

		ready := conditionFor(fetchRB(rb.Name), string(ConditionReady))
		Expect(ready.Reason).To(Equal(string(ReasonNamespaceNotObserved)))
		Expect(ready.Message).To(ContainSubstring("is not in Environment"))
		_, err = releaseOf(rb)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("reports nothing observed without claims", func() {
		makePRB(openchoreov1alpha1.BindingModeObserve, namespace)
		makeComponent("obs-empty")
		rb := makeBinding("obs-empty")

		mustReconcile(testReconciler(), reconcileRequest(rb.Name))

		Expect(conditionFor(fetchRB(rb.Name), string(ConditionReady)).Reason).
			To(Equal(string(ReasonNothingObserved)))
		_, err := releaseOf(rb)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("leaves a release in Manage mode alone when it cannot observe", func() {
		makeComponent("obs-managed")
		rb := makeBinding("obs-managed", deployment)
		rr := releaseFixture(releaseline.RenderedReleaseName("obs-managed", "", env), project, "obs-managed", env)
		Expect(controllerutil.SetControllerReference(rb, rr, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		mustReconcile(testReconciler(), reconcileRequest(rb.Name))

		Expect(conditionFor(fetchRB(rb.Name), string(ConditionReady)).Reason).
			To(Equal(string(ReasonProjectReleaseBindingNotFound)))
		_, err := releaseOf(rb)
		Expect(err).NotTo(HaveOccurred())
	})
})
