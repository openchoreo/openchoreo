// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("DataPlane Controller", func() {
	Context("When reconciling a resource", func() {
		const dpName = "test-dataplane"

		// Namespace to keep the dataplane
		namespaceName := "test-namespace"

		ctx := context.Background()

		dpNamespacedName := types.NamespacedName{
			Name:      dpName,
			Namespace: namespaceName,
		}
		dataplane := &openchoreov1alpha1.DataPlane{}

		BeforeEach(func() {
			namespaceNamespacedName := types.NamespacedName{
				Name: namespaceName,
			}
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
				},
			}
			By("Creating namespace", func() {
				err := k8sClient.Get(ctx, namespaceNamespacedName, namespace)
				if err != nil && errors.IsNotFound(err) {
					Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
				}
			})

		})

		AfterEach(func() {
			By("Deleting the namespace", func() {
				namespace := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
			})
		})

		It("should successfully Create and reconcile the dataplane resource", func() {
			By("Creating the dataplane resource", func() {
				err := k8sClient.Get(ctx, dpNamespacedName, dataplane)
				if err != nil && errors.IsNotFound(err) {
					dp := &openchoreov1alpha1.DataPlane{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dpName,
							Namespace: namespaceName,
						},
					}
					Expect(k8sClient.Create(ctx, dp)).To(Succeed())
				}
			})

			By("Reconciling the dataplane resource", func() {
				dpReconciler := &Reconciler{
					Client:   k8sClient,
					Scheme:   k8sClient.Scheme(),
					Recorder: record.NewFakeRecorder(100),
				}
				result, err := dpReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: dpNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
			})

			By("Checking the dataplane resource", func() {
				dataPlane := &openchoreov1alpha1.DataPlane{}
				Eventually(func() error {
					return k8sClient.Get(ctx, dpNamespacedName, dataPlane)
				}, time.Second*10, time.Millisecond*500).Should(Succeed())
				Expect(dataPlane.Name).To(Equal(dpName))
				Expect(dataPlane.Namespace).To(Equal(namespaceName))
				Expect(dataPlane.Spec).NotTo(BeNil())
			})

			By("Deleting the dataplane resource", func() {
				err := k8sClient.Get(ctx, dpNamespacedName, dataplane)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Delete(ctx, dataplane)).To(Succeed())
			})

			By("Reconciling the dataplane resource after deletion", func() {
				dpReconciler := &Reconciler{
					Client:   k8sClient,
					Scheme:   k8sClient.Scheme(),
					Recorder: record.NewFakeRecorder(100),
				}
				result, err := dpReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: dpNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
			})

			// TODO: Need to find a way to get the index working inside tests
			// By("Checking the dataplane resource deletion", func() {
			// 	Eventually(func() error {
			// 		return k8sClient.Get(ctx, dpNamespacedName, dataplane)
			// 	}, time.Second*10, time.Millisecond*500).ShouldNot(Succeed())
			// })
		})
	})
})
