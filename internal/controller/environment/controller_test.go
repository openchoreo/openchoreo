// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	dp "github.com/openchoreo/openchoreo/internal/controller/dataplane"
	org "github.com/openchoreo/openchoreo/internal/controller/organization"
	"github.com/openchoreo/openchoreo/internal/controller/testutils"
	dpKubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("Environment Controller", func() {
	const orgName = "test-org"
	const dpName = "test-dataplane"

	orgNamespacedName := types.NamespacedName{
		Name: orgName,
	}
	organization := &openchoreov1alpha1.Organization{
		ObjectMeta: metav1.ObjectMeta{
			Name: orgName,
		},
	}

	dpClientMgr := dpKubernetes.NewManager()

	BeforeEach(func() {
		By("Creating and reconciling organization resource", func() {
			orgReconciler := &org.Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}
			testutils.CreateAndReconcileResourceWithCycles(ctx, k8sClient, organization, orgReconciler,
				orgNamespacedName, 2)
		})

		dpNamespacedName := types.NamespacedName{
			Name:      dpName,
			Namespace: orgName,
		}

		dataplane := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dpName,
				Namespace: orgName,
				Labels: map[string]string{
					labels.LabelKeyOrganizationName: organization.Name,
					labels.LabelKeyName:             dpName,
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

	AfterEach(func() {
		By("Deleting the organization resource", func() {
			testutils.DeleteResource(ctx, k8sClient, organization, orgNamespacedName)
		})
	})

	It("should successfully create and reconcile environment resource", func() {
		const envName = "test-env"

		envNamespacedName := types.NamespacedName{
			Namespace: orgName,
			Name:      envName,
		}
		environment := &openchoreov1alpha1.Environment{}
		By("Creating the environment resource", func() {
			err := k8sClient.Get(ctx, envNamespacedName, environment)
			if err != nil && errors.IsNotFound(err) {
				dp := &openchoreov1alpha1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: orgName,
						Labels: map[string]string{
							labels.LabelKeyOrganizationName: orgName,
							labels.LabelKeyName:             envName,
						},
						Annotations: map[string]string{
							controller.AnnotationKeyDisplayName: "Test Environment",
							controller.AnnotationKeyDescription: "Test Environment Description",
						},
					},
					Spec: openchoreov1alpha1.EnvironmentSpec{
						DataPlaneRef: dpName,
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
				Client:      k8sClient,
				DpClientMgr: dpClientMgr,
				Scheme:      k8sClient.Scheme(),
				Recorder:    record.NewFakeRecorder(100),
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
			Expect(environment.Namespace).To(Equal(orgName))
			Expect(environment.Spec).NotTo(BeNil())
		})

		By("Deleting the environment resource", func() {
			err := k8sClient.Get(ctx, envNamespacedName, environment)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, environment)).To(Succeed())
		})

		By("Reconciling the environment resource after deletion - attempt 1 to update status conditions", func() {
			envReconciler := &Reconciler{
				Client:      k8sClient,
				DpClientMgr: dpClientMgr,
				Scheme:      k8sClient.Scheme(),
				Recorder:    record.NewFakeRecorder(100),
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
})
