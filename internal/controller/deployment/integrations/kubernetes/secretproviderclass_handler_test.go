/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package kubernetes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	csisecretv1 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/secretstorecsi/v1"
)

var _ = Describe("makeSecretProviderClasses", func() {
	var (
		deployCtx             *dataplane.DeploymentContext
		secretProviderClasses []*csisecretv1.SecretProviderClass

		expectedLabels = map[string]string{
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
	)

	// Prepare fresh DeploymentContext before each test
	BeforeEach(func() {
		deployCtx = newTestDeploymentContext()
	})

	JustBeforeEach(func() {
		secretProviderClasses = makeSecretProviderClasses(deployCtx)
	})

	Context("for two Configuration Groups", func() {
		BeforeEach(func() {
			deployCtx.Component.Spec.Type = choreov1.ComponentTypeService
			deployCtx.ConfigurationGroups = []*choreov1.ConfigurationGroup{
				newTestRedisConfigurationGroup(),
				newTestMysqlConfigurationGroup(),
			}
		})

		It("should create two SecretProviderClasses with correct name and namespace", func() {
			By("checking the generated SecretProviderClass count")
			Expect(secretProviderClasses).To(HaveLen(2))

			By("checking the SecretProviderClass 1 name and namespace")
			Expect(secretProviderClasses[0].Name).To(Equal("my-component-my-main-track-redis-config-group-b8ef9df9"))
			Expect(secretProviderClasses[0].Namespace).To(Equal("dp-test-organiza-my-project-test-environ-04bdf416"))

			By("checking the SecretProviderClass 2 name and namespace")
			Expect(secretProviderClasses[1].Name).To(Equal("my-component-my-main-track-mysql-config-group-e7d2f2be"))
			Expect(secretProviderClasses[1].Namespace).To(Equal("dp-test-organiza-my-project-test-environ-04bdf416"))
		})

		It("should create SecretProviderClasses with valid labels", func() {
			Expect(secretProviderClasses[0].Labels).To(BeComparableTo(expectedLabels))
		})

		It("should create the SecretProviderClas 1 with correct spec", func() {
			spec := secretProviderClasses[0].Spec
			By("checking the Provider")
			Expect(spec.Provider).To(Equal(csisecretv1.Provider("vault")))

			By("checking the Parameters")
			Expect(spec.Parameters).To(BeComparableTo(map[string]string{
				"roleName":     "choreo-secret-reader-role",
				"vaultAddress": "http://choreo-dp-vault:8200",
				"objects":      "- objectName: password\n  secretPath: secret/test/redis/password\n  secretKey: value\n",
			}))

			By("checking the SecretObject count")
			Expect(spec.SecretObjects).To(HaveLen(1))

			By("checking the SecretObject 1")
			secretObj := spec.SecretObjects[0]
			Expect(secretObj.SecretName).To(Equal("my-component-my-main-track-redis-config-group-b8ef9df9"))
			Expect(secretObj.Type).To(Equal("Opaque"))
			Expect(secretObj.Labels).To(BeComparableTo(expectedLabels))
			Expect(secretObj.Data).To(BeComparableTo([]*csisecretv1.SecretObjectData{
				{
					ObjectName: "password",
					Key:        "password",
				},
			}))
		})

		It("should create the SecretProviderClas 2 with correct spec", func() {
			spec := secretProviderClasses[1].Spec
			By("checking the Provider")
			Expect(spec.Provider).To(Equal(csisecretv1.Provider("vault")))

			By("checking the Parameters")
			Expect(spec.Parameters).To(BeComparableTo(map[string]string{
				"roleName":     "choreo-secret-reader-role",
				"vaultAddress": "http://choreo-dp-vault:8200",
				"objects":      "- objectName: password\n  secretPath: secret/test/mysql/password\n  secretKey: value\n",
			}))

			By("checking the SecretObject count")
			Expect(spec.SecretObjects).To(HaveLen(1))

			By("checking the SecretObject 1")
			secretObj := spec.SecretObjects[0]
			Expect(secretObj.SecretName).To(Equal("my-component-my-main-track-mysql-config-group-e7d2f2be"))
			Expect(secretObj.Type).To(Equal("Opaque"))
			Expect(secretObj.Labels).To(BeComparableTo(expectedLabels))
			Expect(secretObj.Data).To(BeComparableTo([]*csisecretv1.SecretObjectData{
				{
					ObjectName: "password",
					Key:        "password",
				},
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
								Key: "client-secret",
								Values: []choreov1.ConfigurationValue{
									{
										EnvironmentGroupRef: "non-prod",
										VaultKey:            "secret/test/salesforce/client-secret",
									},
								},
							},
						},
					},
				),
			}
		})

		It("should create a SecretProviderClass with correct spec", func() {
			spec := secretProviderClasses[0].Spec
			By("checking the Provider")
			Expect(spec.Provider).To(Equal(csisecretv1.Provider("vault")))

			By("checking the Parameters")
			Expect(spec.Parameters).To(BeComparableTo(map[string]string{
				"roleName":     "choreo-secret-reader-role",
				"vaultAddress": "http://choreo-dp-vault:8200",
				"objects":      "- objectName: client-secret\n  secretPath: secret/test/salesforce/client-secret\n  secretKey: value\n",
			}))

			By("checking the SecretObject 1")
			secretObj := spec.SecretObjects[0]
			Expect(secretObj.SecretName).To(Equal("my-component-my-main-track-salesforce-config-group-02c7b6b2"))
			Expect(secretObj.Type).To(Equal("Opaque"))
			Expect(secretObj.Labels).To(BeComparableTo(expectedLabels))
			Expect(secretObj.Data).To(BeComparableTo([]*csisecretv1.SecretObjectData{
				{
					ObjectName: "client-secret",
					Key:        "client-secret",
				},
			}))
		})
	})

})
