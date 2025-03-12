package resources

import (
	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
	"github.com/choreo-idp/choreo/internal/labels"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"strings"
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
				Expect(len(name)).To(BeNumerically("==", 47))
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
