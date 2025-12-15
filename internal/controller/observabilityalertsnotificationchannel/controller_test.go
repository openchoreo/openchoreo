// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ObservabilityAlertsNotificationChannel Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		observabilityalertsnotificationchannel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ObservabilityAlertsNotificationChannel")
			err := k8sClient.Get(ctx, typeNamespacedName, observabilityalertsnotificationchannel)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelSpec{
						Type: openchoreodevv1alpha1.NotificationChannelTypeEmail,
						Config: openchoreodevv1alpha1.NotificationChannelConfig{
							EmailConfig: openchoreodevv1alpha1.EmailConfig{
								From: "test@example.com",
								To:   []string{"test@example.com"},
								SMTP: openchoreodevv1alpha1.SMTPConfig{
									Host: "smtp.example.com",
									Port: 587,
								},
								Template: &openchoreodevv1alpha1.EmailTemplate{
									Subject: "[${alert.severity}] - ${alert.name} Triggered",
									Body:    "Alert: ${alert.name} triggered at ${alert.startsAt}.\nSummary: ${alert.description}",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ObservabilityAlertsNotificationChannel")
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
