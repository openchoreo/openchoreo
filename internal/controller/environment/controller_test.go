// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
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
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	dp "github.com/openchoreo/openchoreo/internal/controller/dataplane"
	"github.com/openchoreo/openchoreo/internal/controller/testutils"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("Environment Controller", Ordered, func() {
	const namespaceName = "test-namespace"
	const dpName = "test-dataplane"

	namespaceNamespacedName := types.NamespacedName{
		Name: namespaceName,
	}
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}

	k8sClientMgr := kubernetesClient.NewManager()

	BeforeAll(func() {
		By("Creating namespace", func() {
			err := k8sClient.Get(ctx, namespaceNamespacedName, namespace)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
			}
		})

		dpNamespacedName := types.NamespacedName{
			Name:      dpName,
			Namespace: namespaceName,
		}

		dataplane := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dpName,
				Namespace: namespaceName,
				Labels: map[string]string{
					labels.LabelKeyNamespaceName: namespaceName,
					labels.LabelKeyName:          dpName,
				},
			},
		}

		By("Creating and reconciling the dataplane resource", func() {
			dpReconciler := &dp.Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}
			testutils.CreateAndReconcileResource(ctx, k8sClient, dataplane, dpReconciler, dpNamespacedName)
		})
	})

	AfterAll(func() {
		By("Deleting the namespace", func() {
			testutils.DeleteResource(ctx, k8sClient, namespace, namespaceNamespacedName)
		})
	})

	It("should successfully create and reconcile environment resource", func() {
		const envName = "test-env"

		envNamespacedName := types.NamespacedName{
			Namespace: namespaceName,
			Name:      envName,
		}
		environment := &openchoreov1alpha1.Environment{}
		By("Creating the environment resource", func() {
			err := k8sClient.Get(ctx, envNamespacedName, environment)
			if err != nil && errors.IsNotFound(err) {
				dp := &openchoreov1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: namespaceName,
						Labels: map[string]string{
							labels.LabelKeyNamespaceName: namespaceName,
							labels.LabelKeyName:          envName,
						},
						Annotations: map[string]string{
							controller.AnnotationKeyDisplayName: "Test Environment",
							controller.AnnotationKeyDescription: "Test Environment Description",
						},
					},
					Spec: openchoreov1alpha1.EnvironmentSpec{
						DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
							Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
							Name: dpName,
						},
						IsProduction: false,
						Gateway: openchoreov1alpha1.GatewayConfig{
							DNSPrefix: envName,
						},
					},
				}
				Expect(k8sClient.Create(ctx, dp)).To(Succeed())
			}
		})

		By("Reconciling the environment resource", func() {
			envReconciler := &Reconciler{
				Client:       k8sClient,
				K8sClientMgr: k8sClientMgr,
				Scheme:       k8sClient.Scheme(),
				Recorder:     record.NewFakeRecorder(100),
			}
			result, err := envReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: envNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})

		By("Checking the environment resource", func() {
			environment := &openchoreov1alpha1.Environment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, envNamespacedName, environment)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())
			Expect(environment.Name).To(Equal(envName))
			Expect(environment.Namespace).To(Equal(namespaceName))
			Expect(environment.Spec).NotTo(BeNil())
		})

		By("Deleting the environment resource", func() {
			err := k8sClient.Get(ctx, envNamespacedName, environment)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, environment)).To(Succeed())
		})

		By("Reconciling the environment resource after deletion - attempt 1 to update status conditions", func() {
			envReconciler := &Reconciler{
				Client:       k8sClient,
				K8sClientMgr: k8sClientMgr,
				Scheme:       k8sClient.Scheme(),
				Recorder:     record.NewFakeRecorder(100),
			}
			result, err := envReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: envNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})

		By("Checking the status condition after first reconcile of deletion", func() {
			environment := &openchoreov1alpha1.Environment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, envNamespacedName, environment)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())
			Expect(environment.Status.Conditions).NotTo(BeNil())
			Expect(environment.Status.Conditions[0].Reason).To(Equal("EnvironmentFinalizing"))
			Expect(environment.Status.Conditions[0].Message).To(Equal("Environment is finalizing"))
		})

		// TODO: Come up with a way to test DP namespace deletion part
		// By("Reconciling the environment resource after deletion - attempt 2 to remove the finalizer", func() {
		//	envReconciler := &Reconciler{
		//		Client:      k8sClient,
		//		DpClientMgr: dpClientMgr,
		//		Scheme:      k8sClient.Scheme(),
		//		Recorder:    record.NewFakeRecorder(100),
		//	}
		//	envReconciler.Reconcile(ctx, reconcile.Request{
		//		NamespacedName: envNamespacedName,
		//	})
		// Expect(err).NotTo(HaveOccurred())
		// Expect(result.Requeue).To(BeFalse())
		// })

		// By("Checking the environment resource deletion", func() {
		//	Eventually(func() error {
		//		return k8sClient.Get(ctx, envNamespacedName, environment)
		//	}, time.Second*10, time.Millisecond*500).ShouldNot(Succeed())
		// })

	})

	It("should enforce dataPlaneRef immutability", func() {
		const envName = "test-env-immutability"

		envNamespacedName := types.NamespacedName{
			Namespace: namespaceName,
			Name:      envName,
		}

		environment := &openchoreov1alpha1.Environment{}

		By("Creating environment without dataPlaneRef", func() {
			env := &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      envName,
					Namespace: namespaceName,
					Labels: map[string]string{
						labels.LabelKeyNamespaceName: namespaceName,
						labels.LabelKeyName:          envName,
					},
				},
				Spec: openchoreov1alpha1.EnvironmentSpec{
					IsProduction: false,
				},
			}
			Expect(k8sClient.Create(ctx, env)).To(Succeed())
		})

		By("Setting dataPlaneRef from empty to a value", func() {
			Expect(k8sClient.Get(ctx, envNamespacedName, environment)).To(Succeed())
			environment.Spec.DataPlaneRef = &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: dpName,
			}
			Expect(k8sClient.Update(ctx, environment)).To(Succeed())
		})

		By("Attempting to change dataPlaneRef (should fail)", func() {
			Expect(k8sClient.Get(ctx, envNamespacedName, environment)).To(Succeed())
			environment.Spec.DataPlaneRef = &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: "different-dataplane",
			}
			err := k8sClient.Update(ctx, environment)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dataPlaneRef is immutable once set"))
		})

		By("Updating other fields while keeping dataPlaneRef same", func() {
			Expect(k8sClient.Get(ctx, envNamespacedName, environment)).To(Succeed())
			environment.Spec.IsProduction = true
			Expect(k8sClient.Update(ctx, environment)).To(Succeed())
		})
	})
})
