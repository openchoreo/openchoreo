/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package component

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiv1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
	dp "github.com/choreo-idp/choreo/internal/controller/dataplane"
	deppip "github.com/choreo-idp/choreo/internal/controller/deploymentpipeline"
	env "github.com/choreo-idp/choreo/internal/controller/environment"
	org "github.com/choreo-idp/choreo/internal/controller/organization"
	proj "github.com/choreo-idp/choreo/internal/controller/project"
	"github.com/choreo-idp/choreo/internal/controller/testutils"
	"github.com/choreo-idp/choreo/internal/labels"
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

	organization := &apiv1.Organization{
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

		dataplane := &apiv1.DataPlane{
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

		environment := &apiv1.Environment{
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

		depPip := &apiv1.DeploymentPipeline{
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
			Spec: apiv1.DeploymentPipelineSpec{
				PromotionPaths: []apiv1.PromotionPath{
					{
						SourceEnvironmentRef:  "test-env",
						TargetEnvironmentRefs: make([]apiv1.TargetEnvironmentRef, 0),
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

		project := &apiv1.Project{
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

		component := &apiv1.Component{}

		By("Creating the component resource", func() {
			err := k8sClient.Get(ctx, componentNamespacedName, component)
			if err != nil && errors.IsNotFound(err) {
				cmp := &apiv1.Component{
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
					Spec: apiv1.ComponentSpec{
						Type: apiv1.ComponentTypeService,
						Source: apiv1.ComponentSource{
							GitRepository: &apiv1.GitRepository{
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
			component := &apiv1.Component{}
			Eventually(func() error {
				return k8sClient.Get(ctx, componentNamespacedName, component)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())
			Expect(component.Name).To(Equal(componentName))
			Expect(component.Namespace).To(Equal(orgName))
			Expect(component.Spec).NotTo(BeNil())
			Expect(component.Spec.Type).To(Equal(apiv1.ComponentTypeService))
		})

		By("Deleting the component resource", func() {
			err := k8sClient.Get(ctx, componentNamespacedName, component)
			Expect(err).NotTo(HaveOccurred())

			// Check if the finalizer exists and remove it (for testing purposes)
			if controllerutil.ContainsFinalizer(component, ComponentCleanupFinalizer) {
				controllerutil.RemoveFinalizer(component, ComponentCleanupFinalizer)
				Expect(k8sClient.Update(ctx, component)).To(Succeed())
			}

			Expect(k8sClient.Delete(ctx, component)).To(Succeed())
		})

		By("Checking the component resource deletion", func() {
			Eventually(func() bool {
				err := k8sClient.Get(ctx, componentNamespacedName, component)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*500).Should(BeTrue())
		})

		By("Reconciling the component resource after deletion", func() {
			componentReconciler := &Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}
			result, err := componentReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: componentNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})
	})

	AfterEach(func() {
		By("Deleting the organization resource", func() {
			org := &apiv1.Organization{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name: orgName,
			}, org)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, org)).To(Succeed())
		})
	})
})
