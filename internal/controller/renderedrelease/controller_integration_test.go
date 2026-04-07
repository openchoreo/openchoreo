// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
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

// makeEnvironment returns an Environment referencing a ClusterDataPlane.
func makeEnvironment(name, namespace, cdpName string) *openchoreov1alpha1.Environment {
	return &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
				Name: cdpName,
			},
		},
	}
}

// testReconcilerWithDPClient creates a Reconciler with a pre-seeded KubeMultiClientManager
// so that getDPClient resolves successfully without a real data-plane cluster.
func testReconcilerWithDPClient(dpClient client.Client, planeID, cdpName string) *Reconciler {
	mgr := kubernetesClient.NewManager()
	key := "v2/clusterdataplane/" + planeID + "/" + cdpName
	//nolint:errcheck // test helper; panics are acceptable
	mgr.GetOrAddClient(key, func() (client.Client, error) {
		return dpClient, nil
	})
	return &Reconciler{
		Client:       k8sClient,
		Scheme:       k8sClient.Scheme(),
		K8sClientMgr: mgr,
		GatewayURL:   "https://gateway.test:443",
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
			Expect(k8sClient.Create(ctx, makeEnvironment(envName, testNs, cdpName))).To(Succeed())

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
			Expect(k8sClient.Create(ctx, makeEnvironment(envName, testNs, cdpName))).To(Succeed())

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
