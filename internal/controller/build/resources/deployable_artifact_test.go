/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package resources

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("Deployable Artifact Creation", func() {

	var (
		buildCtx           *integrations.BuildContext
		deployableArtifact *choreov1.DeployableArtifact
		endpoints          *[]choreov1.EndpointTemplate
	)

	BeforeEach(func() {
		buildCtx = newTestBuildContext()
		deployableArtifact = newTestDeployableArtifact()
		endpoints = newTestEndpoints()
	})

	Context("Make deployable artifact name", func() {
		It("should return the build name as the artifact name", func() {
			name := MakeDeployableArtifactName(buildCtx.Build)
			Expect(name).To(Equal("test-build"))
		})
	})

	Context("Make deployable artifact", func() {
		It("should create a deployable artifact with correct metadata and spec sections", func() {
			buildCtx.Build = newTestBuildpackBasedBuild()

			artifact := MakeDeployableArtifact(buildCtx.Build)

			Expect(artifact).NotTo(BeNil())
			Expect(artifact.Kind).To(Equal("DeployableArtifact"))
			Expect(artifact.APIVersion).To(Equal("core.choreo.dev/v1"))

			Expect(artifact.ObjectMeta.Name).To(Equal("test-build"))
			Expect(artifact.ObjectMeta.Namespace).To(Equal("test-organization"))
			Expect(artifact.ObjectMeta.Annotations).To(HaveKeyWithValue(controller.AnnotationKeyDisplayName, "test-build"))
			Expect(artifact.ObjectMeta.Annotations).To(HaveKeyWithValue(controller.AnnotationKeyDescription, "Deployable Artifact was created by the build."))

			Expect(artifact.ObjectMeta.Labels).To(HaveKeyWithValue(labels.LabelKeyOrganizationName, "test-organization"))
			Expect(artifact.ObjectMeta.Labels).To(HaveKeyWithValue(labels.LabelKeyProjectName, "test-project"))
			Expect(artifact.ObjectMeta.Labels).To(HaveKeyWithValue(labels.LabelKeyComponentName, "test-component"))
			Expect(artifact.ObjectMeta.Labels).To(HaveKeyWithValue(labels.LabelKeyDeploymentTrackName, "test-main"))
			Expect(artifact.ObjectMeta.Labels).To(HaveKeyWithValue(labels.LabelKeyName, "test-build"))

			Expect(artifact.Spec.TargetArtifact.FromBuildRef).NotTo(BeNil())
			Expect(artifact.Spec.TargetArtifact.FromBuildRef.Name).To(Equal("test-build"))
		})
	})

	Context("Add component specific configs", func() {
		BeforeEach(func() {
			buildCtx.Build = newTestBuildpackBasedBuild()
		})

		It("should add endpoint templates for service components", func() {
			buildCtx.Component.Spec.Type = choreov1.ComponentTypeService
			AddComponentSpecificConfigs(buildCtx, deployableArtifact, endpoints)
			Expect(deployableArtifact.Spec.Configuration).NotTo(BeNil())
			Expect(deployableArtifact.Spec.Configuration.EndpointTemplates).To(HaveLen(1))
			Expect(deployableArtifact.Spec.Configuration.EndpointTemplates[0].Spec.Type).To(Equal(choreov1.EndpointTypeHTTP))
			Expect(deployableArtifact.Spec.Configuration.EndpointTemplates[0].Spec.Service.Port).To(BeEquivalentTo(80))
		})

		It("should add scheduled task configuration for task components", func() {
			buildCtx.Component.Spec.Type = choreov1.ComponentTypeScheduledTask
			AddComponentSpecificConfigs(buildCtx, deployableArtifact, endpoints)
			Expect(deployableArtifact.Spec.Configuration).NotTo(BeNil())
			Expect(deployableArtifact.Spec.Configuration.Application).NotTo(BeNil())
			Expect(deployableArtifact.Spec.Configuration.Application.Task).NotTo(BeNil())
			Expect(deployableArtifact.Spec.Configuration.Application.Task.Schedule.Cron).To(Equal("*/5 * * * *"))
			Expect(deployableArtifact.Spec.Configuration.Application.Task.Schedule.Timezone).To(Equal("Asia/Colombo"))
		})

		It("should add web application endpoint template for web app components", func() {
			buildCtx.Component.Spec.Type = choreov1.ComponentTypeWebApplication
			AddComponentSpecificConfigs(buildCtx, deployableArtifact, endpoints)
			Expect(deployableArtifact.Spec.Configuration).NotTo(BeNil())
			Expect(deployableArtifact.Spec.Configuration.EndpointTemplates).To(HaveLen(1))
			Expect(deployableArtifact.Spec.Configuration.EndpointTemplates[0].ObjectMeta.Name).To(Equal("webapp"))
			Expect(deployableArtifact.Spec.Configuration.EndpointTemplates[0].Spec.Type).To(Equal(choreov1.EndpointTypeHTTP))
			Expect(deployableArtifact.Spec.Configuration.EndpointTemplates[0].Spec.Service.BasePath).To(Equal("/"))
			Expect(deployableArtifact.Spec.Configuration.EndpointTemplates[0].Spec.Service.Port).To(BeEquivalentTo(80))
		})
	})
})
