/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package organization

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiv1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/labels"
	"github.com/choreo-idp/choreo/internal/ptr"
)

var _ = Context("Organization Controller", func() {
	const orgName = "test-organization"

	typeNamespacedName := types.NamespacedName{
		Name: orgName,
	}

	Describe("create and reconcile an organization resource", func() {

		ctx := context.Background()

		organization := &apiv1.Organization{}

		It("should successfully create a custom resource for the kind organization", func() {
			By("creating a custom resource for the Kind Organization", func() {
				err := k8sClient.Get(ctx, typeNamespacedName, organization)
				if err != nil && errors.IsNotFound(err) {
					org := &apiv1.Organization{
						ObjectMeta: metav1.ObjectMeta{
							Name: orgName,
						},
					}
					Expect(k8sClient.Create(ctx, org)).To(Succeed())
				}
			})
		})

		It("should successfully reconcile the organization resource", func() {
			controllerReconciler := &Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})

		It("should have successfully created the expected namespace for organization", func() {
			By("checking that the namespace is eventually created")
			namespace := &corev1.Namespace{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: orgName}, namespace)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("verifying the namespace has the expected attributes")
			Expect(namespace.Name).To(Equal(orgName))
			Expect(namespace.Labels).To(HaveKeyWithValue(labels.LabelKeyManagedBy, labels.LabelValueManagedBy))
			Expect(namespace.Labels).To(HaveKeyWithValue(labels.LabelKeyOrganizationName, orgName))
			Expect(namespace.Labels).To(HaveKeyWithValue(labels.LabelKeyName, orgName))
		})

		It("should add finalizer to the organization resource upon creation", func() {
			resource := &apiv1.Organization{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Ensuring the finalizer is added")
			Expect(resource.Finalizers).To(ContainElement(organizationFinalizer))
		})

		It("should not return an error for reconciling non-existing organization", func() {
			const nonExistOrgName = "non-existing-organization"

			By("Reconciling the non-existing organization resource", func() {
				controllerReconciler := &Reconciler{
					Client:   k8sClient,
					Scheme:   k8sClient.Scheme(),
					Recorder: record.NewFakeRecorder(100),
				}

				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name: nonExistOrgName,
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
			})
		})

		When("update the organization", func() {
			It("should be able to update organization's namespace", func() {
				orgNamespace := &corev1.Namespace{}
				err := k8sClient.Get(ctx, typeNamespacedName, orgNamespace)
				Expect(err).NotTo(HaveOccurred())
				Expect(orgNamespace.ObjectMeta.Labels).NotTo(BeNil())

				By("Updating the organization's namespace resource labels", func() {
					orgNamespace.ObjectMeta.Labels = map[string]string{
						labels.LabelKeyManagedBy:        labels.LabelValueManagedBy,
						labels.LabelKeyOrganizationName: "new-org-name",
						labels.LabelKeyName:             "new-org-name",
					}
					err = k8sClient.Update(ctx, orgNamespace)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			It("should successfully reconcile the organization resource after update", func() {
				By("Reconciling the organization resource with updated namespace labels", func() {
					controllerReconciler := &Reconciler{
						Client:   k8sClient,
						Scheme:   k8sClient.Scheme(),
						Recorder: record.NewFakeRecorder(100),
					}

					result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
				})
			})

			It("should have successfully updated the namespace labels back original", func() {
				By("update the namespace labels with something else", func() {
					namespace := &corev1.Namespace{}
					err := k8sClient.Get(ctx, client.ObjectKey{Name: orgName}, namespace)
					Expect(err).NotTo(HaveOccurred())
					Expect(namespace.Name).To(Equal(orgName))
					Expect(namespace.Labels).To(HaveKeyWithValue(labels.LabelKeyManagedBy, labels.LabelValueManagedBy))
					Expect(namespace.Labels).To(HaveKeyWithValue(labels.LabelKeyOrganizationName, orgName))
					Expect(namespace.Labels).To(HaveKeyWithValue(labels.LabelKeyName, orgName))
				})
			})
		})
	})

	Describe("delete organization resource", func() {
		var uuidOfOrgResource types.UID
		It("should be able to delete the organization resource", func() {
			resource := &apiv1.Organization{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
			// saving the UUID of the resource to verify the owner reference
			uuidOfOrgResource = resource.GetUID()
			By("Cleanup the specific resource instance Organization")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should reconcile the organization resource and process finalizer removal", func() {
			controllerReconciler := &Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			// Reconcile after deletion to trigger finalizer logic
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Fetch resource again to check if finalizer is removed
			resource := &apiv1.Organization{}
			err = k8sClient.Get(ctx, typeNamespacedName, resource)

			if apierrors.IsNotFound(err) {
				// Resource should be deleted after finalizer handling
				By("Organization resource should be fully deleted after finalizer execution")
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(resource.Finalizers).NotTo(ContainElement(organizationFinalizer))
			}
		})

		It("should have deleted the namespace for the organization", func() {
			namespace := &corev1.Namespace{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: orgName}, namespace)

			// Since the envtest api server does not support owner reference deletion, the namespace will not be deleted
			// and the error will be nil. Hence validating just the owner reference
			// https://github.com/kubernetes-sigs/kubebuilder/blob/master/docs/book/src/reference/envtest.md#testing-considerations
			expectedOwnerReference := metav1.OwnerReference{
				Kind:               "Organization",
				APIVersion:         "core.choreo.dev/v1",
				UID:                uuidOfOrgResource,
				Name:               orgName,
				Controller:         ptr.Bool(true),
				BlockOwnerDeletion: ptr.Bool(true),
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(namespace.ObjectMeta.OwnerReferences).To(ContainElement(expectedOwnerReference))
		})
	})
})
