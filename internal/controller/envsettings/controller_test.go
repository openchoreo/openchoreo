// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package envsettings

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

var _ = Describe("EnvSettings Controller", func() {
	Context("When reconciling an EnvSettings resource", func() {
		const envSettingsName = "test-envsettings"
		const namespace = "default"

		envSettingsNamespacedName := types.NamespacedName{
			Name:      envSettingsName,
			Namespace: namespace,
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the EnvSettings resource")
			envSettings := &openchoreov1alpha1.EnvSettings{}
			err := k8sClient.Get(ctx, envSettingsNamespacedName, envSettings)
			if err != nil && errors.IsNotFound(err) {
				envSettings = &openchoreov1alpha1.EnvSettings{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envSettingsName,
						Namespace: namespace,
					},
					Spec: openchoreov1alpha1.EnvSettingsSpec{
						Owner: openchoreov1alpha1.EnvSettingsOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						Environment: "dev",
					},
				}
				Expect(k8sClient.Create(ctx, envSettings)).To(Succeed())
			}

			By("Reconciling the EnvSettings resource")
			envSettingsReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = envSettingsReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: envSettingsNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the EnvSettings resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, envSettingsNamespacedName, envSettings)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the EnvSettings resource")
			Expect(k8sClient.Delete(ctx, envSettings)).To(Succeed())
		})
	})
})
