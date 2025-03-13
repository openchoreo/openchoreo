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

var _ = Describe("makePodSpec", func() {
	var (
		deployCtx *dataplane.DeploymentContext
		podSpec   *corev1.PodSpec
	)

	// Prepare fresh DeploymentContext before each test
	BeforeEach(func() {
		deployCtx = newTestDeploymentContext()
	})

	JustBeforeEach(func() {
		podSpec = makePodSpec(deployCtx)
	})

	Context("for a Service component", func() {
		BeforeEach(func() {
			deployCtx.Component.Spec.Type = choreov1.ComponentTypeService
		})

		It("should create a PodSpec with correct RestartPolicy", func() {
			Expect(podSpec.RestartPolicy).To(Equal(corev1.RestartPolicyAlways))
		})
	})

	Context("for a Scheduled Task component", func() {
		BeforeEach(func() {
			deployCtx.Component.Spec.Type = choreov1.ComponentTypeScheduledTask
		})

		It("should create a PodSpec with correct RestartPolicy", func() {
			Expect(podSpec.RestartPolicy).To(Equal(corev1.RestartPolicyNever))
		})
	})

	Context("when the deployable artifact has direct environment variables", func() {
		BeforeEach(func() {
			deployCtx.DeployableArtifact.Spec.Configuration = &choreov1.Configuration{
				Application: &choreov1.Application{
					Env: []choreov1.EnvVar{
						{
							Key:   "LOG_FORMAT",
							Value: "json",
						},
					},
				},
			}
		})

		It("should create a PodSpec with correct environment variables", func() {
			Expect(podSpec.Containers).To(HaveLen(1))
			Expect(podSpec.Containers[0].Env).To(ConsistOf(
				corev1.EnvVar{
					Name:  "LOG_FORMAT",
					Value: "json",
				},
			))
		})
	})

	Context("when the deployable artifact has environment variables mapped from configuration groups", func() {
		BeforeEach(func() {
			deployCtx.DeployableArtifact.Spec.Configuration = &choreov1.Configuration{
				Application: &choreov1.Application{
					Env: []choreov1.EnvVar{
						{
							Key: "REDIS_HOST",
							ValueFrom: &choreov1.EnvVarValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "redis-config-group",
									Key:  "host",
								},
							},
						},
						{
							Key: "REDIS_PORT",
							ValueFrom: &choreov1.EnvVarValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "redis-config-group",
									Key:  "port",
								},
							},
						},
						{
							Key: "MYSQL_HOST",
							ValueFrom: &choreov1.EnvVarValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "mysql-config-group",
									Key:  "host",
								},
							},
						},
						{
							Key: "MYSQL_PORT",
							ValueFrom: &choreov1.EnvVarValueFrom{
								ConfigurationGroupRef: &choreov1.ConfigurationGroupKeyRef{
									Name: "mysql-config-group",
									Key:  "port",
								},
							},
						},
					},
				},
			}

			deployCtx.ConfigurationGroups = []*choreov1.ConfigurationGroup{
				newTestRedisConfigurationGroup(),
				newTestMysqlConfigurationGroup(),
			}
		})

		It("should create a PodSpec with correct environment variables", func() {
			Expect(podSpec.Containers).To(HaveLen(1))

			envs := podSpec.Containers[0].Env
			By("checking the environment variables count")
			Expect(envs).To(HaveLen(4))

			By("checking the REDIS_HOST environment variable")
			Expect(envs[0].Name).To(Equal("REDIS_HOST"))
			Expect(envs[0].ValueFrom).To(BeComparableTo(&corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "my-component-my-main-track-redis-config-group-b8ef9df9",
					},
					Key: "host",
				},
			}))

			By("checking the MYSQL_PORT environment variable")
			Expect(envs[3].Name).To(Equal("MYSQL_PORT"))
			Expect(envs[3].ValueFrom).To(BeComparableTo(&corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "my-component-my-main-track-mysql-config-group-e7d2f2be",
					},
					Key: "port",
				},
			}))
		})
	})

	// Bulk mapping means that the entire configuration group is mapped to the environment variables
	// without specifying the individual keys. The generator must sanitize the keys.
	// Example Configuration group injection:
	// envFrom:
	//   - configurationGroupRef:
	//       name: redis-config
	Context("when the deployable artifact has environment variables bulk mapping a configuration group", func() {
		BeforeEach(func() {
			deployCtx.DeployableArtifact.Spec.Configuration = &choreov1.Configuration{
				Application: &choreov1.Application{
					EnvFrom: []choreov1.EnvFromSource{
						{
							ConfigurationGroupRef: &choreov1.ConfigurationGroupRef{
								Name: "redis-config-group",
							},
						},
					},
				},
			}

			deployCtx.ConfigurationGroups = []*choreov1.ConfigurationGroup{
				newTestRedisConfigurationGroup(),
				newTestMysqlConfigurationGroup(),
			}
		})

		It("should create a PodSpec with correct environment variables", func() {
			Expect(podSpec.Containers).To(HaveLen(1))

			envs := podSpec.Containers[0].Env
			By("checking the environment variables count")
			Expect(envs).To(HaveLen(2))

			By("checking the sanitized 'host' environment variable")
			Expect(envs[0].Name).To(Equal("HOST"))
			Expect(envs[0].ValueFrom).To(BeComparableTo(&corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "my-component-my-main-track-redis-config-group-b8ef9df9",
					},
					Key: "host",
				},
			}))

			By("checking the `port` environment variable")
			Expect(envs[1].Name).To(Equal("PORT"))
			Expect(envs[1].ValueFrom).To(BeComparableTo(&corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "my-component-my-main-track-redis-config-group-b8ef9df9",
					},
					Key: "port",
				},
			}))
		})
	})
})
