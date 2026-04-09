// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sMocks "github.com/openchoreo/openchoreo/internal/clients/kubernetes/mocks"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	testNs       = "default"
	testTimeout  = 10 * time.Second
	testInterval = 250 * time.Millisecond
)

// forceDelete removes the DataPlaneCleanupFinalizer from a RenderedRelease and deletes it.
// Safe to call even if the resource does not exist.
func forceDelete(ctx context.Context, nn types.NamespacedName) {
	r := &openchoreov1alpha1.RenderedRelease{}
	if err := k8sClient.Get(ctx, nn, r); err != nil {
		return
	}
	if controllerutil.ContainsFinalizer(r, DataPlaneCleanupFinalizer) {
		controllerutil.RemoveFinalizer(r, DataPlaneCleanupFinalizer)
		_ = k8sClient.Update(ctx, r)
	}
	_ = k8sClient.Delete(ctx, r)
}

// forceDeleteClusterDataPlane removes a ClusterDataPlane by name.
func forceDeleteClusterDataPlane(ctx context.Context, name string) {
	cdp := &openchoreov1alpha1.ClusterDataPlane{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, cdp); err != nil {
		return
	}
	_ = k8sClient.Delete(ctx, cdp)
}

// forceDeleteClusterObservabilityPlane removes a ClusterObservabilityPlane by name.
func forceDeleteClusterObservabilityPlane(ctx context.Context, name string) {
	cop := &openchoreov1alpha1.ClusterObservabilityPlane{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, cop); err != nil {
		return
	}
	_ = k8sClient.Delete(ctx, cop)
}

// forceDeleteEnvironment removes an Environment by namespace/name.
func forceDeleteEnvironment(ctx context.Context, nn types.NamespacedName) {
	env := &openchoreov1alpha1.Environment{}
	if err := k8sClient.Get(ctx, nn, env); err != nil {
		return
	}
	_ = k8sClient.Delete(ctx, env)
}

// makeMinimalRelease returns a RenderedRelease with the minimum required spec fields.
func makeMinimalRelease(name, envName string) *openchoreov1alpha1.RenderedRelease {
	return &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNs,
		},
		Spec: openchoreov1alpha1.RenderedReleaseSpec{
			Owner: openchoreov1alpha1.RenderedReleaseOwner{
				ProjectName:   "test-project",
				ComponentName: "test-component",
			},
			EnvironmentName: envName,
		},
	}
}

// makeClusterDataPlane returns a ClusterDataPlane with the required fields.
func makeClusterDataPlane(name, planeID string) *openchoreov1alpha1.ClusterDataPlane {
	return &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID: planeID,
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: "test-ca-cert"},
			},
		},
	}
}

// makeClusterObservabilityPlane returns a ClusterObservabilityPlane with the required fields.
func makeClusterObservabilityPlane(name, planeID string) *openchoreov1alpha1.ClusterObservabilityPlane {
	return &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
			PlaneID: planeID,
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: "test-ca-cert"},
			},
			ObserverURL: "http://observer.test",
		},
	}
}

// makeClusterDataPlaneWithOPRef returns a ClusterDataPlane that references a ClusterObservabilityPlane.
func makeClusterDataPlaneWithOPRef(name, planeID, copName string) *openchoreov1alpha1.ClusterDataPlane {
	cdp := makeClusterDataPlane(name, planeID)
	cdp.Spec.ObservabilityPlaneRef = &openchoreov1alpha1.ClusterObservabilityPlaneRef{
		Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
		Name: copName,
	}
	return cdp
}

// makeEnvironment returns an Environment referencing a ClusterDataPlane.
func makeEnvironment(name, cdpName string) *openchoreov1alpha1.Environment {
	return &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNs,
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
				Name: cdpName,
			},
		},
	}
}

// testReconcilerWithDPClient creates a Reconciler with a mock PlaneClientProvider
// that returns dpClient for any data plane. This allows testing without a real data-plane cluster.
func testReconcilerWithDPClient(dpClient client.Client, _, _ string) *Reconciler {
	mockProvider := &k8sMocks.MockPlaneClientProvider{}
	mockProvider.EXPECT().DataPlaneClient(mock.Anything).Return(dpClient, nil).Maybe()
	mockProvider.EXPECT().ClusterDataPlaneClient(mock.Anything).Return(dpClient, nil).Maybe()
	mockProvider.EXPECT().ObservabilityPlaneClient(mock.Anything).Return(dpClient, nil).Maybe()
	mockProvider.EXPECT().ClusterObservabilityPlaneClient(mock.Anything).Return(dpClient, nil).Maybe()
	mockProvider.EXPECT().WorkflowPlaneClient(mock.Anything).Return(dpClient, nil).Maybe()
	mockProvider.EXPECT().ClusterWorkflowPlaneClient(mock.Anything).Return(dpClient, nil).Maybe()
	return &Reconciler{
		Client:              k8sClient,
		Scheme:              k8sClient.Scheme(),
		PlaneClientProvider: mockProvider,
	}
}

// makeLabeledUnstructured creates an unstructured resource with the tracking labels
// that listLiveResourcesByGVKs uses to find managed resources.
func makeLabeledUnstructured(apiVersion, kind, namespace, name, resourceID string, release *openchoreov1alpha1.RenderedRelease) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(apiVersion)
	obj.SetKind(kind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	obj.SetLabels(map[string]string{
		labels.LabelKeyManagedBy:                 ControllerName,
		labels.LabelKeyRenderedReleaseResourceID: resourceID,
		labels.LabelKeyRenderedReleaseUID:        string(release.UID),
		labels.LabelKeyRenderedReleaseName:       release.Name,
		labels.LabelKeyRenderedReleaseNamespace:  release.Namespace,
	})
	return obj
}

// reconcileRequest builds a reconcile.Request for the given name in the default test namespace.
func reconcileRequest(name string) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: name}}
}

// mustReconcile calls Reconcile and asserts no error is returned.
func mustReconcile(r *Reconciler, req reconcile.Request) reconcile.Result {
	GinkgoHelper()
	result, err := r.Reconcile(ctx, req)
	Expect(err).NotTo(HaveOccurred())
	return result
}

// fetchRelease re-reads a RenderedRelease from the API server.
func fetchRelease(name string) *openchoreov1alpha1.RenderedRelease {
	GinkgoHelper()
	release := &openchoreov1alpha1.RenderedRelease{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNs, Name: name}, release)).To(Succeed())
	return release
}

var _ = Describe("RenderedRelease Controller", func() {

	// ─────────────────────────────────────────────────────────────
	// Non-existent resource
	// ─────────────────────────────────────────────────────────────

	Context("when the RenderedRelease resource does not exist", func() {
		It("should return no error and not requeue", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "ghost-release", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	// ─────────────────────────────────────────────────────────────
	// New release — first reconcile adds finalizer
	// ─────────────────────────────────────────────────────────────

	Context("when a new RenderedRelease is created", func() {
		const releaseName = "release-first-reconcile"
		nn := types.NamespacedName{Name: releaseName, Namespace: "default"}

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, makeMinimalRelease(releaseName, "my-env"))).To(Succeed())
		})

		AfterEach(func() {
			forceDelete(ctx, nn)
		})

		It("should add DataPlaneCleanupFinalizer on first reconcile", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// First reconcile returns early after adding the finalizer — no requeue
			Expect(result.Requeue).To(BeFalse())

			got := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(got, DataPlaneCleanupFinalizer)).To(BeTrue())
		})

		It("should return error on second reconcile when environment does not exist", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			By("first reconcile: adds finalizer")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("second reconcile: tries to get environment, which is missing")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("my-env"))
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Deleted release — finalization flow
	// ─────────────────────────────────────────────────────────────

	Context("when a RenderedRelease with a finalizer is being deleted", func() {
		const releaseName = "release-finalizing"
		nn := types.NamespacedName{Name: releaseName, Namespace: "default"}

		BeforeEach(func() {
			By("creating release with pre-set finalizer")
			release := makeMinimalRelease(releaseName, "my-env")
			release.Finalizers = []string{DataPlaneCleanupFinalizer}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			By("deleting release (sets DeletionTimestamp, finalizer blocks removal)")
			Expect(k8sClient.Delete(ctx, release)).To(Succeed())
		})

		AfterEach(func() {
			forceDelete(ctx, nn)
		})

		It("should set Finalizing condition on first finalize reconcile", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// Condition is set and status updated; returns empty Result (no requeue)
			Expect(result.Requeue).To(BeFalse())

			got := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())

			cond := apimeta.FindStatusCondition(got.Status.Conditions, string(ConditionFinalizing))
			Expect(cond).NotTo(BeNil(), "Finalizing condition should be set")
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonCleanupInProgress)))
		})

		It("should return error on second finalize reconcile when environment does not exist", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			By("first reconcile: sets Finalizing condition")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("second reconcile: Finalizing condition already set, tries to get DP client")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("my-env"))
		})

		It("should keep the finalizer while cleanup is pending", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			// After one reconcile the finalizer should still be present (cleanup didn't succeed)
			_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			got := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(got, DataPlaneCleanupFinalizer)).To(BeTrue())
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Deleted release — no finalizer (immediate deletion)
	// ─────────────────────────────────────────────────────────────

	Context("when a RenderedRelease without a finalizer is deleted", func() {
		It("should return no error (resource already gone)", func() {
			const releaseName = "release-no-finalizer"
			nn := types.NamespacedName{Name: releaseName, Namespace: "default"}

			release := makeMinimalRelease(releaseName, "my-env")
			// Explicitly no Finalizers
			Expect(k8sClient.Create(ctx, release)).To(Succeed())
			Expect(k8sClient.Delete(ctx, release)).To(Succeed())

			// Without a finalizer the API server removes the object immediately
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.RenderedRelease{})
				return apierrors.IsNotFound(err)
			}, "5s", "100ms").Should(BeTrue())

			// Reconcile on a missing resource should be a no-op
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Deleted release — finalizer absent, DeletionTimestamp present
	// ─────────────────────────────────────────────────────────────

	Context("when a RenderedRelease has a DeletionTimestamp but no finalizer", func() {
		const releaseName = "release-del-no-finalizer"
		nn := types.NamespacedName{Name: releaseName, Namespace: "default"}

		BeforeEach(func() {
			By("creating with finalizer so we can control DeletionTimestamp")
			release := makeMinimalRelease(releaseName, "env-x")
			release.Finalizers = []string{DataPlaneCleanupFinalizer}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			By("deleting to set DeletionTimestamp")
			Expect(k8sClient.Delete(ctx, release)).To(Succeed())

			By("stripping the finalizer so finalize() returns early")
			fetched := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			controllerutil.RemoveFinalizer(fetched, DataPlaneCleanupFinalizer)
			Expect(k8sClient.Update(ctx, fetched)).To(Succeed())
		})

		AfterEach(func() {
			forceDelete(ctx, nn)
		})

		It("should return no error when finalizer is absent during finalization", func() {
			// After removing the finalizer the API server may already delete the object;
			// either way reconcile should succeed without error.
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Status persistence
	// ─────────────────────────────────────────────────────────────

	Context("when status is updated on a Release", func() {
		const releaseName = "release-status-persist"
		nn := types.NamespacedName{Name: releaseName, Namespace: "default"}

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, makeMinimalRelease(releaseName, "env-status"))).To(Succeed())
		})

		AfterEach(func() {
			forceDelete(ctx, nn)
		})

		It("should persist status conditions via status subresource", func() {
			By("fetching release")
			release := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, release)).To(Succeed())

			By("updating status with a condition")
			release.Status.Conditions = []metav1.Condition{
				{
					Type:               "TestCondition",
					Status:             metav1.ConditionTrue,
					Reason:             "TestReason",
					Message:            "test message",
					LastTransitionTime: metav1.Now(),
					ObservedGeneration: release.Generation,
				},
			}
			Expect(k8sClient.Status().Update(ctx, release)).To(Succeed())

			By("re-fetching and verifying condition persisted")
			fetched := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, "TestCondition")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("TestReason"))
		})

		It("should persist resource inventory in status", func() {
			By("fetching release")
			release := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, release)).To(Succeed())

			By("writing resource status entries")
			release.Status.Resources = []openchoreov1alpha1.ResourceStatus{
				{
					ID:           "res-1",
					Group:        "apps",
					Version:      "v1",
					Kind:         "Deployment",
					Name:         "my-deploy",
					Namespace:    "dp-ns",
					HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
				},
			}
			Expect(k8sClient.Status().Update(ctx, release)).To(Succeed())

			By("re-fetching and verifying resources persisted")
			fetched := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Resources).To(HaveLen(1))
			Expect(fetched.Status.Resources[0].ID).To(Equal("res-1"))
			Expect(fetched.Status.Resources[0].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusHealthy))
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Happy path: full Reconcile applies resources and updates status
	// ─────────────────────────────────────────────────────────────

	Context("when a RenderedRelease has resources to apply", func() {
		const (
			releaseName = "release-happy-path"
			envName     = "env-happy-path"
			cdpName     = "cdp-happy-path"
			planeID     = "plane-happy-path"
		)
		nn := types.NamespacedName{Name: releaseName, Namespace: testNs}
		envNN := types.NamespacedName{Name: envName, Namespace: testNs}

		AfterEach(func() {
			forceDelete(ctx, nn)
			forceDeleteEnvironment(ctx, envNN)
			forceDeleteClusterDataPlane(ctx, cdpName)
		})

		It("should apply resources to data plane and update status", func() {
			By("Creating prerequisite ClusterDataPlane and Environment")
			Expect(k8sClient.Create(ctx, makeClusterDataPlane(cdpName, planeID))).To(Succeed())
			Expect(k8sClient.Create(ctx, makeEnvironment(envName, cdpName))).To(Succeed())

			By("Creating a RenderedRelease with a ConfigMap resource in spec")
			release := makeMinimalRelease(releaseName, envName)
			cmJSON := []byte(`{
				"apiVersion": "v1",
				"kind": "ConfigMap",
				"metadata": {
					"name": "test-cm-happy",
					"namespace": "default"
				},
				"data": {
					"key": "value"
				}
			}`)
			release.Spec.Resources = []openchoreov1alpha1.Resource{
				{
					ID:     "cm-happy-1",
					Object: &runtime.RawExtension{Raw: cmJSON},
				},
			}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			By("Using the envtest client as the data-plane client (server-side apply requires a real API server)")
			r := testReconcilerWithDPClient(k8sClient, planeID, cdpName)

			By("First reconcile: adds finalizer")
			mustReconcile(r, reconcileRequest(releaseName))
			updated := fetchRelease(releaseName)
			Expect(controllerutil.ContainsFinalizer(updated, DataPlaneCleanupFinalizer)).To(BeTrue())

			By("Second reconcile: applies resources and persists status")
			mustReconcile(r, reconcileRequest(releaseName))

			By("Third reconcile: status already persisted, reaches requeue logic")
			result := mustReconcile(r, reconcileRequest(releaseName))
			Expect(result.RequeueAfter).NotTo(BeZero(), "should requeue for status polling")

			By("Verifying the ConfigMap was applied to the data plane")
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-cm-happy", Namespace: testNs}, cm)).To(Succeed())
			Expect(cm.Data["key"]).To(Equal("value"))
			Expect(cm.Labels[labels.LabelKeyRenderedReleaseResourceID]).To(Equal("cm-happy-1"))
			Expect(cm.Labels[labels.LabelKeyManagedBy]).To(Equal(ControllerName))

			By("Verifying the ResourcesApplied condition is set to True")
			updated = fetchRelease(releaseName)
			appliedCond := apimeta.FindStatusCondition(updated.Status.Conditions, ConditionResourcesApplied)
			Expect(appliedCond).NotTo(BeNil())
			Expect(appliedCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(appliedCond.Reason).To(Equal(ReasonApplySucceeded))

			By("Verifying the status resource inventory")
			Expect(updated.Status.Resources).To(HaveLen(1))
			Expect(updated.Status.Resources[0].ID).To(Equal("cm-happy-1"))
			Expect(updated.Status.Resources[0].Kind).To(Equal("ConfigMap"))
			Expect(updated.Status.Resources[0].Name).To(Equal("test-cm-happy"))
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Reconcile: transitioning resources use progressing requeue interval
	// ─────────────────────────────────────────────────────────────

	Context("when applied resources are in a transitioning state", func() {
		const (
			releaseName = "release-transitioning"
			envName     = "env-transitioning"
			cdpName     = "cdp-transitioning"
			planeID     = "plane-transitioning"
		)
		nn := types.NamespacedName{Name: releaseName, Namespace: testNs}
		envNN := types.NamespacedName{Name: envName, Namespace: testNs}

		AfterEach(func() {
			forceDelete(ctx, nn)
			forceDeleteEnvironment(ctx, envNN)
			forceDeleteClusterDataPlane(ctx, cdpName)
		})

		It("should requeue with a shorter progressing interval", func() {
			By("Creating prerequisite ClusterDataPlane and Environment")
			Expect(k8sClient.Create(ctx, makeClusterDataPlane(cdpName, planeID))).To(Succeed())
			Expect(k8sClient.Create(ctx, makeEnvironment(envName, cdpName))).To(Succeed())

			By("Creating a RenderedRelease with a Deployment resource")
			release := makeMinimalRelease(releaseName, envName)
			// A Deployment in envtest will have ObservedGeneration=0 (no real deployment controller),
			// so getDeploymentHealth returns Progressing, triggering the transitioning requeue path.
			deployJSON := []byte(`{
				"apiVersion": "apps/v1",
				"kind": "Deployment",
				"metadata": {
					"name": "test-deploy-transitioning",
					"namespace": "default"
				},
				"spec": {
					"replicas": 1,
					"selector": {
						"matchLabels": {
							"app": "test-transitioning"
						}
					},
					"template": {
						"metadata": {
							"labels": {
								"app": "test-transitioning"
							}
						},
						"spec": {
							"containers": [{
								"name": "nginx",
								"image": "nginx:latest"
							}]
						}
					}
				}
			}`)
			release.Spec.Resources = []openchoreov1alpha1.Resource{
				{
					ID:     "deploy-trans-1",
					Object: &runtime.RawExtension{Raw: deployJSON},
				},
			}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			r := testReconcilerWithDPClient(k8sClient, planeID, cdpName)

			By("First reconcile: adds finalizer")
			mustReconcile(r, reconcileRequest(releaseName))

			By("Second reconcile: applies Deployment and persists status")
			mustReconcile(r, reconcileRequest(releaseName))

			By("Third reconcile: detects transitioning resource, requeues with progressing interval")
			result := mustReconcile(r, reconcileRequest(releaseName))
			// Default progressing interval is 10s with up to 20% jitter → [10s, 12s)
			Expect(result.RequeueAfter).To(BeNumerically(">=", 10*time.Second))
			Expect(result.RequeueAfter).To(BeNumerically("<", 12*time.Second))

			By("Verifying the Deployment resource has Progressing health in status")
			updated := fetchRelease(releaseName)
			Expect(updated.Status.Resources).To(HaveLen(1))
			Expect(updated.Status.Resources[0].ID).To(Equal("deploy-trans-1"))
			Expect(updated.Status.Resources[0].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusProgressing))
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Finalization: successful cleanup with no live resources
	// ─────────────────────────────────────────────────────────────

	Context("Finalization with no live resources in data plane", func() {
		const (
			releaseName = "release-finalize-clean"
			envName     = "env-finalize-clean"
			cdpName     = "cdp-finalize-clean"
			planeID     = "plane-finalize-clean"
		)
		nn := types.NamespacedName{Name: releaseName, Namespace: testNs}
		envNN := types.NamespacedName{Name: envName, Namespace: testNs}

		AfterEach(func() {
			forceDelete(ctx, nn)
			forceDeleteEnvironment(ctx, envNN)
			forceDeleteClusterDataPlane(ctx, cdpName)
		})

		It("sets Finalizing condition then removes finalizer when no resources exist", func() {
			By("Creating prerequisite ClusterDataPlane and Environment")
			Expect(k8sClient.Create(ctx, makeClusterDataPlane(cdpName, planeID))).To(Succeed())
			Expect(k8sClient.Create(ctx, makeEnvironment(envName, cdpName))).To(Succeed())

			By("Creating an empty fake data-plane client (no live resources)")
			dpClient := fake.NewClientBuilder().Build()
			r := testReconcilerWithDPClient(dpClient, planeID, cdpName)

			By("Creating a RenderedRelease with the finalizer pre-set")
			release := makeMinimalRelease(releaseName, envName)
			release.Finalizers = []string{DataPlaneCleanupFinalizer}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			By("Deleting the release (sets DeletionTimestamp; finalizer blocks removal)")
			Expect(k8sClient.Delete(ctx, release)).To(Succeed())

			By("Verifying the object is marked for deletion but still present")
			Eventually(func() bool {
				updated := &openchoreov1alpha1.RenderedRelease{}
				if err := k8sClient.Get(ctx, nn, updated); err != nil {
					return false
				}
				return !updated.DeletionTimestamp.IsZero()
			}, testTimeout, testInterval).Should(BeTrue())

			By("First finalize reconcile: sets the Finalizing=True condition and returns early")
			mustReconcile(r, reconcileRequest(releaseName))

			By("Verifying the Finalizing condition is True")
			updated := fetchRelease(releaseName)
			cond := apimeta.FindStatusCondition(updated.Status.Conditions, string(ConditionFinalizing))
			Expect(cond).NotTo(BeNil(), "Finalizing condition must be present")
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonCleanupInProgress)))
			Expect(updated.Finalizers).To(ContainElement(DataPlaneCleanupFinalizer),
				"finalizer must still be present after first finalize reconcile")

			By("Second finalize reconcile: no live resources → removes finalizer")
			mustReconcile(r, reconcileRequest(releaseName))

			By("Verifying the RenderedRelease is deleted after finalizer removal")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.RenderedRelease{})
				return apierrors.IsNotFound(err)
			}, testTimeout, testInterval).Should(BeTrue(),
				"RenderedRelease should be fully deleted after finalizer removal")
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Finalization: successful cleanup with live resources
	// ─────────────────────────────────────────────────────────────

	Context("Finalization with live resources in data plane", func() {
		const (
			releaseName = "release-finalize-live"
			envName     = "env-finalize-live"
			cdpName     = "cdp-finalize-live"
			planeID     = "plane-finalize-live"
		)
		nn := types.NamespacedName{Name: releaseName, Namespace: testNs}
		envNN := types.NamespacedName{Name: envName, Namespace: testNs}

		AfterEach(func() {
			forceDelete(ctx, nn)
			forceDeleteEnvironment(ctx, envNN)
			forceDeleteClusterDataPlane(ctx, cdpName)
		})

		It("deletes live resources, requeues, then removes finalizer on next reconcile", func() {
			By("Creating prerequisite ClusterDataPlane and Environment")
			Expect(k8sClient.Create(ctx, makeClusterDataPlane(cdpName, planeID))).To(Succeed())
			Expect(k8sClient.Create(ctx, makeEnvironment(envName, cdpName))).To(Succeed())

			By("Creating a RenderedRelease with the finalizer pre-set")
			release := makeMinimalRelease(releaseName, envName)
			release.Finalizers = []string{DataPlaneCleanupFinalizer}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			By("Re-fetching the release to get UID for tracking labels")
			fetched := fetchRelease(releaseName)

			By("Creating a fake data-plane client with a live ConfigMap that has tracking labels")
			liveConfigMap := makeLabeledUnstructured("v1", "ConfigMap", "dp-ns", "my-config", "res-cm-1", fetched)
			dpClient := fake.NewClientBuilder().
				WithObjects(liveConfigMap).
				Build()
			r := testReconcilerWithDPClient(dpClient, planeID, cdpName)

			By("Deleting the release (sets DeletionTimestamp; finalizer blocks removal)")
			Expect(k8sClient.Delete(ctx, release)).To(Succeed())
			Eventually(func() bool {
				updated := &openchoreov1alpha1.RenderedRelease{}
				if err := k8sClient.Get(ctx, nn, updated); err != nil {
					return false
				}
				return !updated.DeletionTimestamp.IsZero()
			}, testTimeout, testInterval).Should(BeTrue())

			By("First finalize reconcile: sets the Finalizing condition")
			mustReconcile(r, reconcileRequest(releaseName))

			By("Second finalize reconcile: finds live resources, deletes them, requeues")
			result := mustReconcile(r, reconcileRequest(releaseName))
			Expect(result.RequeueAfter).To(Equal(5*time.Second),
				"should requeue after 5s when live resources existed")

			By("Verifying the finalizer is still present (waiting for resources to disappear)")
			updated := fetchRelease(releaseName)
			Expect(updated.Finalizers).To(ContainElement(DataPlaneCleanupFinalizer))

			By("Third finalize reconcile: no more live resources → removes finalizer")
			mustReconcile(r, reconcileRequest(releaseName))

			By("Verifying the RenderedRelease is fully deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.RenderedRelease{})
				return apierrors.IsNotFound(err)
			}, testTimeout, testInterval).Should(BeTrue())
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Finalization: CleanupFailed condition on DP client error
	// ─────────────────────────────────────────────────────────────

	Context("Finalization sets CleanupFailed condition when DP client cannot be obtained", func() {
		const (
			releaseName = "release-finalize-dpfail"
			envName     = "env-finalize-dpfail"
		)
		nn := types.NamespacedName{Name: releaseName, Namespace: testNs}

		AfterEach(func() {
			forceDelete(ctx, nn)
		})

		It("sets CleanupFailed condition and returns error when environment is missing", func() {
			By("Creating a RenderedRelease with the finalizer and Finalizing condition pre-set")
			release := makeMinimalRelease(releaseName, envName)
			release.Finalizers = []string{DataPlaneCleanupFinalizer}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			By("Deleting the release")
			Expect(k8sClient.Delete(ctx, release)).To(Succeed())
			Eventually(func() bool {
				updated := &openchoreov1alpha1.RenderedRelease{}
				if err := k8sClient.Get(ctx, nn, updated); err != nil {
					return false
				}
				return !updated.DeletionTimestamp.IsZero()
			}, testTimeout, testInterval).Should(BeTrue())

			By("First reconcile: sets Finalizing condition")
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			mustReconcile(r, reconcileRequest(releaseName))

			By("Second reconcile: environment does not exist → error + CleanupFailed condition")
			_, err := r.Reconcile(ctx, reconcileRequest(releaseName))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(envName))

			By("Verifying the CleanupFailed condition is set")
			updated := fetchRelease(releaseName)
			cond := apimeta.FindStatusCondition(updated.Status.Conditions, string(ConditionFinalizing))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal(string(ReasonCleanupFailed)))

			By("Verifying the finalizer is still present (cleanup did not complete)")
			Expect(updated.Finalizers).To(ContainElement(DataPlaneCleanupFinalizer))
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Finalization: does not add finalizer on deleted resource
	// ─────────────────────────────────────────────────────────────

	Context("Finalizer management edge cases", func() {
		const releaseName = "release-finalizer-mgmt"
		req := reconcileRequest(releaseName)

		AfterEach(func() { forceDelete(ctx, types.NamespacedName{Name: releaseName, Namespace: testNs}) })

		It("does not add the finalizer more than once on repeated reconciles", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			By("Creating a RenderedRelease without a finalizer")
			Expect(k8sClient.Create(ctx,
				makeMinimalRelease(releaseName, "env-mgmt"),
			)).To(Succeed())

			By("First reconcile adds the finalizer")
			mustReconcile(r, req)

			By("Second reconcile: finalizer already present, proceeds past ensureFinalizer")
			// The reconcile will error because the environment doesn't exist,
			// but the point is that ensureFinalizer is a no-op this time.
			_, _ = r.Reconcile(ctx, req)

			By("Verifying the finalizer appears exactly once")
			release := fetchRelease(releaseName)
			count := 0
			for _, f := range release.Finalizers {
				if f == DataPlaneCleanupFinalizer {
					count++
				}
			}
			Expect(count).To(Equal(1), "finalizer should appear exactly once")
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Reconcile: observability plane target path
	// ─────────────────────────────────────────────────────────────

	Context("when a RenderedRelease targets the observability plane", func() {
		const (
			releaseName = "release-op-target"
			envName     = "env-op-target"
			cdpName     = "cdp-op-target"
			copName     = "cop-op-target"
			planeID     = "plane-op-target"
			opPlaneID   = "obs-plane-op-target"
		)
		nn := types.NamespacedName{Name: releaseName, Namespace: testNs}
		envNN := types.NamespacedName{Name: envName, Namespace: testNs}

		AfterEach(func() {
			forceDelete(ctx, nn)
			forceDeleteEnvironment(ctx, envNN)
			forceDeleteClusterDataPlane(ctx, cdpName)
			forceDeleteClusterObservabilityPlane(ctx, copName)
		})

		It("should resolve the observability plane client and apply resources", func() {
			By("Creating prerequisite ClusterObservabilityPlane, ClusterDataPlane, and Environment")
			Expect(k8sClient.Create(ctx, makeClusterObservabilityPlane(copName, opPlaneID))).To(Succeed())
			Expect(k8sClient.Create(ctx, makeClusterDataPlaneWithOPRef(cdpName, planeID, copName))).To(Succeed())
			Expect(k8sClient.Create(ctx, makeEnvironment(envName, cdpName))).To(Succeed())

			By("Creating a RenderedRelease targeting the observability plane with a ConfigMap resource")
			release := makeMinimalRelease(releaseName, envName)
			release.Spec.TargetPlane = targetPlaneObservabilityPlane
			cmJSON := []byte(`{
				"apiVersion": "v1",
				"kind": "ConfigMap",
				"metadata": {
					"name": "test-cm-op-target",
					"namespace": "default"
				},
				"data": {
					"key": "op-value"
				}
			}`)
			release.Spec.Resources = []openchoreov1alpha1.Resource{
				{
					ID:     "cm-op-1",
					Object: &runtime.RawExtension{Raw: cmJSON},
				},
			}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			By("Using the envtest client as the observability plane client")
			r := testReconcilerWithDPClient(k8sClient, planeID, cdpName)

			By("First reconcile: adds finalizer")
			mustReconcile(r, reconcileRequest(releaseName))

			By("Second reconcile: applies resources via OP client path")
			mustReconcile(r, reconcileRequest(releaseName))

			By("Third reconcile: status already persisted, reaches requeue")
			result := mustReconcile(r, reconcileRequest(releaseName))
			Expect(result.RequeueAfter).NotTo(BeZero(), "should requeue for status polling")

			By("Verifying the ConfigMap was applied")
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-cm-op-target", Namespace: testNs}, cm)).To(Succeed())
			Expect(cm.Data["key"]).To(Equal("op-value"))
			Expect(cm.Labels[labels.LabelKeyRenderedReleaseResourceID]).To(Equal("cm-op-1"))

			By("Verifying the ResourcesApplied condition is set to True")
			updated := fetchRelease(releaseName)
			appliedCond := apimeta.FindStatusCondition(updated.Status.Conditions, ConditionResourcesApplied)
			Expect(appliedCond).NotTo(BeNil())
			Expect(appliedCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(appliedCond.Reason).To(Equal(ReasonApplySucceeded))
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Reconcile: apply failure sets ResourcesApplied=False
	// ─────────────────────────────────────────────────────────────

	Context("when resource apply fails", func() {
		const (
			releaseName = "release-apply-fail"
			envName     = "env-apply-fail"
			cdpName     = "cdp-apply-fail"
			planeID     = "plane-apply-fail"
		)
		nn := types.NamespacedName{Name: releaseName, Namespace: testNs}
		envNN := types.NamespacedName{Name: envName, Namespace: testNs}

		AfterEach(func() {
			forceDelete(ctx, nn)
			forceDeleteEnvironment(ctx, envNN)
			forceDeleteClusterDataPlane(ctx, cdpName)
		})

		It("should set ResourcesApplied=False condition when apply fails", func() {
			By("Creating prerequisite ClusterDataPlane and Environment")
			Expect(k8sClient.Create(ctx, makeClusterDataPlane(cdpName, planeID))).To(Succeed())
			Expect(k8sClient.Create(ctx, makeEnvironment(envName, cdpName))).To(Succeed())

			By("Creating a RenderedRelease with a resource that will fail to apply")
			release := makeMinimalRelease(releaseName, envName)
			// Use a resource with an invalid GVK that the fake client cannot handle
			badJSON := []byte(`{
				"apiVersion": "nonexistent.example.com/v1",
				"kind": "DoesNotExist",
				"metadata": {
					"name": "bad-resource",
					"namespace": "default"
				}
			}`)
			release.Spec.Resources = []openchoreov1alpha1.Resource{
				{
					ID:     "bad-1",
					Object: &runtime.RawExtension{Raw: badJSON},
				},
			}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			By("Using a fake DP client that does not know the CRD (will fail on apply)")
			dpClient := fake.NewClientBuilder().Build()
			r := testReconcilerWithDPClient(dpClient, planeID, cdpName)

			By("First reconcile: adds finalizer")
			mustReconcile(r, reconcileRequest(releaseName))

			By("Second reconcile: apply fails, should set ResourcesApplied=False and return error")
			_, err := r.Reconcile(ctx, reconcileRequest(releaseName))
			Expect(err).To(HaveOccurred())

			By("Verifying the ResourcesApplied condition is False")
			updated := fetchRelease(releaseName)
			appliedCond := apimeta.FindStatusCondition(updated.Status.Conditions, ConditionResourcesApplied)
			Expect(appliedCond).NotTo(BeNil())
			Expect(appliedCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(appliedCond.Reason).To(Equal(ReasonApplyFailed))
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Reconcile: stale resource cleanup
	// ─────────────────────────────────────────────────────────────

	Context("when a resource is removed from spec", func() {
		const (
			releaseName = "release-stale-cleanup"
			envName     = "env-stale-cleanup"
			cdpName     = "cdp-stale-cleanup"
			planeID     = "plane-stale-cleanup"
		)
		nn := types.NamespacedName{Name: releaseName, Namespace: testNs}
		envNN := types.NamespacedName{Name: envName, Namespace: testNs}

		AfterEach(func() {
			forceDelete(ctx, nn)
			forceDeleteEnvironment(ctx, envNN)
			forceDeleteClusterDataPlane(ctx, cdpName)
		})

		It("should delete the stale resource from the data plane", func() {
			By("Creating prerequisite ClusterDataPlane and Environment")
			Expect(k8sClient.Create(ctx, makeClusterDataPlane(cdpName, planeID))).To(Succeed())
			Expect(k8sClient.Create(ctx, makeEnvironment(envName, cdpName))).To(Succeed())

			By("Creating a RenderedRelease with two ConfigMap resources")
			release := makeMinimalRelease(releaseName, envName)
			cm1JSON := []byte(`{
				"apiVersion": "v1",
				"kind": "ConfigMap",
				"metadata": {
					"name": "cm-keep",
					"namespace": "default"
				},
				"data": {"key": "keep"}
			}`)
			cm2JSON := []byte(`{
				"apiVersion": "v1",
				"kind": "ConfigMap",
				"metadata": {
					"name": "cm-remove",
					"namespace": "default"
				},
				"data": {"key": "remove"}
			}`)
			release.Spec.Resources = []openchoreov1alpha1.Resource{
				{ID: "cm-keep-1", Object: &runtime.RawExtension{Raw: cm1JSON}},
				{ID: "cm-remove-1", Object: &runtime.RawExtension{Raw: cm2JSON}},
			}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			r := testReconcilerWithDPClient(k8sClient, planeID, cdpName)

			By("Reconciling until resources are applied and status is updated")
			mustReconcile(r, reconcileRequest(releaseName)) // adds finalizer
			mustReconcile(r, reconcileRequest(releaseName)) // applies resources + updates status
			mustReconcile(r, reconcileRequest(releaseName)) // status already persisted, reaches requeue

			By("Verifying both ConfigMaps exist")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cm-keep", Namespace: testNs}, &corev1.ConfigMap{})).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cm-remove", Namespace: testNs}, &corev1.ConfigMap{})).To(Succeed())

			By("Removing cm-remove from the spec")
			updated := fetchRelease(releaseName)
			updated.Spec.Resources = []openchoreov1alpha1.Resource{
				{ID: "cm-keep-1", Object: &runtime.RawExtension{Raw: cm1JSON}},
			}
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())

			By("Reconciling to trigger stale resource cleanup")
			mustReconcile(r, reconcileRequest(releaseName))

			By("Verifying cm-keep still exists and cm-remove was deleted")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cm-keep", Namespace: testNs}, &corev1.ConfigMap{})).To(Succeed())
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "cm-remove", Namespace: testNs}, &corev1.ConfigMap{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "stale ConfigMap should have been deleted")
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Reconcile: default targetPlane when empty
	// ─────────────────────────────────────────────────────────────

	Context("when targetPlane is empty", func() {
		const (
			releaseName = "release-default-tp"
			envName     = "env-default-tp"
			cdpName     = "cdp-default-tp"
			planeID     = "plane-default-tp"
		)
		nn := types.NamespacedName{Name: releaseName, Namespace: testNs}
		envNN := types.NamespacedName{Name: envName, Namespace: testNs}

		AfterEach(func() {
			forceDelete(ctx, nn)
			forceDeleteEnvironment(ctx, envNN)
			forceDeleteClusterDataPlane(ctx, cdpName)
		})

		It("should default to dataplane and apply resources successfully", func() {
			By("Creating prerequisite ClusterDataPlane and Environment")
			Expect(k8sClient.Create(ctx, makeClusterDataPlane(cdpName, planeID))).To(Succeed())
			Expect(k8sClient.Create(ctx, makeEnvironment(envName, cdpName))).To(Succeed())

			By("Creating a RenderedRelease with empty targetPlane")
			release := makeMinimalRelease(releaseName, envName)
			release.Spec.TargetPlane = "" // explicitly empty
			cmJSON := []byte(`{
				"apiVersion": "v1",
				"kind": "ConfigMap",
				"metadata": {
					"name": "cm-default-tp",
					"namespace": "default"
				},
				"data": {"key": "default-tp"}
			}`)
			release.Spec.Resources = []openchoreov1alpha1.Resource{
				{ID: "cm-dtp-1", Object: &runtime.RawExtension{Raw: cmJSON}},
			}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			r := testReconcilerWithDPClient(k8sClient, planeID, cdpName)

			By("Reconciling through to completion")
			mustReconcile(r, reconcileRequest(releaseName)) // adds finalizer
			mustReconcile(r, reconcileRequest(releaseName)) // applies resources
			mustReconcile(r, reconcileRequest(releaseName)) // reaches requeue

			By("Verifying resource was applied via the default dataplane path")
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cm-default-tp", Namespace: testNs}, cm)).To(Succeed())
			Expect(cm.Data["key"]).To(Equal("default-tp"))
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Finalization: observability plane target path
	// ─────────────────────────────────────────────────────────────

	Context("when finalizing a RenderedRelease that targets the observability plane", func() {
		const (
			releaseName = "release-finalize-op"
			envName     = "env-finalize-op"
			cdpName     = "cdp-finalize-op"
			copName     = "cop-finalize-op"
			planeID     = "plane-finalize-op"
			opPlaneID   = "obs-plane-finalize-op"
		)
		nn := types.NamespacedName{Name: releaseName, Namespace: testNs}
		envNN := types.NamespacedName{Name: envName, Namespace: testNs}

		AfterEach(func() {
			forceDelete(ctx, nn)
			forceDeleteEnvironment(ctx, envNN)
			forceDeleteClusterDataPlane(ctx, cdpName)
			forceDeleteClusterObservabilityPlane(ctx, copName)
		})

		It("should resolve the OP client and clean up resources during finalization", func() {
			By("Creating prerequisite ClusterObservabilityPlane, ClusterDataPlane, and Environment")
			Expect(k8sClient.Create(ctx, makeClusterObservabilityPlane(copName, opPlaneID))).To(Succeed())
			Expect(k8sClient.Create(ctx, makeClusterDataPlaneWithOPRef(cdpName, planeID, copName))).To(Succeed())
			Expect(k8sClient.Create(ctx, makeEnvironment(envName, cdpName))).To(Succeed())

			By("Creating a RenderedRelease targeting observabilityplane with finalizer pre-set")
			release := makeMinimalRelease(releaseName, envName)
			release.Spec.TargetPlane = targetPlaneObservabilityPlane
			release.Finalizers = []string{DataPlaneCleanupFinalizer}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			By("Using an empty fake OP client (no live resources)")
			dpClient := fake.NewClientBuilder().Build()
			r := testReconcilerWithDPClient(dpClient, planeID, cdpName)

			By("Deleting the release")
			Expect(k8sClient.Delete(ctx, release)).To(Succeed())
			Eventually(func() bool {
				updated := &openchoreov1alpha1.RenderedRelease{}
				if err := k8sClient.Get(ctx, nn, updated); err != nil {
					return false
				}
				return !updated.DeletionTimestamp.IsZero()
			}, testTimeout, testInterval).Should(BeTrue())

			By("First finalize reconcile: sets Finalizing condition")
			mustReconcile(r, reconcileRequest(releaseName))
			updated := fetchRelease(releaseName)
			cond := apimeta.FindStatusCondition(updated.Status.Conditions, string(ConditionFinalizing))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			By("Second finalize reconcile: no live resources, removes finalizer")
			mustReconcile(r, reconcileRequest(releaseName))

			By("Verifying the RenderedRelease is fully deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.RenderedRelease{})
				return apierrors.IsNotFound(err)
			}, testTimeout, testInterval).Should(BeTrue())
		})
	})
})

// ─────────────────────────────────────────────────────────────
// ensureNamespaces
// ─────────────────────────────────────────────────────────────

var _ = Describe("ensureNamespaces", func() {
	makeNS := func(name string) *corev1.Namespace {
		return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}

	deleteNS := func(name string) {
		ns := &corev1.Namespace{}
		ns.Name = name
		_ = k8sClient.Delete(ctx, ns)
	}

	Context("with an empty namespace list", func() {
		It("should be a no-op", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(r.ensureNamespaces(ctx, k8sClient, nil)).To(Succeed())
		})
	})

	Context("when namespace does not exist", func() {
		const nsName = "test-ensure-ns-new"

		AfterEach(func() { deleteNS(nsName) })

		It("should create the namespace", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(r.ensureNamespaces(ctx, k8sClient, []*corev1.Namespace{makeNS(nsName)})).To(Succeed())

			existing := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, existing)).To(Succeed())
		})
	})

	Context("when namespace already exists", func() {
		const nsName = "test-ensure-ns-exists"

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, makeNS(nsName))).To(Succeed())
		})
		AfterEach(func() { deleteNS(nsName) })

		It("should not return an error", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(r.ensureNamespaces(ctx, k8sClient, []*corev1.Namespace{makeNS(nsName)})).To(Succeed())
		})
	})

	Context("when multiple namespaces are provided", func() {
		nsNames := []string{"test-multi-ns-a", "test-multi-ns-b", "test-multi-ns-c"}

		AfterEach(func() {
			for _, name := range nsNames {
				deleteNS(name)
			}
		})

		It("should create all namespaces", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			nsList := make([]*corev1.Namespace, len(nsNames))
			for i, name := range nsNames {
				nsList[i] = makeNS(name)
			}
			Expect(r.ensureNamespaces(ctx, k8sClient, nsList)).To(Succeed())

			for _, name := range nsNames {
				existing := &corev1.Namespace{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, existing)).To(Succeed())
			}
		})
	})
})

// ─────────────────────────────────────────────────────────────
// applyResources and deleteResources
// ─────────────────────────────────────────────────────────────

var _ = Describe("applyResources and deleteResources", func() {
	const testNS = "default"
	configMapGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}

	makeTrackedCM := func(name, resourceID, releaseUID string) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(configMapGVK)
		obj.SetName(name)
		obj.SetNamespace(testNS)
		obj.SetLabels(map[string]string{
			labels.LabelKeyManagedBy:                 ControllerName,
			labels.LabelKeyRenderedReleaseResourceID: resourceID,
			labels.LabelKeyRenderedReleaseUID:        releaseUID,
			labels.LabelKeyRenderedReleaseName:       "test-release",
			labels.LabelKeyRenderedReleaseNamespace:  testNS,
		})
		return obj
	}

	deleteCM := func(name string) {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(configMapGVK)
		obj.SetName(name)
		obj.SetNamespace(testNS)
		_ = k8sClient.Delete(ctx, obj)
	}

	Context("with an empty resource list", func() {
		It("applyResources should be a no-op", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(r.applyResources(ctx, k8sClient, nil)).To(Succeed())
		})

		It("deleteResources should be a no-op", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(r.deleteResources(ctx, k8sClient, nil)).To(Succeed())
		})
	})

	Context("when applying a ConfigMap resource", func() {
		const cmName = "test-apply-cm"
		const resourceID = "apply-res-1"
		const releaseUID = "apply-uid-1"

		AfterEach(func() { deleteCM(cmName) })

		It("should apply the resource with tracking labels", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			obj := makeTrackedCM(cmName, resourceID, releaseUID)
			Expect(r.applyResources(ctx, k8sClient, []*unstructured.Unstructured{obj})).To(Succeed())

			existing := &unstructured.Unstructured{}
			existing.SetGroupVersionKind(configMapGVK)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: testNS}, existing)).To(Succeed())
			Expect(existing.GetLabels()[labels.LabelKeyRenderedReleaseResourceID]).To(Equal(resourceID))
		})

		It("should be idempotent when applied twice", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			obj := makeTrackedCM(cmName, resourceID, releaseUID)
			Expect(r.applyResources(ctx, k8sClient, []*unstructured.Unstructured{obj})).To(Succeed())
			obj2 := makeTrackedCM(cmName, resourceID, releaseUID)
			Expect(r.applyResources(ctx, k8sClient, []*unstructured.Unstructured{obj2})).To(Succeed())
		})
	})

	Context("when deleting a previously applied resource", func() {
		const cmName = "test-delete-cm"
		const resourceID = "delete-res-1"
		const releaseUID = "delete-uid-1"

		BeforeEach(func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			obj := makeTrackedCM(cmName, resourceID, releaseUID)
			Expect(r.applyResources(ctx, k8sClient, []*unstructured.Unstructured{obj})).To(Succeed())
		})

		AfterEach(func() { deleteCM(cmName) })

		It("should delete the resource", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			obj := makeTrackedCM(cmName, resourceID, releaseUID)
			Expect(r.deleteResources(ctx, k8sClient, []*unstructured.Unstructured{obj})).To(Succeed())

			existing := &unstructured.Unstructured{}
			existing.SetGroupVersionKind(configMapGVK)
			err := k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: testNS}, existing)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})
})

// ─────────────────────────────────────────────────────────────
// listLiveResourcesByGVKs
// ─────────────────────────────────────────────────────────────

var _ = Describe("listLiveResourcesByGVKs", func() {
	const testNS = "default"
	configMapGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}

	makeTrackedCM := func(name, releaseUID string) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(configMapGVK)
		obj.SetName(name)
		obj.SetNamespace(testNS)
		obj.SetLabels(map[string]string{
			labels.LabelKeyManagedBy:          ControllerName,
			labels.LabelKeyRenderedReleaseUID: releaseUID,
		})
		return obj
	}

	deleteCM := func(name string) {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(configMapGVK)
		obj.SetName(name)
		obj.SetNamespace(testNS)
		_ = k8sClient.Delete(ctx, obj)
	}

	makeRelease := func(uid string) *openchoreov1alpha1.RenderedRelease {
		r := &openchoreov1alpha1.RenderedRelease{}
		r.UID = types.UID(uid)
		return r
	}

	Context("when no resources match the label selector", func() {
		It("should return an empty list", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			release := makeRelease("nonexistent-uid-99999")
			result, err := r.listLiveResourcesByGVKs(ctx, k8sClient, release, []schema.GroupVersionKind{configMapGVK})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	Context("when resources with matching labels exist", func() {
		const releaseUID = "list-live-uid-match"
		const cmName = "test-list-cm-match"

		BeforeEach(func() {
			obj := makeTrackedCM(cmName, releaseUID)
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		})
		AfterEach(func() { deleteCM(cmName) })

		It("should find the matching resources", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			release := makeRelease(releaseUID)
			result, err := r.listLiveResourcesByGVKs(ctx, k8sClient, release, []schema.GroupVersionKind{configMapGVK})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0].GetName()).To(Equal(cmName))
		})
	})

	Context("when resources without matching labels exist", func() {
		const cmName = "test-list-cm-nomatch"

		BeforeEach(func() {
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(configMapGVK)
			obj.SetName(cmName)
			obj.SetNamespace(testNS)
			// No tracking labels
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		})
		AfterEach(func() { deleteCM(cmName) })

		It("should exclude resources without matching labels", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			release := makeRelease("uid-for-nomatch-test")
			result, err := r.listLiveResourcesByGVKs(ctx, k8sClient, release, []schema.GroupVersionKind{configMapGVK})
			Expect(err).NotTo(HaveOccurred())
			for _, res := range result {
				Expect(res.GetName()).NotTo(Equal(cmName))
			}
		})
	})

	Context("when an unknown GVK is queried", func() {
		It("should continue without returning an error", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			unknownGVK := schema.GroupVersionKind{Group: "unknown.example.com", Version: "v1", Kind: "NonExistentResource"}
			release := makeRelease("some-uid-unknown-gvk")
			result, err := r.listLiveResourcesByGVKs(ctx, k8sClient, release, []schema.GroupVersionKind{unknownGVK})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	Context("when list fails for non-NoMatch reasons", func() {
		It("should return an error", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel()
			release := makeRelease("uid-list-failure")
			_, err := r.listLiveResourcesByGVKs(cancelledCtx, k8sClient, release, []schema.GroupVersionKind{configMapGVK})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when multiple GVKs are queried", func() {
		const releaseUID = "list-live-uid-multi"
		const cmName = "test-list-cm-multi"

		BeforeEach(func() {
			obj := makeTrackedCM(cmName, releaseUID)
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		})
		AfterEach(func() { deleteCM(cmName) })

		It("should collect resources from each GVK", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			release := makeRelease(releaseUID)
			serviceGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
			result, err := r.listLiveResourcesByGVKs(ctx, k8sClient, release, []schema.GroupVersionKind{configMapGVK, serviceGVK})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0].GetName()).To(Equal(cmName))
		})
	})
})
