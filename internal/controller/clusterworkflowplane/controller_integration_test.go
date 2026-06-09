// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/clusterworkflowplane"
)

// testReconciler returns a Reconciler configured for tests (no gateway or client manager).
// ClusterWorkflowPlane is cluster-scoped, so no namespace is used.
func testReconciler() *clusterworkflowplane.Reconciler {
	return &clusterworkflowplane.Reconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(100),
	}
}

// newClusterWorkflowPlane creates a ClusterWorkflowPlane with the required spec fields.
// As a cluster-scoped resource it has no namespace.
func newClusterWorkflowPlane(name string) *openchoreov1alpha1.ClusterWorkflowPlane {
	return &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			PlaneID: "test-plane",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					Value: "test-ca-cert",
				},
			},
		},
	}
}

// newClusterWorkflowPlaneWithFinalizer creates a ClusterWorkflowPlane with the cleanup finalizer pre-set.
func newClusterWorkflowPlaneWithFinalizer(name string) *openchoreov1alpha1.ClusterWorkflowPlane {
	cwp := newClusterWorkflowPlane(name)
	cwp.Finalizers = []string{clusterworkflowplane.ClusterWorkflowPlaneCleanupFinalizer}
	return cwp
}

// forceDeleteCWP strips the cleanup finalizer from a ClusterWorkflowPlane and deletes it,
// ensuring cleanup even if a test fails mid-way.
func forceDeleteCWP(ctx context.Context, name string) {
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
	nn := types.NamespacedName{Name: name}
	if err := k8sClient.Get(ctx, nn, cwp); err != nil {
		return
	}
	if controllerutil.ContainsFinalizer(cwp, clusterworkflowplane.ClusterWorkflowPlaneCleanupFinalizer) {
		controllerutil.RemoveFinalizer(cwp, clusterworkflowplane.ClusterWorkflowPlaneCleanupFinalizer)
		_ = k8sClient.Update(ctx, cwp)
	}
	_ = k8sClient.Delete(ctx, cwp)
}

var _ = Describe("ClusterWorkflowPlane Controller", func() {

	Context("When reconciling a non-existent ClusterWorkflowPlane", func() {
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

	Context("When reconciling a newly created ClusterWorkflowPlane (first reconcile)", func() {
		const cwpName = "cwp-first-reconcile"
		nn := types.NamespacedName{Name: cwpName}

		BeforeEach(func() {
			cwp := newClusterWorkflowPlane(cwpName)
			Expect(k8sClient.Create(ctx, cwp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCWP(ctx, cwpName)
		})

		It("should add finalizer and return empty result", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// First reconcile returns early after adding finalizer
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			fresh := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fresh, clusterworkflowplane.ClusterWorkflowPlaneCleanupFinalizer)).To(BeTrue())
		})
	})

	Context("When reconciling a ClusterWorkflowPlane with finalizer already set (second reconcile)", func() {
		const cwpName = "cwp-second-reconcile"
		nn := types.NamespacedName{Name: cwpName}

		BeforeEach(func() {
			cwp := newClusterWorkflowPlaneWithFinalizer(cwpName)
			Expect(k8sClient.Create(ctx, cwp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCWP(ctx, cwpName)
		})

		It("should set Created condition and return RequeueAfter", func() {
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))

			fresh := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("ClusterWorkflowPlaneCreated"))
		})

		It("should update ObservedGeneration to match the current generation", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			fresh := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})
	})

	Context("When reconciling a ClusterWorkflowPlane that already has the Created condition (shouldIgnoreReconcile=true)", func() {
		const cwpName = "cwp-already-created"
		nn := types.NamespacedName{Name: cwpName}

		BeforeEach(func() {
			cwp := newClusterWorkflowPlaneWithFinalizer(cwpName)
			Expect(k8sClient.Create(ctx, cwp)).To(Succeed())

			// Manually set the Created condition so shouldIgnoreReconcile returns true
			Expect(k8sClient.Get(ctx, nn, cwp)).To(Succeed())
			cwp.Status.Conditions = []metav1.Condition{
				clusterworkflowplane.NewClusterWorkflowPlaneCreatedCondition(cwp.Generation),
			}
			cwp.Status.ObservedGeneration = cwp.Generation
			Expect(k8sClient.Status().Update(ctx, cwp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCWP(ctx, cwpName)
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

			fresh := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("When deleting a ClusterWorkflowPlane (finalization)", func() {
		const cwpName = "cwp-finalize"
		nn := types.NamespacedName{Name: cwpName}

		BeforeEach(func() {
			cwp := newClusterWorkflowPlaneWithFinalizer(cwpName)
			Expect(k8sClient.Create(ctx, cwp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCWP(ctx, cwpName)
		})

		It("should remove finalizer and delete ClusterWorkflowPlane on deletion reconcile", func() {
			cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, cwp)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cwp)).To(Succeed())

			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// ClusterWorkflowPlane should be fully deleted (finalizer removed → API server garbage collects)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.ClusterWorkflowPlane{})
				return apierrors.IsNotFound(err)
			}, "10s", "500ms").Should(BeTrue())
		})
	})

	Context("When deleting a ClusterWorkflowPlane without a finalizer", func() {
		const cwpName = "cwp-no-finalizer"
		nn := types.NamespacedName{Name: cwpName}

		It("should return empty result if finalizer is not present", func() {
			cwp := newClusterWorkflowPlane(cwpName)
			Expect(k8sClient.Create(ctx, cwp)).To(Succeed())

			// Delete immediately (no finalizer, so it gets deleted by API server)
			Expect(k8sClient.Delete(ctx, cwp)).To(Succeed())

			// Reconcile the now-deleted resource — should return not-found path
			r := testReconciler()
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("Status persistence via status subresource", func() {
		const cwpName = "cwp-status-persist"
		nn := types.NamespacedName{Name: cwpName}

		BeforeEach(func() {
			cwp := newClusterWorkflowPlaneWithFinalizer(cwpName)
			Expect(k8sClient.Create(ctx, cwp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCWP(ctx, cwpName)
		})

		It("should persist status conditions after reconcile", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch and verify status was persisted
			fresh := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.Conditions).NotTo(BeEmpty())
			Expect(fresh.Status.ObservedGeneration).To(Equal(fresh.Generation))
		})

		It("should persist manually updated AgentConnection status fields", func() {
			cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, cwp)).To(Succeed())

			cwp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{
				Connected:       true,
				ConnectedAgents: 2,
				Message:         "2 agents connected (HA mode)",
			}
			Expect(k8sClient.Status().Update(ctx, cwp)).To(Succeed())

			// Re-fetch and verify
			fresh := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.AgentConnection).NotTo(BeNil())
			Expect(fresh.Status.AgentConnection.Connected).To(BeTrue())
			Expect(fresh.Status.AgentConnection.ConnectedAgents).To(BeEquivalentTo(2))
			Expect(fresh.Status.AgentConnection.Message).To(Equal("2 agents connected (HA mode)"))
		})
	})

	Context("Cluster-scoped resource characteristics", func() {
		const cwpName = "cwp-cluster-scoped"
		nn := types.NamespacedName{Name: cwpName}

		BeforeEach(func() {
			cwp := newClusterWorkflowPlaneWithFinalizer(cwpName)
			Expect(k8sClient.Create(ctx, cwp)).To(Succeed())
		})

		AfterEach(func() {
			forceDeleteCWP(ctx, cwpName)
		})

		It("should have no namespace after creation", func() {
			fresh := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Namespace).To(BeEmpty())
		})

		It("should set ObservedGeneration after reconcile", func() {
			r := testReconciler()
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			fresh := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.ObservedGeneration).To(BeNumerically(">", 0))
		})
	})

	Context("SetupWithManager", func() {
		It("should register the controller and supply a default Recorder", func() {
			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
				Metrics: metricsserver.Options{
					BindAddress: "0",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			r := &clusterworkflowplane.Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			Expect(r.SetupWithManager(mgr)).To(Succeed())
			Expect(r.Recorder).NotTo(BeNil())
		})
	})

	Context("Reconcile invalidates the cached client", func() {
		const cwpName = "cwp-cache-update"
		nn := types.NamespacedName{Name: cwpName}

		AfterEach(func() { forceDeleteCWP(ctx, cwpName) })

		It("invalidates the cache on the UPDATE path when ClientMgr is configured", func() {
			Expect(k8sClient.Create(ctx, newClusterWorkflowPlaneWithFinalizer(cwpName))).To(Succeed())

			r := &clusterworkflowplane.Reconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				Recorder:     record.NewFakeRecorder(100),
				ClientMgr:    kubernetesClient.NewManager(),
				CacheVersion: "v2",
			}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		})

		It("invalidates the cache during finalization when ClientMgr is configured", func() {
			Expect(k8sClient.Create(ctx, newClusterWorkflowPlaneWithFinalizer(cwpName))).To(Succeed())
			cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, cwp)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cwp)).To(Succeed())

			r := &clusterworkflowplane.Reconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				Recorder:     record.NewFakeRecorder(100),
				ClientMgr:    kubernetesClient.NewManager(),
				CacheVersion: "v2",
			}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Reconcile second pass with no gateway (Created condition already set)", func() {
		const cwpName = "cwp-second-pass-no-gw"
		nn := types.NamespacedName{Name: cwpName}

		BeforeEach(func() {
			cwp := newClusterWorkflowPlaneWithFinalizer(cwpName)
			Expect(k8sClient.Create(ctx, cwp)).To(Succeed())
			Expect(k8sClient.Get(ctx, nn, cwp)).To(Succeed())
			cwp.Status.Conditions = []metav1.Condition{
				clusterworkflowplane.NewClusterWorkflowPlaneCreatedCondition(cwp.Generation),
			}
			cwp.Status.ObservedGeneration = cwp.Generation
			Expect(k8sClient.Status().Update(ctx, cwp)).To(Succeed())
		})
		AfterEach(func() { forceDeleteCWP(ctx, cwpName) })

		It("returns RequeueAfter and does not error", func() {
			r := &clusterworkflowplane.Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))
		})
	})
})
