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
	corev1 "k8s.io/api/core/v1"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/dataplane"
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
			deployCtx.Component.Spec.Type = choreov1.ComponentTypeService
			deployCtx.ConfigurationGroups = []*choreov1.ConfigurationGroup{
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
			deployCtx.Component.Spec.Type = choreov1.ComponentTypeService
			deployCtx.ConfigurationGroups = []*choreov1.ConfigurationGroup{
				newTestConfigurationGroup("salesforce-config-group",
					choreov1.ConfigurationGroupSpec{
						EnvironmentGroups: []choreov1.EnvironmentGroup{
							{
								Name:         "non-prod",
								Environments: []string{"test-environment"},
							},
						},
						Configurations: []choreov1.ConfigurationGroupConfiguration{
							{
								Key: "host",
								Values: []choreov1.ConfigurationValue{
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

})
