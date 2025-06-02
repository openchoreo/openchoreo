// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kinds

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// ComponentResource provides operations for Component CRs.
type ComponentResource struct {
	*resources.BaseResource[*choreov1.Component, *choreov1.ComponentList]
}

// NewComponentResource constructs a ComponentResource with CRDConfig and optionally sets organization and project.
func NewComponentResource(cfg constants.CRDConfig, org string, project string) (*ComponentResource, error) {
	cli, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf(ErrCreateKubeClient, err)
	}

	options := []resources.ResourceOption[*choreov1.Component, *choreov1.ComponentList]{
		resources.WithClient[*choreov1.Component, *choreov1.ComponentList](cli),
		resources.WithConfig[*choreov1.Component, *choreov1.ComponentList](cfg),
	}

	// Add organization namespace if provided
	if org != "" {
		options = append(options, resources.WithNamespace[*choreov1.Component, *choreov1.ComponentList](org))
	}

	// Add project label if provided
	if project != "" {
		options = append(options, resources.WithLabels[*choreov1.Component, *choreov1.ComponentList](
			map[string]string{constants.LabelProject: project}))
	}

	return &ComponentResource{
		BaseResource: resources.NewBaseResource[*choreov1.Component, *choreov1.ComponentList](options...),
	}, nil
}

// WithNamespace sets the namespace for the component resource (usually the organization name)
func (c *ComponentResource) WithNamespace(namespace string) {
	c.BaseResource.WithNamespace(namespace)
}

// GetStatus returns the status of a Component with detailed information.
func (c *ComponentResource) GetStatus(comp *choreov1.Component) string {
	return resources.GetReadyStatus(
		comp.Status.Conditions,
		StatusPending,
		StatusReady,
		StatusNotReady,
	)
}

// GetAge returns the age of a Component.
func (c *ComponentResource) GetAge(comp *choreov1.Component) string {
	return resources.FormatAge(comp.GetCreationTimestamp().Time)
}

// PrintTableItems formats components into a table
func (c *ComponentResource) PrintTableItems(components []resources.ResourceWrapper[*choreov1.Component]) error {
	if len(components) == 0 {
		namespaceName := c.GetNamespace()
		labels := c.GetLabels()

		message := "No components found"

		if namespaceName != "" {
			message += " in organization " + namespaceName
		}

		if project, ok := labels[constants.LabelProject]; ok {
			message += ", project " + project
		}

		fmt.Println(message)
		return nil
	}

	rows := make([][]string, 0, len(components))

	for _, wrapper := range components {
		comp := wrapper.Resource
		rows = append(rows, []string{
			wrapper.LogicalName,
			string(comp.Spec.Type),
			c.GetStatus(comp),
			resources.FormatAge(comp.GetCreationTimestamp().Time),
			comp.GetLabels()[constants.LabelProject],
			comp.GetLabels()[constants.LabelOrganization],
		})
	}
	return resources.PrintTable(HeadersComponent, rows)
}

// Print overrides the base Print method to ensure our custom PrintTableItems is called
func (c *ComponentResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	components, err := c.List()
	if err != nil {
		return err
	}

	if filter != nil && filter.Name != "" {
		filtered, err := resources.FilterByName(components, filter.Name)
		if err != nil {
			return err
		}
		components = filtered
	}

	switch format {
	case resources.OutputFormatTable:
		return c.PrintTableItems(components)
	case resources.OutputFormatYAML:
		return c.BaseResource.PrintItems(components, format)
	default:
		return fmt.Errorf(ErrFormatUnsupported, format)
	}
}

// CreateComponent creates a new Component CR and its default deployment track.
func (c *ComponentResource) CreateComponent(params api.CreateComponentParams) error {
	k8sName := resources.GenerateResourceName(params.Organization, params.Project, params.Name)

	// Create the Component resource
	component := &choreov1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sName,
			Namespace: params.Organization,
			Annotations: map[string]string{
				constants.AnnotationDisplayName: params.DisplayName,
				constants.AnnotationDescription: params.Description,
			},
			Labels: map[string]string{
				constants.LabelName:         params.Name,
				constants.LabelOrganization: params.Organization,
				constants.LabelProject:      params.Project,
				constants.LabelType:         string(params.Type),
			},
		},
		Spec: choreov1.ComponentSpec{
			Type: params.Type,
			Source: choreov1.ComponentSource{
				GitRepository: &choreov1.GitRepository{
					URL: params.GitRepositoryURL,
				},
			},
		},
	}

	// Create the component using the base create method
	if err := c.Create(component); err != nil {
		return fmt.Errorf(ErrCreateComponent, err)
	}

	// Create default deployment track
	trackName := resources.GenerateResourceName(params.Organization, params.Project, params.Name, DefaultTrackName)
	deploymentTrack := &choreov1.DeploymentTrack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trackName,
			Namespace: params.Organization,
			Labels: map[string]string{
				constants.LabelName:         DefaultTrackName,
				constants.LabelOrganization: params.Organization,
				constants.LabelProject:      params.Project,
				constants.LabelComponent:    params.Name,
			},
		},
		Spec: choreov1.DeploymentTrackSpec{
			BuildTemplateSpec: &choreov1.BuildTemplateSpec{
				Branch: resources.DefaultIfEmpty(params.Branch, DefaultBranch),
				Path:   resources.DefaultIfEmpty(params.Path, DefaultPath),
			},
		},
	}

	// Add build configuration to deployment track based on component's build type
	if params.DockerFile != "" || params.DockerContext != "" {
		deploymentTrack.Spec.BuildTemplateSpec.BuildConfiguration = &choreov1.BuildConfiguration{
			Docker: &choreov1.DockerConfiguration{
				Context:        resources.DefaultIfEmpty(params.DockerContext, DefaultContext),
				DockerfilePath: resources.DefaultIfEmpty(params.DockerFile, DefaultDockerfile),
			},
		}
	} else if params.BuildpackName != "" || params.BuildpackVersion != "" {
		deploymentTrack.Spec.BuildTemplateSpec.BuildConfiguration = &choreov1.BuildConfiguration{
			Buildpack: &choreov1.BuildpackConfiguration{
				Name:    choreov1.BuildpackName(params.BuildpackName),
				Version: params.BuildpackVersion,
			},
		}
	}

	// We need to access the client directly to create resources of a different type
	ctx := context.Background()
	if err := c.BaseResource.GetClient().Create(ctx, deploymentTrack); err != nil {
		return fmt.Errorf(ErrCreateDepTrack, err)
	}

	fmt.Printf(FmtComponentSuccess,
		params.Name, params.Project, params.Organization)
	return nil
}

// GetComponentsForProject returns components filtered by project
func (c *ComponentResource) GetComponentsForProject(projectName string) ([]resources.ResourceWrapper[*choreov1.Component], error) {
	allComponents, err := c.List()
	if err != nil {
		return nil, err
	}

	var components []resources.ResourceWrapper[*choreov1.Component]
	for _, wrapper := range allComponents {
		if wrapper.Resource.GetLabels()[constants.LabelProject] == projectName {
			components = append(components, wrapper)
		}
	}

	return components, nil
}
