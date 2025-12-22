// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentworkflowrun

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ComponentWorkflowRun Finalizer", func() {
	Context("When managing finalizers", func() {
		const resourceName = "test-finalizer-run"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		AfterEach(func() {
			By("Cleaning up the ComponentWorkflowRun resource")
			resource := &openchoreodevv1alpha1.ComponentWorkflowRun{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				// Remove finalizer if present to allow deletion
				if controllerutil.ContainsFinalizer(resource, ComponentWorkflowRunCleanupFinalizer) {
					controllerutil.RemoveFinalizer(resource, ComponentWorkflowRunCleanupFinalizer)
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
				}
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should add the finalizer correctly", func() {
			By("Creating a ComponentWorkflowRun without finalizer")
			cwf := &openchoreodevv1alpha1.ComponentWorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.ComponentWorkflowRunSpec{
					Owner: openchoreodevv1alpha1.ComponentWorkflowOwner{
						ProjectName:   "test-project",
						ComponentName: "test-component",
					},
					Workflow: openchoreodevv1alpha1.ComponentWorkflowRunConfig{
						Name: "test-workflow",
						SystemParameters: openchoreodevv1alpha1.SystemParametersValues{
							Repository: openchoreodevv1alpha1.RepositoryValues{
								URL: "https://github.com/openchoreo/test-repo",
								Revision: openchoreodevv1alpha1.RepositoryRevisionValues{
									Branch: "main",
								},
								AppPath: ".",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cwf)).To(Succeed())

			By("Verifying the resource was created without finalizer")
			resource := &openchoreodevv1alpha1.ComponentWorkflowRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, ComponentWorkflowRunCleanupFinalizer)).To(BeFalse())

			By("Adding the finalizer")
			controllerutil.AddFinalizer(resource, ComponentWorkflowRunCleanupFinalizer)
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			By("Verifying the finalizer was added")
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, ComponentWorkflowRunCleanupFinalizer)).To(BeTrue())
		})

		It("should remove the finalizer correctly", func() {
			By("Creating a ComponentWorkflowRun with finalizer")
			cwf := &openchoreodevv1alpha1.ComponentWorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       resourceName,
					Namespace:  "default",
					Finalizers: []string{ComponentWorkflowRunCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.ComponentWorkflowRunSpec{
					Owner: openchoreodevv1alpha1.ComponentWorkflowOwner{
						ProjectName:   "test-project",
						ComponentName: "test-component",
					},
					Workflow: openchoreodevv1alpha1.ComponentWorkflowRunConfig{
						Name: "test-workflow",
						SystemParameters: openchoreodevv1alpha1.SystemParametersValues{
							Repository: openchoreodevv1alpha1.RepositoryValues{
								URL: "https://github.com/openchoreo/test-repo",
								Revision: openchoreodevv1alpha1.RepositoryRevisionValues{
									Branch: "main",
								},
								AppPath: ".",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cwf)).To(Succeed())

			By("Verifying the finalizer is present")
			resource := &openchoreodevv1alpha1.ComponentWorkflowRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, ComponentWorkflowRunCleanupFinalizer)).To(BeTrue())

			By("Removing the finalizer")
			controllerutil.RemoveFinalizer(resource, ComponentWorkflowRunCleanupFinalizer)
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			By("Verifying the finalizer was removed")
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, ComponentWorkflowRunCleanupFinalizer)).To(BeFalse())
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
			By("Cleaning up the ComponentWorkflowRun resource")
			resource := &openchoreodevv1alpha1.ComponentWorkflowRun{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				// Remove finalizer if present to allow deletion
				if controllerutil.ContainsFinalizer(resource, ComponentWorkflowRunCleanupFinalizer) {
					controllerutil.RemoveFinalizer(resource, ComponentWorkflowRunCleanupFinalizer)
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
				}
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should persist RunReference in status", func() {
			By("Creating a ComponentWorkflowRun")
			cwf := &openchoreodevv1alpha1.ComponentWorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.ComponentWorkflowRunSpec{
					Owner: openchoreodevv1alpha1.ComponentWorkflowOwner{
						ProjectName:   "test-project",
						ComponentName: "test-component",
					},
					Workflow: openchoreodevv1alpha1.ComponentWorkflowRunConfig{
						Name: "test-workflow",
						SystemParameters: openchoreodevv1alpha1.SystemParametersValues{
							Repository: openchoreodevv1alpha1.RepositoryValues{
								URL: "https://github.com/openchoreo/test-repo",
								Revision: openchoreodevv1alpha1.RepositoryRevisionValues{
									Branch: "main",
								},
								AppPath: ".",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cwf)).To(Succeed())

			By("Updating status with RunReference")
			resource := &openchoreodevv1alpha1.ComponentWorkflowRun{}
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
			By("Creating a ComponentWorkflowRun")
			cwf := &openchoreodevv1alpha1.ComponentWorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.ComponentWorkflowRunSpec{
					Owner: openchoreodevv1alpha1.ComponentWorkflowOwner{
						ProjectName:   "test-project",
						ComponentName: "test-component",
					},
					Workflow: openchoreodevv1alpha1.ComponentWorkflowRunConfig{
						Name: "test-workflow",
						SystemParameters: openchoreodevv1alpha1.SystemParametersValues{
							Repository: openchoreodevv1alpha1.RepositoryValues{
								URL: "https://github.com/openchoreo/test-repo",
								Revision: openchoreodevv1alpha1.RepositoryRevisionValues{
									Branch: "main",
								},
								AppPath: ".",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cwf)).To(Succeed())

			By("Updating status with Resources")
			resource := &openchoreodevv1alpha1.ComponentWorkflowRun{}
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

		It("should persist ImageStatus in status", func() {
			By("Creating a ComponentWorkflowRun")
			cwf := &openchoreodevv1alpha1.ComponentWorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.ComponentWorkflowRunSpec{
					Owner: openchoreodevv1alpha1.ComponentWorkflowOwner{
						ProjectName:   "test-project",
						ComponentName: "test-component",
					},
					Workflow: openchoreodevv1alpha1.ComponentWorkflowRunConfig{
						Name: "test-workflow",
						SystemParameters: openchoreodevv1alpha1.SystemParametersValues{
							Repository: openchoreodevv1alpha1.RepositoryValues{
								URL: "https://github.com/openchoreo/test-repo",
								Revision: openchoreodevv1alpha1.RepositoryRevisionValues{
									Branch: "main",
								},
								AppPath: ".",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cwf)).To(Succeed())

			By("Updating status with ImageStatus")
			resource := &openchoreodevv1alpha1.ComponentWorkflowRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())

			resource.Status.ImageStatus = openchoreodevv1alpha1.ComponentWorkflowImage{
				Image: "registry.example.com/myapp:v1.0.0",
			}
			Expect(k8sClient.Status().Update(ctx, resource)).To(Succeed())

			By("Verifying the ImageStatus was persisted")
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.ImageStatus.Image).To(Equal("registry.example.com/myapp:v1.0.0"))
		})
	})
})
