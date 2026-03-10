// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterbuildplane_test

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
	"github.com/openchoreo/openchoreo/internal/controller/clusterbuildplane"
)

// testReconciler returns a Reconciler configured for tests (no gateway or client manager).
// ClusterBuildPlane is cluster-scoped, so no namespace is used.
func testReconciler() *clusterbuildplane.Reconciler {
	return &clusterbuildplane.Reconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(100),
	}
}

// newClusterBuildPlane creates a ClusterBuildPlane with the required spec fields.
// As a cluster-scoped resource it has no namespace.
func newClusterBuildPlane(name string) *openchoreov1alpha1.ClusterBuildPlane {
	return &openchoreov1alpha1.ClusterBuildPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterBuildPlaneSpec{
			PlaneID: "test-plane",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					Value: "test-ca-cert",
				},
			},
		},
	}
}

// newClusterBuildPlaneWithFinalizer creates a ClusterBuildPlane with the cleanup finalizer pre-set.
func newClusterBuildPlaneWithFinalizer(name string) *openchoreov1alpha1.ClusterBuildPlane {
	cbp := newClusterBuildPlane(name)
	cbp.Finalizers = []string{clusterbuildplane.ClusterBuildPlaneCleanupFinalizer}
	return cbp
}

// forceDeleteCBP strips the cleanup finalizer from a ClusterBuildPlane and deletes it,
// ensuring cleanup even if a test fails mid-way.
func forceDeleteCBP(ctx context.Context, name string) {
	cbp := &openchoreov1alpha1.ClusterBuildPlane{}
	nn := types.NamespacedName{Name: name}
	if err := k8sClient.Get(ctx, nn, cbp); err != nil {
		return
	}
	if controllerutil.ContainsFinalizer(cbp, clusterbuildplane.ClusterBuildPlaneCleanupFinalizer) {
		controllerutil.RemoveFinalizer(cbp, clusterbuildplane.ClusterBuildPlaneCleanupFinalizer)
		_ = k8sClient.Update(ctx, cbp)
	}
	_ = k8sClient.Delete(ctx, cbp)
}

var _ = Describe("ClusterBuildPlane Controller", func() {

	Context("When reconciling a non-existent ClusterBuildPlane", func() {
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

	Context("When reconciling a newly created ClusterBuildPlane (first reconcile)", func() {
		const cbpName = "cbp-first-reconcile"
		nn := types.NamespacedName{Name: cbpName}

		BeforeEach(func() {
			cbp := newClusterBuildPlane(cbpName)
			Expect(k8sClient.Create(ctx, cbp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCBP(ctx, cbpName)
		})

		It("should add finalizer and return empty result", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// First reconcile returns early after adding finalizer
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			fresh := &openchoreov1alpha1.ClusterBuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fresh, clusterbuildplane.ClusterBuildPlaneCleanupFinalizer)).To(BeTrue())
		})
	})

	Context("When reconciling a ClusterBuildPlane with finalizer already set (second reconcile)", func() {
		const cbpName = "cbp-second-reconcile"
		nn := types.NamespacedName{Name: cbpName}

		BeforeEach(func() {
			cbp := newClusterBuildPlaneWithFinalizer(cbpName)
			Expect(k8sClient.Create(ctx, cbp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCBP(ctx, cbpName)
		})

		It("should set Created condition and return RequeueAfter", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))

			fresh := &openchoreov1alpha1.ClusterBuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("ClusterBuildPlaneCreated"))
		})

		It("should update ObservedGeneration to match the current generation", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			fresh := &openchoreov1alpha1.ClusterBuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})
	})

	Context("When reconciling a ClusterBuildPlane that already has the Created condition (shouldIgnoreReconcile=true)", func() {
		const cbpName = "cbp-already-created"
		nn := types.NamespacedName{Name: cbpName}

		BeforeEach(func() {
			cbp := newClusterBuildPlaneWithFinalizer(cbpName)
			Expect(k8sClient.Create(ctx, cbp)).To(Succeed())

			// Manually set the Created condition so shouldIgnoreReconcile returns true
			Expect(k8sClient.Get(ctx, nn, cbp)).To(Succeed())
			cbp.Status.Conditions = []metav1.Condition{
				clusterbuildplane.NewClusterBuildPlaneCreatedCondition(cbp.Generation),
			}
			cbp.Status.ObservedGeneration = cbp.Generation
			Expect(k8sClient.Status().Update(ctx, cbp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCBP(ctx, cbpName)
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

			fresh := &openchoreov1alpha1.ClusterBuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("When deleting a ClusterBuildPlane (finalization)", func() {
		const cbpName = "cbp-finalize"
		nn := types.NamespacedName{Name: cbpName}

		BeforeEach(func() {
			cbp := newClusterBuildPlaneWithFinalizer(cbpName)
			Expect(k8sClient.Create(ctx, cbp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCBP(ctx, cbpName)
		})

		It("should remove finalizer and delete ClusterBuildPlane on deletion reconcile", func() {
			cbp := &openchoreov1alpha1.ClusterBuildPlane{}
			Expect(k8sClient.Get(ctx, nn, cbp)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cbp)).To(Succeed())

			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// ClusterBuildPlane should be fully deleted (finalizer removed → API server garbage collects)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.ClusterBuildPlane{})
				return apierrors.IsNotFound(err)
			}, "10s", "500ms").Should(BeTrue())
		})
	})

	Context("When deleting a ClusterBuildPlane without a finalizer", func() {
		const cbpName = "cbp-no-finalizer"
		nn := types.NamespacedName{Name: cbpName}

		It("should return empty result if finalizer is not present", func() {
			cbp := newClusterBuildPlane(cbpName)
			Expect(k8sClient.Create(ctx, cbp)).To(Succeed())

			// Delete immediately (no finalizer, so it gets deleted by API server)
			Expect(k8sClient.Delete(ctx, cbp)).To(Succeed())

			// Reconcile the now-deleted resource — should return not-found path
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("Status persistence via status subresource", func() {
		const cbpName = "cbp-status-persist"
		nn := types.NamespacedName{Name: cbpName}

		BeforeEach(func() {
			cbp := newClusterBuildPlaneWithFinalizer(cbpName)
			Expect(k8sClient.Create(ctx, cbp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCBP(ctx, cbpName)
		})

		It("should persist status conditions after reconcile", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch and verify status was persisted
			fresh := &openchoreov1alpha1.ClusterBuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.Conditions).NotTo(BeEmpty())
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})

		It("should persist manually updated AgentConnection status fields", func() {
			cbp := &openchoreov1alpha1.ClusterBuildPlane{}
			Expect(k8sClient.Get(ctx, nn, cbp)).To(Succeed())

			cbp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{
				Connected:       true,
				ConnectedAgents: 2,
				Message:         "2 agents connected (HA mode)",
			}
			Expect(k8sClient.Status().Update(ctx, cbp)).To(Succeed())

			// Re-fetch and verify
			fresh := &openchoreov1alpha1.ClusterBuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.AgentConnection).NotTo(BeNil())
			Expect(fresh.Status.AgentConnection.Connected).To(BeTrue())
			Expect(fresh.Status.AgentConnection.ConnectedAgents).To(BeEquivalentTo(2))
			Expect(fresh.Status.AgentConnection.Message).To(Equal("2 agents connected (HA mode)"))
		})
	})

	Context("Cluster-scoped resource characteristics", func() {
		const cbpName = "cbp-cluster-scoped"
		nn := types.NamespacedName{Name: cbpName}

		BeforeEach(func() {
			cbp := newClusterBuildPlaneWithFinalizer(cbpName)
			Expect(k8sClient.Create(ctx, cbp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCBP(ctx, cbpName)
		})

		It("should have no namespace after creation", func() {
			fresh := &openchoreov1alpha1.ClusterBuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Namespace).To(BeEmpty())
		})

		It("should set ObservedGeneration after reconcile", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			fresh := &openchoreov1alpha1.ClusterBuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.ObservedGeneration).To(BeNumerically(">", 0))
		})
	})
})
