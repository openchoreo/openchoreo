// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sMocks "github.com/openchoreo/openchoreo/internal/clients/kubernetes/mocks"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/planenamespace"
)

var _ = Describe("ClusterDataPlane plane namespace cleanup", func() {
	// "default" exists in envtest; "kv-cdp-gone" never does.
	const liveNamespace = "default"
	const goneNamespace = "kv-cdp-gone"

	newPlane := func() client.Client {
		return fake.NewClientBuilder().WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: planenamespace.KV.Name(liveNamespace), Labels: planenamespace.KV.Labels(liveNamespace),
			}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: planenamespace.KV.Name(goneNamespace), Labels: planenamespace.KV.Labels(goneNamespace),
			}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: planenamespace.Workflow.Name(liveNamespace), Labels: planenamespace.Workflow.Labels(liveNamespace),
			}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: planenamespace.Workflow.Name(goneNamespace), Labels: planenamespace.Workflow.Labels(goneNamespace),
			}},
		).Build()
	}

	exists := func(plane client.Client, name string) bool {
		err := plane.Get(ctx, client.ObjectKey{Name: name}, &corev1.Namespace{})
		if apierrors.IsNotFound(err) {
			return false
		}
		Expect(err).NotTo(HaveOccurred())
		return true
	}

	reconcilerFor := func(plane client.Client) *Reconciler {
		provider := k8sMocks.NewMockDataPlaneClientProvider(GinkgoT())
		if plane != nil {
			provider.EXPECT().ClusterDataPlaneClient(mock.Anything).Return(plane, nil).Maybe()
		}
		r := testReconciler()
		r.PlaneClientProvider = provider
		r.APIReader = k8sClient
		return r
	}

	Context("When a ClusterDataPlane is deleted", func() {
		const cdpName = "cdp-kv-finalize"
		nn := types.NamespacedName{Name: cdpName}
		srNN := types.NamespacedName{Name: "cdp-kv-db", Namespace: liveNamespace}

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, newClusterDataPlaneWithFinalizer(cdpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, &openchoreov1alpha1.SecretReference{
				ObjectMeta: metav1.ObjectMeta{
					Name: srNN.Name, Namespace: srNN.Namespace,
					Labels: map[string]string{"openchoreo.dev/managed-by": "openchoreo-api"},
				},
				Spec: openchoreov1alpha1.SecretReferenceSpec{
					TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: "ClusterDataPlane", Name: cdpName},
					Data: []openchoreov1alpha1.SecretDataSource{{
						SecretKey: "password",
						RemoteRef: openchoreov1alpha1.RemoteReference{Key: "default/generic/db", Property: "password"},
					}},
				},
			})).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.SecretReference{
				ObjectMeta: metav1.ObjectMeta{Name: srNN.Name, Namespace: srNN.Namespace},
			})
			forceDeleteCDP(ctx, nn)
		})

		It("waits while a secret targets it, then sweeps stale plane namespaces", func() {
			cdp := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, cdp)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cdp)).To(Succeed())

			plane := newPlane()
			r := reconcilerFor(plane)

			By("blocking while the SecretReference targets the plane")
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.PlaneDeletionRetryInterval))

			Expect(k8sClient.Get(ctx, nn, cdp)).To(Succeed())
			cond := apimeta.FindStatusCondition(cdp.Status.Conditions, string(controller.ConditionPlaneFinalizing))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal(string(controller.ReasonPlaneDeletionBlocked)))
			Expect(cond.Message).To(ContainSubstring("1 secret(s)"))
			Expect(exists(plane, planenamespace.KV.Name(goneNamespace))).To(BeTrue(), "nothing is swept while blocked")

			By("finalizing once the SecretReference is gone")
			Expect(k8sClient.Delete(ctx, &openchoreov1alpha1.SecretReference{
				ObjectMeta: metav1.ObjectMeta{Name: srNN.Name, Namespace: srNN.Namespace},
			})).To(Succeed())
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, nn, &openchoreov1alpha1.ClusterDataPlane{}))).To(BeTrue())

			Expect(exists(plane, planenamespace.KV.Name(goneNamespace))).To(BeFalse())
			Expect(exists(plane, planenamespace.KV.Name(liveNamespace))).To(BeTrue())
		})
	})

	Context("When a ClusterDataPlane is reconciled in steady state", func() {
		const cdpName = "cdp-kv-sweep"
		nn := types.NamespacedName{Name: cdpName}

		setStatus := func(connected bool) {
			cdp := newClusterDataPlaneWithFinalizer(cdpName)
			Expect(k8sClient.Create(ctx, cdp)).To(Succeed())
			Expect(k8sClient.Get(ctx, nn, cdp)).To(Succeed())
			cdp.Status.Conditions = []metav1.Condition{NewClusterDataPlaneCreatedCondition(cdp.Generation)}
			cdp.Status.ObservedGeneration = cdp.Generation
			cdp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{Connected: connected}
			Expect(k8sClient.Status().Update(ctx, cdp)).To(Succeed())
		}

		AfterEach(func() {
			forceDeleteCDP(ctx, nn)
		})

		It("deletes plane namespaces of removed control plane namespaces", func() {
			setStatus(true)
			plane := newPlane()

			result, err := reconcilerFor(plane).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))

			Expect(exists(plane, planenamespace.KV.Name(goneNamespace))).To(BeFalse())
			Expect(exists(plane, planenamespace.KV.Name(liveNamespace))).To(BeTrue())
			Expect(exists(plane, planenamespace.Workflow.Name(goneNamespace))).To(BeFalse())
			Expect(exists(plane, planenamespace.Workflow.Name(liveNamespace))).To(BeTrue())
		})

		It("does not reach the plane while no agent is connected", func() {
			setStatus(false)

			// A provider with no expectations fails the test if it is called.
			_, err := reconcilerFor(nil).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
