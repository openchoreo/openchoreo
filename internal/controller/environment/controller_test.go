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

	apiv1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
	dp "github.com/choreo-idp/choreo/internal/controller/dataplane"
	org "github.com/choreo-idp/choreo/internal/controller/organization"
	"github.com/choreo-idp/choreo/internal/controller/testutils"
	"github.com/choreo-idp/choreo/internal/labels"
)

var _ = Describe("Environment Controller", func() {
	const orgName = "test-org"

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

		const dpName = "test-dataplane"

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
		environment := &apiv1.Environment{}
		By("Creating the environment resource", func() {
			err := k8sClient.Get(ctx, envNamespacedName, environment)
			if err != nil && errors.IsNotFound(err) {
				dp := &apiv1.Environment{
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
				Expect(k8sClient.Create(ctx, dp)).To(Succeed())
			}
		})

		By("Reconciling the environment resource", func() {
			envReconciler := &Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}
			result, err := envReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: envNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})

		By("Checking the environment resource", func() {
			environment := &apiv1.Environment{}
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

		By("Checking the environment resource deletion", func() {
			Eventually(func() error {
				return k8sClient.Get(ctx, envNamespacedName, environment)
			}, time.Second*10, time.Millisecond*500).ShouldNot(Succeed())
		})

		By("Reconciling the environment resource after deletion", func() {
			dpReconciler := &Reconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}
			result, err := dpReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: envNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})
	})
})
