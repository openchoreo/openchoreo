// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

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

// newClusterDataPlane creates a ClusterDataPlane object with the required spec fields.
// ClusterDataPlane is cluster-scoped so no namespace is needed.
func newClusterDataPlane(name string) *openchoreov1alpha1.ClusterDataPlane {
	return &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID: "test-plane-" + name,
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					Value: "test-ca-cert",
				},
			},
		},
	}
}

// newClusterDataPlaneWithFinalizer creates a ClusterDataPlane with the cleanup finalizer pre-set.
func newClusterDataPlaneWithFinalizer(name string) *openchoreov1alpha1.ClusterDataPlane {
	cdp := newClusterDataPlane(name)
	cdp.Finalizers = []string{ClusterDataPlaneCleanupFinalizer}
	return cdp
}

// forceDeleteCDP strips the cleanup finalizer from a ClusterDataPlane and deletes it,
// ensuring cleanup even if a test fails mid-way.
func forceDeleteCDP(ctx context.Context, nn types.NamespacedName) {
	cdp := &openchoreov1alpha1.ClusterDataPlane{}
	if err := k8sClient.Get(ctx, nn, cdp); err != nil {
		return
	}
	if controllerutil.ContainsFinalizer(cdp, ClusterDataPlaneCleanupFinalizer) {
		controllerutil.RemoveFinalizer(cdp, ClusterDataPlaneCleanupFinalizer)
		_ = k8sClient.Update(ctx, cdp)
	}
	_ = k8sClient.Delete(ctx, cdp)
}

var _ = Describe("ClusterDataPlane Controller", func() {

	Context("When reconciling a non-existent ClusterDataPlane", func() {
		It("should return no error and no requeue", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "does-not-exist"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("When reconciling a newly created ClusterDataPlane (first reconcile)", func() {
		const cdpName = "cdp-first-reconcile"
		nn := types.NamespacedName{Name: cdpName}

		BeforeEach(func() {
			cdp := newClusterDataPlane(cdpName)
			Expect(k8sClient.Create(ctx, cdp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCDP(ctx, nn)
		})

		It("should add finalizer and return empty result", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// First reconcile returns early after adding finalizer
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			fresh := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fresh, ClusterDataPlaneCleanupFinalizer)).To(BeTrue())
		})
	})

	Context("When reconciling a ClusterDataPlane with finalizer already set (second reconcile)", func() {
		const cdpName = "cdp-second-reconcile"
		nn := types.NamespacedName{Name: cdpName}

		BeforeEach(func() {
			cdp := newClusterDataPlaneWithFinalizer(cdpName)
			Expect(k8sClient.Create(ctx, cdp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCDP(ctx, nn)
		})

		It("should set Created condition and return RequeueAfter", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))

			fresh := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("ClusterDataPlaneCreated"))
		})

		It("should update ObservedGeneration to match the current generation", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			fresh := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})

		It("should emit a ReconcileComplete event", func() {
			fakeRecorder := record.NewFakeRecorder(10)
			r := &Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: fakeRecorder,
			}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("ReconcileComplete")))
		})
	})

	Context("When reconciling a ClusterDataPlane that already has the Created condition (shouldIgnoreReconcile=true)", func() {
		const cdpName = "cdp-already-created"
		nn := types.NamespacedName{Name: cdpName}

		BeforeEach(func() {
			cdp := newClusterDataPlaneWithFinalizer(cdpName)
			Expect(k8sClient.Create(ctx, cdp)).To(Succeed())

			// Manually set the Created condition so shouldIgnoreReconcile returns true
			Expect(k8sClient.Get(ctx, nn, cdp)).To(Succeed())
			cdp.Status.Conditions = []metav1.Condition{
				NewClusterDataPlaneCreatedCondition(cdp.Generation),
			}
			cdp.Status.ObservedGeneration = cdp.Generation
			Expect(k8sClient.Status().Update(ctx, cdp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCDP(ctx, nn)
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

			fresh := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("When deleting a ClusterDataPlane (finalization)", func() {
		const cdpName = "cdp-finalize"
		nn := types.NamespacedName{Name: cdpName}

		BeforeEach(func() {
			cdp := newClusterDataPlaneWithFinalizer(cdpName)
			Expect(k8sClient.Create(ctx, cdp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCDP(ctx, nn)
		})

		It("should remove finalizer and delete ClusterDataPlane on deletion reconcile", func() {
			cdp := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, cdp)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cdp)).To(Succeed())

			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// ClusterDataPlane should be fully deleted (finalizer removed → API server garbage collects)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.ClusterDataPlane{})
				return apierrors.IsNotFound(err)
			}, "10s", "500ms").Should(BeTrue())
		})
	})

	Context("When deleting a ClusterDataPlane without a finalizer", func() {
		const cdpName = "cdp-no-finalizer"
		nn := types.NamespacedName{Name: cdpName}

		It("should return not-found result when resource is already gone", func() {
			cdp := newClusterDataPlane(cdpName)
			Expect(k8sClient.Create(ctx, cdp)).To(Succeed())

			// Delete immediately — no finalizer so API server removes it
			Expect(k8sClient.Delete(ctx, cdp)).To(Succeed())

			// Reconcile the now-deleted resource — should return not-found path
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("Status persistence via status subresource", func() {
		const cdpName = "cdp-status-persist"
		nn := types.NamespacedName{Name: cdpName}

		BeforeEach(func() {
			cdp := newClusterDataPlaneWithFinalizer(cdpName)
			Expect(k8sClient.Create(ctx, cdp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCDP(ctx, nn)
		})

		It("should persist status conditions after reconcile", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch and verify status was persisted
			fresh := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.Conditions).NotTo(BeEmpty())
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})

		It("should persist manually updated AgentConnection status fields", func() {
			cdp := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, cdp)).To(Succeed())

			cdp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{
				Connected:       true,
				ConnectedAgents: 2,
				Message:         "2 agents connected (HA mode)",
			}
			Expect(k8sClient.Status().Update(ctx, cdp)).To(Succeed())

			// Re-fetch and verify
			fresh := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.AgentConnection).NotTo(BeNil())
			Expect(fresh.Status.AgentConnection.Connected).To(BeTrue())
			Expect(fresh.Status.AgentConnection.ConnectedAgents).To(BeEquivalentTo(2))
			Expect(fresh.Status.AgentConnection.Message).To(Equal("2 agents connected (HA mode)"))
		})

		It("should persist single-agent AgentConnection status fields", func() {
			cdp := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, cdp)).To(Succeed())

			cdp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{
				Connected:       true,
				ConnectedAgents: 1,
				Message:         "1 agent connected",
			}
			Expect(k8sClient.Status().Update(ctx, cdp)).To(Succeed())

			fresh := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.AgentConnection.ConnectedAgents).To(BeEquivalentTo(1))
			Expect(fresh.Status.AgentConnection.Message).To(Equal("1 agent connected"))
		})
	})

	Context("Full lifecycle: create, reconcile twice, delete, and reconcile delete", func() {
		const cdpName = "cdp-lifecycle"
		nn := types.NamespacedName{Name: cdpName}

		It("should complete the full resource lifecycle without errors", func() {
			By("Creating the ClusterDataPlane resource")
			cdp := newClusterDataPlane(cdpName)
			Expect(k8sClient.Create(ctx, cdp)).To(Succeed())

			r := testReconciler()

			By("First reconcile — adds finalizer")
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			fresh := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fresh, ClusterDataPlaneCleanupFinalizer)).To(BeTrue())

			By("Second reconcile — sets Created condition")
			result, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))

			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			By("Third reconcile — shouldIgnoreReconcile=true, still returns RequeueAfter")
			result, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))

			By("Deleting the ClusterDataPlane resource")
			Expect(k8sClient.Delete(ctx, fresh)).To(Succeed())

			Eventually(func() bool {
				updatedCDP := &openchoreov1alpha1.ClusterDataPlane{}
				err := k8sClient.Get(ctx, nn, updatedCDP)
				if err != nil {
					return false
				}
				return !updatedCDP.DeletionTimestamp.IsZero()
			}, "10s", "500ms").Should(BeTrue())

			By("Reconciling deletion — removes finalizer")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the ClusterDataPlane is fully deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.ClusterDataPlane{})
				return apierrors.IsNotFound(err)
			}, "10s", "500ms").Should(BeTrue())
		})
	})

	Context("Multiple reconciles on already-created resource (idempotency)", func() {
		const cdpName = "cdp-idempotent"
		nn := types.NamespacedName{Name: cdpName}

		BeforeEach(func() {
			cdp := newClusterDataPlaneWithFinalizer(cdpName)
			Expect(k8sClient.Create(ctx, cdp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCDP(ctx, nn)
		})

		It("should produce the same conditions regardless of how many times it is reconciled", func() {
			r := testReconciler()

			// First reconcile sets the Created condition
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			fresh := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			condAfterFirst := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(condAfterFirst).NotTo(BeNil())

			// Second reconcile should keep Created condition stable
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			condAfterSecond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(condAfterSecond).NotTo(BeNil())
			Expect(condAfterSecond.Status).To(Equal(metav1.ConditionTrue))
			Expect(fresh.Status.Conditions).To(HaveLen(1))
		})
	})
})
