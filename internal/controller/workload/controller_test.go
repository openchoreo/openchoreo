// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

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

var _ = Describe("Workload Controller", func() {
	Context("When reconciling a resource with containers map", func() {
		const resourceName = "test-resource-containers"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		workload := &openchoreov1alpha1.Workload{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Workload with containers map")
			err := k8sClient.Get(ctx, typeNamespacedName, workload)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: openchoreov1alpha1.WorkloadSpec{
						Owner: openchoreov1alpha1.WorkloadOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
							Containers: map[string]openchoreov1alpha1.Container{
								"main": {Image: "nginx:latest"},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &openchoreov1alpha1.Workload{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Workload")
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
		})
	})

	Context("When reconciling a resource with single container", func() {
		const resourceName = "test-resource-container"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		workload := &openchoreov1alpha1.Workload{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Workload with single container")
			err := k8sClient.Get(ctx, typeNamespacedName, workload)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreov1alpha1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: openchoreov1alpha1.WorkloadSpec{
						Owner: openchoreov1alpha1.WorkloadOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
							Container: &openchoreov1alpha1.Container{
								Image: "nginx:latest",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &openchoreov1alpha1.Workload{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Workload")
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
		})
	})
})
