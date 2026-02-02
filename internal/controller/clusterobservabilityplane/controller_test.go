// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ClusterObservabilityPlane Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
			// No namespace for cluster-scoped resources
		}
		clusterobservabilityplane := &openchoreov1alpha1.ClusterObservabilityPlane{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ClusterObservabilityPlane")
			err := k8sClient.Get(ctx, typeNamespacedName, clusterobservabilityplane)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreov1alpha1.ClusterObservabilityPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
						// No namespace for cluster-scoped resources
					},
					Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
						PlaneID: "test-plane",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &openchoreov1alpha1.ClusterObservabilityPlane{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ClusterObservabilityPlane")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
