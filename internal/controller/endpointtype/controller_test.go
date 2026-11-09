// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointtype

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

var _ = Describe("EndpointType Controller", func() {
	Context("When reconciling a EndpointType resource", func() {
		const rtName = "test-endpointtype"
		const rtNamespace = "default"

		rtNamespacedName := types.NamespacedName{
			Name:      rtName,
			Namespace: rtNamespace,
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the EndpointType resource")
			rt := &openchoreov1alpha1.EndpointType{}
			err := k8sClient.Get(ctx, rtNamespacedName, rt)
			if err != nil && errors.IsNotFound(err) {
				rt = &openchoreov1alpha1.EndpointType{
					ObjectMeta: metav1.ObjectMeta{
						Name:      rtName,
						Namespace: rtNamespace,
					},
					Spec: openchoreov1alpha1.EndpointTypeSpec{
						Resources: []openchoreov1alpha1.ResourceTemplate{
							{
								ID: "route",
								Template: &runtime.RawExtension{
									Raw: []byte(`{"apiVersion":"gateway.networking.k8s.io/v1","kind":"HTTPRoute","metadata":{"name":"test"}}`),
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, rt)).To(Succeed())
			}

			By("Reconciling the EndpointType resource")
			rtReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = rtReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: rtNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the EndpointType resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, rtNamespacedName, rt)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the EndpointType resource")
			Expect(k8sClient.Delete(ctx, rt)).To(Succeed())
		})
	})
})
