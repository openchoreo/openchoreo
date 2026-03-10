// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
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

		It("notifies gateway and populates AgentConnection when status GET succeeds", func() {
			gwClient, calls, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{
				Connected: true, ConnectedAgents: 1,
			})
			defer shutdown()

			result, err := cdpReconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))
			Expect(*calls).To(Equal(1))

			fresh := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			Expect(fresh.Status.AgentConnection).NotTo(BeNil())
			Expect(fresh.Status.AgentConnection.Connected).To(BeTrue())
			Expect(fresh.Status.AgentConnection.Message).To(Equal("1 agent connected"))
		})

		// ClusterDataPlane uses `else if` for Status().Update, unlike BuildPlane which always
		// runs Status().Update. When populateAgentConnectionStatus returns an error (status GET
		// fails), Status().Update is skipped and the Created condition is not persisted.
		It("does not persist Created condition when status GET fails (else-if Status.Update)", func() {
			gwClient, _, shutdown := testgateway.StartFakeGateway(http.StatusOK, nil) // notify OK, status → 500
			defer shutdown()

			result, err := cdpReconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred()) // status error is swallowed
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))

			fresh := &openchoreov1alpha1.ClusterDataPlane{}
			Expect(k8sClient.Get(ctx, nn, fresh)).To(Succeed())
			cond := apimeta.FindStatusCondition(fresh.Status.Conditions, string(controller.TypeCreated))
			Expect(cond).To(BeNil()) // not persisted: Status().Update was skipped
		})
	})
})
