// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterbuildplane_test

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
	"github.com/openchoreo/openchoreo/internal/controller/clusterbuildplane"
	"github.com/openchoreo/openchoreo/internal/controller/testutils/testgateway"
)

func cbpReconcilerWithGateway(gwClient *gw.Client) *clusterbuildplane.Reconciler {
	return &clusterbuildplane.Reconciler{
		Client:        k8sClient,
		Scheme:        k8sClient.Scheme(),
		Recorder:      record.NewFakeRecorder(100),
		GatewayClient: gwClient,
	}
}

var _ = Describe("ClusterBuildPlane Controller — gateway paths", func() {

	// ClusterBuildPlane intentionally omits the specChanged re-notification that
	// BuildPlane and ObservabilityPlane have. This test guards against accidentally
	// introducing that logic (e.g. by copying from another controller).
	Describe("shouldIgnoreReconcile=true path has no gateway re-notification", func() {
		const cbpName = "cbp-gw-ignore"
		nn := types.NamespacedName{Name: cbpName}

		BeforeEach(func() {
			cbp := newClusterBuildPlaneWithFinalizer(cbpName)
			Expect(k8sClient.Create(ctx, cbp)).To(Succeed())
			Expect(k8sClient.Get(ctx, nn, cbp)).To(Succeed())
			cbp.Status.Conditions = []metav1.Condition{clusterbuildplane.NewClusterBuildPlaneCreatedCondition(cbp.Generation)}
			cbp.Status.ObservedGeneration = cbp.Generation
			Expect(k8sClient.Status().Update(ctx, cbp)).To(Succeed())
		})
		AfterEach(func() { forceDeleteCBP(ctx, cbpName) })

		It("does not notify gateway even when generation has advanced beyond ObservedGeneration", func() {
			cbp := &openchoreov1alpha1.ClusterBuildPlane{}
			Expect(k8sClient.Get(ctx, nn, cbp)).To(Succeed())
			cbp.Spec.ClusterAgent.ClientCA.Value = "updated-ca-cert"
			Expect(k8sClient.Update(ctx, cbp)).To(Succeed())
			Expect(k8sClient.Get(ctx, nn, cbp)).To(Succeed())
			Expect(cbp.Generation).To(BeNumerically(">", cbp.Status.ObservedGeneration))

			gwClient, calls, shutdown := testgateway.StartFakeGateway(http.StatusOK, &gw.PlaneConnectionStatus{Connected: false})
			defer shutdown()

			result, err := cbpReconcilerWithGateway(gwClient).Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(controller.StatusUpdateInterval))
			Expect(*calls).To(Equal(0))
		})
	})
})
