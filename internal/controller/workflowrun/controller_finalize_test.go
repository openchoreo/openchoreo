// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("WorkflowRun Finalizer", func() {
	Context("When managing finalizers", func() {
		const resourceName = "test-finalizer-run"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		AfterEach(func() {
			By("Cleaning up the WorkflowRun resource")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				// Remove finalizer if present to allow deletion
				if controllerutil.ContainsFinalizer(resource, WorkflowRunCleanupFinalizer) {
					controllerutil.RemoveFinalizer(resource, WorkflowRunCleanupFinalizer)
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
				}
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should add the finalizer correctly", func() {
			By("Creating a WorkflowRun without finalizer")
			cwf := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{
						Name: "test-workflow",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cwf)).To(Succeed())

			By("Verifying the resource was created without finalizer")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, WorkflowRunCleanupFinalizer)).To(BeFalse())

			By("Adding the finalizer")
			controllerutil.AddFinalizer(resource, WorkflowRunCleanupFinalizer)
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			By("Verifying the finalizer was added")
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, WorkflowRunCleanupFinalizer)).To(BeTrue())
		})

		It("should remove the finalizer correctly", func() {
			By("Creating a WorkflowRun with finalizer")
			cwf := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{WorkflowRunCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{
						Name: "test-workflow",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cwf)).To(Succeed())

			By("Verifying the finalizer is present")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, WorkflowRunCleanupFinalizer)).To(BeTrue())

			By("Removing the finalizer")
			controllerutil.RemoveFinalizer(resource, WorkflowRunCleanupFinalizer)
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			By("Verifying the finalizer was removed")
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, WorkflowRunCleanupFinalizer)).To(BeFalse())
		})
	})

	Context("When testing status with Resources and RunReference", func() {
		const resourceName = "test-status-run"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		AfterEach(func() {
			By("Cleaning up the WorkflowRun resource")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				// Remove finalizer if present to allow deletion
				if controllerutil.ContainsFinalizer(resource, WorkflowRunCleanupFinalizer) {
					controllerutil.RemoveFinalizer(resource, WorkflowRunCleanupFinalizer)
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
				}
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should persist RunReference in status", func() {
			By("Creating a WorkflowRun")
			cwf := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{
						Name: "test-workflow",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cwf)).To(Succeed())

			By("Updating status with RunReference")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())

			resource.Status.RunReference = &openchoreodevv1alpha1.ResourceReference{
				APIVersion: "argoproj.io/v1alpha1",
				Kind:       "Workflow",
				Name:       "test-workflow-run",
				Namespace:  "build-namespace",
			}
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())

			By("Verifying the RunReference was persisted")
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.RunReference).NotTo(BeNil())
			Expect(resource.Status.RunReference.Name).To(Equal("test-workflow-run"))
			Expect(resource.Status.RunReference.Namespace).To(Equal("build-namespace"))
		})

		It("should persist Resources in status", func() {
			By("Creating a WorkflowRun")
			cwf := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{
						Name: "test-workflow",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cwf)).To(Succeed())

			By("Updating status with Resources")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())

			resources := []openchoreodevv1alpha1.ResourceReference{
				{
					APIVersion: "v1",
					Kind:       "Secret",
					Name:       "registry-credentials",
					Namespace:  "build-namespace",
				},
				{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "build-config",
					Namespace:  "build-namespace",
				},
			}
			resource.Status.Resources = &resources
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())

			By("Verifying the Resources were persisted")
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.Resources).NotTo(BeNil())
			Expect(*resource.Status.Resources).To(HaveLen(2))
			Expect((*resource.Status.Resources)[0].Kind).To(Equal("Secret"))
			Expect((*resource.Status.Resources)[1].Kind).To(Equal("ConfigMap"))
		})
	})
})
