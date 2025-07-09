// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

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
	deppip "github.com/openchoreo/openchoreo/internal/controller/deploymentpipeline"
	env "github.com/openchoreo/openchoreo/internal/controller/environment"
	org "github.com/openchoreo/openchoreo/internal/controller/organization"
	proj "github.com/openchoreo/openchoreo/internal/controller/project"
	"github.com/openchoreo/openchoreo/internal/controller/testutils"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("Component Controller", func() {
	const (
		orgName     = "test-org"
		dpName      = "test-dataplane"
		envName     = "test-env"
		deppipName  = "test-deployment-pipeline"
		projectName = "test-project"
	)

	orgNamespacedName := types.NamespacedName{
		Name: orgName,
	}

	organization := &openchoreov1alpha1.Organization{
		ObjectMeta: metav1.ObjectMeta{
			Name: orgName,
		},
	}

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
			Namespace: orgName,
			Name:      envName,
		}

		environment := &openchoreov1alpha1.Environment{
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

		By("Creating and reconciling the environment resource", func() {
			envReconciler := &env.Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}
			testutils.CreateAndReconcileResource(ctx, k8sClient, environment, envReconciler, envNamespacedName)
		})

		depPipelineNamespacedName := types.NamespacedName{
			Namespace: orgName,
			Name:      deppipName,
		}

		depPip := &openchoreov1alpha1.DeploymentPipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deppipName,
				Namespace: orgName,
				Labels: map[string]string{
					labels.LabelKeyOrganizationName: orgName,
					labels.LabelKeyName:             deppipName,
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

		By("Creating and reconciling the deployment pipeline resource", func() {
			depPipelineReconciler := &deppip.Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			testutils.CreateAndReconcileResource(ctx, k8sClient, depPip, depPipelineReconciler, depPipelineNamespacedName)
		})

		projectNamespacedName := types.NamespacedName{
			Namespace: orgName,
			Name:      projectName,
		}

		project := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{
				Name:      projectName,
				Namespace: orgName,
				Labels: map[string]string{
					labels.LabelKeyOrganizationName: orgName,
					labels.LabelKeyName:             projectName,
				},
				Annotations: map[string]string{
					controller.AnnotationKeyDisplayName: "Test Project",
					controller.AnnotationKeyDescription: "Test Project Description",
				},
			},
		}

		By("Creating and reconciling the project resource", func() {
			projectReconciler := &proj.Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			testutils.CreateAndReconcileResource(ctx, k8sClient, project, projectReconciler, projectNamespacedName)
		})
	})

	It("should successfully create and reconcile component resource", func() {
		const componentName = "test-component"

		componentNamespacedName := types.NamespacedName{
			Namespace: orgName,
			Name:      componentName,
		}

		component := &openchoreov1alpha1.Component{}

		By("Creating the component resource", func() {
			err := k8sClient.Get(ctx, componentNamespacedName, component)
			if err != nil && errors.IsNotFound(err) {
				cmp := &openchoreov1alpha1.Component{
					ObjectMeta: metav1.ObjectMeta{
						Name:      componentName,
						Namespace: orgName,
						Labels: map[string]string{
							labels.LabelKeyOrganizationName: orgName,
							labels.LabelKeyName:             componentName,
						},
						Annotations: map[string]string{
							controller.AnnotationKeyDisplayName: "Test Component",
							controller.AnnotationKeyDescription: "Test Component Description",
						},
					},
					Spec: openchoreov1alpha1.ComponentSpec{
						Type: openchoreov1alpha1.ComponentTypeService,
						Source: openchoreov1alpha1.ComponentSource{
							GitRepository: &openchoreov1alpha1.GitRepository{
								URL: "https://github.com/test-org/test-repo",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, cmp)).To(Succeed())
			}
		})

		By("Reconciling the component resource", func() {
			componentReconciler := &Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}
			_, err := componentReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: componentNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		By("Checking the component resource", func() {
			component := &openchoreov1alpha1.Component{}
			Eventually(func() error {
				return k8sClient.Get(ctx, componentNamespacedName, component)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())
			Expect(component.Name).To(Equal(componentName))
			Expect(component.Namespace).To(Equal(orgName))
			Expect(component.Spec).NotTo(BeNil())
			Expect(component.Spec.Type).To(Equal(openchoreov1alpha1.ComponentTypeService))
		})

		By("Deleting the component resource", func() {
			err := k8sClient.Get(ctx, componentNamespacedName, component)
			Expect(err).NotTo(HaveOccurred())

			// Delete the component - this marks it for deletion but won't remove it yet due to finalizer
			Expect(k8sClient.Delete(ctx, component)).To(Succeed())
		})

		By("Reconciling the component after deletion request", func() {
			// The finalizer should trigger during reconciliation
			componentReconciler := &Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			// Component should exist but be marked for deletion
			updatedComponent := &openchoreov1alpha1.Component{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, componentNamespacedName, updatedComponent)
				if err != nil {
					return false
				}
				return !updatedComponent.DeletionTimestamp.IsZero()
			}, time.Second*10, time.Millisecond*500).Should(BeTrue())

			// Run the finalizer reconciliation
			_, err := componentReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: componentNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Run finalizer again to complete deletion
			_, err = componentReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: componentNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		By("Checking the component resource deletion", func() {
			// Now the component should be fully deleted
			Eventually(func() error {
				return k8sClient.Get(ctx, componentNamespacedName, &openchoreov1alpha1.Component{})
			}, time.Second*10, time.Millisecond*500).Should(Satisfy(errors.IsNotFound))
		})
	})

	AfterEach(func() {
		By("Deleting the organization resource", func() {
			org := &openchoreov1alpha1.Organization{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name: orgName,
			}, org)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, org)).To(Succeed())
		})
	})
})
