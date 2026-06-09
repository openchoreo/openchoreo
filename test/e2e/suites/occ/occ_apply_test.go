// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

func describeApply() {
	Context("apply platform resources", Ordered, func() {
		It("creates the control-plane namespace via kubectl", func() {
			out, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
			Expect(err).NotTo(HaveOccurred(), "kubectl apply namespace failed: %s", out)

			By("verifying namespace exists via kubectl")
			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, "", "namespace", cpNs)
			}, 1*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying occ can see the namespace")
			Eventually(func(g Gomega) {
				stdout, _, err := occ.Run("namespace", "get", cpNs)
				g.Expect(err).NotTo(HaveOccurred(), "occ namespace get failed")
				g.Expect(stdout).To(ContainSubstring(cpNs))
			}, 1*time.Minute, 2*time.Second).Should(Succeed())
		})

		It("creates project, pipeline, and environments via occ apply", func() {
			stdout, _, err := occApply(platformResourcesYAML())
			Expect(err).NotTo(HaveOccurred(), "occ apply platform resources failed")
			expectApplySucceeded(stdout)

			By("verifying project exists via kubectl")
			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, cpNs, "project", projectName)
			}, 1*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying deployment pipeline exists via kubectl")
			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, cpNs, "deploymentpipeline", "default")
			}, 1*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying environments exist via kubectl")
			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, cpNs, "environment", envDev)
			}, 1*time.Minute, 2*time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, cpNs, "environment", envStaging)
			}, 1*time.Minute, 2*time.Second).Should(Succeed())
		})
	})

	Context("apply user resources", Ordered, func() {
		It("creates component and workload via occ apply", func() {
			stdout, _, err := occApply(componentWithWorkloadYAML())
			Expect(err).NotTo(HaveOccurred(), "occ apply component+workload failed")
			expectApplySucceeded(stdout)

			By("verifying component exists via kubectl")
			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, cpNs, "component", componentName)
			}, 1*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying ComponentRelease is created (release chain triggered)")
			Eventually(func(g Gomega) {
				out, err := framework.KubectlGet(kubeContext, cpNs, "componentrelease",
					"-o", "jsonpath={.items[*].metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				found := false
				for _, name := range strings.Fields(out) {
					if strings.HasPrefix(name, componentName+"-") {
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "no ComponentRelease found for component %s", componentName)
			}, 3*time.Minute, 2*time.Second).Should(Succeed())
		})

		It("updates existing workload via occ apply", func() {
			stdout, _, err := occApply(workloadUpdatedYAML())
			Expect(err).NotTo(HaveOccurred(), "occ apply workload update failed")
			expectApplySucceeded(stdout)

			By("verifying workload has the updated label")
			Eventually(func(g Gomega) {
				val, err := framework.KubectlGetJsonpath(kubeContext, cpNs,
					"workload", componentName, `{.metadata.labels.e2e-updated}`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(val).To(Equal("true"), "expected e2e-updated=true label on workload")
			}, 1*time.Minute, 2*time.Second).Should(Succeed())
		})
	})

	Context("apply additional resources", Ordered, func() {
		It("creates ClusterAuthzRole via occ apply", func() {
			stdout, _, err := occApply(clusterAuthzRoleYAML())
			Expect(err).NotTo(HaveOccurred(), "occ apply ClusterAuthzRole failed")
			expectApplySucceeded(stdout)

			Eventually(func(g Gomega) {
				framework.AssertClusterResourceExists(g, kubeContext, "clusterauthzrole", clusterAuthzRoleName)
			}, 1*time.Minute, 2*time.Second).Should(Succeed())
		})

		It("creates AuthzRole and AuthzRoleBinding via occ apply", func() {
			stdout, _, err := occApply(authzRoleWithBindingYAML())
			Expect(err).NotTo(HaveOccurred(), "occ apply AuthzRole+AuthzRoleBinding failed")
			expectApplySucceeded(stdout)

			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, cpNs, "authzrole", authzRoleName)
			}, 1*time.Minute, 2*time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, cpNs, "authzrolebinding", authzRoleBindingName)
			}, 1*time.Minute, 2*time.Second).Should(Succeed())
		})

		It("creates SecretReference via occ apply", func() {
			stdout, _, err := occApply(secretReferenceYAML())
			Expect(err).NotTo(HaveOccurred(), "occ apply SecretReference failed")
			expectApplySucceeded(stdout)

			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, cpNs, "secretreference", secretRefName)
			}, 1*time.Minute, 2*time.Second).Should(Succeed())
		})

		It("creates ObservabilityAlertsNotificationChannel via occ apply", func() {
			stdout, _, err := occApply(oancWebhookYAML())
			Expect(err).NotTo(HaveOccurred(), "occ apply OANC failed")
			expectApplySucceeded(stdout)

			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, cpNs,
					"observabilityalertsnotificationchannel", oancName)
			}, 1*time.Minute, 2*time.Second).Should(Succeed())
		})
	})
}
