/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
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

	apiv1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/ptr"
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

			By("Ensuring the finalizer is added", func() {
				resource := &apiv1.Organization{}
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				Expect(err).NotTo(HaveOccurred())

				Expect(resource.Finalizers).To(ContainElement(OrgCleanUpFinalizer))
			})

			result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
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

	Describe("delete an organization resource", func() {
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

		It("should successfully reconcile the organization resource even after deletion", func() {
			controllerReconciler := &Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			// first reconciliation to get the finalizer created
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// second reconciliation to get the namespace created
			result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})

		// since the namespace deletion not supported in envtest, the following test only checks
		// if the namespace resource's status is terminating and owner reference
		// more info: https://github.com/kubernetes-sigs/kubebuilder/blob/master/docs/book/src/reference/envtest.md#testing-considerations
		It("should have deleted the namespace for the organization", func() {
			namespace := &corev1.Namespace{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: orgName}, namespace)

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

			Expect(namespace.Status.Phase).To(Equal(corev1.NamespaceTerminating))
		})
	})

	Describe("test organization delete finalizer", func() {
		const orgNameDelete = "test-organization-delete"

		typeNamespacedName := types.NamespacedName{
			Name: orgNameDelete,
		}

		It("should create, reconcile, delete, and verify deletion of the organization resource in order", func() {
			// 1. Check if the organization exists, if not, create it
			By("Ensuring the organization resource exists")
			orgDelete := &apiv1.Organization{}
			err := k8sClient.Get(ctx, typeNamespacedName, orgDelete)
			if err != nil && apierrors.IsNotFound(err) {
				org := &apiv1.Organization{
					ObjectMeta: metav1.ObjectMeta{
						Name: orgNameDelete,
					},
				}
				Expect(k8sClient.Create(ctx, org)).To(Succeed())
			}

			// 2. Reconcile to add finalizer
			// In the following test, it is trying to simulate the finalizer behavior when organization is being deleted.
			// There is a limitation with namespace deletion in envtest,
			//   more info: https://github.com/kubernetes-sigs/kubebuilder/blob/master/docs/book/src/reference/envtest.md#namespace-usage-limitation
			// So it cannot simulate the full flow of organization deletion.
			// Due to this, organization resource will only reconcile once to ensure the
			//   finalizer is added, but not the namespace created.
			By("Reconciling to add finalizer")
			controllerReconciler := &Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(result.Requeue).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())

			// 3. Ensure finalizer is added
			By("Ensuring the finalizer is added")
			resource := &apiv1.Organization{}
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(resource.Finalizers).To(ContainElement(OrgCleanUpFinalizer))

			// 4. Delete the organization resource
			By("Deleting the organization resource")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			// 5. Reconcile after deletion to trigger finalizer logic - Attempt 1 to update status conditions
			By("Reconciling the organization resource and processing finalizer removal")
			result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// 6. Reconcile after deletion to trigger finalizer logic - Attempt 2 to delete finalizer
			By("Reconciling the organization resource and processing finalizer removal")
			result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// 6. Verify the organization resource is deleted
			By("Ensuring the organization resource is deleted after finalizer removal")
			resource = &apiv1.Organization{}
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, typeNamespacedName, resource))).To(BeTrue())
		})
	})
})
