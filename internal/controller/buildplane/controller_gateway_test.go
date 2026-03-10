// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gw "github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/testutils/testgateway"
)

func reconcilerWithGateway(gwClient *gw.Client) *Reconciler {
	return &Reconciler{
		Client:        k8sClient,
		Scheme:        k8sClient.Scheme(),
		Recorder:      record.NewFakeRecorder(100),
		GatewayClient: gwClient,
	}
}

var _ = Describe("BuildPlane Controller — gateway paths", func() {

	Describe("Create reconcile path", func() {
		const bpName = "bp-gw-create"
		nn := types.NamespacedName{Name: bpName, Namespace: "default"}

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, newBuildPlaneWithFinalizer(bpName))).To(Succeed())
		})
		AfterEach(func() { forceDeleteBP(ctx, nn) })

		It("notifies gateway once and populates AgentConnection status", func() {
			gwClient, calls, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
				Connected: true, ConnectedAgents: 2,
			})
			defer shutdown()

			result, err := reconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))
			Expect(*calls).To(Equal(1))

			fresh := &openchoreov1alpha1.BuildPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.AgentConnection).NotTo(BeNil())
			Expect(fresh.Status.AgentConnection.Connected).To(BeTrue())
			Expect(fresh.Status.AgentConnection.ConnectedAgents).To(BeEquivalentTo(2))
			Expect(fresh.Status.AgentConnection.Message).To(Equal("2 agents connected (HA mode)"))
		})

		It("returns error on transient gateway failure (5xx)", func() {
			gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusServiceUnavailable, nil)
			defer shutdown()

			_, err := reconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())
		})

		It("swallows permanent gateway failure (4xx) and returns RequeueAfter", func() {
			gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusBadRequest, nil)
			defer shutdown()

			result, err := reconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))
		})
	})

	Describe("Spec-change re-notification (shouldIgnoreReconcile=true)", func() {
		const bpName = "bp-gw-spec-change"
		nn := types.NamespacedName{Name: bpName, Namespace: "default"}

		BeforeEach(func() {
			bp := newBuildPlaneWithFinalizer(bpName)
			Expect(k8sClient.Create(ctx, bp)).To(Succeed())
			Expect(k8sClient.Get(ctx, nn, bp)).To(Succeed())
			bp.Status.Conditions = []metav1.Condition{NewBuildPlaneCreatedCondition(bp.Generation)}
			bp.Status.ObservedGeneration = bp.Generation
			Expect(k8sClient.Status().Update(ctx, bp)).To(Succeed())
		})
		AfterEach(func() { forceDeleteBP(ctx, nn) })

		It("re-notifies gateway when generation has advanced beyond ObservedGeneration", func() {
			bp := &openchoreov1alpha1.BuildPlane{}
			Expect(k8sClient.Get(ctx, nn, bp)).To(Succeed())
			bp.Spec.ClusterAgent.ClientCA.Value = "updated-ca-cert"
			Expect(k8sClient.Update(ctx, bp)).To(Succeed())
			Expect(k8sClient.Get(ctx, nn, bp)).To(Succeed())
			Expect(bp.Generation).To(BeNumerically(">", bp.Status.ObservedGeneration))

			gwClient, calls, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{Connected: false})
			defer shutdown()

			_, err := reconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(*calls).To(Equal(1))
		})
	})

	Describe("Finalization path", func() {
		const bpName = "bp-gw-finalize"
		nn := types.NamespacedName{Name: bpName, Namespace: "default"}

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, newBuildPlaneWithFinalizer(bpName))).To(Succeed())
		})
		AfterEach(func() { forceDeleteBP(ctx, nn) })

		It("notifies gateway on deletion", func() {
			bp := &openchoreov1alpha1.BuildPlane{}
			Expect(k8sClient.Get(ctx, nn, bp)).To(Succeed())
			Expect(k8sClient.Delete(ctx, bp)).To(Succeed())

			gwClient, calls, shutdown := testgateway.StartFakeGateway(http.StatusOK, nil)
			defer shutdown()

			_, err := reconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(*calls).To(Equal(1))
		})
	})
})
