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

package kubernetes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

var _ = Describe("makeDeployment", func() {
	var (
		deployCtx  *dataplane.DeploymentContext
		deployment *appsv1.Deployment
	)

	// Prepare fresh DeploymentContext before each test
	BeforeEach(func() {
		deployCtx = newTestDeploymentContext()
	})

	JustBeforeEach(func() {
		deployment = makeDeployment(deployCtx)
	})

	Context("for a Service component", func() {
		BeforeEach(func() {
			deployCtx.Component.Spec.Type = choreov1.ComponentTypeService
		})

		It("should create a Deployment with correct name and namespace", func() {
			Expect(deployment).NotTo(BeNil())
			Expect(deployment.Name).To(Equal("my-component-my-main-track-a43a18e7"))
			Expect(deployment.Namespace).To(Equal("dp-test-organiza-my-project-test-environ-04bdf416"))
		})

		expectedLabels := map[string]string{
			"organization-name":     "test-organization",
			"project-name":          "my-project",
			"environment-name":      "test-environment",
			"component-name":        "my-component",
			"component-type":        "Service",
			"deployment-track-name": "my-main-track",
			"deployment-name":       "my-deployment",
			"managed-by":            "choreo-deployment-controller",
			"belong-to":             "user-workloads",
		}

		It("should create a Deployment with valid labels", func() {
			Expect(deployment.Labels).To(BeComparableTo(expectedLabels))
		})

		It("should create a Deployment with correct selector", func() {
			Expect(deployment.Spec.Selector.MatchLabels).To(BeComparableTo(expectedLabels))
		})

		It("should create a Deployment with a correct container", func() {
			containers := deployment.Spec.Template.Spec.Containers

			By("checking the container length")
			Expect(containers).To(HaveLen(1))

			By("checking the container")
			Expect(containers[0].Name).To(Equal("main"))
			Expect(containers[0].Image).To(Equal("my-image:latest"))
		})
	})

	Context("for a Service component with one TCP and one UDP endpoint", func() {
		BeforeEach(func() {
			deployCtx.Component.Spec.Type = choreov1.ComponentTypeService
			deployCtx.DeployableArtifact.Spec.Configuration = &choreov1.Configuration{
				EndpointTemplates: []choreov1.EndpointTemplate{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-endpoint-tcp",
						},
						Spec: choreov1.EndpointSpec{
							Type: choreov1.EndpointTypeREST,
							Service: choreov1.EndpointServiceSpec{
								BasePath: "/test",
								Port:     8080,
							},
							NetworkVisibilities: &choreov1.NetworkVisibility{
								Public: &choreov1.VisibilityConfig{
									Enable: true,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-endpoint-udp",
						},
						Spec: choreov1.EndpointSpec{
							Type: choreov1.EndpointTypeUDP,
							Service: choreov1.EndpointServiceSpec{
								Port: 8080,
							},
							NetworkVisibilities: &choreov1.NetworkVisibility{
								Public: &choreov1.VisibilityConfig{
									Enable: true,
								},
							},
						},
					},
				},
			}

		})

		It("should create a Deployment with a correct container ports TCP and UDP", func() {
			containers := deployment.Spec.Template.Spec.Containers
			By("checking the container port length")
			Expect(containers[0].Ports).To(HaveLen(2))

			By("checking the TCP port")
			Expect(containers[0].Ports[0].Name).To(Equal("ep-8080-tcp"))
			Expect(containers[0].Ports[0].ContainerPort).To(Equal(int32(8080)))
			Expect(containers[0].Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))

			By("checking the UDP port")
			Expect(containers[0].Ports[1].Name).To(Equal("ep-8080-udp"))
			Expect(containers[0].Ports[1].ContainerPort).To(Equal(int32(8080)))
			Expect(containers[0].Ports[1].Protocol).To(Equal(corev1.ProtocolUDP))
		})
	})
})
