// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// These specs run the reconciler against envtest with the envtest client as the
// plane client, so reads, server-side applies and deletes go to a real API server.

// observeTestPlane creates a ClusterDataPlane and an Environment for one spec and
// returns the environment name and a reconciler whose plane client is envtest.
func observeTestPlane(suffix string) (string, *Reconciler) {
	GinkgoHelper()
	envName, cdpName, planeID := "env-"+suffix, "cdp-"+suffix, "plane-"+suffix
	Expect(k8sClient.Create(ctx, makeClusterDataPlane(cdpName, planeID))).To(Succeed())
	Expect(k8sClient.Create(ctx, makeEnvironment(envName, cdpName))).To(Succeed())
	DeferCleanup(func() {
		forceDeleteEnvironment(ctx, types.NamespacedName{Name: envName, Namespace: testNs})
		forceDeleteClusterDataPlane(ctx, cdpName)
	})
	return envName, testReconcilerWithDPClient(k8sClient, planeID, cdpName)
}

// teamConfigMap creates a ConfigMap the way a team's own tooling would, outside
// this controller, and removes it after the spec.
func teamConfigMap(name string) *corev1.ConfigMap {
	GinkgoHelper()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNs, Labels: map[string]string{"team": "payments"}},
		Data:       map[string]string{"owner": "team"},
	}
	Expect(k8sClient.Create(ctx, cm)).To(Succeed())
	DeferCleanup(func() { _ = k8sClient.Delete(ctx, cm) })
	return cm
}

// teamDeployment creates a Deployment outside this controller and gives it the
// status a finished rollout would have.
func teamDeployment(name string) *appsv1.Deployment {
	GinkgoHelper()
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNs},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx:latest"}}},
			},
		},
	}
	Expect(k8sClient.Create(ctx, dep)).To(Succeed())
	DeferCleanup(func() { _ = k8sClient.Delete(ctx, dep) })
	dep.Status = appsv1.DeploymentStatus{
		ObservedGeneration: dep.Generation,
		Replicas:           1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1,
		Conditions: []appsv1.DeploymentCondition{
			{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue, Reason: "MinimumReplicasAvailable"},
			{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable"},
		},
	}
	Expect(k8sClient.Status().Update(ctx, dep)).To(Succeed())
	return dep
}

// expectUntouched asserts that obj is exactly as the team left it: same
// resourceVersion, none of this controller's labels, and no field manager entry
// from this controller.
func expectUntouched(obj client.Object) {
	GinkgoHelper()
	live := obj.DeepCopyObject().(client.Object)
	Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), live)).To(Succeed(), "observed object must still exist")
	Expect(live.GetResourceVersion()).To(Equal(obj.GetResourceVersion()), "observed object must not be written")
	Expect(live.GetLabels()).NotTo(HaveKey(labels.LabelKeyManagedBy))
	Expect(live.GetLabels()).NotTo(HaveKey(labels.LabelKeyRenderedReleaseUID))
	for _, entry := range live.GetManagedFields() {
		Expect(entry.Manager).NotTo(Equal(ControllerName))
	}
}

func configMapManifest(name string) *runtime.RawExtension {
	return &runtime.RawExtension{Raw: fmt.Appendf(nil,
		`{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": %q, "namespace": %q}, "data": {"key": "value"}}`,
		name, testNs)}
}

func configMapExists(name string) bool {
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNs}, &corev1.ConfigMap{})
	if apierrors.IsNotFound(err) {
		return false
	}
	Expect(err).NotTo(HaveOccurred())
	return true
}

func findCondition(release *openchoreov1alpha1.RenderedRelease, conditionType string) *metav1.Condition {
	return apimeta.FindStatusCondition(release.Status.Conditions, conditionType)
}

func setMode(name string, mode openchoreov1alpha1.RenderedReleaseMode) {
	GinkgoHelper()
	release := fetchRelease(name)
	release.Spec.Mode = mode
	Expect(k8sClient.Update(ctx, release)).To(Succeed())
}

var _ = Describe("RenderedRelease observe mode", func() {

	Context("in Observe mode with objects the team runs", func() {
		const releaseName = "release-observe-read"

		AfterEach(func() {
			forceDelete(ctx, types.NamespacedName{Name: releaseName, Namespace: testNs})
		})

		It("reads them, reports their health, and never writes them", func() {
			envName, r := observeTestPlane("observe-read")
			dep := teamDeployment("observe-read-api")
			cm := teamConfigMap("observe-read-settings")

			By("Creating an Observe release listing the team's objects, a missing one and an unknown kind")
			release := makeMinimalRelease(releaseName, envName)
			release.Spec.Mode = openchoreov1alpha1.RenderedReleaseModeObserve
			release.Spec.ObservedResources = []openchoreov1alpha1.ObservedResource{
				{ID: "api", APIVersion: "apps/v1", Kind: "Deployment", Namespace: testNs, Name: dep.Name},
				{ID: "settings", APIVersion: "v1", Kind: "ConfigMap", Namespace: testNs, Name: cm.Name},
				{ID: "missing", APIVersion: "v1", Kind: "Service", Namespace: testNs, Name: "observe-read-missing"},
				{ID: "widget", APIVersion: "example.com/v1", Kind: "Widget", Namespace: testNs, Name: "w"},
			}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			mustReconcile(r, reconcileRequest(releaseName)) // finalizer
			result := mustReconcile(r, reconcileRequest(releaseName))

			updated := fetchRelease(releaseName)
			Expect(updated.Status.Resources).To(BeEmpty())
			observed := updated.Status.ObservedResources
			Expect(observed).To(HaveLen(4))

			By("The Deployment is judged by its own check, the ConfigMap by existing")
			Expect(observed[0].ID).To(Equal("api"))
			Expect(observed[0].Group).To(Equal("apps"))
			Expect(observed[0].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusHealthy))
			Expect(string(observed[0].Status.Raw)).To(ContainSubstring(`"availableReplicas":1`))
			Expect(observed[0].LastObservedTime).NotTo(BeNil())
			Expect(observed[1].ID).To(Equal("settings"))
			Expect(observed[1].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusHealthy))

			By("A missing object and an unknown kind are Unknown, with the reasons in the condition")
			Expect(observed[2].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusUnknown))
			Expect(observed[2].LastObservedTime).To(BeNil())
			Expect(observed[3].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusUnknown))
			read := findCondition(updated, string(ConditionObservedResourcesRead))
			Expect(read).NotTo(BeNil())
			Expect(read.Status).To(Equal(metav1.ConditionFalse))
			Expect(read.Reason).To(Equal(string(ReasonObservedReadFailed)), "two different causes")
			Expect(read.Message).To(ContainSubstring("2 of 4"))
			Expect(read.Message).To(ContainSubstring("missing (v1 Service default/observe-read-missing): not found"))
			Expect(read.Message).To(ContainSubstring("widget (example.com/v1 Widget default/w): kind is not served"))

			By("Nothing was applied, and the apply condition says why")
			applied := findCondition(updated, ConditionResourcesApplied)
			Expect(applied).NotTo(BeNil())
			Expect(applied.Status).To(Equal(metav1.ConditionFalse), "nothing in spec.resources is applied")
			Expect(applied.Reason).To(Equal(ReasonObserveMode), "the reason tells it from an apply failure")
			Expect(applied.ObservedGeneration).To(Equal(updated.Generation))

			By("Unknown observed objects are re-read on the progressing interval, at poll priority")
			Expect(result.RequeueAfter).To(BeNumerically(">=", 10*time.Second))
			Expect(result.RequeueAfter).To(BeNumerically("<", 12*time.Second))
			Expect(result.Priority).To(HaveValue(Equal(requeuePriorityPoll)))

			By("The team's objects were not written")
			expectUntouched(dep)
			expectUntouched(cm)

			By("A reconcile with nothing changed does not rewrite the status")
			before := fetchRelease(releaseName)
			mustReconcile(r, reconcileRequest(releaseName))
			Expect(fetchRelease(releaseName).ResourceVersion).To(Equal(before.ResourceVersion))

			By("The Deployment's rollout stalls: it reads as Degraded")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(dep), dep)).To(Succeed())
			dep.Status.Conditions[1] = appsv1.DeploymentCondition{
				Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: reasonProgressDeadlineExceeded,
			}
			Expect(k8sClient.Status().Update(ctx, dep)).To(Succeed())
			mustReconcile(r, reconcileRequest(releaseName))
			updated = fetchRelease(releaseName)
			// LastObservedTime is stored to the second, so whether it moved is checked
			// in TestReadObservedResourcesKeepsLastObservedTime, not here.
			Expect(updated.Status.ObservedResources[0].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusDegraded))

			By("The missing Service appears: it reads as Healthy, and only the unknown kind is left")
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "observe-read-missing", Namespace: testNs},
				Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 80}}},
			}
			Expect(k8sClient.Create(ctx, svc)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, svc) })
			mustReconcile(r, reconcileRequest(releaseName))
			updated = fetchRelease(releaseName)
			Expect(updated.Status.ObservedResources[2].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusHealthy))
			read = findCondition(updated, string(ConditionObservedResourcesRead))
			Expect(read.Reason).To(Equal(string(ReasonObservedKindNotFound)))
			Expect(read.Message).To(ContainSubstring("1 of 4"))
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc)).To(Succeed())
			expectUntouched(svc)
		})
	})

	Context("when a Manage release moves to Observe", func() {
		const releaseName = "release-manage-to-observe"

		AfterEach(func() {
			forceDelete(ctx, types.NamespacedName{Name: releaseName, Namespace: testNs})
		})

		It("removes what it applied and leaves observed objects alone", func() {
			envName, r := observeTestPlane("manage-to-observe")
			team := teamConfigMap("m2o-team")

			release := makeMinimalRelease(releaseName, envName)
			release.Spec.Resources = []openchoreov1alpha1.RenderedManifest{
				{ID: "applied", Object: configMapManifest("m2o-applied")},
			}
			release.Spec.ObservedResources = []openchoreov1alpha1.ObservedResource{
				{ID: "team", APIVersion: "v1", Kind: "ConfigMap", Namespace: testNs, Name: team.Name},
			}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "m2o-applied", Namespace: testNs}})
			})

			By("Manage applies spec.resources and still reads the observed object")
			mustReconcile(r, reconcileRequest(releaseName))
			mustReconcile(r, reconcileRequest(releaseName))
			Expect(configMapExists("m2o-applied")).To(BeTrue())
			updated := fetchRelease(releaseName)
			Expect(updated.Spec.Mode).To(BeEmpty(), "unset mode is not defaulted by the API server, and means Manage")
			Expect(updated.Status.Resources).To(HaveLen(1))
			Expect(updated.Status.ObservedResources).To(HaveLen(1))
			Expect(updated.Status.ObservedResources[0].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusHealthy))
			Expect(findCondition(updated, string(ConditionObservedResourcesRead)).Status).To(Equal(metav1.ConditionTrue))
			expectUntouched(team)

			By("Observe removes only the object this release applied")
			setMode(releaseName, openchoreov1alpha1.RenderedReleaseModeObserve)
			mustReconcile(r, reconcileRequest(releaseName))
			Expect(configMapExists("m2o-applied")).To(BeFalse())
			expectUntouched(team)

			updated = fetchRelease(releaseName)
			Expect(updated.Status.Resources).To(BeEmpty())
			Expect(updated.Status.ObservedResources).To(HaveLen(1))
			Expect(updated.Status.ObservedResources[0].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusHealthy))
			applied := findCondition(updated, ConditionResourcesApplied)
			Expect(applied.Status).To(Equal(metav1.ConditionFalse))
			Expect(applied.Reason).To(Equal(ReasonObserveMode))
			Expect(applied.ObservedGeneration).To(Equal(updated.Generation))
		})
	})

	Context("when an Observe release moves to Manage", func() {
		const releaseName = "release-observe-to-manage"

		AfterEach(func() {
			forceDelete(ctx, types.NamespacedName{Name: releaseName, Namespace: testNs})
		})

		It("applies spec.resources only once it manages them", func() {
			envName, r := observeTestPlane("observe-to-manage")

			release := makeMinimalRelease(releaseName, envName)
			release.Spec.Mode = openchoreov1alpha1.RenderedReleaseModeObserve
			release.Spec.Resources = []openchoreov1alpha1.RenderedManifest{
				{ID: "applied", Object: configMapManifest("o2m-applied")},
			}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "o2m-applied", Namespace: testNs}})
			})

			By("Observe does not apply")
			mustReconcile(r, reconcileRequest(releaseName))
			mustReconcile(r, reconcileRequest(releaseName))
			Expect(configMapExists("o2m-applied")).To(BeFalse())
			updated := fetchRelease(releaseName)
			Expect(updated.Status.Resources).To(BeEmpty())
			Expect(updated.Status.ObservedResources).To(BeEmpty())
			Expect(findCondition(updated, string(ConditionObservedResourcesRead))).To(BeNil(),
				"no observed resources, no observed conditions")

			By("Manage applies")
			setMode(releaseName, openchoreov1alpha1.RenderedReleaseModeManage)
			mustReconcile(r, reconcileRequest(releaseName))
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "o2m-applied", Namespace: testNs}, cm)).To(Succeed())
			Expect(cm.Labels[labels.LabelKeyRenderedReleaseResourceID]).To(Equal("applied"))
			updated = fetchRelease(releaseName)
			Expect(updated.Status.Resources).To(HaveLen(1))
			applied := findCondition(updated, ConditionResourcesApplied)
			Expect(applied.Reason).To(Equal(ReasonApplySucceeded))
			Expect(applied.ObservedGeneration).To(Equal(updated.Generation))
		})
	})

	Context("when the release is deleted", func() {
		const releaseName = "release-observe-finalize"
		nn := types.NamespacedName{Name: releaseName, Namespace: testNs}

		AfterEach(func() {
			forceDelete(ctx, nn)
		})

		It("deletes what it applied and leaves every observed object, even one it applied earlier", func() {
			envName, r := observeTestPlane("observe-finalize")
			team := teamConfigMap("fin-team")

			release := makeMinimalRelease(releaseName, envName)
			release.Spec.Resources = []openchoreov1alpha1.RenderedManifest{
				{ID: "applied", Object: configMapManifest("fin-applied")},
				{ID: "handover", Object: configMapManifest("fin-handover")},
			}
			release.Spec.ObservedResources = []openchoreov1alpha1.ObservedResource{
				{ID: "team", APIVersion: "v1", Kind: "ConfigMap", Namespace: testNs, Name: team.Name},
			}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())
			DeferCleanup(func() {
				for _, name := range []string{"fin-applied", "fin-handover"} {
					_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNs}})
				}
			})
			mustReconcile(r, reconcileRequest(releaseName))
			mustReconcile(r, reconcileRequest(releaseName))
			Expect(configMapExists("fin-applied")).To(BeTrue())
			Expect(configMapExists("fin-handover")).To(BeTrue())

			By("Moving fin-handover from spec.resources to observedResources keeps it")
			// It still carries this release's labels and its apply entry, so without
			// the observed check both the stale cleanup and the finalizer would delete it.
			updated := fetchRelease(releaseName)
			updated.Spec.Resources = updated.Spec.Resources[:1]
			updated.Spec.ObservedResources = append(updated.Spec.ObservedResources, openchoreov1alpha1.ObservedResource{
				ID: "handover", APIVersion: "v1", Kind: "ConfigMap", Namespace: testNs, Name: "fin-handover",
			})
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())
			mustReconcile(r, reconcileRequest(releaseName))
			Expect(configMapExists("fin-handover")).To(BeTrue())
			updated = fetchRelease(releaseName)
			Expect(updated.Status.Resources).To(HaveLen(1))
			Expect(updated.Status.ObservedResources).To(HaveLen(2))
			handover := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "fin-handover", Namespace: testNs}, handover)).To(Succeed())

			By("Deleting the release")
			Expect(k8sClient.Delete(ctx, updated)).To(Succeed())
			Eventually(func() bool {
				return !fetchRelease(releaseName).DeletionTimestamp.IsZero()
			}, testTimeout, testInterval).Should(BeTrue())

			By("Finalizing: Finalizing condition, then the delete, then the finalizer removal")
			for range 3 {
				if apierrors.IsNotFound(k8sClient.Get(ctx, nn, &openchoreov1alpha1.RenderedRelease{})) {
					break
				}
				mustReconcile(r, reconcileRequest(releaseName))
			}
			Eventually(func() bool {
				return apierrors.IsNotFound(k8sClient.Get(ctx, nn, &openchoreov1alpha1.RenderedRelease{}))
			}, testTimeout, testInterval).Should(BeTrue(), "the finalizer must not wait for observed objects")

			Expect(configMapExists("fin-applied")).To(BeFalse())
			expectUntouched(team)
			Expect(configMapExists("fin-handover")).To(BeTrue())
			after := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "fin-handover", Namespace: testNs}, after)).To(Succeed())
			Expect(after.ResourceVersion).To(Equal(handover.ResourceVersion), "an observed object is not written while observed")
		})
	})

	Context("when spec.resources and observedResources name the same object", func() {
		const releaseName = "release-observe-overlap"

		AfterEach(func() {
			forceDelete(ctx, types.NamespacedName{Name: releaseName, Namespace: testNs})
		})

		It("refuses to apply it in Manage mode", func() {
			envName, r := observeTestPlane("observe-overlap")
			team := teamConfigMap("overlap-team")

			release := makeMinimalRelease(releaseName, envName)
			release.Spec.Resources = []openchoreov1alpha1.RenderedManifest{
				{ID: "rendered", Object: configMapManifest(team.Name)},
			}
			release.Spec.ObservedResources = []openchoreov1alpha1.ObservedResource{
				{ID: "team", APIVersion: "v1", Kind: "ConfigMap", Namespace: testNs, Name: team.Name},
			}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			mustReconcile(r, reconcileRequest(releaseName))
			_, err := r.Reconcile(ctx, reconcileRequest(releaseName))
			Expect(err).To(MatchError(ContainSubstring("never written")))

			updated := fetchRelease(releaseName)
			applied := findCondition(updated, ConditionResourcesApplied)
			Expect(applied).NotTo(BeNil())
			Expect(applied.Status).To(Equal(metav1.ConditionFalse))
			Expect(applied.Reason).To(Equal(ReasonApplyFailed))
			Expect(applied.Message).To(ContainSubstring("observed resource team"))
			expectUntouched(team)

			By("The failed apply still publishes the observed reads")
			Expect(updated.Status.ObservedResources).To(HaveLen(1))
			Expect(updated.Status.ObservedResources[0].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusHealthy))
			Expect(findCondition(updated, string(ConditionObservedResourcesRead)).Status).To(Equal(metav1.ConditionTrue))

			By("A repeat of the same failure still writes what changed on the observed side")
			Expect(k8sClient.Delete(ctx, team)).To(Succeed())
			_, err = r.Reconcile(ctx, reconcileRequest(releaseName))
			Expect(err).To(MatchError(ContainSubstring("never written")))
			updated = fetchRelease(releaseName)
			Expect(updated.Status.ObservedResources[0].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusUnknown))
			Expect(findCondition(updated, string(ConditionObservedResourcesRead)).Reason).
				To(Equal(string(ReasonObservedResourceNotFound)))
		})
	})
})

var _ = Describe("RenderedRelease observe mode API validation", func() {
	newRelease := func(name string, observed ...openchoreov1alpha1.ObservedResource) *openchoreov1alpha1.RenderedRelease {
		release := makeMinimalRelease(name, "env-validation")
		release.Spec.ObservedResources = observed
		DeferCleanup(func() { forceDelete(ctx, types.NamespacedName{Name: name, Namespace: testNs}) })
		return release
	}
	deploy := func(id, name string) openchoreov1alpha1.ObservedResource {
		return openchoreov1alpha1.ObservedResource{ID: id, APIVersion: "apps/v1", Kind: "Deployment", Namespace: testNs, Name: name}
	}

	It("rejects an id used twice", func() {
		err := k8sClient.Create(ctx, newRelease("val-dup-ids", deploy("api", "a"), deploy("api", "b")))
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "got %v", err)
		Expect(err.Error()).To(ContainSubstring("Duplicate value"))
	})

	It("accepts distinct ids", func() {
		Expect(k8sClient.Create(ctx, newRelease("val-ids-ok", deploy("api", "a"), deploy("web", "b")))).To(Succeed())
	})

	It("requires an id on every entry", func() {
		By("an empty id")
		err := k8sClient.Create(ctx, newRelease("val-empty-id", deploy("", "a")))
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "got %v", err)

		By("no id field at all, as a hand-written manifest would leave it")
		raw := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "openchoreo.dev/v1alpha1",
			"kind":       "RenderedRelease",
			"metadata":   map[string]any{"name": "val-missing-id", "namespace": testNs},
			"spec": map[string]any{
				"owner":           map[string]any{"projectName": "p", "componentName": "c"},
				"environmentName": "env-validation",
				"observedResources": []any{map[string]any{
					"apiVersion": "apps/v1", "kind": "Deployment", "namespace": testNs, "name": "a",
				}},
			},
		}}
		DeferCleanup(func() { forceDelete(ctx, types.NamespacedName{Name: "val-missing-id", Namespace: testNs}) })
		err = k8sClient.Create(ctx, raw)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "got %v", err)
		Expect(err.Error()).To(ContainSubstring("Required value"))
	})

	It("rejects a kind that does not start with an uppercase letter", func() {
		// A lowercase kind still reads through the plane, but would not match the
		// live object in the checks that keep observed objects from being written.
		bad := deploy("a", "a")
		bad.Kind = "deployment"
		err := k8sClient.Create(ctx, newRelease("val-lowercase-kind", bad))
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "got %v", err)
		Expect(err.Error()).To(ContainSubstring("spec.observedResources[0].kind"))
	})

	It("rejects a malformed apiVersion", func() {
		bad := deploy("a", "a")
		bad.APIVersion = "apps/v1/extra"
		err := k8sClient.Create(ctx, newRelease("val-bad-apiversion", bad))
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "got %v", err)
	})

	It("leaves an unset mode unset and rejects unknown modes", func() {
		// Binding controllers write the whole spec without a mode. A server default
		// would differ from what they write and make every reconcile an update.
		release := newRelease("cel-mode")
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		stored := fetchRelease("cel-mode")
		Expect(stored.Spec.Mode).To(BeEmpty())
		Expect(effectiveMode(stored)).To(Equal(openchoreov1alpha1.RenderedReleaseModeManage))

		bad := newRelease("cel-bad-mode")
		bad.Spec.Mode = "Adopt"
		err := k8sClient.Create(ctx, bad)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "got %v", err)
	})
})
