// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Workload Controller — additional coverage", func() {

	Context("SetupWithManager", func() {
		It("should register the controller with a manager successfully", func() {
			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
				Metrics: metricsserver.Options{
					BindAddress: "0",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			r := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			Expect(r.SetupWithManager(mgr)).To(Succeed())
		})
	})

	Context("Reconcile for non-existent Workload", func() {
		It("should return an empty result without error", func() {
			r := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			result, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "missing-workload",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})
})
