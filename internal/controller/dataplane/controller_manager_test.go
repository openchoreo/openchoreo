// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// reconcileCountingClient counts reads of a DataPlane. Every reconcile starts
// with one, so the count moves exactly when the controller reconciles.
type reconcileCountingClient struct {
	client.Client
	dataPlaneGets atomic.Int32
}

func (c *reconcileCountingClient) Get(
	ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption,
) error {
	if _, ok := obj.(*openchoreov1alpha1.DataPlane); ok {
		c.dataPlaneGets.Add(1)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

var _ = Describe("DataPlane controller event filter", func() {
	// A plane controller writes agent connection status on every periodic
	// refresh. If that write queued the DataPlane again, every DataPlane would
	// be reconciled twice per interval. With thousands of them that backlog is
	// what a new DataPlane waits behind before the gateway is told about it.
	It("does not reconcile a DataPlane again for a status write", func() {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:     k8sClient.Scheme(),
			Metrics:    metricsserver.Options{BindAddress: "0"},
			Controller: config.Controller{SkipNameValidation: ptr.To(true)},
		})
		Expect(err).NotTo(HaveOccurred())

		counting := &reconcileCountingClient{Client: mgr.GetClient()}
		r := &Reconciler{Client: counting, Scheme: mgr.GetScheme(), Recorder: record.NewFakeRecorder(100)}
		Expect(r.SetupWithManager(mgr)).To(Succeed())

		mgrCtx, stop := context.WithCancel(ctx)
		stopped := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			defer close(stopped)
			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()
		DeferCleanup(func() {
			stop()
			<-stopped
		})

		nn := types.NamespacedName{Name: "dp-event-filter", Namespace: "default"}
		Expect(k8sClient.Create(ctx, &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace},
		})).To(Succeed())
		DeferCleanup(forceDeleteDP, ctx, nn)

		By("waiting for the controller to finish reconciling the new DataPlane")
		dp := &openchoreov1alpha1.DataPlane{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, nn, dp)).To(Succeed())
			g.Expect(apimeta.FindStatusCondition(dp.Status.Conditions, string(controller.TypeCreated))).NotTo(BeNil())
		}, "10s", "100ms").Should(Succeed())
		settled := int32(-1)
		Eventually(func() bool {
			n := counting.dataPlaneGets.Load()
			unchanged := n == settled
			settled = n
			return unchanged
		}, "10s", "500ms").Should(BeTrue())

		By("writing status the way a periodic refresh does")
		Expect(k8sClient.Get(ctx, nn, dp)).To(Succeed())
		now := metav1.Now()
		dp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{
			Connected: true, ConnectedAgents: 1, LastHeartbeatTime: &now,
		}
		Expect(k8sClient.Status().Update(ctx, dp)).To(Succeed())
		Consistently(counting.dataPlaneGets.Load, "2s", "100ms").Should(Equal(settled))

		By("changing a label, which must still be reconciled")
		Expect(k8sClient.Get(ctx, nn, dp)).To(Succeed())
		dp.Labels = map[string]string{"team": "a"}
		Expect(k8sClient.Update(ctx, dp)).To(Succeed())
		Eventually(counting.dataPlaneGets.Load, "10s", "100ms").Should(BeNumerically(">", settled))
	})
})
