// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterprojecttype

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

var _ = Describe("ClusterProjectType Controller", func() {
	Context("When reconciling a ClusterProjectType resource", func() {
		const cptName = "test-clusterprojecttype"

		cptNamespacedName := types.NamespacedName{
			Name: cptName,
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the ClusterProjectType resource")
			cpt := &openchoreov1alpha1.ClusterProjectType{}
			err := k8sClient.Get(ctx, cptNamespacedName, cpt)
			if err != nil && errors.IsNotFound(err) {
				cpt = &openchoreov1alpha1.ClusterProjectType{
					ObjectMeta: metav1.ObjectMeta{
						Name: cptName,
					},
					Spec: openchoreov1alpha1.ClusterProjectTypeSpec{
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
				Expect(k8sClient.Create(ctx, cpt)).To(Succeed())
			}

			By("Reconciling the ClusterProjectType resource")
			cptReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = cptReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: cptNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the ClusterProjectType resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, cptNamespacedName, cpt)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the ClusterProjectType resource")
			Expect(k8sClient.Delete(ctx, cpt)).To(Succeed())
		})
	})
})
