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

package resources

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("Deployment Kind", func() {
	var (
		buildCtx        *integrations.BuildContext
		environmentName string
	)

	BeforeEach(func() {
		buildCtx = newTestBuildContext()
		environmentName = "dev-environment"
	})

	Context("Make deployment label name", func() {
		When("environment name is longer than 63 characters", func() {
			BeforeEach(func() {
				longEnvName := strings.Repeat("a", 100)
				environmentName = longEnvName
			})
			It("should return the workflow name of 63 characters", func() {
				name := MakeDeploymentLabelName(environmentName)
				Expect(name).To(HaveLen(47))
				Expect(name).To(Equal("aaaaaaaaaaaaaaaaaaaaaaaaaaa-deployment-4a18e108"))
			})
		})

		When("environment name is smaller than 63 characters", func() {
			BeforeEach(func() {
				smallEnvName := strings.Repeat("a", 40)
				environmentName = smallEnvName
			})
			It("should return the workflow name with less than 63 characters", func() {
				name := MakeDeploymentLabelName(environmentName)
				Expect(len(name)).To(BeNumerically("<", 63))
				Expect(name).To(Equal("aaaaaaaaaaaaaaaaaaaaaaaaaaa-deployment-1533720c"))
			})
		})
	})

	Describe("Make deployment name", func() {
		It("should generate a valid deployment name", func() {
			name := MakeDeploymentName(buildCtx.Build, environmentName)
			Expect(name).To(Equal("test-organization-test-project-test-component-test-main-dev-environment-230a338d"))
		})
	})

	Describe("Make deployment", func() {
		It("should create a Deployment resource with correct metadata and spec", func() {
			deployment := MakeDeployment(buildCtx, environmentName)

			Expect(deployment.Name).To(Equal(MakeDeploymentName(buildCtx.Build, environmentName)))
			Expect(deployment.Namespace).To(Equal(buildCtx.Build.Namespace))
			Expect(deployment.Annotations[controller.AnnotationKeyDisplayName]).To(Equal("dev-environment-deployment-dcd132b0"))
			Expect(deployment.Annotations[controller.AnnotationKeyDescription]).To(Equal("Deployment was created by the build."))
			Expect(deployment.Labels[labels.LabelKeyOrganizationName]).To(Equal("test-organization"))
			Expect(deployment.Labels[labels.LabelKeyProjectName]).To(Equal("test-project"))
			Expect(deployment.Labels[labels.LabelKeyComponentName]).To(Equal("test-component"))
			Expect(deployment.Labels[labels.LabelKeyDeploymentTrackName]).To(Equal("test-main"))
			Expect(deployment.Labels[labels.LabelKeyEnvironmentName]).To(Equal(environmentName))
			Expect(deployment.Labels[labels.LabelKeyName]).To(Equal("dev-environment-deployment-dcd132b0"))

			Expect(deployment.Spec.DeploymentArtifactRef).To(Equal(buildCtx.Build.Name))
		})
	})
})
