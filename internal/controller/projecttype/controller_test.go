// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projecttype

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

var _ = Describe("ProjectType Controller", func() {
	Context("When reconciling a ProjectType resource", func() {
		const ptName = "test-projecttype"
		const ptNamespace = "default"

		ptNamespacedName := types.NamespacedName{
			Name:      ptName,
			Namespace: ptNamespace,
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the ProjectType resource")
			pt := &openchoreov1alpha1.ProjectType{}
			err := k8sClient.Get(ctx, ptNamespacedName, pt)
			if err != nil && errors.IsNotFound(err) {
				pt = &openchoreov1alpha1.ProjectType{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ptName,
						Namespace: ptNamespace,
					},
					Spec: openchoreov1alpha1.ProjectTypeSpec{
						Resources: []openchoreov1alpha1.ResourceTemplate{
							{
								ID: "default-deny-egress",
								Template: &runtime.RawExtension{
									Raw: []byte(`{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"deny"}}`),
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, pt)).To(Succeed())
			}

			By("Reconciling the ProjectType resource")
			ptReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = ptReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: ptNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the ProjectType resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, ptNamespacedName, pt)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the ProjectType resource")
			Expect(k8sClient.Delete(ctx, pt)).To(Succeed())
		})
	})
})
