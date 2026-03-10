// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

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
)

// testReconciler returns a Reconciler configured for tests (no gateway or client manager).
func testReconciler() *Reconciler {
	return &Reconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(100),
	}
}

// newBuildPlane creates a BuildPlane object with the required spec fields.
func newBuildPlane(name, namespace string) *openchoreov1alpha1.BuildPlane {
	return &openchoreov1alpha1.BuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.BuildPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					Value: "test-ca-cert",
				},
			},
		},
	}
}

// newBuildPlaneWithFinalizer creates a BuildPlane with the cleanup finalizer pre-set.
func newBuildPlaneWithFinalizer(name string) *openchoreov1alpha1.BuildPlane {
	bp := newBuildPlane(name, "default")
	bp.Finalizers = []string{BuildPlaneCleanupFinalizer}
	return bp
}

// forceDeleteBP strips the cleanup finalizer from a BuildPlane and deletes it,
// ensuring cleanup even if a test fails mid-way.
func forceDeleteBP(ctx context.Context, nn types.NamespacedName) {
	bp := &openchoreov1alpha1.BuildPlane{}
	if err := k8sClient.Get(ctx, nn, bp); err != nil {
		return
	}
	if controllerutil.ContainsFinalizer(bp, BuildPlaneCleanupFinalizer) {
		controllerutil.RemoveFinalizer(bp, BuildPlaneCleanupFinalizer)
		_ = k8sClient.Update(ctx, bp)
	}
	_ = k8sClient.Delete(ctx, bp)
}

var _ = Describe("BuildPlane Controller", func() {

	Context("When reconciling a non-existent BuildPlane", func() {
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

	Context("When reconciling a newly created BuildPlane (first reconcile)", func() {
		const bpName = "bp-first-reconcile"
		nn := types.NamespacedName{Name: bpName, Namespace: "default"}

		BeforeEach(func() {
			bp := newBuildPlane(bpName, "default")
			Expect(k8sClient.Create(ctx, bp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteBP(ctx, nn)
		})

		It("should add finalizer and return empty result", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// First reconcile returns early after adding finalizer
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			fresh := &openchoreov1alpha1.BuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fresh, BuildPlaneCleanupFinalizer)).To(BeTrue())
		})
	})

	Context("When reconciling a BuildPlane with finalizer already set (second reconcile)", func() {
		const bpName = "bp-second-reconcile"
		nn := types.NamespacedName{Name: bpName, Namespace: "default"}

		BeforeEach(func() {
			bp := newBuildPlaneWithFinalizer(bpName)
			Expect(k8sClient.Create(ctx, bp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteBP(ctx, nn)
		})

		It("should set Created condition and return RequeueAfter", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))

			fresh := &openchoreov1alpha1.BuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("BuildPlaneCreated"))
		})

		It("should update ObservedGeneration to match the current generation", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			fresh := &openchoreov1alpha1.BuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})
	})

	Context("When reconciling a BuildPlane that already has the Created condition (shouldIgnoreReconcile=true)", func() {
		const bpName = "bp-already-created"
		nn := types.NamespacedName{Name: bpName, Namespace: "default"}

		BeforeEach(func() {
			bp := newBuildPlaneWithFinalizer(bpName)
			Expect(k8sClient.Create(ctx, bp)).To(Succeed())

			// Manually set the Created condition so shouldIgnoreReconcile returns true
			Expect(k8sClient.Get(ctx, nn, bp)).To(Succeed())
			bp.Status.Conditions = []metav1.Condition{
				NewBuildPlaneCreatedCondition(bp.Generation),
			}
			bp.Status.ObservedGeneration = bp.Generation
			Expect(k8sClient.Status().Update(ctx, bp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteBP(ctx, nn)
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

			fresh := &openchoreov1alpha1.BuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("When deleting a BuildPlane (finalization)", func() {
		const bpName = "bp-finalize"
		nn := types.NamespacedName{Name: bpName, Namespace: "default"}

		BeforeEach(func() {
			bp := newBuildPlaneWithFinalizer(bpName)
			Expect(k8sClient.Create(ctx, bp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteBP(ctx, nn)
		})

		It("should remove finalizer and delete BuildPlane on deletion reconcile", func() {
			bp := &openchoreov1alpha1.BuildPlane{}
			Expect(k8sClient.Get(ctx, nn, bp)).To(Succeed())
			Expect(k8sClient.Delete(ctx, bp)).To(Succeed())

			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// BuildPlane should be fully deleted (finalizer removed → API server garbage collects)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.BuildPlane{})
				return apierrors.IsNotFound(err)
			}, "10s", "500ms").Should(BeTrue())
		})
	})

	Context("When deleting a BuildPlane without a finalizer", func() {
		const bpName = "bp-no-finalizer"
		nn := types.NamespacedName{Name: bpName, Namespace: "default"}

		It("should return empty result if finalizer is not present", func() {
			bp := newBuildPlane(bpName, "default")
			Expect(k8sClient.Create(ctx, bp)).To(Succeed())

			// Delete immediately (no finalizer, so it gets deleted by API server)
			Expect(k8sClient.Delete(ctx, bp)).To(Succeed())

			// Reconcile the now-deleted resource — should return not-found path
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("Status persistence via status subresource", func() {
		const bpName = "bp-status-persist"
		nn := types.NamespacedName{Name: bpName, Namespace: "default"}

		BeforeEach(func() {
			bp := newBuildPlaneWithFinalizer(bpName)
			Expect(k8sClient.Create(ctx, bp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteBP(ctx, nn)
		})

		It("should persist status conditions after reconcile", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch and verify status was persisted
			fresh := &openchoreov1alpha1.BuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.Conditions).NotTo(BeEmpty())
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})

		It("should persist manually updated status fields", func() {
			bp := &openchoreov1alpha1.BuildPlane{}
			Expect(k8sClient.Get(ctx, nn, bp)).To(Succeed())

			bp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{
				Connected:       true,
				ConnectedAgents: 2,
				Message:         "2 agents connected (HA mode)",
			}
			Expect(k8sClient.Status().Update(ctx, bp)).To(Succeed())

			// Re-fetch and verify
			fresh := &openchoreov1alpha1.BuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.AgentConnection).NotTo(BeNil())
			Expect(fresh.Status.AgentConnection.Connected).To(BeTrue())
			Expect(fresh.Status.AgentConnection.ConnectedAgents).To(BeEquivalentTo(2))
			Expect(fresh.Status.AgentConnection.Message).To(Equal("2 agents connected (HA mode)"))
		})
	})
})
