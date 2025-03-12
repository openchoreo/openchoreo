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

package ci

import (
	"fmt"
	choreov1 "github.com/choreo-idp/choreo/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"strings"
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
			Expect(len(tag)).To(BeNumerically("==", 119))
		})
	})
})
