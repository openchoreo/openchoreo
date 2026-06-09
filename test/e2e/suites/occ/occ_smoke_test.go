// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func describeSmoke() {
	Context("smoke", func() {
		It("occ version returns version info", func() {
			stdout, _, err := occ.Run("version")
			Expect(err).NotTo(HaveOccurred(), "occ version failed")
			Expect(stdout).To(ContainSubstring("Version:"), "expected version info in output")
		})

		It("occ namespace list succeeds", func() {
			_, _, err := occ.Run("namespace", "list")
			Expect(err).NotTo(HaveOccurred(), "occ namespace list failed")
		})
	})
}

func describeNegative() {
	Context("negative", func() {
		It("rejects apply of read-only RenderedRelease resource", func() {
			stdout, stderr, err := occApply(renderedReleaseYAML())
			Expect(err).To(HaveOccurred(), "occ apply should have rejected RenderedRelease")
			combined := stdout + "\n" + stderr
			Expect(combined).To(ContainSubstring("read-only resource"),
				"expected read-only rejection message in output")
		})
	})
}
