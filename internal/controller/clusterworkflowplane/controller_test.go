// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/clusterworkflowplane"
)

var _ = Describe("ClusterWorkflowPlane Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ClusterWorkflowPlane")
			err := k8sClient.Get(ctx, typeNamespacedName, cwp)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreov1alpha1.ClusterWorkflowPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
						PlaneID: "test-plane",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &openchoreov1alpha1.ClusterWorkflowPlane{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ClusterWorkflowPlane")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &clusterworkflowplane.Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
