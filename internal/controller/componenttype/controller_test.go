// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

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

var _ = Describe("ComponentType Controller", func() {
	Context("When reconciling a ComponentType resource", func() {
		const ctName = "test-componenttype"
		const ctNamespace = "default"

		ctNamespacedName := types.NamespacedName{
			Name:      ctName,
			Namespace: ctNamespace,
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the ComponentType resource")
			ct := &openchoreov1alpha1.ComponentType{}
			err := k8sClient.Get(ctx, ctNamespacedName, ct)
			if err != nil && errors.IsNotFound(err) {
				ct = &openchoreov1alpha1.ComponentType{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ctName,
						Namespace: ctNamespace,
					},
					Spec: openchoreov1alpha1.ComponentTypeSpec{
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
				Expect(k8sClient.Create(ctx, ct)).To(Succeed())
			}

			By("Reconciling the ComponentType resource")
			ctReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = ctReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: ctNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the ComponentType resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, ctNamespacedName, ct)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the ComponentType resource")
			Expect(k8sClient.Delete(ctx, ct)).To(Succeed())
		})
	})
})
