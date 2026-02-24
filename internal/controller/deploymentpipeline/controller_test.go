// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

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
	"github.com/openchoreo/openchoreo/internal/controller"
	dp "github.com/openchoreo/openchoreo/internal/controller/dataplane"
	env "github.com/openchoreo/openchoreo/internal/controller/environment"
	"github.com/openchoreo/openchoreo/internal/controller/testutils"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("DeploymentPipeline Controller", func() {
	const (
		namespaceName = "test-namespace"
		dpName        = "test-dataplane"
		envName       = "test-env"
	)

	namespaceNamespacedName := types.NamespacedName{
		Name: namespaceName,
	}
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}

	BeforeEach(func() {
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

		envNamespacedName := types.NamespacedName{
			Namespace: namespaceName,
			Name:      envName,
		}

		environment := &openchoreov1alpha1.Environment{
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
			},
		}

		By("Creating and reconciling the environment resource", func() {
			envReconciler := &env.Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}
			testutils.CreateAndReconcileResource(ctx, k8sClient, environment, envReconciler, envNamespacedName)
		})
	})

	AfterEach(func() {
		By("Deleting the namespace", func() {
			testutils.DeleteResource(ctx, k8sClient, namespace, namespaceNamespacedName)
		})
	})

	const pipelineName = "test-deployment-pipeline"

	pipelineNamespacedName := types.NamespacedName{
		Namespace: namespaceName,
		Name:      pipelineName,
	}

	It("should successfully create and reconcile deployment pipeline resource", func() {
		pipeline := &openchoreov1alpha1.DeploymentPipeline{}

		By("creating a custom resource for the Kind DeploymentPipeline", func() {
			err := k8sClient.Get(ctx, pipelineNamespacedName, pipeline)
			if err != nil && errors.IsNotFound(err) {
				dp := &openchoreov1alpha1.DeploymentPipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pipelineName,
						Namespace: namespaceName,
						Labels: map[string]string{
							labels.LabelKeyNamespaceName: namespaceName,
							labels.LabelKeyName:          pipelineName,
						},
						Annotations: map[string]string{
							controller.AnnotationKeyDisplayName: "Test Deployment pipeline",
							controller.AnnotationKeyDescription: "Test Deployment pipeline Description",
						},
					},
					Spec: openchoreov1alpha1.DeploymentPipelineSpec{
						PromotionPaths: []openchoreov1alpha1.PromotionPath{
							{
								SourceEnvironmentRef:  "test-env",
								TargetEnvironmentRefs: make([]openchoreov1alpha1.TargetEnvironmentRef, 0),
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, dp)).To(Succeed())
			}
		})

		depReconciler := &Reconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: record.NewFakeRecorder(100),
		}

		By("Reconciling the deploymentPipeline resource to add finalizer", func() {
			result, err := depReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: pipelineNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})

		By("Reconciling the deploymentPipeline resource to set status", func() {
			result, err := depReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: pipelineNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})

		By("Checking the deploymentPipeline resource has finalizer", func() {
			deploymentPipeline := &openchoreov1alpha1.DeploymentPipeline{}
			Eventually(func() error {
				return k8sClient.Get(ctx, pipelineNamespacedName, deploymentPipeline)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())
			Expect(deploymentPipeline.Name).To(Equal(pipelineName))
			Expect(deploymentPipeline.Namespace).To(Equal(namespaceName))
			Expect(deploymentPipeline.Spec).NotTo(BeNil())
			Expect(deploymentPipeline.Finalizers).To(ContainElement(PipelineCleanupFinalizer))
		})

		By("Deleting the deploymentPipeline resource", func() {
			err := k8sClient.Get(ctx, pipelineNamespacedName, pipeline)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())
		})

		By("Reconciling the deploymentPipeline resource to run finalizer", func() {
			result, err := depReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: pipelineNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})

		By("Checking the deploymentPipeline resource deletion", func() {
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pipelineNamespacedName, pipeline)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*500).Should(BeTrue())
		})
	})
})

var _ = Describe("DeploymentPipeline Controller - Finalizer with referencing Projects", func() {
	const (
		namespaceName = "test-ns-ref-projects"
		dpName        = "test-dataplane"
		envName       = "test-env"
		pipelineName  = "test-deployment-pipeline"
	)

	BeforeEach(func() {
		By("Creating namespace", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
				},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: namespaceName}, ns)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			}
		})

		By("Creating and reconciling the dataplane resource", func() {
			dataplane := &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dpName,
					Namespace: namespaceName,
				},
			}
			dpReconciler := &dp.Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}
			testutils.CreateAndReconcileResource(ctx, k8sClient, dataplane, dpReconciler, types.NamespacedName{
				Name:      dpName,
				Namespace: namespaceName,
			})
		})

		By("Creating and reconciling the environment resource", func() {
			environment := &openchoreov1alpha1.Environment{
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
				},
			}
			envReconciler := &env.Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}
			testutils.CreateAndReconcileResource(ctx, k8sClient, environment, envReconciler, types.NamespacedName{
				Namespace: namespaceName,
				Name:      envName,
			})
		})
	})

	It("should wait for referencing projects before removing finalizer", func() {
		pipelineNamespacedName := types.NamespacedName{
			Namespace: namespaceName,
			Name:      pipelineName,
		}
		pipeline := &openchoreov1alpha1.DeploymentPipeline{}

		By("Creating the deployment pipeline", func() {
			dp := &openchoreov1alpha1.DeploymentPipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pipelineName,
					Namespace: namespaceName,
					Labels: map[string]string{
						labels.LabelKeyNamespaceName: namespaceName,
						labels.LabelKeyName:          pipelineName,
					},
					Annotations: map[string]string{
						controller.AnnotationKeyDisplayName: "Test Deployment pipeline",
						controller.AnnotationKeyDescription: "Test Deployment pipeline Description",
					},
				},
				Spec: openchoreov1alpha1.DeploymentPipelineSpec{
					PromotionPaths: []openchoreov1alpha1.PromotionPath{
						{
							SourceEnvironmentRef:  envName,
							TargetEnvironmentRefs: make([]openchoreov1alpha1.TargetEnvironmentRef, 0),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, dp)).To(Succeed())
		})

		depReconciler := &Reconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: record.NewFakeRecorder(100),
		}

		By("Reconciling to add finalizer and set status", func() {
			_, err := depReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: pipelineNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = depReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: pipelineNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating a project that references the pipeline", func() {
			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-project",
					Namespace: namespaceName,
					Labels: map[string]string{
						labels.LabelKeyNamespaceName: namespaceName,
						labels.LabelKeyName:          "test-project",
					},
				},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: pipelineName,
				},
			}
			Expect(k8sClient.Create(ctx, project)).To(Succeed())
		})

		By("Deleting the deployment pipeline", func() {
			err := k8sClient.Get(ctx, pipelineNamespacedName, pipeline)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())
		})

		By("Reconciling - should requeue because project still references it", func() {
			result, err := depReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: pipelineNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5 * time.Second))
		})

		By("Verifying the pipeline still exists (finalizer not removed)", func() {
			err := k8sClient.Get(ctx, pipelineNamespacedName, pipeline)
			Expect(err).NotTo(HaveOccurred())
			Expect(pipeline.Finalizers).To(ContainElement(PipelineCleanupFinalizer))
		})

		By("Deleting the referencing project", func() {
			project := &openchoreov1alpha1.Project{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: namespaceName,
				Name:      "test-project",
			}, project)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, project)).To(Succeed())
		})

		By("Reconciling again - should remove finalizer now", func() {
			result, err := depReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: pipelineNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})

		By("Verifying the pipeline is fully deleted", func() {
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pipelineNamespacedName, pipeline)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*500).Should(BeTrue())
		})
	})

	AfterEach(func() {
		By("Deleting the namespace", func() {
			ns := &corev1.Namespace{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: namespaceName}, ns)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
		})
	})
})
