// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane_test

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
	"github.com/openchoreo/openchoreo/internal/controller/clusterobservabilityplane"
)

// testReconciler returns a Reconciler configured for tests (no gateway or client manager).
// ClusterObservabilityPlane is cluster-scoped, so no namespace is used.
func testReconciler() *clusterobservabilityplane.Reconciler {
	return &clusterobservabilityplane.Reconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(100),
	}
}

// newClusterObservabilityPlane creates a ClusterObservabilityPlane with the required spec fields.
// As a cluster-scoped resource it has no namespace.
func newClusterObservabilityPlane(name string) *openchoreov1alpha1.ClusterObservabilityPlane {
	return &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
			PlaneID: "test-plane",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					Value: "test-ca-cert",
				},
			},
			ObserverURL: "http://observer.example.com",
		},
	}
}

// newClusterObservabilityPlaneWithFinalizer creates a ClusterObservabilityPlane with the cleanup finalizer pre-set.
func newClusterObservabilityPlaneWithFinalizer(name string) *openchoreov1alpha1.ClusterObservabilityPlane {
	cop := newClusterObservabilityPlane(name)
	cop.Finalizers = []string{clusterobservabilityplane.ClusterObservabilityPlaneCleanupFinalizer}
	return cop
}

// forceDeleteCOP strips the cleanup finalizer from a ClusterObservabilityPlane and deletes it,
// ensuring cleanup even if a test fails mid-way.
func forceDeleteCOP(ctx context.Context, name string) {
	cop := &openchoreov1alpha1.ClusterObservabilityPlane{}
	nn := types.NamespacedName{Name: name}
	if err := k8sClient.Get(ctx, nn, cop); err != nil {
		return
	}
	if controllerutil.ContainsFinalizer(cop, clusterobservabilityplane.ClusterObservabilityPlaneCleanupFinalizer) {
		controllerutil.RemoveFinalizer(cop, clusterobservabilityplane.ClusterObservabilityPlaneCleanupFinalizer)
		_ = k8sClient.Update(ctx, cop)
	}
	_ = k8sClient.Delete(ctx, cop)
}

var _ = Describe("ClusterObservabilityPlane Controller", func() {

	Context("When reconciling a non-existent ClusterObservabilityPlane", func() {
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

	Context("When reconciling a newly created ClusterObservabilityPlane (first reconcile)", func() {
		const copName = "cop-first-reconcile"
		nn := types.NamespacedName{Name: copName}

		BeforeEach(func() {
			cop := newClusterObservabilityPlane(copName)
			Expect(k8sClient.Create(ctx, cop)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCOP(ctx, copName)
		})

		It("should add finalizer and return empty result", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// First reconcile returns early after adding finalizer
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			fresh := &openchoreov1alpha1.ClusterObservabilityPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fresh, clusterobservabilityplane.ClusterObservabilityPlaneCleanupFinalizer)).To(BeTrue())
		})
	})

	Context("When reconciling a ClusterObservabilityPlane with finalizer already set (second reconcile)", func() {
		const copName = "cop-second-reconcile"
		nn := types.NamespacedName{Name: copName}

		BeforeEach(func() {
			cop := newClusterObservabilityPlaneWithFinalizer(copName)
			Expect(k8sClient.Create(ctx, cop)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCOP(ctx, copName)
		})

		It("should set Created condition and return RequeueAfter", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))

			fresh := &openchoreov1alpha1.ClusterObservabilityPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("ClusterObservabilityPlaneCreated"))
		})

		It("should update ObservedGeneration to match the current generation", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			fresh := &openchoreov1alpha1.ClusterObservabilityPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})
	})

	Context("When reconciling a ClusterObservabilityPlane that already has the Created condition (shouldIgnoreReconcile=true)", func() {
		const copName = "cop-already-created"
		nn := types.NamespacedName{Name: copName}

		BeforeEach(func() {
			cop := newClusterObservabilityPlaneWithFinalizer(copName)
			Expect(k8sClient.Create(ctx, cop)).To(Succeed())

			// Manually set the Created condition so shouldIgnoreReconcile returns true
			Expect(k8sClient.Get(ctx, nn, cop)).To(Succeed())
			cop.Status.Conditions = []metav1.Condition{
				clusterobservabilityplane.NewClusterObservabilityPlaneCreatedCondition(cop.Generation),
			}
			cop.Status.ObservedGeneration = cop.Generation
			Expect(k8sClient.Status().Update(ctx, cop)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCOP(ctx, copName)
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

			fresh := &openchoreov1alpha1.ClusterObservabilityPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("When deleting a ClusterObservabilityPlane (finalization)", func() {
		const copName = "cop-finalize"
		nn := types.NamespacedName{Name: copName}

		BeforeEach(func() {
			cop := newClusterObservabilityPlaneWithFinalizer(copName)
			Expect(k8sClient.Create(ctx, cop)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCOP(ctx, copName)
		})

		It("should remove finalizer and delete ClusterObservabilityPlane on deletion reconcile", func() {
			cop := &openchoreov1alpha1.ClusterObservabilityPlane{}
			Expect(k8sClient.Get(ctx, nn, cop)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cop)).To(Succeed())

			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// ClusterObservabilityPlane should be fully deleted (finalizer removed → API server garbage collects)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.ClusterObservabilityPlane{})
				return apierrors.IsNotFound(err)
			}, "10s", "500ms").Should(BeTrue())
		})
	})

	Context("When deleting a ClusterObservabilityPlane without a finalizer", func() {
		const copName = "cop-no-finalizer"
		nn := types.NamespacedName{Name: copName}

		It("should return empty result if finalizer is not present", func() {
			cop := newClusterObservabilityPlane(copName)
			Expect(k8sClient.Create(ctx, cop)).To(Succeed())

			// Delete immediately (no finalizer, so it gets deleted by API server)
			Expect(k8sClient.Delete(ctx, cop)).To(Succeed())

			// Reconcile the now-deleted resource — should return not-found path
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("Status persistence via status subresource", func() {
		const copName = "cop-status-persist"
		nn := types.NamespacedName{Name: copName}

		BeforeEach(func() {
			cop := newClusterObservabilityPlaneWithFinalizer(copName)
			Expect(k8sClient.Create(ctx, cop)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCOP(ctx, copName)
		})

		It("should persist status conditions after reconcile", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch and verify status was persisted
			fresh := &openchoreov1alpha1.ClusterObservabilityPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.Conditions).NotTo(BeEmpty())
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})

		It("should persist manually updated AgentConnection status fields", func() {
			cop := &openchoreov1alpha1.ClusterObservabilityPlane{}
			Expect(k8sClient.Get(ctx, nn, cop)).To(Succeed())

			cop.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{
				Connected:       true,
				ConnectedAgents: 2,
				Message:         "2 agents connected (HA mode)",
			}
			Expect(k8sClient.Status().Update(ctx, cop)).To(Succeed())

			// Re-fetch and verify
			fresh := &openchoreov1alpha1.ClusterObservabilityPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.AgentConnection).NotTo(BeNil())
			Expect(fresh.Status.AgentConnection.Connected).To(BeTrue())
			Expect(fresh.Status.AgentConnection.ConnectedAgents).To(BeEquivalentTo(2))
			Expect(fresh.Status.AgentConnection.Message).To(Equal("2 agents connected (HA mode)"))
		})
	})

	Context("Cluster-scoped resource characteristics", func() {
		const copName = "cop-cluster-scoped"
		nn := types.NamespacedName{Name: copName}

		BeforeEach(func() {
			cop := newClusterObservabilityPlaneWithFinalizer(copName)
			Expect(k8sClient.Create(ctx, cop)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCOP(ctx, copName)
		})

		It("should have no namespace after creation", func() {
			fresh := &openchoreov1alpha1.ClusterObservabilityPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Namespace).To(BeEmpty())
		})

		It("should set ObservedGeneration after reconcile", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			fresh := &openchoreov1alpha1.ClusterObservabilityPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.ObservedGeneration).To(BeNumerically(">", 0))
		})
	})
})
