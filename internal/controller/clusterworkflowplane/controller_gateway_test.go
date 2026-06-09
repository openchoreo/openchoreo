// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane_test

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
	"github.com/openchoreo/openchoreo/internal/controller/clusterworkflowplane"
	"github.com/openchoreo/openchoreo/internal/controller/testutils/testgateway"
)

func cwpReconcilerWithGateway(gwClient *gw.Client) *clusterworkflowplane.Reconciler {
	return &clusterworkflowplane.Reconciler{
		Client:        k8sClient,
		Scheme:        k8sClient.Scheme(),
		Recorder:      record.NewFakeRecorder(100),
		GatewayClient: gwClient,
	}
}

var _ = Describe("ClusterWorkflowPlane Controller — gateway create/delete paths", func() {

	Describe("Create reconcile path", func() {
		const cwpName = "cwp-gw-create"
		nn := types.NamespacedName{Name: cwpName}

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, newClusterWorkflowPlaneWithFinalizer(cwpName))).To(Succeed())
		})
		AfterEach(func() { forceDeleteCWP(ctx, cwpName) })

		It("notifies gateway exactly once and skips status poll to avoid HA flapping", func() {
			gwClient, calls, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
				Connected: true, ConnectedAgents: 2,
			})
			defer shutdown()

			result, err := cwpReconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))
			Expect(*calls).To(Equal(1))

			// Status poll skipped to avoid HA flapping → AgentConnection should still be nil.
			fresh := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.AgentConnection).To(BeNil())
		})

		It("returns error on transient gateway failure (5xx)", func() {
			gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusServiceUnavailable, nil)
			defer shutdown()

			_, err := cwpReconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())
		})

		It("swallows permanent gateway failure (4xx) and returns RequeueAfter", func() {
			gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusBadRequest, nil)
			defer shutdown()

			result, err := cwpReconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))
		})
	})

	Describe("Finalization path", func() {
		const cwpName = "cwp-gw-finalize"
		nn := types.NamespacedName{Name: cwpName}

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, newClusterWorkflowPlaneWithFinalizer(cwpName))).To(Succeed())
		})
		AfterEach(func() { forceDeleteCWP(ctx, cwpName) })

		It("notifies gateway on deletion", func() {
			cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, cwp)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cwp)).To(Succeed())

			gwClient, calls, shutdown := testgateway.StartFakeGateway(http.StatusOK, nil)
			defer shutdown()

			_, err := cwpReconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(*calls).To(Equal(1))
		})
	})
})

var _ = Describe("ClusterWorkflowPlane Controller — gateway paths", func() {

	// ClusterWorkflowPlane intentionally omits the specChanged re-notification that
	// WorkflowPlane and ObservabilityPlane have. This test guards against accidentally
	// introducing that logic (e.g. by copying from another controller).
	Describe("shouldIgnoreReconcile=true path has no gateway re-notification", func() {
		const cwpName = "cwp-gw-ignore"
		nn := types.NamespacedName{Name: cwpName}

		BeforeEach(func() {
			cwp := newClusterWorkflowPlaneWithFinalizer(cwpName)
			Expect(k8sClient.Create(ctx, cwp)).To(Succeed())
			Expect(k8sClient.Get(ctx, nn, cwp)).To(Succeed())
			cwp.Status.Conditions = []metav1.Condition{clusterworkflowplane.NewClusterWorkflowPlaneCreatedCondition(cwp.Generation)}
			cwp.Status.ObservedGeneration = cwp.Generation
			Expect(k8sClient.Status().Update(ctx, cwp)).To(Succeed())
		})
		AfterEach(func() { forceDeleteCWP(ctx, cwpName) })

		It("does not notify gateway even when generation has advanced beyond ObservedGeneration", func() {
			cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
			Expect(k8sClient.Get(ctx, nn, cwp)).To(Succeed())
			cwp.Spec.ClusterAgent.ClientCA.Value = "updated-ca-cert"
			Expect(k8sClient.Update(ctx, cwp)).To(Succeed())
			Expect(k8sClient.Get(ctx, nn, cwp)).To(Succeed())
			Expect(cwp.Generation).To(BeNumerically(">", cwp.Status.ObservedGeneration))

			gwClient, calls, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{Connected: false})
			defer shutdown()

			result, err := cwpReconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))
			Expect(*calls).To(Equal(0))
		})
	})
})
