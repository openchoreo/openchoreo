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
	"github.com/openchoreo/openchoreo/internal/controller/projectreleasebinding"
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

	setManagedStatus := func(rb *openchoreov1alpha1.ReleaseBinding) {
		rb.Status.Endpoints = []openchoreov1alpha1.EndpointURLStatus{{
			Name:       "http",
			ServiceURL: &openchoreov1alpha1.EndpointURL{Scheme: "http", Host: "api.payments.svc", Port: 8080},
		}}
		rb.Status.Delivery = &openchoreov1alpha1.DeliveryStatus{RolloutID: "rollout-1"}
		apimeta.SetStatusCondition(&rb.Status.Conditions, metav1.Condition{
			Type:   string(ConditionConnectionsResolved),
			Status: metav1.ConditionTrue,
			Reason: string(ReasonNoConnections),
		})
		Expect(k8sClient.Status().Update(ctx, rb)).To(Succeed())
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
		Expect(synced.Message).To(ContainSubstring("Undeploy has no effect"))

		rr, err := releaseOf(rb)
		Expect(err).NotTo(HaveOccurred())
		Expect(metav1.IsControlledBy(rr, updated)).To(BeTrue())
		Expect(rr.Spec.Mode).To(Equal(openchoreov1alpha1.RenderedReleaseModeObserve))
		Expect(rr.Spec.Resources).To(BeEmpty())
		Expect(rr.Labels).To(HaveKeyWithValue(labels.LabelKeyComponentName, "obs-api"))
		Expect(rr.Labels).NotTo(HaveKey(labels.LabelKeyComponentReleaseName))
		Expect(rr.Spec.ObservedResources).To(Equal([]openchoreov1alpha1.ObservedResource{
			{ID: controller.ObservedResourceID("apps/v1", "Deployment", namespace, "api"), APIVersion: "apps/v1", Kind: "Deployment", Name: "api", Namespace: namespace},
			{ID: controller.ObservedResourceID("v1", "Service", namespace, "api"), APIVersion: "v1", Kind: "Service", Name: "api", Namespace: namespace},
			{ID: controller.ObservedResourceID("autoscaling/v1", "HorizontalPodAutoscaler", namespace, "api"), APIVersion: "autoscaling/v1", Kind: "HorizontalPodAutoscaler", Name: "api", Namespace: namespace},
			{ID: controller.ObservedResourceID("autoscaling/v2", "HorizontalPodAutoscaler", namespace, "api"), APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler", Name: "api", Namespace: namespace},
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
		apimeta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
			Type:               string(renderedrelease.ConditionObservedResourcesRead),
			Status:             metav1.ConditionTrue,
			Reason:             string(renderedrelease.ReasonObservedReadSucceeded),
			Message:            "Read",
			ObservedGeneration: rr.Generation,
		})
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		mustReconcile(testReconciler(), reconcileRequest(rb.Name))

		updated := fetchRB(rb.Name)
		resources := conditionFor(updated, string(ConditionResourcesReady))
		Expect(resources.Status).To(Equal(metav1.ConditionTrue))
		Expect(resources.Reason).To(Equal(string(ReasonReadyWithSuspendedResources)))
		Expect(conditionFor(updated, string(ConditionReady)).Status).To(Equal(metav1.ConditionTrue))

		By("A new claim waits for the release to read it")
		updated.Spec.Observe.Resources = append(updated.Spec.Observe.Resources,
			openchoreov1alpha1.ObservedResourceClaim{APIVersion: "v1", Kind: "ConfigMap", Name: "missing"})
		Expect(k8sClient.Update(ctx, updated)).To(Succeed())
		mustReconcile(testReconciler(), reconcileRequest(rb.Name))
		resources = conditionFor(fetchRB(rb.Name), string(ConditionResourcesReady))
		Expect(resources.Status).To(Equal(metav1.ConditionFalse))
		Expect(resources.Reason).To(Equal(string(ReasonResourcesProgressing)))
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

	It("observes on a release line", func() {
		prb := makePRB(openchoreov1alpha1.BindingModeObserve, namespace)
		makeComponent("obs-line")
		rb := rbFixture("obs-line-canary", project, "obs-line", env, "", true)
		rb.Spec.Owner.ReleaseLine = "canary"
		rb.Spec.Mode = openchoreov1alpha1.BindingModeObserve
		rb.Spec.Observe = &openchoreov1alpha1.ReleaseBindingObserve{
			Resources: []openchoreov1alpha1.ObservedResourceClaim{deployment},
		}
		Expect(k8sClient.Create(ctx, rb)).To(Succeed())
		rrKey := types.NamespacedName{Namespace: ns, Name: releaseline.RenderedReleaseName("obs-line", "canary", env)}
		DeferCleanup(func() {
			forceDelete(rb.Name)
			forceDeleteRelease(rrKey.Name)
		})

		By("A missing ReleaseLine stops it")
		mustReconcile(testReconciler(), reconcileRequest(rb.Name))
		Expect(conditionFor(fetchRB(rb.Name), string(ConditionReleaseSynced)).Reason).
			To(Equal(string(ReasonReleaseLineNotFound)))

		By("With the ReleaseLine, it observes under the line's release name")
		line := &openchoreov1alpha1.ReleaseLine{
			ObjectMeta: metav1.ObjectMeta{Name: "canary", Namespace: ns},
			Spec: openchoreov1alpha1.ReleaseLineSpec{
				Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: project, ComponentName: "obs-line"},
			},
		}
		Expect(k8sClient.Create(ctx, line)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, line) })
		mustReconcile(testReconciler(), reconcileRequest(rb.Name))

		rr := &openchoreov1alpha1.RenderedRelease{}
		Expect(k8sClient.Get(ctx, rrKey, rr)).To(Succeed())
		Expect(rr.Spec.Mode).To(Equal(openchoreov1alpha1.RenderedReleaseModeObserve))
		Expect(rr.Labels).To(HaveKeyWithValue(labels.LabelKeyReleaseLine, "canary"))
		Expect(rr.Spec.ObservedResources).To(HaveLen(1))
		Expect(rr.Spec.ObservedResources[0].Namespace).To(Equal(namespace))

		By("Revoking the namespace stops it, even once the ReleaseLine is gone")
		Expect(k8sClient.Delete(ctx, line)).To(Succeed())
		prb.Status.Namespace = ""
		Expect(k8sClient.Status().Update(ctx, prb)).To(Succeed())
		mustReconcile(testReconciler(), reconcileRequest(rb.Name))
		err := k8sClient.Get(ctx, rrKey, &openchoreov1alpha1.RenderedRelease{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("stops observing when its claims are emptied, even once the ReleaseLine is gone", func() {
		makePRB(openchoreov1alpha1.BindingModeObserve, namespace)
		makeComponent("obs-line-empty")
		line := &openchoreov1alpha1.ReleaseLine{
			ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: ns},
			Spec: openchoreov1alpha1.ReleaseLineSpec{
				Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: project, ComponentName: "obs-line-empty"},
			},
		}
		Expect(k8sClient.Create(ctx, line)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, line) })
		rb := rbFixture("obs-line-empty-edge", project, "obs-line-empty", env, "", true)
		rb.Spec.Owner.ReleaseLine = "edge"
		rb.Spec.Mode = openchoreov1alpha1.BindingModeObserve
		rb.Spec.Observe = &openchoreov1alpha1.ReleaseBindingObserve{
			Resources: []openchoreov1alpha1.ObservedResourceClaim{deployment},
		}
		Expect(k8sClient.Create(ctx, rb)).To(Succeed())
		rrKey := types.NamespacedName{Namespace: ns, Name: releaseline.RenderedReleaseName("obs-line-empty", "edge", env)}
		DeferCleanup(func() {
			forceDelete(rb.Name)
			forceDeleteRelease(rrKey.Name)
		})
		mustReconcile(testReconciler(), reconcileRequest(rb.Name))
		Expect(k8sClient.Get(ctx, rrKey, &openchoreov1alpha1.RenderedRelease{})).To(Succeed())

		Expect(k8sClient.Delete(ctx, line)).To(Succeed())
		current := fetchRB(rb.Name)
		current.Spec.Observe.Resources = nil
		Expect(k8sClient.Update(ctx, current)).To(Succeed())
		mustReconcile(testReconciler(), reconcileRequest(rb.Name))

		Expect(conditionFor(fetchRB(rb.Name), string(ConditionReady)).Reason).
			To(Equal(string(ReasonNothingObserved)))
		err := k8sClient.Get(ctx, rrKey, &openchoreov1alpha1.RenderedRelease{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("stops an Observe-mode release left by a switch to Manage once the namespace is revoked", func() {
		prb := makePRB(openchoreov1alpha1.BindingModeObserve, namespace)
		makeComponent("obs-flip")
		rb := makeBinding("obs-flip", deployment)
		mustReconcile(testReconciler(), reconcileRequest(rb.Name))
		_, err := releaseOf(rb)
		Expect(err).NotTo(HaveOccurred())

		By("Switched to Manage under the observed ProjectReleaseBinding, it keeps observing and renders nothing")
		updated := fetchRB(rb.Name)
		updated.Spec.Mode = openchoreov1alpha1.BindingModeManage
		updated.Spec.ReleaseName = "not-a-release"
		Expect(k8sClient.Update(ctx, updated)).To(Succeed())
		mustReconcile(testReconciler(), reconcileRequest(rb.Name))
		rr, err := releaseOf(rb)
		Expect(err).NotTo(HaveOccurred())
		Expect(rr.Spec.Mode).To(Equal(openchoreov1alpha1.RenderedReleaseModeObserve))
		Expect(conditionFor(fetchRB(rb.Name), string(ConditionReleaseSynced)).Reason).
			To(Equal(string(projectreleasebinding.ReasonProjectObserved)))

		By("Revoking the namespace removes it")
		prb.Status.Namespace = ""
		Expect(k8sClient.Status().Update(ctx, prb)).To(Succeed())
		mustReconcile(testReconciler(), reconcileRequest(rb.Name))
		_, err = releaseOf(rb)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("does not delete a release from a stale read", func() {
		makePRB(openchoreov1alpha1.BindingModeObserve, namespace)
		makeComponent("obs-stale")
		rb := makeBinding("obs-stale", deployment)
		mustReconcile(testReconciler(), reconcileRequest(rb.Name))
		stale, err := releaseOf(rb)
		Expect(err).NotTo(HaveOccurred())

		current := stale.DeepCopy()
		current.Spec.Mode = openchoreov1alpha1.RenderedReleaseModeManage
		Expect(k8sClient.Update(ctx, current)).To(Succeed())

		Expect(renderedrelease.DeleteObserving(ctx, k8sClient, stale)).NotTo(Succeed())
		_, err = releaseOf(rb)
		Expect(err).NotTo(HaveOccurred())
	})

	It("leaves a release in Manage mode and its endpoints alone when it cannot observe", func() {
		makeComponent("obs-managed")
		rb := makeBinding("obs-managed", deployment)
		setManagedStatus(rb)
		rr := releaseFixture(releaseline.RenderedReleaseName("obs-managed", "", env), project, "obs-managed", env)
		Expect(controllerutil.SetControllerReference(rb, rr, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		mustReconcile(testReconciler(), reconcileRequest(rb.Name))

		updated := fetchRB(rb.Name)
		Expect(conditionFor(updated, string(ConditionReady)).Reason).
			To(Equal(string(ReasonProjectReleaseBindingNotFound)))
		Expect(updated.Status.Endpoints).To(HaveLen(1), "consumers keep resolving the running release's endpoints")
		Expect(conditionFor(updated, string(ConditionConnectionsResolved))).NotTo(BeNil())
		_, err := releaseOf(rb)
		Expect(err).NotTo(HaveOccurred())
	})

	It("clears what Manage mode reported once it observes", func() {
		makePRB(openchoreov1alpha1.BindingModeObserve, namespace)
		makeComponent("obs-cleared")
		rb := makeBinding("obs-cleared", deployment)
		setManagedStatus(rb)

		mustReconcile(testReconciler(), reconcileRequest(rb.Name))

		updated := fetchRB(rb.Name)
		Expect(updated.Status.Endpoints).To(BeEmpty())
		Expect(updated.Status.Delivery).To(BeNil())
		Expect(conditionFor(updated, string(ConditionConnectionsResolved))).To(BeNil())
		rr, err := releaseOf(rb)
		Expect(err).NotTo(HaveOccurred())
		Expect(rr.Spec.Mode).To(Equal(openchoreov1alpha1.RenderedReleaseModeObserve))
	})
})
