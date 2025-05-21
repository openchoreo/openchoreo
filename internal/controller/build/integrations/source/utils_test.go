/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package source

import (
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
)

var _ = Describe("Source Utilities", func() {
	var (
		buildCtx *integrations.BuildContext
	)

	BeforeEach(func() {
		buildCtx = newTestBuildContext()
	})

	Describe("Extract repository info", func() {
		It("should return an error for an empty URL", func() {
			owner, repo, err := ExtractRepositoryInfo("")
			Expect(owner).To(BeEmpty())
			Expect(repo).To(BeEmpty())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("repository URL is empty"))
		})

		It("should return an error for an invalid URL scheme", func() {
			owner, repo, err := ExtractRepositoryInfo("ftp://github.com/user/repo")
			Expect(owner).To(BeEmpty())
			Expect(repo).To(BeEmpty())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid repository URL"))
		})

		It("should extract owner and repo from a valid HTTPS URL", func() {
			owner, repo, err := ExtractRepositoryInfo("https://github.com/user/repo")
			Expect(err).ToNot(HaveOccurred())
			Expect(owner).To(Equal("user"))
			Expect(repo).To(Equal("repo"))
		})
	})

	Describe("Make component descriptorPath", func() {
		It("should return the component descriptor path based on the build context", func() {
			buildCtx.Build = newTestBuildpackBasedBuild()
			expectedPath := path.Clean("./test-service/.choreo/component.yaml")
			actualPath := MakeComponentDescriptorPath(buildCtx)
			Expect(actualPath).To(Equal(expectedPath))
		})

		It("should return the default component descriptor path when build context is empty", func() {
			actualPath := MakeComponentDescriptorPath(buildCtx)
			Expect(actualPath).To(Equal("./.choreo/component.yaml"))
		})
	})
})
