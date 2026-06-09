// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func describeSecretCommands() {
	Context("secret commands", Ordered, func() {
		const secretName = "occ-e2e-test-secret"

		It("creates a generic secret", func() {
			_, _, err := occ.Run("secret", "create", "generic", secretName,
				"-n", cpNs,
				"--target-plane", "ClusterDataPlane/default",
				"--from-literal", "username=admin",
				"--from-literal", "password=s3cret")
			Expect(err).NotTo(HaveOccurred(), "occ secret create generic failed")
		})

		It("lists secrets and shows the created secret", func() {
			stdout, _, err := occ.Run("secret", "list", "-n", cpNs)
			Expect(err).NotTo(HaveOccurred(), "occ secret list failed")
			Expect(stdout).To(ContainSubstring(secretName))
		})

		It("gets the secret by name", func() {
			stdout, _, err := occ.Run("secret", "get", secretName, "-n", cpNs)
			Expect(err).NotTo(HaveOccurred(), "occ secret get failed")
			Expect(stdout).To(ContainSubstring(secretName))
		})

		It("updates the secret", func() {
			_, _, err := occ.Run("secret", "update", secretName,
				"-n", cpNs,
				"--from-literal", "password=n3ws3cret")
			Expect(err).NotTo(HaveOccurred(), "occ secret update failed")
		})

		It("creates a docker-registry secret", func() {
			_, _, err := occ.Run("secret", "create", "docker-registry", "occ-e2e-regcred",
				"-n", cpNs,
				"--target-plane", "ClusterDataPlane/default",
				"--docker-server", "https://index.docker.io/v1/",
				"--docker-username", "testuser",
				"--docker-password", "testpass")
			Expect(err).NotTo(HaveOccurred(), "occ secret create docker-registry failed")
		})

		It("deletes the secrets", func() {
			_, _, err := occ.Run("secret", "delete", secretName, "-n", cpNs)
			Expect(err).NotTo(HaveOccurred(), "occ secret delete failed")

			_, _, err = occ.Run("secret", "delete", "occ-e2e-regcred", "-n", cpNs)
			Expect(err).NotTo(HaveOccurred(), "occ secret delete docker-registry failed")
		})
	})
}
