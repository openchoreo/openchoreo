// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("Resource Controller", func() {
	Context("When reconciling a Resource resource", func() {
		const rName = "test-resource"
		const rNamespace = "default"

		rNamespacedName := types.NamespacedName{
			Name:      rName,
			Namespace: rNamespace,
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the Resource resource")
			r := &openchoreov1alpha1.Resource{}
			err := k8sClient.Get(ctx, rNamespacedName, r)
			if err != nil && errors.IsNotFound(err) {
				r = &openchoreov1alpha1.Resource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      rName,
						Namespace: rNamespace,
					},
					Spec: openchoreov1alpha1.ResourceSpec{
						Owner: openchoreov1alpha1.ResourceOwner{
							ProjectName: "test-project",
						},
						Type: openchoreov1alpha1.ResourceTypeRef{
							Name: "mysql",
						},
					},
				}
				Expect(k8sClient.Create(ctx, r)).To(Succeed())
			}

			By("Reconciling the Resource resource")
			rReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = rReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: rNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the Resource resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, rNamespacedName, r)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the Resource resource")
			Expect(k8sClient.Delete(ctx, r)).To(Succeed())
		})
	})
})
