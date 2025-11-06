// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentenvsnapshot

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("ComponentEnvSnapshot Controller", func() {
	Context("When reconciling a ComponentEnvSnapshot resource", func() {
		It("should successfully reconcile (no-op)", func() {
			By("Reconciling the ComponentEnvSnapshot resource")
			snapshotReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Reconcile should succeed even with a non-existent resource (no-op behavior)
			_, err := snapshotReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "any-name",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
