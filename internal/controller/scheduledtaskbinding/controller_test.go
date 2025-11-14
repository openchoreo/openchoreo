// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtaskbinding

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

var _ = Describe("ScheduledTaskBinding Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		scheduledtaskbinding := &openchoreov1alpha1.ScheduledTaskBinding{}

		BeforeEach(func() {
			By("creating the ScheduledTaskClass")
			scheduledTaskClass := &openchoreov1alpha1.ScheduledTaskClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default",
					Namespace: "default",
				},
			}
			err := k8sClient.Create(ctx, scheduledTaskClass)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("creating the test DataPlane")
			dataplane := &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dataplane",
					Namespace: "default",
				},
				Spec: openchoreov1alpha1.DataPlaneSpec{
					// Minimal required fields for DataPlane
				},
			}
			err = k8sClient.Create(ctx, dataplane)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("creating the test Environment")
			environment := &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env",
					Namespace: "default",
				},
				Spec: openchoreov1alpha1.EnvironmentSpec{
					DataPlaneRef: "test-dataplane",
				},
			}
			err = k8sClient.Create(ctx, environment)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("creating the custom resource for the Kind ScheduledTaskBinding")
			err = k8sClient.Get(ctx, typeNamespacedName, scheduledtaskbinding)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreov1alpha1.ScheduledTaskBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: openchoreov1alpha1.ScheduledTaskBindingSpec{
						Owner: openchoreov1alpha1.ScheduledTaskOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						Environment:  "test-env",
						ClassName:    "default",
						WorkloadSpec: openchoreov1alpha1.WorkloadTemplateSpec{},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &openchoreov1alpha1.ScheduledTaskBinding{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ScheduledTaskBinding")
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
