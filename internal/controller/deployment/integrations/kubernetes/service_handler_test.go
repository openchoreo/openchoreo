// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

var _ = Describe("makeService", func() {
	var (
		deployCtx *dataplane.DeploymentContext
		service   *corev1.Service
	)

	// Prepare fresh DeploymentContext before each test
	BeforeEach(func() {
		deployCtx = newTestDeploymentContext()
	})

	JustBeforeEach(func() {
		service = makeService(deployCtx)
	})

	Context("for a Service component with one endpoint", func() {
		BeforeEach(func() {
			deployCtx.Component.Spec.Type = choreov1.ComponentTypeService
			deployCtx.DeployableArtifact.Spec.Configuration = &choreov1.Configuration{
				EndpointTemplates: []choreov1.EndpointTemplate{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-service-endpoint",
						},
						Spec: choreov1.EndpointSpec{
							Type: choreov1.EndpointTypeREST,
							BackendRef: choreov1.BackendRef{
								BasePath: "/test",
								Type:     choreov1.BackendRefTypeComponentRef,
								ComponentRef: &choreov1.ComponentRef{
									Port: 8080,
								},
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

		It("should create a Service with correct name and namespace", func() {
			Expect(service).NotTo(BeNil())
			Expect(service.Name).To(Equal("my-component-my-main-track-a43a18e7"))
			Expect(service.Namespace).To(Equal("dp-test-organiza-my-project-test-environ-04bdf416"))
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

		It("should create a Service with valid labels", func() {
			Expect(service.Labels).To(BeComparableTo(expectedLabels))
		})

		It("should create a Service with correct selector", func() {
			Expect(service.Spec.Selector).To(BeComparableTo(expectedLabels))
		})

		It("should create a Service with a correct port", func() {
			ports := service.Spec.Ports

			By("checking the port length")
			Expect(ports).To(HaveLen(1))

			By("checking the port")
			Expect(ports[0].Name).To(Equal("ep-8080-tcp"))
			Expect(ports[0].Port).To(Equal(int32(8080)))
			Expect(ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
		})
	})

	Context("for a Service component with one TCP and one UDP endpoint", func() {
		BeforeEach(func() {
			deployCtx.Component.Spec.Type = choreov1.ComponentTypeService
			deployCtx.DeployableArtifact.Spec.Configuration = &choreov1.Configuration{
				EndpointTemplates: []choreov1.EndpointTemplate{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-service-endpoint-tcp",
						},
						Spec: choreov1.EndpointSpec{
							Type: choreov1.EndpointTypeREST,
							BackendRef: choreov1.BackendRef{
								BasePath: "/test",
								Type:     choreov1.BackendRefTypeComponentRef,
								ComponentRef: &choreov1.ComponentRef{
									Port: 8080,
								},
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
							Name: "my-service-endpoint-udp",
						},
						Spec: choreov1.EndpointSpec{
							Type: choreov1.EndpointTypeUDP,
							BackendRef: choreov1.BackendRef{
								Type: choreov1.BackendRefTypeComponentRef,
								ComponentRef: &choreov1.ComponentRef{
									Port: 8080,
								},
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

		It("should create a Service with a correct port TCP and UDP port", func() {
			ports := service.Spec.Ports

			By("checking the port length")
			Expect(ports).To(HaveLen(2))

			By("checking the TCP port")
			Expect(ports[0].Name).To(Equal("ep-8080-tcp"))
			Expect(ports[0].Port).To(Equal(int32(8080)))
			Expect(ports[0].Protocol).To(Equal(corev1.ProtocolTCP))

			By("checking the UDP port")
			Expect(ports[1].Name).To(Equal("ep-8080-udp"))
			Expect(ports[1].Port).To(Equal(int32(8080)))
			Expect(ports[1].Protocol).To(Equal(corev1.ProtocolUDP))
		})
	})
})
