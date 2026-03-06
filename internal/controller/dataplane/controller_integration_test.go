// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// testReconciler returns a Reconciler configured for tests (no gateway or client manager).
func testReconciler() *Reconciler {
	return &Reconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(100),
	}
}

// forceDeleteDP strips the cleanup finalizer from a DataPlane and deletes it,
// ensuring cleanup even if a test fails mid-way.
func forceDeleteDP(ctx context.Context, nn types.NamespacedName) {
	dp := &openchoreov1alpha1.DataPlane{}
	if err := k8sClient.Get(ctx, nn, dp); err != nil {
		return
	}
	if controllerutil.ContainsFinalizer(dp, DataPlaneCleanupFinalizer) {
		controllerutil.RemoveFinalizer(dp, DataPlaneCleanupFinalizer)
		_ = k8sClient.Update(ctx, dp)
	}
	_ = k8sClient.Delete(ctx, dp)
}

// forceDeleteEnv deletes an Environment, ignoring not-found errors.
func forceDeleteEnv(ctx context.Context, nn types.NamespacedName) {
	env := &openchoreov1alpha1.Environment{}
	if err := k8sClient.Get(ctx, nn, env); err != nil {
		return
	}
	_ = k8sClient.Delete(ctx, env)
}

var _ = Describe("DataPlane Controller", func() {

	Context("When reconciling a non-existent DataPlane", func() {
		It("should return no error and no requeue", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "does-not-exist", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("When reconciling a newly created DataPlane (first reconcile)", func() {
		const dpName = "dp-first-reconcile"
		nn := types.NamespacedName{Name: dpName, Namespace: "default"}

		BeforeEach(func() {
			dp := &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dpName,
					Namespace: "default",
				},
			}
			Expect(k8sClient.Create(ctx, dp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteDP(ctx, nn)
		})

		It("should add finalizer and return empty result", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// First reconcile returns early after adding finalizer
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			fresh := &openchoreov1alpha1.DataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fresh, DataPlaneCleanupFinalizer)).To(BeTrue())
		})
	})

	Context("When reconciling a DataPlane with finalizer already set (second reconcile)", func() {
		const dpName = "dp-second-reconcile"
		nn := types.NamespacedName{Name: dpName, Namespace: "default"}

		BeforeEach(func() {
			dp := &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:       dpName,
					Namespace:  "default",
					Finalizers: []string{DataPlaneCleanupFinalizer},
				},
			}
			Expect(k8sClient.Create(ctx, dp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteDP(ctx, nn)
		})

		It("should set Created condition and return RequeueAfter", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))

			fresh := &openchoreov1alpha1.DataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(ConditionCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonDataPlaneCreated)))
		})

		It("should update ObservedGeneration to match the current generation", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			fresh := &openchoreov1alpha1.DataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})
	})

	Context("When reconciling a DataPlane that already has the Created condition (shouldIgnoreReconcile=true)", func() {
		const dpName = "dp-already-created"
		nn := types.NamespacedName{Name: dpName, Namespace: "default"}

		BeforeEach(func() {
			dp := &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:       dpName,
					Namespace:  "default",
					Finalizers: []string{DataPlaneCleanupFinalizer},
				},
			}
			Expect(k8sClient.Create(ctx, dp)).To(Succeed())

			// Manually set the Created condition so shouldIgnoreReconcile returns true
			Expect(k8sClient.Get(ctx, nn, dp)).To(Succeed())
			dp.Status.Conditions = []metav1.Condition{
				NewDataPlaneCreatedCondition(dp.Generation),
			}
			dp.Status.ObservedGeneration = dp.Generation
			Expect(k8sClient.Status().Update(ctx, dp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteDP(ctx, nn)
		})

		It("should return RequeueAfter without error", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))
		})

		It("should not overwrite the Created condition", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			fresh := &openchoreov1alpha1.DataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(ConditionCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("When deleting a DataPlane with no owned Environments (finalization)", func() {
		const dpName = "dp-finalize-no-env"
		nn := types.NamespacedName{Name: dpName, Namespace: "default"}

		BeforeEach(func() {
			dp := &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:       dpName,
					Namespace:  "default",
					Finalizers: []string{DataPlaneCleanupFinalizer},
				},
			}
			Expect(k8sClient.Create(ctx, dp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteDP(ctx, nn)
		})

		It("should set Finalizing condition on first deletion reconcile", func() {
			dp := &openchoreov1alpha1.DataPlane{}
			Expect(k8sClient.Get(ctx, nn, dp)).To(Succeed())
			Expect(k8sClient.Delete(ctx, dp)).To(Succeed())

			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			fresh := &openchoreov1alpha1.DataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(ConditionFinalizing))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonDataplaneFinalizing)))
		})

		It("should remove finalizer and delete DataPlane on second deletion reconcile", func() {
			dp := &openchoreov1alpha1.DataPlane{}
			Expect(k8sClient.Get(ctx, nn, dp)).To(Succeed())
			Expect(k8sClient.Delete(ctx, dp)).To(Succeed())

			r := testReconciler()

			// First reconcile: sets Finalizing condition
			By("first deletion reconcile sets Finalizing condition")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: Finalizing condition already set → deleteEnvironmentsAndWait → no envs → remove finalizer
			By("second deletion reconcile removes finalizer")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// DataPlane should be fully deleted (finalizer removed → API server garbage collects)
			By("DataPlane should be deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.DataPlane{})
				return apierrors.IsNotFound(err)
			}, "10s", "500ms").Should(BeTrue())
		})
	})

	Context("When deleting a DataPlane that has owned Environments (finalization with children)", func() {
		const dpName = "dp-finalize-with-env"
		const envName = "env-owned-by-dp"
		const nsLabelValue = "test-finalize-env-ns"
		dpNN := types.NamespacedName{Name: dpName, Namespace: "default"}
		envNN := types.NamespacedName{Name: envName, Namespace: "default"}

		BeforeEach(func() {
			dp := &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dpName,
					Namespace: "default",
					Labels: map[string]string{
						labels.LabelKeyNamespaceName: nsLabelValue,
					},
					Finalizers: []string{DataPlaneCleanupFinalizer},
				},
			}
			Expect(k8sClient.Create(ctx, dp)).To(Succeed())

			env := &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      envName,
					Namespace: "default",
					Labels: map[string]string{
						labels.LabelKeyNamespaceName: nsLabelValue,
					},
				},
				Spec: openchoreov1alpha1.EnvironmentSpec{
					DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
						Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
						Name: dpName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, env)).To(Succeed())

			// Wait for the Environment to appear in the cache (field index requires cache)
			Eventually(func() error {
				return k8sClient.Get(ctx, envNN, &openchoreov1alpha1.Environment{})
			}, "5s", "100ms").Should(Succeed())
		})

		AfterEach(func() {
			forceDeleteEnv(ctx, envNN)
			forceDeleteDP(ctx, dpNN)
		})

		It("should trigger Environment deletion and eventually delete the DataPlane", func() {
			dp := &openchoreov1alpha1.DataPlane{}
			Expect(k8sClient.Get(ctx, dpNN, dp)).To(Succeed())
			Expect(k8sClient.Delete(ctx, dp)).To(Succeed())

			r := testReconciler()

			// Keep reconciling until the DataPlane is gone.
			// Sequence:
			//   reconcile #1 → sets Finalizing condition
			//   reconcile #2 → triggers Environment deletion (no finalizer on env → API server deletes it)
			//   reconcile #3+ → environment gone from cache → removes DataPlane finalizer → DataPlane deleted
			Eventually(func() bool {
				_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: dpNN})
				Expect(err).NotTo(HaveOccurred())
				return apierrors.IsNotFound(k8sClient.Get(ctx, dpNN, &openchoreov1alpha1.DataPlane{}))
			}, "15s", "500ms").Should(BeTrue())

			// The Environment should also be gone
			Eventually(func() bool {
				return apierrors.IsNotFound(k8sClient.Get(ctx, envNN, &openchoreov1alpha1.Environment{}))
			}, "10s", "500ms").Should(BeTrue())
		})
	})
})
