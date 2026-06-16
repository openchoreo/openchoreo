// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectrelease

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ProjectRelease Controller", func() {
	Context("When reconciling a ProjectRelease resource", func() {
		const prName = "test-projectrelease"
		const prNamespace = "default"

		prNamespacedName := types.NamespacedName{
			Name:      prName,
			Namespace: prNamespace,
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the ProjectRelease resource")
			pr := &openchoreov1alpha1.ProjectRelease{}
			err := k8sClient.Get(ctx, prNamespacedName, pr)
			if err != nil && errors.IsNotFound(err) {
				pr = &openchoreov1alpha1.ProjectRelease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      prName,
						Namespace: prNamespace,
					},
					Spec: openchoreov1alpha1.ProjectReleaseSpec{
						Owner: openchoreov1alpha1.ProjectReleaseOwner{
							ProjectName: "test-project",
						},
						ProjectType: openchoreov1alpha1.ProjectReleaseProjectType{
							Kind: openchoreov1alpha1.ProjectTypeRefKindClusterProjectType,
							Name: "standard-project",
							Spec: openchoreov1alpha1.ProjectTypeSpec{
								Resources: []openchoreov1alpha1.ResourceTemplate{
									{
										ID: "default-deny-egress",
										Template: &runtime.RawExtension{
											Raw: []byte(`{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"deny"}}`),
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, pr)).To(Succeed())
			}

			By("Reconciling the ProjectRelease resource")
			prReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = prReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: prNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the ProjectRelease resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, prNamespacedName, pr)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the ProjectRelease resource")
			Expect(k8sClient.Delete(ctx, pr)).To(Succeed())
		})
	})
})
