// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package kinds

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// DeployableArtifactResource provides operations for DeployableArtifact CRs.
type DeployableArtifactResource struct {
	*resources.BaseResource[*choreov1.DeployableArtifact, *choreov1.DeployableArtifactList]
}

// NewDeployableArtifactResource constructs a DeployableArtifactResource with CRDConfig and optionally sets organization, project, component, and deploymentTrack.
func NewDeployableArtifactResource(cfg constants.CRDConfig, org string, project string, component string, deploymentTrack string) (*DeployableArtifactResource, error) {
	cli, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	options := []resources.ResourceOption[*choreov1.DeployableArtifact, *choreov1.DeployableArtifactList]{
		resources.WithClient[*choreov1.DeployableArtifact, *choreov1.DeployableArtifactList](cli),
		resources.WithConfig[*choreov1.DeployableArtifact, *choreov1.DeployableArtifactList](cfg),
	}

	// Add organization namespace if provided
	if org != "" {
		options = append(options, resources.WithNamespace[*choreov1.DeployableArtifact, *choreov1.DeployableArtifactList](org))
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
		options = append(options, resources.WithLabels[*choreov1.DeployableArtifact, *choreov1.DeployableArtifactList](labels))
	}

	return &DeployableArtifactResource{
		BaseResource: resources.NewBaseResource[*choreov1.DeployableArtifact, *choreov1.DeployableArtifactList](options...),
	}, nil
}

// WithNamespace sets the namespace for the deployable artifact resource (usually the organization name)
func (d *DeployableArtifactResource) WithNamespace(namespace string) {
	d.BaseResource.WithNamespace(namespace)
}

// GetStatus returns the status of a DeployableArtifact with detailed information.
func (d *DeployableArtifactResource) GetStatus(artifact *choreov1.DeployableArtifact) string {
	return resources.GetReadyStatus(
		nil,
		StatusPending,
		StatusReady,
		StatusNotReady,
	)
}

// GetAge returns the age of a DeployableArtifact.
func (d *DeployableArtifactResource) GetAge(artifact *choreov1.DeployableArtifact) string {
	return resources.FormatAge(artifact.GetCreationTimestamp().Time)
}

// GetSource returns the source of a DeployableArtifact.
func (d *DeployableArtifactResource) GetSource(artifact *choreov1.DeployableArtifact) string {
	return "Manual"
}

// GetArtifactSource returns a string describing the source of the deployable artifact.
func (d *DeployableArtifactResource) GetArtifactSource(artifact *choreov1.DeployableArtifact) string {
	if artifact.Spec.TargetArtifact.FromBuildRef != nil {
		return fmt.Sprintf("build:%s", artifact.Spec.TargetArtifact.FromBuildRef.Name)
	}
	if artifact.Spec.TargetArtifact.FromImageRef != nil {
		return fmt.Sprintf("image:%s", artifact.Spec.TargetArtifact.FromImageRef.Tag)
	}
	return "unknown"
}

// PrintTableItems formats deployable artifacts into a table
func (d *DeployableArtifactResource) PrintTableItems(artifacts []resources.ResourceWrapper[*choreov1.DeployableArtifact]) error {
	if len(artifacts) == 0 {
		// Provide a more descriptive message
		namespaceName := d.GetNamespace()
		labels := d.GetLabels()

		message := "No deployable artifacts found"

		if namespaceName != "" {
			message += " in organization " + namespaceName
		}

		if project, ok := labels[constants.LabelProject]; ok {
			message += ", project " + project
		}

		if component, ok := labels[constants.LabelComponent]; ok {
			message += ", component " + component
		}

		if deploymentTrack, ok := labels[constants.LabelDeploymentTrack]; ok {
			message += ", deployment track " + deploymentTrack
		}

		fmt.Println(message)
		return nil
	}

	rows := make([][]string, 0, len(artifacts))

	for _, wrapper := range artifacts {
		artifact := wrapper.Resource
		rows = append(rows, []string{
			wrapper.LogicalName,
			d.GetSource(artifact),
			d.GetStatus(artifact),
			resources.FormatAge(artifact.GetCreationTimestamp().Time),
			artifact.GetLabels()[constants.LabelComponent],
			artifact.GetLabels()[constants.LabelProject],
			artifact.GetLabels()[constants.LabelOrganization],
		})
	}
	return resources.PrintTable(HeadersDeployableArtifact, rows)
}

// Print overrides the base Print method to ensure our custom PrintTableItems is called
func (d *DeployableArtifactResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	// List resources
	artifacts, err := d.List()
	if err != nil {
		return err
	}

	// Apply name filter if specified
	if filter != nil && filter.Name != "" {
		filtered, err := resources.FilterByName(artifacts, filter.Name)
		if err != nil {
			return err
		}
		artifacts = filtered
	}

	// Call the appropriate print method based on format
	switch format {
	case resources.OutputFormatTable:
		return d.PrintTableItems(artifacts)
	case resources.OutputFormatYAML:
		return d.BaseResource.PrintItems(artifacts, format)
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
}

// CreateDeployableArtifact creates a new DeployableArtifact CR.
func (d *DeployableArtifactResource) CreateDeployableArtifact(params api.CreateDeployableArtifactParams) error {
	k8sName := resources.GenerateResourceName(
		params.Organization,
		params.Project,
		params.Component,
		params.DeploymentTrack,
		params.Name,
	)

	// Create the DeployableArtifact resource
	deployableArtifact := &choreov1.DeployableArtifact{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sName,
			Namespace: params.Organization,
			Annotations: map[string]string{
				constants.AnnotationDisplayName: resources.DefaultIfEmpty(params.DisplayName, params.Name),
				constants.AnnotationDescription: params.Description,
			},
			Labels: map[string]string{
				constants.LabelName:            params.Name,
				constants.LabelOrganization:    params.Organization,
				constants.LabelProject:         params.Project,
				constants.LabelComponent:       params.Component,
				constants.LabelDeploymentTrack: params.DeploymentTrack,
			},
		},
		Spec: choreov1.DeployableArtifactSpec{
			TargetArtifact: choreov1.TargetArtifact{
				FromBuildRef: params.FromBuildRef,
				FromImageRef: params.FromImageRef,
			},
			Configuration: params.Configuration,
		},
	}

	// Create the deployable artifact using the base create method
	if err := d.Create(deployableArtifact); err != nil {
		return fmt.Errorf("failed to create deployable artifact: %w", err)
	}

	fmt.Printf("Deployable artifact '%s' created successfully in component '%s' of project '%s' in organization '%s'\n",
		params.Name, params.Component, params.Project, params.Organization)
	return nil
}

// GetDeployableArtifactsForComponent returns deployable artifacts filtered by component
func (d *DeployableArtifactResource) GetDeployableArtifactsForComponent(componentName string) ([]resources.ResourceWrapper[*choreov1.DeployableArtifact], error) {
	// List all deployable artifacts in the namespace
	allArtifacts, err := d.List()
	if err != nil {
		return nil, err
	}

	// Filter by component
	var artifacts []resources.ResourceWrapper[*choreov1.DeployableArtifact]
	for _, wrapper := range allArtifacts {
		if wrapper.Resource.GetLabels()[constants.LabelComponent] == componentName {
			artifacts = append(artifacts, wrapper)
		}
	}

	return artifacts, nil
}

// GetDeployableArtifactsForDeploymentTrack returns deployable artifacts filtered by deployment track
func (d *DeployableArtifactResource) GetDeployableArtifactsForDeploymentTrack(deploymentTrack string) ([]resources.ResourceWrapper[*choreov1.DeployableArtifact], error) {
	allArtifacts, err := d.List()
	if err != nil {
		return nil, err
	}

	var artifacts []resources.ResourceWrapper[*choreov1.DeployableArtifact]
	for _, wrapper := range allArtifacts {
		if wrapper.Resource.GetLabels()[constants.LabelDeploymentTrack] == deploymentTrack {
			artifacts = append(artifacts, wrapper)
		}
	}

	return artifacts, nil
}
