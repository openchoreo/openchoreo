// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

// expectDeleted verifies a resource is either fully removed or terminating.
// Resources with finalizers may linger with a deletionTimestamp after the
// delete command succeeds; both outcomes confirm the CLI delete worked.
func expectDeleted(resource, name string, args ...string) {
	Eventually(func(g Gomega) {
		getArgs := append([]string{resource, "get", name}, args...)
		stdout, stderr, err := occ.Run(getArgs...)
		if err != nil {
			g.Expect(stderr).To(ContainSubstring("not found"),
				"expected not-found error for %s/%s, got: %s", resource, name, stderr)
			return
		}
		g.Expect(stdout).To(ContainSubstring("deletionTimestamp"),
			"%s/%s still exists and is not terminating", resource, name)
	}, 2*time.Minute, 2*time.Second).Should(Succeed())
}

func describeDeleteCommands() {
	Context("delete resources", Ordered, func() {
		DescribeTable("delete namespace-scoped leaf resources",
			func(resource, name string) {
				By(fmt.Sprintf("deleting %s/%s", resource, name))
				_, _, err := occ.Run(resource, "delete", name, "-n", cpNs)
				Expect(err).NotTo(HaveOccurred(), "occ %s delete %s failed", resource, name)

				By(fmt.Sprintf("verifying %s/%s is deleted or terminating", resource, name))
				expectDeleted(resource, name, "-n", cpNs)
			},
			Entry("observabilityalertsnotificationchannel",
				"observabilityalertsnotificationchannel", oancName),
			Entry("secretreference", "secretreference", secretRefName),
			Entry("authzrolebinding", "authzrolebinding", authzRoleBindingName),
			Entry("authzrole", "authzrole", authzRoleName),
		)

		It("deletes component and verifies cascade cleanup", func() {
			_, _, err := occ.Run("component", "delete", componentName, "-n", cpNs)
			Expect(err).NotTo(HaveOccurred(), "occ component delete failed")

			By("verifying component is deleted or terminating")
			expectDeleted("component", componentName, "-n", cpNs)

			By("verifying workload was cascade-deleted")
			expectDeleted("workload", componentName, "-n", cpNs)
		})

		It("deletes project", func() {
			_, _, err := occ.Run("project", "delete", projectName, "-n", cpNs)
			Expect(err).NotTo(HaveOccurred(), "occ project delete failed")

			expectDeleted("project", projectName, "-n", cpNs)
		})

		It("deletes deployment pipeline", func() {
			_, _, err := occ.Run("deploymentpipeline", "delete", "default", "-n", cpNs)
			Expect(err).NotTo(HaveOccurred(), "occ deploymentpipeline delete failed")

			expectDeleted("deploymentpipeline", "default", "-n", cpNs)
		})

		It("deletes environments", func() {
			for _, env := range []string{envDev, envStaging} {
				By(fmt.Sprintf("deleting environment %s", env))
				_, _, err := occ.Run("environment", "delete", env, "-n", cpNs)
				Expect(err).NotTo(HaveOccurred(),
					"occ environment delete %s failed", env)
			}

			for _, env := range []string{envDev, envStaging} {
				By(fmt.Sprintf("verifying environment %s is deleted or terminating", env))
				expectDeleted("environment", env, "-n", cpNs)
			}
		})

		It("deletes cluster-scoped authz role", func() {
			_, _, err := occ.Run("clusterauthzrole", "delete", clusterAuthzRoleName)
			Expect(err).NotTo(HaveOccurred(), "occ clusterauthzrole delete failed")

			expectDeleted("clusterauthzrole", clusterAuthzRoleName)
		})
	})
}
