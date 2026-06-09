// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

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

var _ = Describe("ResourceRelease Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		resourcerelease := &openchoreov1alpha1.ResourceRelease{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ResourceRelease")
			err := k8sClient.Get(ctx, typeNamespacedName, resourcerelease)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreov1alpha1.ResourceRelease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: openchoreov1alpha1.ResourceReleaseSpec{
						Owner: openchoreov1alpha1.ResourceReleaseOwner{
							ProjectName:  "test-project",
							ResourceName: "test-resource",
						},
						ResourceType: openchoreov1alpha1.ResourceReleaseResourceType{
							Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
							Name: "mysql",
							Spec: openchoreov1alpha1.ResourceTypeSpec{
								Resources: []openchoreov1alpha1.ResourceTypeManifest{
									{
										ID: "claim",
										Template: &runtime.RawExtension{
											Raw: []byte(`{"apiVersion":"example.org/v1alpha1","kind":"MySQL","metadata":{"name":"test"},"spec":{"version":"8.0"}}`),
										},
									},
								},
							},
						},
						Parameters: &runtime.RawExtension{
							Raw: []byte(`{"version":"8.0"}`),
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup logic after each test, like removing the resource instance.
			resource := &openchoreov1alpha1.ResourceRelease{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ResourceRelease")
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
