// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gw "github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/testutils/testgateway"
)

func cdpReconcilerWithGateway(gwClient *gw.Client) *Reconciler {
	return &Reconciler{
		Client:        k8sClient,
		Scheme:        k8sClient.Scheme(),
		Recorder:      record.NewFakeRecorder(100),
		GatewayClient: gwClient,
	}
}

var _ = Describe("ClusterDataPlane Controller — gateway paths", func() {

	Describe("Create reconcile path", func() {
		const cdpName = "cdp-gw-create"
		nn := types.NamespacedName{Name: cdpName}

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, newClusterDataPlaneWithFinalizer(cdpName))).To(Succeed())
		})
		AfterEach(func() { forceDeleteCDP(ctx, nn) })

		It("notifies gateway and skips immediate status poll to avoid HA flapping", func() {
			gwClient, calls, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
				Connected: true, ConnectedAgents: 1,
			})
			defer shutdown()

			result, err := cdpReconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))
			Expect(*calls).To(Equal(1))

			// After gateway notification, status poll is intentionally skipped to avoid
			// catching agents mid-reconnect (HA flapping). AgentConnection will be
			// populated on the next periodic requeue.
			fresh := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.AgentConnection).To(BeNil())
		})

		// When populateAgentConnectionStatus returns an error (status GET fails),
		// Status().Update should still run so the Created condition is persisted.
		// This prevents the controller from re-entering the create path on every reconcile.
		It("persists Created condition even when status GET fails", func() {
			gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusOK, nil) // notify OK, status → 500
			defer shutdown()

			result, err := cdpReconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred()) // status error is swallowed
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))

			fresh := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).NotTo(BeNil()) // persisted: Status().Update always runs
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})
})
