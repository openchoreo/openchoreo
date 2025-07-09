// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

var _ = Describe("makeConfigMaps", func() {
	var (
		deployCtx  *dataplane.DeploymentContext
		configMaps []*corev1.ConfigMap
	)

	// Prepare fresh DeploymentContext before each test
	BeforeEach(func() {
		deployCtx = newTestDeploymentContext()
	})

	JustBeforeEach(func() {
		configMaps = makeConfigMaps(deployCtx)
	})

	Context("for two Configuration Groups", func() {
		BeforeEach(func() {
			deployCtx.Component.Spec.Type = openchoreov1alpha1.ComponentTypeService
			deployCtx.ConfigurationGroups = []*openchoreov1alpha1.ConfigurationGroup{
				newTestRedisConfigurationGroup(),
				newTestMysqlConfigurationGroup(),
			}
		})

		It("should create two ConfigMaps with correct name and namespace", func() {
			By("checking the generated ConfigMaps count")
			Expect(configMaps).To(HaveLen(2))

			By("checking the ConfigMap 1 names and namespaces")
			Expect(configMaps[0].Name).To(Equal("my-component-my-main-track-redis-config-group-b8ef9df9"))
			Expect(configMaps[0].Namespace).To(Equal("dp-test-organiza-my-project-test-environ-04bdf416"))

			By("checking the ConfigMap 2 names and namespaces")
			Expect(configMaps[1].Name).To(Equal("my-component-my-main-track-mysql-config-group-e7d2f2be"))
			Expect(configMaps[1].Namespace).To(Equal("dp-test-organiza-my-project-test-environ-04bdf416"))
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

		It("should create ConfigMaps with valid labels", func() {
			Expect(configMaps[0].Labels).To(BeComparableTo(expectedLabels))
		})

		It("should create the ConfigMap 1 with correct data", func() {
			data := configMaps[0].Data
			Expect(data).To(BeComparableTo(map[string]string{
				"host": "redis-dev.test.com",
				"port": "6379",
			}))
		})

		It("should create the ConfigMap 2 with correct data", func() {
			data := configMaps[1].Data
			Expect(data).To(BeComparableTo(map[string]string{
				"host": "mysql-dev.test.com",
				"port": "3306",
			}))
		})
	})

	Context("for a Configuration Group with Environment Group", func() {
		BeforeEach(func() {
			deployCtx.Component.Spec.Type = openchoreov1alpha1.ComponentTypeService
			deployCtx.ConfigurationGroups = []*openchoreov1alpha1.ConfigurationGroup{
				newTestConfigurationGroup("salesforce-config-group",
					openchoreov1alpha1.ConfigurationGroupSpec{
						EnvironmentGroups: []openchoreov1alpha1.EnvironmentGroup{
							{
								Name:         "non-prod",
								Environments: []string{"test-environment"},
							},
						},
						Configurations: []openchoreov1alpha1.ConfigurationGroupConfiguration{
							{
								Key: "host",
								Values: []openchoreov1alpha1.ConfigurationValue{
									{
										EnvironmentGroupRef: "non-prod",
										Value:               "sandbox.salesforce.com",
									},
								},
							},
						},
					},
				),
			}
		})

		It("should create a ConfigMap with correct data", func() {
			data := configMaps[0].Data
			Expect(data).To(BeComparableTo(map[string]string{
				"host": "sandbox.salesforce.com",
			}))
		})
	})

	Context("for a direct file mount", func() {
		BeforeEach(func() {
			deployCtx.DeployableArtifact.Spec.Configuration = &openchoreov1alpha1.Configuration{
				Application: &openchoreov1alpha1.Application{
					FileMounts: []openchoreov1alpha1.FileMount{
						{
							MountPath: "/app/config.json",
							Value:     "{\"key\":\"value\"}",
						},
					},
				},
			}
		})

		It("should create a ConfigMap with correct data", func() {
			data := configMaps[0].Data
			Expect(data).To(BeComparableTo(map[string]string{
				"content": "{\"key\":\"value\"}",
			}))
		})
	})

})
