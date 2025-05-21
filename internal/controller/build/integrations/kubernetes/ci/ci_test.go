/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package ci

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

var _ = Describe("CI", func() {
	var (
		build *choreov1.Build
	)

	BeforeEach(func() {
		build = newBuildpackBasedBuild()
	})

	When("Generating image name", func() {
		It("should construct the correct image name and tag", func() {
			expectedImageName := "test-organization-test-project-test-component-999d9b43"
			expectedTag := build.Labels["core.choreo.dev/deployment-track"] + "-507223bf"

			result := ConstructImageNameWithTag(build)

			Expect(result).To(Equal(fmt.Sprintf("%s:%s", expectedImageName, expectedTag)))
		})

		It("should respect the length limit for image name", func() {
			longOrgName := strings.Repeat("a", 129)
			longProjName := strings.Repeat("b", 129)
			longComponentName := strings.Repeat("c", 129)

			build.Labels["core.choreo.dev/organization"] = longOrgName
			build.Labels["core.choreo.dev/project"] = longProjName
			build.Labels["core.choreo.dev/component"] = longComponentName

			result := ConstructImageNameWithTag(build)

			Expect(len(result)).To(BeNumerically("<", 256)) // Ensure the combined length of image name + tag is less than 256
		})

		It("should respect the length limit for image tag", func() {
			longDtName := strings.Repeat("d", 130)

			build.Labels["core.choreo.dev/deployment-track"] = longDtName

			result := ConstructImageNameWithTag(build)

			tag := result[strings.LastIndex(result, ":")+1:]
			Expect(tag).To(HaveLen(119))
		})
	})
})
