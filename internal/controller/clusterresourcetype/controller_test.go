// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterresourcetype

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

var _ = Describe("ClusterResourceType Controller", func() {
	Context("When reconciling a ClusterResourceType resource", func() {
		const crtName = "test-clusterresourcetype"

		crtNamespacedName := types.NamespacedName{
			Name: crtName,
			// ClusterResourceType is cluster-scoped, so no namespace
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the ClusterResourceType resource")
			crt := &openchoreov1alpha1.ClusterResourceType{}
			err := k8sClient.Get(ctx, crtNamespacedName, crt)
			if err != nil && !errors.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())
			}
			if errors.IsNotFound(err) {
				crt = &openchoreov1alpha1.ClusterResourceType{
					ObjectMeta: metav1.ObjectMeta{
						Name: crtName,
					},
					Spec: openchoreov1alpha1.ClusterResourceTypeSpec{
						Resources: []openchoreov1alpha1.ResourceTypeManifest{
							{
								ID: "claim",
								Template: &runtime.RawExtension{
									Raw: []byte(`{"apiVersion":"example.org/v1alpha1","kind":"MySQL","metadata":{"name":"test"}}`),
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, crt)).To(Succeed())
			}

			By("Reconciling the ClusterResourceType resource")
			crtReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = crtReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: crtNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the ClusterResourceType resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, crtNamespacedName, crt)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the ClusterResourceType resource")
			Expect(k8sClient.Delete(ctx, crt)).To(Succeed())
		})
	})
})
