// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("ObservabilityAlertsNotificationChannel Controller", func() {
	var (
		testCtx   context.Context
		namespace string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		namespace = "default"
	})

	Context("When reconciling a non-existent resource", func() {
		It("should not return an error", func() {
			reconciler := &Reconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				K8sClientMgr: kubernetesClient.NewManager(),
				GatewayURL:   "http://localhost:8080",
			}

			result, err := reconciler.Reconcile(testCtx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent-channel",
					Namespace: namespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("When reconciling a resource without Environment", func() {
		var channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel

		BeforeEach(func() {
			channel = &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-channel-no-env",
					Namespace: namespace,
					Labels: map[string]string{
						labels.LabelKeyOrganizationName: "test-org",
					},
				},
				Spec: openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelSpec{
					Environment: "development",
					Type:        openchoreodevv1alpha1.NotificationChannelTypeEmail,
					Config: openchoreodevv1alpha1.NotificationChannelConfig{
						EmailConfig: openchoreodevv1alpha1.EmailConfig{
							From: "test@example.com",
							To:   []string{"test@example.com"},
							SMTP: openchoreodevv1alpha1.SMTPConfig{
								Host: "smtp.example.com",
								Port: 587,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(testCtx, channel)).To(Succeed())
		})

		AfterEach(func() {
			if channel != nil {
				_ = k8sClient.Delete(testCtx, channel)
			}
		})

		It("should return an error when no Environment exists", func() {
			reconciler := &Reconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				K8sClientMgr: kubernetesClient.NewManager(),
				GatewayURL:   "http://localhost:8080",
			}

			_, err := reconciler.Reconcile(testCtx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      channel.Name,
					Namespace: namespace,
				},
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get environment"))
		})
	})

	Context("When reconciling a resource with full hierarchy", func() {
		var (
			channel            *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel
			organization       *openchoreodevv1alpha1.Organization
			dataPlane          *openchoreodevv1alpha1.DataPlane
			environment        *openchoreodevv1alpha1.Environment
			observabilityPlane *openchoreodevv1alpha1.ObservabilityPlane
			opClient           client.Client
		)

		BeforeEach(func() {
			// Create Organization
			organization = &openchoreodevv1alpha1.Organization{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(testCtx, organization)).To(Succeed())

			// Create ObservabilityPlane with agent enabled
			observabilityPlane = &openchoreodevv1alpha1.ObservabilityPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-observability-plane",
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.ObservabilityPlaneSpec{
					Agent: &openchoreodevv1alpha1.AgentConfig{
						Enabled: true,
					},
					ObserverURL: "http://observer.example.com",
				},
			}
			Expect(k8sClient.Create(testCtx, observabilityPlane)).To(Succeed())

			// Create DataPlane with ObservabilityPlaneRef
			dataPlane = &openchoreodevv1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dataplane",
					Namespace: namespace,
					Labels: map[string]string{
						labels.LabelKeyOrganizationName: namespace,
						labels.LabelKeyName:             "test-dataplane",
					},
				},
				Spec: openchoreodevv1alpha1.DataPlaneSpec{
					ObservabilityPlaneRef: observabilityPlane.Name,
				},
			}
			Expect(k8sClient.Create(testCtx, dataPlane)).To(Succeed())

			// Create Environment with DataPlaneRef
			environment = &openchoreodevv1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "development",
					Namespace: namespace,
					Labels: map[string]string{
						labels.LabelKeyOrganizationName: namespace,
						labels.LabelKeyName:             "development",
					},
				},
				Spec: openchoreodevv1alpha1.EnvironmentSpec{
					DataPlaneRef: dataPlane.Name,
				},
			}
			Expect(k8sClient.Create(testCtx, environment)).To(Succeed())

			// Use the same client for testing (in real scenarios, this would be a proxy client)
			opClient = k8sClient

			// Create channel
			channel = &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "openchoreo.dev/v1alpha1",
					Kind:       "ObservabilityAlertsNotificationChannel",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-channel",
					Namespace: namespace,
					Labels: map[string]string{
						labels.LabelKeyOrganizationName: namespace,
					},
				},
				Spec: openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelSpec{
					Environment: environment.Name,
					Type:        openchoreodevv1alpha1.NotificationChannelTypeEmail,
					Config: openchoreodevv1alpha1.NotificationChannelConfig{
						EmailConfig: openchoreodevv1alpha1.EmailConfig{
							From: "test@example.com",
							To:   []string{"recipient1@example.com", "recipient2@example.com"},
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
			Expect(k8sClient.Create(testCtx, channel)).To(Succeed())
		})

		AfterEach(func() {
			if channel != nil {
				// Clean up ConfigMap and Secret
				configMap := &corev1.ConfigMap{}
				if err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, configMap); err == nil {
					_ = k8sClient.Delete(testCtx, configMap)
				}
				secret := &corev1.Secret{}
				if err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, secret); err == nil {
					_ = k8sClient.Delete(testCtx, secret)
				}
				_ = k8sClient.Delete(testCtx, channel)
			}
			if environment != nil {
				_ = k8sClient.Delete(testCtx, environment)
			}
			if dataPlane != nil {
				_ = k8sClient.Delete(testCtx, dataPlane)
			}
			if observabilityPlane != nil {
				_ = k8sClient.Delete(testCtx, observabilityPlane)
			}
			if organization != nil {
				_ = k8sClient.Delete(testCtx, organization)
			}
		})

		It("should successfully create ConfigMap and Secret", func() {
			// Create a test client manager that returns our test client
			// Pre-populate it with the test client using GetOrAddClient
			clientMgr := kubernetesClient.NewManager()
			key := "observabilityplane/default/test-observability-plane"
			_, err := clientMgr.GetOrAddClient(key, func() (client.Client, error) {
				return opClient, nil
			})
			Expect(err).NotTo(HaveOccurred())

			reconciler := &Reconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				K8sClientMgr: clientMgr,
				GatewayURL:   "http://localhost:8080",
			}

			result, err := reconciler.Reconcile(testCtx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      channel.Name,
					Namespace: namespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify ConfigMap was created
			configMap := &corev1.ConfigMap{}
			err = k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, configMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(configMap.Name).To(Equal(channel.Name))
			Expect(configMap.Namespace).To(Equal(channel.Namespace))
			Expect(configMap.Labels["app.kubernetes.io/managed-by"]).To(Equal("observabilityalertsnotificationchannel-controller"))
			Expect(configMap.Data["type"]).To(Equal("email"))
			Expect(configMap.Data["from"]).To(Equal("test@example.com"))
			Expect(configMap.Data["to"]).To(ContainSubstring("recipient1@example.com"))
			Expect(configMap.Data["smtp.host"]).To(Equal("smtp.example.com"))
			Expect(configMap.Data["smtp.port"]).To(Equal("587"))
			Expect(configMap.Data["template.subject"]).To(Equal("[${alert.severity}] - ${alert.name} Triggered"))
			Expect(configMap.Data["template.body"]).To(ContainSubstring("Alert: ${alert.name}"))

			// Verify Secret was created
			secret := &corev1.Secret{}
			err = k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Name).To(Equal(channel.Name))
			Expect(secret.Namespace).To(Equal(channel.Namespace))
			Expect(secret.Type).To(Equal(corev1.SecretTypeOpaque))
		})
	})

	Context("When reconciling a resource with SMTP auth", func() {
		var (
			channel            *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel
			organization       *openchoreodevv1alpha1.Organization
			dataPlane          *openchoreodevv1alpha1.DataPlane
			environment        *openchoreodevv1alpha1.Environment
			observabilityPlane *openchoreodevv1alpha1.ObservabilityPlane
			opClient           client.Client
		)

		BeforeEach(func() {
			// Create Organization
			organization = &openchoreodevv1alpha1.Organization{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(testCtx, organization)).To(Succeed())

			// Create ObservabilityPlane
			observabilityPlane = &openchoreodevv1alpha1.ObservabilityPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-observability-plane-auth",
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.ObservabilityPlaneSpec{
					Agent: &openchoreodevv1alpha1.AgentConfig{
						Enabled: true,
					},
					ObserverURL: "http://observer.example.com",
				},
			}
			Expect(k8sClient.Create(testCtx, observabilityPlane)).To(Succeed())

			// Create DataPlane with ObservabilityPlaneRef
			dataPlane = &openchoreodevv1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dataplane-auth",
					Namespace: namespace,
					Labels: map[string]string{
						labels.LabelKeyOrganizationName: namespace,
						labels.LabelKeyName:             "test-dataplane-auth",
					},
				},
				Spec: openchoreodevv1alpha1.DataPlaneSpec{
					ObservabilityPlaneRef: observabilityPlane.Name,
				},
			}
			Expect(k8sClient.Create(testCtx, dataPlane)).To(Succeed())

			// Create Environment with DataPlaneRef
			environment = &openchoreodevv1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "development-auth",
					Namespace: namespace,
					Labels: map[string]string{
						labels.LabelKeyOrganizationName: namespace,
						labels.LabelKeyName:             "development-auth",
					},
				},
				Spec: openchoreodevv1alpha1.EnvironmentSpec{
					DataPlaneRef: dataPlane.Name,
				},
			}
			Expect(k8sClient.Create(testCtx, environment)).To(Succeed())

			opClient = k8sClient

			channel = &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-channel-auth",
					Namespace: namespace,
					Labels: map[string]string{
						labels.LabelKeyOrganizationName: namespace,
					},
				},
				Spec: openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelSpec{
					Environment: environment.Name,
					Type:        openchoreodevv1alpha1.NotificationChannelTypeEmail,
					Config: openchoreodevv1alpha1.NotificationChannelConfig{
						EmailConfig: openchoreodevv1alpha1.EmailConfig{
							From: "test@example.com",
							To:   []string{"test@example.com"},
							SMTP: openchoreodevv1alpha1.SMTPConfig{
								Host: "smtp.example.com",
								Port: 587,
								Auth: &openchoreodevv1alpha1.SMTPAuth{
									Username: &openchoreodevv1alpha1.SecretValueFrom{
										SecretKeyRef: &openchoreodevv1alpha1.SecretKeyRef{
											Name: "smtp-auth-secret",
											Key:  "username",
										},
									},
									Password: &openchoreodevv1alpha1.SecretValueFrom{
										SecretKeyRef: &openchoreodevv1alpha1.SecretKeyRef{
											Name: "smtp-auth-secret",
											Key:  "password",
										},
									},
								},
								TLS: &openchoreodevv1alpha1.SMTPTLSConfig{
									InsecureSkipVerify: true,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(testCtx, channel)).To(Succeed())
		})

		AfterEach(func() {
			if channel != nil {
				configMap := &corev1.ConfigMap{}
				if err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, configMap); err == nil {
					_ = k8sClient.Delete(testCtx, configMap)
				}
				secret := &corev1.Secret{}
				if err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, secret); err == nil {
					_ = k8sClient.Delete(testCtx, secret)
				}
				_ = k8sClient.Delete(testCtx, channel)
			}
			if environment != nil {
				_ = k8sClient.Delete(testCtx, environment)
			}
			if dataPlane != nil {
				_ = k8sClient.Delete(testCtx, dataPlane)
			}
			if observabilityPlane != nil {
				_ = k8sClient.Delete(testCtx, observabilityPlane)
			}
			if organization != nil {
				_ = k8sClient.Delete(testCtx, organization)
			}
		})

		It("should create Secret with SMTP auth references and ConfigMap with TLS config", func() {
			// Create a test client manager that returns our test client
			clientMgr := kubernetesClient.NewManager()
			key := "observabilityplane/default/test-observability-plane-auth"
			_, err := clientMgr.GetOrAddClient(key, func() (client.Client, error) {
				return opClient, nil
			})
			Expect(err).NotTo(HaveOccurred())

			reconciler := &Reconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				K8sClientMgr: clientMgr,
				GatewayURL:   "http://localhost:8080",
			}

			result, err2 := reconciler.Reconcile(testCtx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      channel.Name,
					Namespace: namespace,
				},
			})

			Expect(err2).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify ConfigMap has TLS config
			configMap := &corev1.ConfigMap{}
			err = k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, configMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(configMap.Data["smtp.tls.insecureSkipVerify"]).To(Equal("true"))

			// Verify Secret has auth references
			secret := &corev1.Secret{}
			err = k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Data).To(HaveKey("smtp.auth.username.secret"))
			Expect(secret.Data).To(HaveKey("smtp.auth.password.secret"))
			Expect(string(secret.Data["smtp.auth.username.secret"])).To(Equal("smtp-auth-secret/username"))
			Expect(string(secret.Data["smtp.auth.password.secret"])).To(Equal("smtp-auth-secret/password"))
		})
	})

	Context("When testing createConfigMap", func() {
		It("should create ConfigMap with correct data", func() {
			channel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-channel",
					Namespace: namespace,
					UID:       types.UID("test-uid"),
				},
				TypeMeta: metav1.TypeMeta{
					APIVersion: "openchoreo.dev/v1alpha1",
					Kind:       "ObservabilityAlertsNotificationChannel",
				},
				Spec: openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelSpec{
					Environment: "development",
					Type:        openchoreodevv1alpha1.NotificationChannelTypeEmail,
					Config: openchoreodevv1alpha1.NotificationChannelConfig{
						EmailConfig: openchoreodevv1alpha1.EmailConfig{
							From: "sender@example.com",
							To:   []string{"recipient@example.com"},
							SMTP: openchoreodevv1alpha1.SMTPConfig{
								Host: "smtp.example.com",
								Port: 465,
							},
						},
					},
				},
			}

			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			configMap := reconciler.createConfigMap(channel)

			Expect(configMap.Name).To(Equal(channel.Name))
			Expect(configMap.Namespace).To(Equal(channel.Namespace))
			Expect(configMap.Data["type"]).To(Equal("email"))
			Expect(configMap.Data["from"]).To(Equal("sender@example.com"))
			Expect(configMap.Data["smtp.host"]).To(Equal("smtp.example.com"))
			Expect(configMap.Data["smtp.port"]).To(Equal("465"))
			Expect(configMap.Labels["app.kubernetes.io/managed-by"]).To(Equal("observabilityalertsnotificationchannel-controller"))
		})
	})

	Context("When testing createSecret", func() {
		It("should create Secret with correct structure", func() {
			channel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-channel",
					Namespace: namespace,
					UID:       types.UID("test-uid"),
				},
				TypeMeta: metav1.TypeMeta{
					APIVersion: "openchoreo.dev/v1alpha1",
					Kind:       "ObservabilityAlertsNotificationChannel",
				},
				Spec: openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelSpec{
					Environment: "development",
					Type:        openchoreodevv1alpha1.NotificationChannelTypeEmail,
					Config: openchoreodevv1alpha1.NotificationChannelConfig{
						EmailConfig: openchoreodevv1alpha1.EmailConfig{
							From: "test@example.com",
							To:   []string{"test@example.com"},
							SMTP: openchoreodevv1alpha1.SMTPConfig{
								Host: "smtp.example.com",
								Port: 587,
							},
						},
					},
				},
			}

			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			secret := reconciler.createSecret(channel)

			Expect(secret.Name).To(Equal(channel.Name))
			Expect(secret.Namespace).To(Equal(channel.Namespace))
			Expect(secret.Type).To(Equal(corev1.SecretTypeOpaque))
			Expect(secret.Labels["app.kubernetes.io/managed-by"]).To(Equal("observabilityalertsnotificationchannel-controller"))
		})
	})
})
