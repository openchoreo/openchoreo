// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func describeConfigCommands() {
	Context("config commands", func() {
		Context("controlplane CRUD", Ordered, func() {
			It("adds a controlplane", func() {
				_, _, err := occ.Run("config", "controlplane", "add", "test-cp", "--url", "http://localhost:9999")
				Expect(err).NotTo(HaveOccurred())
			})
			It("lists controlplanes and shows both", func() {
				stdout, _, err := occ.Run("config", "controlplane", "list")
				Expect(err).NotTo(HaveOccurred())
				Expect(stdout).To(ContainSubstring("e2e"))
				Expect(stdout).To(ContainSubstring("test-cp"))
			})
			It("updates the controlplane URL", func() {
				_, _, err := occ.Run("config", "controlplane", "update", "test-cp", "--url", "http://localhost:8888")
				Expect(err).NotTo(HaveOccurred())

				stdout, _, err := occ.Run("config", "controlplane", "list")
				Expect(err).NotTo(HaveOccurred())
				Expect(stdout).To(ContainSubstring("http://localhost:8888"))
			})
			It("deletes the controlplane", func() {
				_, _, err := occ.Run("config", "controlplane", "delete", "test-cp")
				Expect(err).NotTo(HaveOccurred())

				stdout, _, err := occ.Run("config", "controlplane", "list")
				Expect(err).NotTo(HaveOccurred())
				Expect(stdout).NotTo(ContainSubstring("test-cp"))
			})
		})

		Context("context CRUD", Ordered, func() {
			It("adds a context", func() {
				_, _, err := occ.Run("config", "context", "add", "test-ctx",
					"--controlplane", "e2e", "--credentials", "e2e-creds")
				Expect(err).NotTo(HaveOccurred())
			})
			It("lists contexts and shows both", func() {
				stdout, _, err := occ.Run("config", "context", "list")
				Expect(err).NotTo(HaveOccurred())
				Expect(stdout).To(ContainSubstring("e2e"))
				Expect(stdout).To(ContainSubstring("test-ctx"))
			})
			It("switches to the new context", func() {
				_, _, err := occ.Run("config", "context", "use", "test-ctx")
				Expect(err).NotTo(HaveOccurred())
			})
			It("updates the context with namespace, project, and resource", func() {
				_, _, err := occ.Run("config", "context", "update", "test-ctx",
					"--namespace", "default", "--project", "default", "--resource", "analytics-db")
				Expect(err).NotTo(HaveOccurred())

				stdout, _, err := occ.Run("config", "context", "list")
				Expect(err).NotTo(HaveOccurred())
				Expect(stdout).To(ContainSubstring("default"),
					"expected updated namespace/project in context list")
				Expect(stdout).To(ContainSubstring("analytics-db"),
					"expected updated resource in context list")
			})
			It("switches back to original context and deletes test context", func() {
				_, _, err := occ.Run("config", "context", "use", "e2e")
				Expect(err).NotTo(HaveOccurred())

				_, _, err = occ.Run("config", "context", "delete", "test-ctx")
				Expect(err).NotTo(HaveOccurred())

				stdout, _, err := occ.Run("config", "context", "list")
				Expect(err).NotTo(HaveOccurred())
				Expect(stdout).NotTo(ContainSubstring("test-ctx"))
			})
		})

		Context("credentials CRUD", Ordered, func() {
			It("adds credentials", func() {
				_, _, err := occ.Run("config", "credentials", "add", "test-creds")
				Expect(err).NotTo(HaveOccurred())
			})
			It("lists credentials and shows both", func() {
				stdout, _, err := occ.Run("config", "credentials", "list")
				Expect(err).NotTo(HaveOccurred())
				Expect(stdout).To(ContainSubstring("e2e-creds"))
				Expect(stdout).To(ContainSubstring("test-creds"))
			})
			It("deletes credentials", func() {
				_, _, err := occ.Run("config", "credentials", "delete", "test-creds")
				Expect(err).NotTo(HaveOccurred())

				stdout, _, err := occ.Run("config", "credentials", "list")
				Expect(err).NotTo(HaveOccurred())
				Expect(stdout).NotTo(ContainSubstring("test-creds"))
			})
		})
	})
}
