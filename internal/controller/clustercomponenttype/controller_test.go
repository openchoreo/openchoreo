// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ClusterComponentType Controller", func() {
	Context("When reconciling a ClusterComponentType resource", func() {
		const cctName = "test-clustercomponenttype"

		cctNamespacedName := types.NamespacedName{
			Name: cctName,
			// ClusterComponentType is cluster-scoped, so no namespace
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the ClusterComponentType resource")
			cct := &openchoreov1alpha1.ClusterComponentType{}
			err := k8sClient.Get(ctx, cctNamespacedName, cct)
			if err != nil && !errors.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())
			}
			if errors.IsNotFound(err) {
				cct = &openchoreov1alpha1.ClusterComponentType{
					ObjectMeta: metav1.ObjectMeta{
						Name: cctName,
					},
					Spec: openchoreov1alpha1.ClusterComponentTypeSpec{
						WorkloadType: "deployment",
						Resources: []openchoreov1alpha1.ResourceTemplate{
							{
								ID: "deployment",
								Template: &runtime.RawExtension{
									Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"test"}},"template":{"metadata":{"labels":{"app":"test"}},"spec":{"containers":[{"name":"test","image":"nginx"}]}}}}`),
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, cct)).To(Succeed())
			}

			By("Reconciling the ClusterComponentType resource")
			cctReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = cctReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: cctNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the ClusterComponentType resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, cctNamespacedName, cct)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the ClusterComponentType resource")
			Expect(k8sClient.Delete(ctx, cct)).To(Succeed())
		})
	})
})
