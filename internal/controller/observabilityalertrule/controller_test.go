// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertrule

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ObservabilityAlertRule Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		observabilityalertrule := &openchoreov1alpha1.ObservabilityAlertRule{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ObservabilityAlertRule")
			err := k8sClient.Get(ctx, typeNamespacedName, observabilityalertrule)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreov1alpha1.ObservabilityAlertRule{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: openchoreov1alpha1.ObservabilityAlertRuleSpec{
						Name:                resourceName,
						NotificationChannel: "test-channel",
						Source: openchoreov1alpha1.ObservabilityAlertSource{
							Type:  openchoreov1alpha1.ObservabilityAlertSourceTypeLog,
							Query: "error",
						},
						Condition: openchoreov1alpha1.ObservabilityAlertCondition{
							Window:    metav1.Duration{Duration: 5 * time.Minute},
							Interval:  metav1.Duration{Duration: 1 * time.Minute},
							Operator:  openchoreov1alpha1.ObservabilityAlertConditionOperatorGt,
							Threshold: 10,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &openchoreov1alpha1.ObservabilityAlertRule{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ObservabilityAlertRule")
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
})
