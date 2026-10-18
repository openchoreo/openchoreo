// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrelease

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("EndpointRelease Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		endpointrelease := &openchoreov1alpha1.EndpointRelease{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind EndpointRelease")
			err := k8sClient.Get(ctx, typeNamespacedName, endpointrelease)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreov1alpha1.EndpointRelease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: openchoreov1alpha1.EndpointReleaseSpec{
						Owner: openchoreov1alpha1.EndpointReleaseOwner{
							ProjectName:  "test-project",
							EndpointName: "test-endpoint",
						},
						EndpointType: openchoreov1alpha1.EndpointReleaseEndpointType{
							Kind: openchoreov1alpha1.EndpointTypeRefKindEndpointType,
							Name: "http-route",
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
						},
						Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
						BackendRefs: []openchoreov1alpha1.EndpointBackendRef{{
							Name:      "stable",
							Kind:      openchoreov1alpha1.EndpointBackendKindComponent,
							Component: &openchoreov1alpha1.EndpointComponentBackend{Name: "greeter", Endpoint: "http"},
						}},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup logic after each test, like removing the resource instance.
			resource := &openchoreov1alpha1.EndpointRelease{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance EndpointRelease")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
