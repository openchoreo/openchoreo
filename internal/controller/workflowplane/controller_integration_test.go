// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

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

// newWorkflowPlane creates a WorkflowPlane object with the required spec fields.
func newWorkflowPlane(name, namespace string) *openchoreov1alpha1.WorkflowPlane {
	return &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					Value: "test-ca-cert",
				},
			},
		},
	}
}

// newWorkflowPlaneWithFinalizer creates a WorkflowPlane with the cleanup finalizer pre-set.
func newWorkflowPlaneWithFinalizer(name string) *openchoreov1alpha1.WorkflowPlane {
	wp := newWorkflowPlane(name, "default")
	wp.Finalizers = []string{WorkflowPlaneCleanupFinalizer}
	return wp
}

// forceDeleteWP strips the cleanup finalizer from a WorkflowPlane and deletes it,
// ensuring cleanup even if a test fails mid-way.
func forceDeleteWP(ctx context.Context, nn types.NamespacedName) {
	wp := &openchoreov1alpha1.WorkflowPlane{}
	if err := k8sClient.Get(ctx, nn, wp); err != nil {
		return
	}
	if controllerutil.ContainsFinalizer(wp, WorkflowPlaneCleanupFinalizer) {
		controllerutil.RemoveFinalizer(wp, WorkflowPlaneCleanupFinalizer)
		_ = k8sClient.Update(ctx, wp)
	}
	_ = k8sClient.Delete(ctx, wp)
}

var _ = Describe("WorkflowPlane Controller", func() {

	Context("When reconciling a non-existent WorkflowPlane", func() {
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

	Context("When reconciling a newly created WorkflowPlane (first reconcile)", func() {
		const wpName = "wp-first-reconcile"
		nn := types.NamespacedName{Name: wpName, Namespace: "default"}

		BeforeEach(func() {
			wp := newWorkflowPlane(wpName, "default")
			Expect(k8sClient.Create(ctx, wp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteWP(ctx, nn)
		})

		It("should add finalizer and return empty result", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// First reconcile returns early after adding finalizer
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			fresh := &openchoreov1alpha1.WorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fresh, WorkflowPlaneCleanupFinalizer)).To(BeTrue())
		})
	})

	Context("When reconciling a WorkflowPlane with finalizer already set (second reconcile)", func() {
		const wpName = "wp-second-reconcile"
		nn := types.NamespacedName{Name: wpName, Namespace: "default"}

		BeforeEach(func() {
			wp := newWorkflowPlaneWithFinalizer(wpName)
			Expect(k8sClient.Create(ctx, wp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteWP(ctx, nn)
		})

		It("should set Created condition and return RequeueAfter", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))

			fresh := &openchoreov1alpha1.WorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("WorkflowPlaneCreated"))
		})

		It("should update ObservedGeneration to match the current generation", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			fresh := &openchoreov1alpha1.WorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})
	})

	Context("When reconciling a WorkflowPlane that already has the Created condition (shouldIgnoreReconcile=true)", func() {
		const wpName = "wp-already-created"
		nn := types.NamespacedName{Name: wpName, Namespace: "default"}

		BeforeEach(func() {
			wp := newWorkflowPlaneWithFinalizer(wpName)
			Expect(k8sClient.Create(ctx, wp)).To(Succeed())

			// Manually set the Created condition so shouldIgnoreReconcile returns true
			Expect(k8sClient.Get(ctx, nn, wp)).To(Succeed())
			wp.Status.Conditions = []metav1.Condition{
				NewWorkflowPlaneCreatedCondition(wp.Generation),
			}
			wp.Status.ObservedGeneration = wp.Generation
			Expect(k8sClient.Status().Update(ctx, wp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteWP(ctx, nn)
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

			fresh := &openchoreov1alpha1.WorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("When deleting a WorkflowPlane (finalization)", func() {
		const wpName = "wp-finalize"
		nn := types.NamespacedName{Name: wpName, Namespace: "default"}

		BeforeEach(func() {
			wp := newWorkflowPlaneWithFinalizer(wpName)
			Expect(k8sClient.Create(ctx, wp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteWP(ctx, nn)
		})

		It("should remove finalizer and delete WorkflowPlane on deletion reconcile", func() {
			wp := &openchoreov1alpha1.WorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, wp)).To(Succeed())
			Expect(k8sClient.Delete(ctx, wp)).To(Succeed())

			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// WorkflowPlane should be fully deleted (finalizer removed → API server garbage collects)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.WorkflowPlane{})
				return apierrors.IsNotFound(err)
			}, "10s", "500ms").Should(BeTrue())
		})
	})

	Context("When deleting a WorkflowPlane without a finalizer", func() {
		const wpName = "wp-no-finalizer"
		nn := types.NamespacedName{Name: wpName, Namespace: "default"}

		It("should return empty result if finalizer is not present", func() {
			wp := newWorkflowPlane(wpName, "default")
			Expect(k8sClient.Create(ctx, wp)).To(Succeed())

			// Delete immediately (no finalizer, so it gets deleted by API server)
			Expect(k8sClient.Delete(ctx, wp)).To(Succeed())

			// Reconcile the now-deleted resource — should return not-found path
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("Status persistence via status subresource", func() {
		const wpName = "wp-status-persist"
		nn := types.NamespacedName{Name: wpName, Namespace: "default"}

		BeforeEach(func() {
			wp := newWorkflowPlaneWithFinalizer(wpName)
			Expect(k8sClient.Create(ctx, wp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteWP(ctx, nn)
		})

		It("should persist status conditions after reconcile", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch and verify status was persisted
			fresh := &openchoreov1alpha1.WorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.Conditions).NotTo(BeEmpty())
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})

		It("should persist manually updated status fields", func() {
			wp := &openchoreov1alpha1.WorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, wp)).To(Succeed())

			wp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{
				Connected:       true,
				ConnectedAgents: 2,
				Message:         "2 agents connected (HA mode)",
			}
			Expect(k8sClient.Status().Update(ctx, wp)).To(Succeed())

			// Re-fetch and verify
			fresh := &openchoreov1alpha1.WorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.AgentConnection).NotTo(BeNil())
			Expect(fresh.Status.AgentConnection.Connected).To(BeTrue())
			Expect(fresh.Status.AgentConnection.ConnectedAgents).To(BeEquivalentTo(2))
			Expect(fresh.Status.AgentConnection.Message).To(Equal("2 agents connected (HA mode)"))
		})
	})
})
