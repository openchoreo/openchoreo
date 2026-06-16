// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ProjectReleaseBinding Controller", func() {
	Context("When reconciling a ProjectReleaseBinding resource", func() {
		const prbName = "test-projectreleasebinding"
		const prbNamespace = "default"

		prbNamespacedName := types.NamespacedName{
			Name:      prbName,
			Namespace: prbNamespace,
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the ProjectReleaseBinding resource")
			prb := &openchoreov1alpha1.ProjectReleaseBinding{}
			err := k8sClient.Get(ctx, prbNamespacedName, prb)
			if err != nil && errors.IsNotFound(err) {
				prb = &openchoreov1alpha1.ProjectReleaseBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      prbName,
						Namespace: prbNamespace,
					},
					Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
						Owner: openchoreov1alpha1.ProjectReleaseBindingOwner{
							ProjectName: "test-project",
						},
						Environment: "dev",
					},
				}
				Expect(k8sClient.Create(ctx, prb)).To(Succeed())
			}

			By("Reconciling the ProjectReleaseBinding resource")
			prbReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = prbReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: prbNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the ProjectReleaseBinding resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, prbNamespacedName, prb)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the ProjectReleaseBinding resource")
			Expect(k8sClient.Delete(ctx, prb)).To(Succeed())
		})
	})
})
