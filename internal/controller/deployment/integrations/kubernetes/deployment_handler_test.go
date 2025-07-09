// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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
			deployCtx.Component.Spec.Type = openchoreov1alpha1.ComponentTypeService
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
			deployCtx.Component.Spec.Type = openchoreov1alpha1.ComponentTypeService
			deployCtx.DeployableArtifact.Spec.Configuration = &openchoreov1alpha1.Configuration{
				EndpointTemplates: []openchoreov1alpha1.EndpointTemplate{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-endpoint-tcp",
						},
						Spec: openchoreov1alpha1.EndpointSpec{
							Type: openchoreov1alpha1.EndpointTypeREST,
							BackendRef: openchoreov1alpha1.BackendRef{
								BasePath: "/test",
								Type:     openchoreov1alpha1.BackendRefTypeComponentRef,
								ComponentRef: &openchoreov1alpha1.ComponentRef{
									Port: 8080,
								},
							},
							NetworkVisibilities: &openchoreov1alpha1.NetworkVisibility{
								Public: &openchoreov1alpha1.VisibilityConfig{
									Enable: true,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-endpoint-udp",
						},
						Spec: openchoreov1alpha1.EndpointSpec{
							Type: openchoreov1alpha1.EndpointTypeUDP,
							BackendRef: openchoreov1alpha1.BackendRef{
								BasePath: "/test",
								Type:     openchoreov1alpha1.BackendRefTypeComponentRef,
								ComponentRef: &openchoreov1alpha1.ComponentRef{
									Port: 8080,
								},
							},
							NetworkVisibilities: &openchoreov1alpha1.NetworkVisibility{
								Public: &openchoreov1alpha1.VisibilityConfig{
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
