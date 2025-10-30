// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package addon

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

var _ = Describe("Addon Controller", func() {
	Context("When reconciling an Addon resource", func() {
		const addonName = "test-addon"
		const namespace = "default"

		addonNamespacedName := types.NamespacedName{
			Name:      addonName,
			Namespace: namespace,
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the Addon resource")
			addon := &openchoreov1alpha1.Addon{}
			err := k8sClient.Get(ctx, addonNamespacedName, addon)
			if err != nil && errors.IsNotFound(err) {
				addon = &openchoreov1alpha1.Addon{
					ObjectMeta: metav1.ObjectMeta{
						Name:      addonName,
						Namespace: namespace,
					},
					Spec: openchoreov1alpha1.AddonSpec{
						// Add minimal spec fields as required
					},
				}
				Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			}

			By("Reconciling the Addon resource")
			addonReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = addonReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: addonNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the Addon resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, addonNamespacedName, addon)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the Addon resource")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
	})
})
