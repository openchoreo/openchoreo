// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttypedefinition

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

var _ = Describe("ComponentTypeDefinition Controller", func() {
	Context("When reconciling a ComponentTypeDefinition resource", func() {
		const ctdName = "test-componenttypedefinition"
		const ctdNamespace = "default"

		ctdNamespacedName := types.NamespacedName{
			Name:      ctdName,
			Namespace: ctdNamespace,
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the ComponentTypeDefinition resource")
			ctd := &openchoreov1alpha1.ComponentTypeDefinition{}
			err := k8sClient.Get(ctx, ctdNamespacedName, ctd)
			if err != nil && errors.IsNotFound(err) {
				ctd = &openchoreov1alpha1.ComponentTypeDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ctdName,
						Namespace: ctdNamespace,
					},
					Spec: openchoreov1alpha1.ComponentTypeDefinitionSpec{
						WorkloadType: "deployment",
						Resources: []openchoreov1alpha1.ResourceTemplate{
							{
								ID: "deployment",
								Template: runtime.RawExtension{
									Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"test"}},"template":{"metadata":{"labels":{"app":"test"}},"spec":{"containers":[{"name":"test","image":"nginx"}]}}}}`),
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, ctd)).To(Succeed())
			}

			By("Reconciling the ComponentTypeDefinition resource")
			ctdReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = ctdReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: ctdNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the ComponentTypeDefinition resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, ctdNamespacedName, ctd)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the ComponentTypeDefinition resource")
			Expect(k8sClient.Delete(ctx, ctd)).To(Succeed())
		})
	})
})
