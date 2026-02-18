// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("ReleaseBinding Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		releasebinding := &openchoreodevv1alpha1.ReleaseBinding{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ReleaseBinding")
			err := k8sClient.Get(ctx, typeNamespacedName, releasebinding)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreodevv1alpha1.ReleaseBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: openchoreodevv1alpha1.ReleaseBindingSpec{
						Owner: openchoreodevv1alpha1.ReleaseBindingOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						Environment: "test-env",
						ReleaseName: "test-release",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup logic after each test, like removing the resource instance.
			resource := &openchoreodevv1alpha1.ReleaseBinding{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ReleaseBinding")
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

	Context("buildMetadataContext", func() {
		var reconciler *Reconciler

		BeforeEach(func() {
			reconciler = &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		It("should include all required fields in pod selectors", func() {
			By("Creating test resources")
			namespaceName := "test-namespace"
			projectName := "test-project"
			componentName := "test-component"
			environmentName := "test-env"
			componentUID := types.UID("component-uid-123")
			projectUID := types.UID("project-uid-456")
			environmentUID := types.UID("environment-uid-789")
			dataPlaneUID := types.UID("dataplane-uid-abc")
			dataPlaneName := "test-dataplane"

			componentRelease := &openchoreodevv1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-release",
					Namespace: namespaceName,
				},
				Spec: openchoreodevv1alpha1.ComponentReleaseSpec{
					Owner: openchoreodevv1alpha1.ComponentReleaseOwner{
						ProjectName:   projectName,
						ComponentName: componentName,
					},
				},
			}

			component := &openchoreodevv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      componentName,
					Namespace: namespaceName,
					UID:       componentUID,
				},
			}

			project := &openchoreodevv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      projectName,
					Namespace: namespaceName,
					UID:       projectUID,
				},
			}

			dataPlane := &openchoreodevv1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name: dataPlaneName,
					UID:  dataPlaneUID,
				},
			}

			environment := &openchoreodevv1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      environmentName,
					Namespace: namespaceName,
					UID:       environmentUID,
				},
			}

			By("Building metadata context")
			metadataContext := reconciler.buildMetadataContext(
				componentRelease,
				component,
				project,
				dataPlane,
				environment,
				environmentName,
			)

			By("Verifying pod selectors include all required fields")
			Expect(metadataContext.PodSelectors).NotTo(BeNil())
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyNamespaceName, namespaceName))
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyProjectName, projectName))
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyComponentName, componentName))
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyEnvironmentName, environmentName))
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyComponentUID, string(componentUID)))
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyEnvironmentUID, string(environmentUID)))
			Expect(metadataContext.PodSelectors).To(HaveKeyWithValue(labels.LabelKeyProjectUID, string(projectUID)))

			By("Verifying pod selectors have exactly 7 entries")
			Expect(metadataContext.PodSelectors).To(HaveLen(7))

			By("Verifying standard labels also include all required fields")
			Expect(metadataContext.Labels).NotTo(BeNil())
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyNamespaceName, namespaceName))
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyProjectName, projectName))
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyComponentName, componentName))
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyEnvironmentName, environmentName))
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyComponentUID, string(componentUID)))
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyEnvironmentUID, string(environmentUID)))
			Expect(metadataContext.Labels).To(HaveKeyWithValue(labels.LabelKeyProjectUID, string(projectUID)))
		})
	})
})
