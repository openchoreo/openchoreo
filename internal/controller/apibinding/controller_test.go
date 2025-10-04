// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apibinding

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("APIBinding Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		apibinding := &openchoreov1alpha1.APIBinding{}

		BeforeEach(func() {
			By("creating the APIClass")
			apiClass := &openchoreov1alpha1.APIClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-api-class",
					Namespace: "default",
				},
			}
			err := k8sClient.Create(ctx, apiClass)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("creating the API")
			api := &openchoreov1alpha1.API{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-api",
					Namespace: "default",
				},
				Spec: openchoreov1alpha1.APISpec{
					Owner: openchoreov1alpha1.EndpointOwner{
						ProjectName:   "test-project",
						ComponentName: "test-component",
					},
					EnvironmentName: "test-env",
					EndpointTemplateSpec: openchoreov1alpha1.EndpointTemplateSpec{
						Type: openchoreov1alpha1.EndpointTypeREST,
					},
				},
			}
			err = k8sClient.Create(ctx, api)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("creating the custom resource for the Kind APIBinding")
			err = k8sClient.Get(ctx, typeNamespacedName, apibinding)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreov1alpha1.APIBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: openchoreov1alpha1.APIBindingSpec{
						APIClassName:    "test-api-class",
						APIName:         "test-api",
						EnvironmentName: "test-env",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &openchoreov1alpha1.APIBinding{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance APIBinding")
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
