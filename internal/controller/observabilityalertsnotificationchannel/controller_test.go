// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
				},
				Spec: openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelSpec{
					Environment: "development",
					Type:        openchoreodevv1alpha1.NotificationChannelTypeEmail,
					EmailConfig: &openchoreodevv1alpha1.EmailConfig{
						From: "test@example.com",
						To:   []string{"test@example.com"},
						SMTP: openchoreodevv1alpha1.SMTPConfig{
							Host: "smtp.example.com",
							Port: 587,
							Auth: &openchoreodevv1alpha1.SMTPAuth{
								Username: &openchoreodevv1alpha1.SecretValueFrom{},
								Password: &openchoreodevv1alpha1.SecretValueFrom{},
							},
							TLS: &openchoreodevv1alpha1.SMTPTLSConfig{},
						},
						Template: &openchoreodevv1alpha1.EmailTemplate{
							Subject: "Test Subject",
							Body:    "Test Body",
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
			dataPlane          *openchoreodevv1alpha1.DataPlane
			environment        *openchoreodevv1alpha1.Environment
			observabilityPlane *openchoreodevv1alpha1.ObservabilityPlane
			opClient           client.Client
		)

		BeforeEach(func() {
			// Create ObservabilityPlane with agent enabled
			observabilityPlane = &openchoreodevv1alpha1.ObservabilityPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-observability-plane",
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.ObservabilityPlaneSpec{
					ClusterAgent: openchoreodevv1alpha1.ClusterAgentConfig{
						ClientCA: openchoreodevv1alpha1.ValueFrom{
							Value: "test-ca-cert",
						},
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
				},
				Spec: openchoreodevv1alpha1.EnvironmentSpec{
					DataPlaneRef: &openchoreodevv1alpha1.DataPlaneRef{
						Kind: openchoreodevv1alpha1.DataPlaneRefKindDataPlane,
						Name: dataPlane.Name,
					},
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
				},
				Spec: openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelSpec{
					Environment: environment.Name,
					Type:        openchoreodevv1alpha1.NotificationChannelTypeEmail,
					EmailConfig: &openchoreodevv1alpha1.EmailConfig{
						From: "test@example.com",
						To:   []string{"recipient1@example.com", "recipient2@example.com"},
						SMTP: openchoreodevv1alpha1.SMTPConfig{
							Host: "smtp.example.com",
							Port: 587,
							Auth: &openchoreodevv1alpha1.SMTPAuth{
								Username: &openchoreodevv1alpha1.SecretValueFrom{},
								Password: &openchoreodevv1alpha1.SecretValueFrom{},
							},
							TLS: &openchoreodevv1alpha1.SMTPTLSConfig{},
						},
						Template: &openchoreodevv1alpha1.EmailTemplate{
							Subject: "[${alert.severity}] - ${alert.name} Triggered",
							Body:    "Alert: ${alert.name} triggered at ${alert.startsAt}.\nSummary: ${alert.description}",
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

				// If deletion gets stuck on a finalizer, remove it to unblock cleanup between specs.
				existing := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}
				if err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, existing); err == nil {
					if controllerutil.RemoveFinalizer(existing, NotificationChannelCleanupFinalizer) {
						_ = k8sClient.Update(testCtx, existing)
					}
				}

				// Wait for the channel (and associated resources) to be fully removed to avoid
				// AlreadyExists errors between specs when a deletion is still in progress.
				Eventually(func() bool {
					err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{})
					return apierrors.IsNotFound(err)
				}, time.Second*30, time.Millisecond*200).Should(BeTrue())
			}
			if environment != nil {
				_ = k8sClient.Delete(testCtx, environment)
				Eventually(func() bool {
					err := k8sClient.Get(testCtx, types.NamespacedName{Name: environment.Name, Namespace: namespace}, &openchoreodevv1alpha1.Environment{})
					return apierrors.IsNotFound(err)
				}, time.Second*30, time.Millisecond*200).Should(BeTrue())
			}
			if dataPlane != nil {
				_ = k8sClient.Delete(testCtx, dataPlane)
				Eventually(func() bool {
					err := k8sClient.Get(testCtx, types.NamespacedName{Name: dataPlane.Name, Namespace: namespace}, &openchoreodevv1alpha1.DataPlane{})
					return apierrors.IsNotFound(err)
				}, time.Second*30, time.Millisecond*200).Should(BeTrue())
			}
			if observabilityPlane != nil {
				_ = k8sClient.Delete(testCtx, observabilityPlane)
				Eventually(func() bool {
					err := k8sClient.Get(testCtx, types.NamespacedName{Name: observabilityPlane.Name, Namespace: namespace}, &openchoreodevv1alpha1.ObservabilityPlane{})
					return apierrors.IsNotFound(err)
				}, time.Second*30, time.Millisecond*200).Should(BeTrue())
			}
		})

		It("should successfully create ConfigMap and Secret", func() {
			// Create a test client manager that returns our test client
			// Pre-populate it with the test client using GetOrAddClient
			clientMgr := kubernetesClient.NewManager()
			key := "v2/observabilityplane/default/test-observability-plane"
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

			// Verify finalizer is added
			updatedChannel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}
			err = k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, updatedChannel)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedChannel.Finalizers).To(ContainElement(NotificationChannelCleanupFinalizer))

			// Verify ConfigMap was created
			configMap := &corev1.ConfigMap{}
			err = k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, configMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(configMap.Name).To(Equal(channel.Name))
			Expect(configMap.Namespace).To(Equal(channel.Namespace))
			Expect(configMap.Labels["app.kubernetes.io/managed-by"]).To(Equal("observabilityalertsnotificationchannel-controller"))
			Expect(configMap.Labels[labels.LabelKeyNotificationChannelName]).To(Equal(channel.Name))
			Expect(configMap.Data["type"]).To(Equal("email"))
			Expect(configMap.Data["isEnvDefault"]).To(Equal("true"))
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
			Expect(secret.Labels[labels.LabelKeyNotificationChannelName]).To(Equal(channel.Name))
		})

		It("should mark the first channel in an environment as default", func() {
			clientMgr := kubernetesClient.NewManager()
			key := "v2/observabilityplane/default/test-observability-plane"
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

			Eventually(func() bool {
				updatedChannel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, updatedChannel)
				if err != nil {
					return false
				}
				return updatedChannel.Spec.IsEnvDefault
			}, time.Second*5, time.Millisecond*200).Should(BeTrue())
		})
	})

	Context("When reconciling a resource with SMTP auth", func() {
		var (
			channel            *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel
			dataPlane          *openchoreodevv1alpha1.DataPlane
			environment        *openchoreodevv1alpha1.Environment
			observabilityPlane *openchoreodevv1alpha1.ObservabilityPlane
			opClient           client.Client
			smtpAuthSecret     *corev1.Secret
		)

		BeforeEach(func() {
			// Create the source secret that contains SMTP credentials
			smtpAuthSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "smtp-auth-secret",
					Namespace: namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"username": []byte("test-smtp-user"),
					"password": []byte("test-smtp-password"),
				},
			}
			Expect(k8sClient.Create(testCtx, smtpAuthSecret)).To(Succeed())

			// Create ObservabilityPlane
			observabilityPlane = &openchoreodevv1alpha1.ObservabilityPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-observability-plane-auth",
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.ObservabilityPlaneSpec{
					ClusterAgent: openchoreodevv1alpha1.ClusterAgentConfig{
						ClientCA: openchoreodevv1alpha1.ValueFrom{
							Value: "test-ca-cert",
						},
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
				},
				Spec: openchoreodevv1alpha1.EnvironmentSpec{
					DataPlaneRef: &openchoreodevv1alpha1.DataPlaneRef{
						Kind: openchoreodevv1alpha1.DataPlaneRefKindDataPlane,
						Name: dataPlane.Name,
					},
				},
			}
			Expect(k8sClient.Create(testCtx, environment)).To(Succeed())

			opClient = k8sClient

			channel = &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-channel-auth",
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelSpec{
					Environment: environment.Name,
					Type:        openchoreodevv1alpha1.NotificationChannelTypeEmail,
					EmailConfig: &openchoreodevv1alpha1.EmailConfig{
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
						Template: &openchoreodevv1alpha1.EmailTemplate{
							Subject: "Test Auth Subject",
							Body:    "Test Auth Body",
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
			if smtpAuthSecret != nil {
				_ = k8sClient.Delete(testCtx, smtpAuthSecret)
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
		})

		It("should create Secret with resolved SMTP auth credentials and ConfigMap with TLS config", func() {
			// Create a test client manager that returns our test client
			clientMgr := kubernetesClient.NewManager()
			key := "v2/observabilityplane/default/test-observability-plane-auth"
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

			// Verify Secret has resolved auth credentials (not references)
			secret := &corev1.Secret{}
			err = k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Data).To(HaveKey("smtp.auth.username"))
			Expect(secret.Data).To(HaveKey("smtp.auth.password"))
			Expect(string(secret.Data["smtp.auth.username"])).To(Equal("test-smtp-user"))
			Expect(string(secret.Data["smtp.auth.password"])).To(Equal("test-smtp-password"))
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
					Environment:  "development",
					IsEnvDefault: true,
					Type:         openchoreodevv1alpha1.NotificationChannelTypeEmail,
					EmailConfig: &openchoreodevv1alpha1.EmailConfig{
						From: "sender@example.com",
						To:   []string{"recipient@example.com"},
						SMTP: openchoreodevv1alpha1.SMTPConfig{
							Host: "smtp.example.com",
							Port: 465,
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
			Expect(configMap.Data["isEnvDefault"]).To(Equal("true"))
			Expect(configMap.Data["from"]).To(Equal("sender@example.com"))
			Expect(configMap.Data["smtp.host"]).To(Equal("smtp.example.com"))
			Expect(configMap.Data["smtp.port"]).To(Equal("465"))
			Expect(configMap.Labels["app.kubernetes.io/managed-by"]).To(Equal("observabilityalertsnotificationchannel-controller"))
			Expect(configMap.Labels[labels.LabelKeyNotificationChannelName]).To(Equal(channel.Name))
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
					EmailConfig: &openchoreodevv1alpha1.EmailConfig{
						From: "test@example.com",
						To:   []string{"test@example.com"},
						SMTP: openchoreodevv1alpha1.SMTPConfig{
							Host: "smtp.example.com",
							Port: 587,
						},
					},
				},
			}

			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			secret, err := reconciler.createSecret(context.Background(), channel)
			Expect(err).NotTo(HaveOccurred())

			Expect(secret.Name).To(Equal(channel.Name))
			Expect(secret.Namespace).To(Equal(channel.Namespace))
			Expect(secret.Type).To(Equal(corev1.SecretTypeOpaque))
			Expect(secret.Labels["app.kubernetes.io/managed-by"]).To(Equal("observabilityalertsnotificationchannel-controller"))
			Expect(secret.Labels[labels.LabelKeyNotificationChannelName]).To(Equal(channel.Name))
		})
	})

	Context("When testing finalizers", func() {
		var (
			channel            *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel
			dataPlane          *openchoreodevv1alpha1.DataPlane
			environment        *openchoreodevv1alpha1.Environment
			observabilityPlane *openchoreodevv1alpha1.ObservabilityPlane
			opClient           client.Client
			clientMgr          *kubernetesClient.KubeMultiClientManager
		)

		BeforeEach(func() {
			// Create ObservabilityPlane with agent enabled
			observabilityPlane = &openchoreodevv1alpha1.ObservabilityPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-observability-plane-finalizer",
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.ObservabilityPlaneSpec{
					ClusterAgent: openchoreodevv1alpha1.ClusterAgentConfig{
						ClientCA: openchoreodevv1alpha1.ValueFrom{
							Value: "test-ca-cert",
						},
					},
					ObserverURL: "http://observer.example.com",
				},
			}
			Expect(k8sClient.Create(testCtx, observabilityPlane)).To(Succeed())

			// Create DataPlane with ObservabilityPlaneRef
			dataPlane = &openchoreodevv1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dataplane-finalizer",
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.DataPlaneSpec{
					ObservabilityPlaneRef: observabilityPlane.Name,
				},
			}
			Expect(k8sClient.Create(testCtx, dataPlane)).To(Succeed())

			// Create Environment with DataPlaneRef
			environment = &openchoreodevv1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "development-finalizer",
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.EnvironmentSpec{
					DataPlaneRef: &openchoreodevv1alpha1.DataPlaneRef{
						Kind: openchoreodevv1alpha1.DataPlaneRefKindDataPlane,
						Name: dataPlane.Name,
					},
				},
			}
			Expect(k8sClient.Create(testCtx, environment)).To(Succeed())

			// Use the same client for testing (in real scenarios, this would be a proxy client)
			opClient = k8sClient

			// Create a test client manager
			clientMgr = kubernetesClient.NewManager()
			key := "v2/observabilityplane/default/test-observability-plane-finalizer"
			_, err := clientMgr.GetOrAddClient(key, func() (client.Client, error) {
				return opClient, nil
			})
			Expect(err).NotTo(HaveOccurred())
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
		})

		It("should add finalizer during reconciliation", func() {
			// Create channel without finalizer
			channel = &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-channel-finalizer",
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelSpec{
					Environment: environment.Name,
					Type:        openchoreodevv1alpha1.NotificationChannelTypeEmail,
					EmailConfig: &openchoreodevv1alpha1.EmailConfig{
						From: "test@example.com",
						To:   []string{"test@example.com"},
						SMTP: openchoreodevv1alpha1.SMTPConfig{
							Host: "smtp.example.com",
							Port: 587,
							Auth: &openchoreodevv1alpha1.SMTPAuth{
								Username: &openchoreodevv1alpha1.SecretValueFrom{},
								Password: &openchoreodevv1alpha1.SecretValueFrom{},
							},
							TLS: &openchoreodevv1alpha1.SMTPTLSConfig{},
						},
						Template: &openchoreodevv1alpha1.EmailTemplate{
							Subject: "Finalizer Test Subject",
							Body:    "Finalizer Test Body",
						},
					},
				},
			}
			Expect(k8sClient.Create(testCtx, channel)).To(Succeed())

			// Verify finalizer is not present initially
			createdChannel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}
			err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, createdChannel)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdChannel.Finalizers).NotTo(ContainElement(NotificationChannelCleanupFinalizer))

			// Reconcile
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

			// Verify finalizer is added
			updatedChannel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}
			err = k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, updatedChannel)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedChannel.Finalizers).To(ContainElement(NotificationChannelCleanupFinalizer))
		})

		It("should delete ConfigMap and Secret and remove finalizer during deletion", func() {
			// Create channel
			channel = &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-channel-delete",
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelSpec{
					Environment: environment.Name,
					Type:        openchoreodevv1alpha1.NotificationChannelTypeEmail,
					EmailConfig: &openchoreodevv1alpha1.EmailConfig{
						From: "test@example.com",
						To:   []string{"test@example.com"},
						SMTP: openchoreodevv1alpha1.SMTPConfig{
							Host: "smtp.example.com",
							Port: 587,
							Auth: &openchoreodevv1alpha1.SMTPAuth{
								Username: &openchoreodevv1alpha1.SecretValueFrom{},
								Password: &openchoreodevv1alpha1.SecretValueFrom{},
							},
							TLS: &openchoreodevv1alpha1.SMTPTLSConfig{},
						},
						Template: &openchoreodevv1alpha1.EmailTemplate{
							Subject: "Delete Finalizer Subject",
							Body:    "Delete Finalizer Body",
						},
					},
				},
			}
			Expect(k8sClient.Create(testCtx, channel)).To(Succeed())

			reconciler := &Reconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				K8sClientMgr: clientMgr,
				GatewayURL:   "http://localhost:8080",
			}

			// Reconcile to create ConfigMap and Secret and add finalizer
			_, err := reconciler.Reconcile(testCtx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      channel.Name,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify ConfigMap and Secret were created
			configMap := &corev1.ConfigMap{}
			err = k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, configMap)
			Expect(err).NotTo(HaveOccurred())

			secret := &corev1.Secret{}
			err = k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, secret)
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer is present
			updatedChannel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}
			err = k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, updatedChannel)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedChannel.Finalizers).To(ContainElement(NotificationChannelCleanupFinalizer))

			// Delete the channel
			Expect(k8sClient.Delete(testCtx, updatedChannel)).To(Succeed())

			// Wait for deletion timestamp to be set
			deletedChannel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}
			Eventually(func() bool {
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, deletedChannel)
				if err != nil {
					return false
				}
				return !deletedChannel.DeletionTimestamp.IsZero()
			}, time.Second*10, time.Millisecond*500).Should(BeTrue())

			// Reconcile to trigger finalization
			result, err := reconciler.Reconcile(testCtx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      channel.Name,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify ConfigMap is deleted
			Eventually(func() bool {
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, &corev1.ConfigMap{})
				return apierrors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*500).Should(BeTrue())

			// Verify Secret is deleted
			Eventually(func() bool {
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, &corev1.Secret{})
				return apierrors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*500).Should(BeTrue())

			// Verify finalizer is removed
			finalChannel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}
			Eventually(func() bool {
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, finalChannel)
				if err != nil {
					return apierrors.IsNotFound(err)
				}
				return !contains(finalChannel.Finalizers, NotificationChannelCleanupFinalizer)
			}, time.Second*10, time.Millisecond*500).Should(BeTrue())

			// Verify resource is eventually deleted
			Eventually(func() bool {
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{})
				return apierrors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*500).Should(BeTrue())
		})

		It("should handle finalization gracefully when ConfigMap and Secret don't exist", func() {
			// Create channel with finalizer already set
			channel = &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-channel-no-resources",
					Namespace:  namespace,
					Finalizers: []string{NotificationChannelCleanupFinalizer},
				},
				Spec: openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelSpec{
					Environment: environment.Name,
					Type:        openchoreodevv1alpha1.NotificationChannelTypeEmail,
					EmailConfig: &openchoreodevv1alpha1.EmailConfig{
						From: "test@example.com",
						To:   []string{"test@example.com"},
						SMTP: openchoreodevv1alpha1.SMTPConfig{
							Host: "smtp.example.com",
							Port: 587,
							Auth: &openchoreodevv1alpha1.SMTPAuth{
								Username: &openchoreodevv1alpha1.SecretValueFrom{},
								Password: &openchoreodevv1alpha1.SecretValueFrom{},
							},
							TLS: &openchoreodevv1alpha1.SMTPTLSConfig{},
						},
						Template: &openchoreodevv1alpha1.EmailTemplate{
							Subject: "No Resources Finalizer Subject",
							Body:    "No Resources Finalizer Body",
						},
					},
				},
			}
			Expect(k8sClient.Create(testCtx, channel)).To(Succeed())

			// Delete the channel (ConfigMap and Secret don't exist)
			Expect(k8sClient.Delete(testCtx, channel)).To(Succeed())

			// Wait for deletion timestamp to be set
			deletedChannel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}
			Eventually(func() bool {
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, deletedChannel)
				if err != nil {
					return false
				}
				return !deletedChannel.DeletionTimestamp.IsZero()
			}, time.Second*10, time.Millisecond*500).Should(BeTrue())

			// Reconcile to trigger finalization
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

			// Should not error even though ConfigMap/Secret don't exist
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify finalizer is removed
			finalChannel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}
			Eventually(func() bool {
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, finalChannel)
				if err != nil {
					return apierrors.IsNotFound(err)
				}
				return !contains(finalChannel.Finalizers, NotificationChannelCleanupFinalizer)
			}, time.Second*10, time.Millisecond*500).Should(BeTrue())
		})

		It("should skip finalization when finalizer is not present", func() {
			// Create channel without finalizer
			channel = &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-channel-no-finalizer",
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelSpec{
					Environment: environment.Name,
					Type:        openchoreodevv1alpha1.NotificationChannelTypeEmail,
					EmailConfig: &openchoreodevv1alpha1.EmailConfig{
						From: "test@example.com",
						To:   []string{"test@example.com"},
						SMTP: openchoreodevv1alpha1.SMTPConfig{
							Host: "smtp.example.com",
							Port: 587,
							Auth: &openchoreodevv1alpha1.SMTPAuth{
								Username: &openchoreodevv1alpha1.SecretValueFrom{},
								Password: &openchoreodevv1alpha1.SecretValueFrom{},
							},
							TLS: &openchoreodevv1alpha1.SMTPTLSConfig{},
						},
						Template: &openchoreodevv1alpha1.EmailTemplate{
							Subject: "No Finalizer Subject",
							Body:    "No Finalizer Body",
						},
					},
				},
			}
			Expect(k8sClient.Create(testCtx, channel)).To(Succeed())

			// Verify finalizer is not present before deletion
			createdChannel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}
			err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, createdChannel)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdChannel.Finalizers).NotTo(ContainElement(NotificationChannelCleanupFinalizer))

			// Delete the channel
			Expect(k8sClient.Delete(testCtx, channel)).To(Succeed())

			// Reconcile - should handle gracefully whether resource is already deleted or has deletion timestamp
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

			// Should not error - controller handles NotFound gracefully, and no finalizer means no cleanup needed
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify resource is eventually deleted (may already be deleted if no finalizer)
			Eventually(func() bool {
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: channel.Name, Namespace: namespace}, &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{})
				return apierrors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*500).Should(BeTrue())
		})
	})
})

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
