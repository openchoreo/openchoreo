// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kinds

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// DeploymentResource provides operations for Deployment CRs.
type DeploymentResource struct {
	*resources.BaseResource[*openchoreov1alpha1.Deployment, *openchoreov1alpha1.DeploymentList]
}

// NewDeploymentResource constructs a DeploymentResource with CRDConfig and optionally sets organization, project, component, and environment.
func NewDeploymentResource(cfg constants.CRDConfig, org string, project string, component string, environment string) (*DeploymentResource, error) {
	cli, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf(ErrCreateKubeClient, err)
	}

	options := []resources.ResourceOption[*openchoreov1alpha1.Deployment, *openchoreov1alpha1.DeploymentList]{
		resources.WithClient[*openchoreov1alpha1.Deployment, *openchoreov1alpha1.DeploymentList](cli),
		resources.WithConfig[*openchoreov1alpha1.Deployment, *openchoreov1alpha1.DeploymentList](cfg),
	}

	// Add organization namespace if provided
	if org != "" {
		options = append(options, resources.WithNamespace[*openchoreov1alpha1.Deployment, *openchoreov1alpha1.DeploymentList](org))
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

	// Add environment label if provided
	if environment != "" {
		labels[constants.LabelEnvironment] = environment
	}

	// Add labels if any were set
	if len(labels) > 0 {
		options = append(options, resources.WithLabels[*openchoreov1alpha1.Deployment, *openchoreov1alpha1.DeploymentList](labels))
	}

	return &DeploymentResource{
		BaseResource: resources.NewBaseResource[*openchoreov1alpha1.Deployment, *openchoreov1alpha1.DeploymentList](options...),
	}, nil
}

// WithNamespace sets the namespace for the deployment resource (usually the organization name)
func (d *DeploymentResource) WithNamespace(namespace string) {
	d.BaseResource.WithNamespace(namespace)
}

// GetStatus returns the status of a Deployment with detailed information.
func (d *DeploymentResource) GetStatus(deployment *openchoreov1alpha1.Deployment) string {
	// Check for important deployment-specific conditions in priority order
	priorityConditions := []string{
		ConditionTypeReady,
		ConditionTypeDeployed,
		ConditionTypeProgressing,
		ConditionTypeAvailable,
	}

	return resources.GetResourceStatus(
		deployment.Status.Conditions,
		priorityConditions,
		StatusPending,
		StatusReady,
		StatusNotReady,
	)
}

// GetAge returns the age of a Deployment.
func (d *DeploymentResource) GetAge(deployment *openchoreov1alpha1.Deployment) string {
	return resources.FormatAge(deployment.GetCreationTimestamp().Time)
}

// PrintTableItems formats deployments into a table
func (d *DeploymentResource) PrintTableItems(deployments []resources.ResourceWrapper[*openchoreov1alpha1.Deployment]) error {
	if len(deployments) == 0 {
		// Provide a more descriptive message
		namespaceName := d.GetNamespace()
		labels := d.GetLabels()

		message := "No deployments found"

		if namespaceName != "" {
			message += " in organization " + namespaceName
		}

		if project, ok := labels[constants.LabelProject]; ok {
			message += ", project " + project
		}

		if component, ok := labels[constants.LabelComponent]; ok {
			message += ", component " + component
		}

		if environment, ok := labels[constants.LabelEnvironment]; ok {
			message += ", environment " + environment
		}

		fmt.Println(message)
		return nil
	}

	rows := make([][]string, 0, len(deployments))

	for _, wrapper := range deployments {
		deploy := wrapper.Resource
		rows = append(rows, []string{
			wrapper.LogicalName,
			deploy.Spec.DeploymentArtifactRef,
			deploy.GetLabels()[constants.LabelEnvironment],
			d.GetStatus(deploy),
			resources.FormatAge(deploy.GetCreationTimestamp().Time),
			deploy.GetLabels()[constants.LabelComponent],
			deploy.GetLabels()[constants.LabelProject],
			deploy.GetLabels()[constants.LabelOrganization],
		})
	}
	return resources.PrintTable(HeadersDeployment, rows)
}

// Print overrides the base Print method to ensure our custom PrintTableItems is called
func (d *DeploymentResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	// List resources
	deployments, err := d.List()
	if err != nil {
		return err
	}

	// Apply name filter if specified
	if filter != nil && filter.Name != "" {
		filtered, err := resources.FilterByName(deployments, filter.Name)
		if err != nil {
			return err
		}
		deployments = filtered
	}

	// Call the appropriate print method based on format
	switch format {
	case resources.OutputFormatTable:
		return d.PrintTableItems(deployments)
	case resources.OutputFormatYAML:
		return d.BaseResource.PrintItems(deployments, format)
	default:
		return fmt.Errorf(ErrFormatUnsupported, format)
	}
}

// CreateDeployment creates a new Deployment CR.
func (d *DeploymentResource) CreateDeployment(params api.CreateDeploymentParams) error {
	k8sName := resources.GenerateResourceName(
		params.Organization,
		params.Project,
		params.Component,
		params.Environment,
		params.Name,
	)

	// Create the Deployment resource
	deployment := &openchoreov1alpha1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sName,
			Namespace: params.Organization,
			Labels: map[string]string{
				constants.LabelName:            params.Name,
				constants.LabelOrganization:    params.Organization,
				constants.LabelProject:         params.Project,
				constants.LabelComponent:       params.Component,
				constants.LabelEnvironment:     params.Environment,
				constants.LabelDeploymentTrack: params.DeploymentTrack,
			},
		},
		Spec: openchoreov1alpha1.DeploymentSpec{
			DeploymentArtifactRef: params.DeployableArtifact,
		},
	}

	// Add configuration overrides if provided
	if params.ConfigOverrides != nil {
		deployment.Spec.ConfigurationOverrides = params.ConfigOverrides
	}

	// Create the deployment using the base create method
	if err := d.Create(deployment); err != nil {
		return fmt.Errorf(ErrCreateDeployment, err)
	}

	fmt.Printf(FmtDeploySuccessMsg,
		params.Name, params.Environment, params.Component, params.Project, params.Organization)
	return nil
}

// GetDeploymentsForComponent returns deployments filtered by component
func (d *DeploymentResource) GetDeploymentsForComponent(componentName string) ([]resources.ResourceWrapper[*openchoreov1alpha1.Deployment], error) {
	// List all deployments in the namespace
	allDeployments, err := d.List()
	if err != nil {
		return nil, err
	}

	// Filter by component
	var deployments []resources.ResourceWrapper[*openchoreov1alpha1.Deployment]
	for _, wrapper := range allDeployments {
		if wrapper.Resource.GetLabels()[constants.LabelComponent] == componentName {
			deployments = append(deployments, wrapper)
		}
	}

	return deployments, nil
}

// GetDeploymentsForEnvironment returns deployments filtered by environment
func (d *DeploymentResource) GetDeploymentsForEnvironment(environmentName string) ([]resources.ResourceWrapper[*openchoreov1alpha1.Deployment], error) {
	// List all deployments in the namespace
	allDeployments, err := d.List()
	if err != nil {
		return nil, err
	}

	// Filter by environment
	var deployments []resources.ResourceWrapper[*openchoreov1alpha1.Deployment]
	for _, wrapper := range allDeployments {
		if wrapper.Resource.GetLabels()[constants.LabelEnvironment] == environmentName {
			deployments = append(deployments, wrapper)
		}
	}

	return deployments, nil
}

// GetDeploymentsForDeploymentTrack returns deployments filtered by deployment track
func (d *DeploymentResource) GetDeploymentsForDeploymentTrack(deploymentTrack string) ([]resources.ResourceWrapper[*openchoreov1alpha1.Deployment], error) {
	// List all deployments in the namespace
	allDeployments, err := d.List()
	if err != nil {
		return nil, err
	}

	// Filter by deployment track
	var deployments []resources.ResourceWrapper[*openchoreov1alpha1.Deployment]
	for _, wrapper := range allDeployments {
		if wrapper.Resource.GetLabels()[constants.LabelDeploymentTrack] == deploymentTrack {
			deployments = append(deployments, wrapper)
		}
	}

	return deployments, nil
}
