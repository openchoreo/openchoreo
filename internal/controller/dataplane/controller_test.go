// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiv1 "github.com/openchoreo/openchoreo/api/v1"
	org "github.com/openchoreo/openchoreo/internal/controller/organization"
	"github.com/openchoreo/openchoreo/internal/controller/testutils"
)

var _ = Describe("DataPlane Controller", func() {
	Context("When reconciling a resource", func() {
		const dpName = "test-dataplane"

		// Organization resource to keep the dataplane
		orgName := "test-organization"

		ctx := context.Background()

		dpNamespacedName := types.NamespacedName{
			Name:      dpName,
			Namespace: orgName,
		}
		dataplane := &apiv1.DataPlane{}

		BeforeEach(func() {
			orgNamespacedName := types.NamespacedName{
				Name: orgName,
			}
			organization := &apiv1.Organization{
				ObjectMeta: metav1.ObjectMeta{
					Name: orgName,
				},
			}
			By("Creating and reconciling organization resource", func() {
				orgReconciler := &org.Reconciler{
					Client:   k8sClient,
					Scheme:   k8sClient.Scheme(),
					Recorder: record.NewFakeRecorder(100),
				}
				testutils.CreateAndReconcileResourceWithCycles(ctx, k8sClient, organization, orgReconciler,
					orgNamespacedName, 2)
			})

		})

		AfterEach(func() {
			By("Deleting the organization resource", func() {
				org := &apiv1.Organization{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: orgName}, org)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Delete(ctx, org)).To(Succeed())
			})
		})

		It("should successfully Create and reconcile the dataplane resource", func() {
			By("Creating the dataplane resource", func() {
				err := k8sClient.Get(ctx, dpNamespacedName, dataplane)
				if err != nil && errors.IsNotFound(err) {
					dp := &apiv1.DataPlane{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dpName,
							Namespace: orgName,
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
				dataPlane := &apiv1.DataPlane{}
				Eventually(func() error {
					return k8sClient.Get(ctx, dpNamespacedName, dataPlane)
				}, time.Second*10, time.Millisecond*500).Should(Succeed())
				Expect(dataPlane.Name).To(Equal(dpName))
				Expect(dataPlane.Namespace).To(Equal(orgName))
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
