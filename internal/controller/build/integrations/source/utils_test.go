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

package source

import (
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
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
