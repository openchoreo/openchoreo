// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// forceDelete removes the DataPlaneCleanupFinalizer from a Release and deletes it.
// Safe to call even if the resource does not exist.
func forceDelete(ctx context.Context, nn types.NamespacedName) {
	r := &openchoreov1alpha1.Release{}
	if err := k8sClient.Get(ctx, nn, r); err != nil {
		return
	}
	if controllerutil.ContainsFinalizer(r, DataPlaneCleanupFinalizer) {
		controllerutil.RemoveFinalizer(r, DataPlaneCleanupFinalizer)
		_ = k8sClient.Update(ctx, r)
	}
	_ = k8sClient.Delete(ctx, r)
}

// makeMinimalRelease returns a Release with the minimum required spec fields.
func makeMinimalRelease(name, envName string) *openchoreov1alpha1.Release {
	return &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: openchoreov1alpha1.ReleaseSpec{
			Owner: openchoreov1alpha1.ReleaseOwner{
				ProjectName:   "test-project",
				ComponentName: "test-component",
			},
			EnvironmentName: envName,
		},
	}
}

var _ = Describe("Release Controller", func() {

	// ─────────────────────────────────────────────────────────────
	// Non-existent resource
	// ─────────────────────────────────────────────────────────────

	Context("when the Release resource does not exist", func() {
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

	Context("when a new Release is created", func() {
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

			got := &openchoreov1alpha1.Release{}
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

	Context("when a Release with a finalizer is being deleted", func() {
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

			got := &openchoreov1alpha1.Release{}
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

			got := &openchoreov1alpha1.Release{}
			Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(got, DataPlaneCleanupFinalizer)).To(BeTrue())
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Deleted release — no finalizer (immediate deletion)
	// ─────────────────────────────────────────────────────────────

	Context("when a Release without a finalizer is deleted", func() {
		It("should return no error (resource already gone)", func() {
			const releaseName = "release-no-finalizer"
			nn := types.NamespacedName{Name: releaseName, Namespace: "default"}

			release := makeMinimalRelease(releaseName, "my-env")
			// Explicitly no Finalizers
			Expect(k8sClient.Create(ctx, release)).To(Succeed())
			Expect(k8sClient.Delete(ctx, release)).To(Succeed())

			// Without a finalizer the API server removes the object immediately
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.Release{})
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

	Context("when a Release has a DeletionTimestamp but no finalizer", func() {
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
			fetched := &openchoreov1alpha1.Release{}
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
			release := &openchoreov1alpha1.Release{}
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
			fetched := &openchoreov1alpha1.Release{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, "TestCondition")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("TestReason"))
		})

		It("should persist resource inventory in status", func() {
			By("fetching release")
			release := &openchoreov1alpha1.Release{}
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
			fetched := &openchoreov1alpha1.Release{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Resources).To(HaveLen(1))
			Expect(fetched.Status.Resources[0].ID).To(Equal("res-1"))
			Expect(fetched.Status.Resources[0].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusHealthy))
		})
	})
})
