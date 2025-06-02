// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kinds

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	buildController "github.com/openchoreo/openchoreo/internal/controller/build"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// BuildResource provides operations for Build CRs.
type BuildResource struct {
	*resources.BaseResource[*choreov1.Build, *choreov1.BuildList]
}

// NewBuildResource constructs a BuildResource with CRDConfig and optionally sets organization, project, component, and deploymentTrack.
func NewBuildResource(cfg constants.CRDConfig, org string, project string, component string, deploymentTrack string) (*BuildResource, error) {
	cli, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf(ErrCreateKubeClient, err)
	}

	options := []resources.ResourceOption[*choreov1.Build, *choreov1.BuildList]{
		resources.WithClient[*choreov1.Build, *choreov1.BuildList](cli),
		resources.WithConfig[*choreov1.Build, *choreov1.BuildList](cfg),
	}

	// Add organization namespace if provided
	if org != "" {
		options = append(options, resources.WithNamespace[*choreov1.Build, *choreov1.BuildList](org))
	}

	// Create labels for filtering
	labels := map[string]string{}

	// Add project label if provided
	if project != "" {
		labels[constants.LabelProject] = project
	}

	// Add component label if provided
	if component != "" {
		labels[constants.LabelComponent] = component
	}

	// Add deployment track label if provided
	if deploymentTrack != "" {
		labels[constants.LabelDeploymentTrack] = deploymentTrack
	}

	// Add labels if any were set
	if len(labels) > 0 {
		options = append(options, resources.WithLabels[*choreov1.Build, *choreov1.BuildList](labels))
	}

	return &BuildResource{
		BaseResource: resources.NewBaseResource(options...),
	}, nil
}

// WithNamespace sets the namespace for the build resource (usually the organization name)
func (b *BuildResource) WithNamespace(namespace string) {
	b.BaseResource.WithNamespace(namespace)
}

// GetStatus returns the status of a Build with detailed information.
func (b *BuildResource) GetStatus(build *choreov1.Build) string {
	priorityConditions := []string{
		ConditionTypeCompleted,
		ConditionTypeDeployableArtifactCreated,
		ConditionTypeDeploymentApplied,
	}

	return resources.GetResourceStatus(
		build.Status.Conditions,
		priorityConditions,
		StatusInitializing,
		StatusReady,
		StatusNotReady,
	)
}

// GetAge returns the age of a Build.
func (b *BuildResource) GetAge(build *choreov1.Build) string {
	return resources.FormatAge(build.GetCreationTimestamp().Time)
}

// GetBuildDuration returns the duration of a build if completed
func (b *BuildResource) GetBuildDuration(build *choreov1.Build) string {
	conditions := build.Status.Conditions
	var startTime, endTime time.Time
	hasEnd := false

	startTime = build.GetCreationTimestamp().Time

	for _, condition := range conditions {
		// Look for build completion conditions
		if condition.Type == ConditionTypeCompleted && condition.Reason != string(buildController.ReasonBuildInProgress) {
			endTime = condition.LastTransitionTime.Time
			hasEnd = true
		}
	}

	if hasEnd {
		duration := endTime.Sub(startTime)
		return resources.FormatDuration(duration)
	}
	return resources.FormatValueOrPlaceholder("")
}

// PrintTableItems formats builds into a table
func (b *BuildResource) PrintTableItems(builds []resources.ResourceWrapper[*choreov1.Build]) error {
	if len(builds) == 0 {
		namespaceName := b.GetNamespace()
		labels := b.GetLabels()

		message := "No builds found"

		if namespaceName != "" {
			message += " in organization " + namespaceName
		}

		if project, ok := labels[constants.LabelProject]; ok {
			message += ", project " + project
		}

		if component, ok := labels[constants.LabelComponent]; ok {
			message += ", component " + component
		}

		fmt.Println(message)
		return nil
	}

	rows := make([][]string, 0, len(builds))

	for _, wrapper := range builds {
		build := wrapper.Resource
		rows = append(rows, []string{
			wrapper.LogicalName,
			b.GetStatus(build),
			build.Spec.GitRevision,
			b.GetBuildDuration(build),
			resources.FormatAge(build.GetCreationTimestamp().Time),
			build.GetLabels()[constants.LabelComponent],
			build.GetLabels()[constants.LabelProject],
			build.GetLabels()[constants.LabelOrganization],
		})
	}
	return resources.PrintTable(HeadersBuild, rows)
}

// Print overrides the base Print method to ensure our custom PrintTableItems is called
func (b *BuildResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	builds, err := b.List()
	if err != nil {
		return err
	}

	// Apply name filter if specified
	if filter != nil && filter.Name != "" {
		filtered, err := resources.FilterByName(builds, filter.Name)
		if err != nil {
			return err
		}
		builds = filtered
	}

	switch format {
	case resources.OutputFormatTable:
		return b.PrintTableItems(builds)
	case resources.OutputFormatYAML:
		return b.BaseResource.PrintItems(builds, format)
	default:
		return fmt.Errorf(ErrFormatUnsupported, format)
	}
}

// CreateBuild creates a new Build CR.
func (b *BuildResource) CreateBuild(params api.CreateBuildParams) error {
	k8sName := resources.GenerateResourceName(
		params.Organization,
		params.Project,
		params.Component,
		params.DeploymentTrack,
		params.Name,
	)

	// Create the Build resource
	build := &choreov1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sName,
			Namespace: params.Organization,
			Labels: map[string]string{
				constants.LabelName:            params.Name,
				constants.LabelOrganization:    params.Organization,
				constants.LabelProject:         params.Project,
				constants.LabelComponent:       params.Component,
				constants.LabelDeploymentTrack: params.DeploymentTrack,
			},
		},
		Spec: choreov1.BuildSpec{
			Branch:      resources.DefaultIfEmpty(params.Branch, DefaultBranch),
			Path:        resources.DefaultIfEmpty(params.Path, DefaultPath),
			GitRevision: params.Revision,
			AutoBuild:   params.AutoBuild,
		},
	}

	// Add build configuration to build based on build type
	if params.Docker != nil {
		build.Spec.BuildConfiguration = choreov1.BuildConfiguration{
			Docker: &choreov1.DockerConfiguration{
				Context:        params.Docker.Context,
				DockerfilePath: params.Docker.DockerfilePath,
			},
		}
	} else if params.Buildpack != nil {
		build.Spec.BuildConfiguration = choreov1.BuildConfiguration{
			Buildpack: &choreov1.BuildpackConfiguration{
				Name:    params.Buildpack.Name,
				Version: params.Buildpack.Version,
			},
		}
	}

	// Create the build using the base create method
	if err := b.Create(build); err != nil {
		return fmt.Errorf(ErrCreateBuild, err)
	}

	fmt.Printf(FmtBuildCreateSuccess,
		params.Name, params.Component, params.Project, params.Organization)
	return nil
}

// GetBuildsForComponent returns builds filtered by component
func (b *BuildResource) GetBuildsForComponent(componentName string) ([]resources.ResourceWrapper[*choreov1.Build], error) {
	allBuilds, err := b.List()
	if err != nil {
		return nil, err
	}

	// Filter by component
	var builds []resources.ResourceWrapper[*choreov1.Build]
	for _, wrapper := range allBuilds {
		if wrapper.Resource.GetLabels()[constants.LabelComponent] == componentName {
			builds = append(builds, wrapper)
		}
	}

	return builds, nil
}

// GetBuildsForDeploymentTrack returns builds filtered by deployment track
func (b *BuildResource) GetBuildsForDeploymentTrack(deploymentTrack string) ([]resources.ResourceWrapper[*choreov1.Build], error) {
	allBuilds, err := b.List()
	if err != nil {
		return nil, err
	}

	// Filter by deployment track
	var builds []resources.ResourceWrapper[*choreov1.Build]
	for _, wrapper := range allBuilds {
		if wrapper.Resource.GetLabels()[constants.LabelDeploymentTrack] == deploymentTrack {
			builds = append(builds, wrapper)
		}
	}

	return builds, nil
}
