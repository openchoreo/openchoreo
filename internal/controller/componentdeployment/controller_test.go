// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentdeployment

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	componentpipeline "github.com/openchoreo/openchoreo/internal/crd-renderer/component-pipeline"
)

var _ = Describe("ComponentDeployment Controller", func() {
	Context("When reconciling an ComponentDeployment resource", func() {
		const componentDeploymentName = "test-componentdeployment"
		const namespace = "default"

		componentDeploymentNamespacedName := types.NamespacedName{
			Name:      componentDeploymentName,
			Namespace: namespace,
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the ComponentDeployment resource")
			componentDeployment := &openchoreov1alpha1.ComponentDeployment{}
			err := k8sClient.Get(ctx, componentDeploymentNamespacedName, componentDeployment)
			if err != nil && errors.IsNotFound(err) {
				componentDeployment = &openchoreov1alpha1.ComponentDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      componentDeploymentName,
						Namespace: namespace,
					},
					Spec: openchoreov1alpha1.ComponentDeploymentSpec{
						Owner: openchoreov1alpha1.ComponentDeploymentOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						Environment: "dev",
					},
				}
				Expect(k8sClient.Create(ctx, componentDeployment)).To(Succeed())
			}

			By("Reconciling the ComponentDeployment resource")
			componentDeploymentReconciler := &Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Pipeline: componentpipeline.NewPipeline(),
			}
			_, err = componentDeploymentReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: componentDeploymentNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the ComponentDeployment resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, componentDeploymentNamespacedName, componentDeployment)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the ComponentDeployment resource")
			Expect(k8sClient.Delete(ctx, componentDeployment)).To(Succeed())
		})
	})
})
