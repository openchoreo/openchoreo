// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

func describeLoginCommands() {
	Context("client-credentials login", Ordered, func() {
		It("logs in with --client-credentials flags", func() {
			stdout, _, err := occ.Run("login",
				"--client-credentials",
				"--client-id", clientID,
				"--client-secret", clientSecret)
			Expect(err).NotTo(HaveOccurred(), "occ login --client-credentials failed")
			Expect(stdout).To(ContainSubstring("Authentication successful"),
				"expected success message in login output")
		})

		It("verifies API access works after login", func() {
			_, _, err := occ.Run("namespace", "list")
			Expect(err).NotTo(HaveOccurred(),
				"occ namespace list should succeed after client-credentials login")
		})

		It("logs in with --credential flag to create a named credential", func() {
			stdout, _, err := occ.Run("login",
				"--client-credentials",
				"--client-id", clientID,
				"--client-secret", clientSecret,
				"--credential", "login-e2e-creds")
			Expect(err).NotTo(HaveOccurred(), "occ login with --credential failed")
			Expect(stdout).To(ContainSubstring("Authentication successful"))

			By("verifying the named credential appears in config")
			stdout, _, err = occ.Run("config", "credentials", "list")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).To(ContainSubstring("login-e2e-creds"),
				"expected named credential in credentials list")
		})
	})

	Context("login via environment variables", Ordered, func() {
		It("logs in using OCC_CLIENT_ID and OCC_CLIENT_SECRET env vars", func() {
			envVars := []string{
				fmt.Sprintf("OCC_CLIENT_ID=%s", clientID),
				fmt.Sprintf("OCC_CLIENT_SECRET=%s", clientSecret),
			}
			stdout, _, err := occ.RunWithEnv(envVars,
				"login", "--client-credentials")
			Expect(err).NotTo(HaveOccurred(), "occ login with env vars failed")
			Expect(stdout).To(ContainSubstring("Authentication successful"),
				"expected success message in env-var login output")
		})

		It("verifies API access works after env var login", func() {
			_, _, err := occ.Run("namespace", "list")
			Expect(err).NotTo(HaveOccurred(),
				"occ namespace list should succeed after env-var login")
		})
	})

	Context("logout", Ordered, func() {
		It("logs out successfully", func() {
			stdout, _, err := occ.Run("logout")
			Expect(err).NotTo(HaveOccurred(), "occ logout failed")
			Expect(stdout).To(ContainSubstring("Logged out successfully"),
				"expected logout success message")
		})

		It("fails API calls after logout", func() {
			_, stderr, err := occ.Run("namespace", "list")
			Expect(err).To(HaveOccurred(),
				"occ namespace list should fail after logout")
			Expect(stderr).To(ContainSubstring("Authentication required"),
				"expected authentication required message after logout")
		})

		It("can re-login after logout", func() {
			stdout, _, err := occ.Run("login",
				"--client-credentials",
				"--client-id", clientID,
				"--client-secret", clientSecret)
			Expect(err).NotTo(HaveOccurred(), "occ login after logout failed")
			Expect(stdout).To(ContainSubstring("Authentication successful"))
		})

		It("verifies API access works after re-login", func() {
			_, _, err := occ.Run("namespace", "list")
			Expect(err).NotTo(HaveOccurred(),
				"occ namespace list should succeed after re-login")
		})
	})

	Context("login negative cases", func() {
		It("fails without --client-id and --client-secret", func() {
			clearEnv := []string{"OCC_CLIENT_ID=", "OCC_CLIENT_SECRET="}
			_, stderr, err := occ.RunWithEnv(clearEnv,
				"login", "--client-credentials")
			Expect(err).To(HaveOccurred(),
				"occ login should fail without client-id and client-secret")
			Expect(stderr).To(ContainSubstring("client ID and client secret are required"),
				"expected missing credentials message")
		})

		It("fails without --client-secret", func() {
			clearEnv := []string{"OCC_CLIENT_SECRET="}
			_, stderr, err := occ.RunWithEnv(clearEnv,
				"login", "--client-credentials", "--client-id", clientID)
			Expect(err).To(HaveOccurred(),
				"occ login should fail without client-secret")
			Expect(stderr).To(ContainSubstring("client ID and client secret are required"),
				"expected missing credentials message")
		})

		It("fails with invalid credentials", func() {
			clearEnv := []string{"OCC_CLIENT_ID=", "OCC_CLIENT_SECRET="}
			_, stderr, err := occ.RunWithEnv(clearEnv,
				"login", "--client-credentials",
				"--client-id", "invalid-id",
				"--client-secret", "invalid-secret")
			Expect(err).To(HaveOccurred(),
				"occ login should fail with invalid credentials")
			Expect(stderr).To(ContainSubstring("failed to get access token"),
				"expected token failure message for invalid credentials")
		})
	})

	AfterAll(func() {
		By("re-seeding occ config to restore state for subsequent tests")
		token, err := framework.FetchClientCredentialsToken(tokenURL, clientID, clientSecret)
		Expect(err).NotTo(HaveOccurred(), "failed to fetch token for config re-seed")
		Expect(occ.SeedConfig(token)).To(Succeed(), "failed to re-seed occ config")
	})
}
