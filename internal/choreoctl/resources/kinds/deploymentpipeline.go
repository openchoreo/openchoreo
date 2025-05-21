/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package kinds

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// DeploymentPipelineResource provides operations for DeploymentPipeline CRs.
type DeploymentPipelineResource struct {
	*resources.BaseResource[*choreov1.DeploymentPipeline, *choreov1.DeploymentPipelineList]
}

// NewDeploymentPipelineResource constructs a DeploymentPipelineResource with CRDConfig and optionally sets organization.
func NewDeploymentPipelineResource(cfg constants.CRDConfig, org string) (*DeploymentPipelineResource, error) {
	cli, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	options := []resources.ResourceOption[*choreov1.DeploymentPipeline, *choreov1.DeploymentPipelineList]{
		resources.WithClient[*choreov1.DeploymentPipeline, *choreov1.DeploymentPipelineList](cli),
		resources.WithConfig[*choreov1.DeploymentPipeline, *choreov1.DeploymentPipelineList](cfg),
	}

	// Add organization namespace if provided
	if org != "" {
		options = append(options, resources.WithNamespace[*choreov1.DeploymentPipeline, *choreov1.DeploymentPipelineList](org))
	}

	// Create labels for filtering
	labels := map[string]string{}
	if org != "" {
		labels[constants.LabelOrganization] = org
	}

	// Add labels if any were set
	if len(labels) > 0 {
		options = append(options, resources.WithLabels[*choreov1.DeploymentPipeline, *choreov1.DeploymentPipelineList](labels))
	}

	return &DeploymentPipelineResource{
		BaseResource: resources.NewBaseResource[*choreov1.DeploymentPipeline, *choreov1.DeploymentPipelineList](options...),
	}, nil
}

// WithNamespace sets the namespace for the deployment pipeline resource (usually the organization name)
func (d *DeploymentPipelineResource) WithNamespace(namespace string) {
	d.BaseResource.WithNamespace(namespace)
}

// GetStatus returns the status of a DeploymentPipeline with detailed information.
func (d *DeploymentPipelineResource) GetStatus(pipeline *choreov1.DeploymentPipeline) string {
	// DeploymentPipeline uses the Available condition type
	priorityConditions := []string{
		controller.TypeAvailable,
	}

	return resources.GetResourceStatus(
		pipeline.Status.Conditions,
		priorityConditions,
		StatusPending,
		StatusReady,
		StatusFailed,
	)
}

// GetAge returns the age of a DeploymentPipeline.
func (d *DeploymentPipelineResource) GetAge(pipeline *choreov1.DeploymentPipeline) string {
	return resources.FormatAge(pipeline.GetCreationTimestamp().Time)
}

// PrintTableItems formats deployment pipelines into a table
func (d *DeploymentPipelineResource) PrintTableItems(pipelines []resources.ResourceWrapper[*choreov1.DeploymentPipeline]) error {
	if len(pipelines) == 0 {
		namespaceName := d.GetNamespace()
		message := "No deployment pipelines found"
		if namespaceName != "" {
			message += " in organization " + namespaceName
		}
		fmt.Println(message)
		return nil
	}

	rows := make([][]string, 0, len(pipelines))

	for _, wrapper := range pipelines {
		pipeline := wrapper.Resource
		rows = append(rows, []string{
			pipeline.Name,
			d.GetStatus(pipeline),
			d.GetAge(pipeline),
		})
	}

	headers := []string{"Name", "Status", "Age"}
	return resources.PrintTable(headers, rows)
}

// Print overrides the base Print method to ensure our custom PrintTableItems is called
func (d *DeploymentPipelineResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	return d.BaseResource.Print(format, filter)
}

// CreateDeploymentPipeline creates a new DeploymentPipeline CR.
func (d *DeploymentPipelineResource) CreateDeploymentPipeline(params api.CreateDeploymentPipelineParams) error {
	// Generate a K8s-compliant name for the deployment pipeline
	k8sName := resources.GenerateResourceName(params.Organization, params.Name)

	// Convert promotion paths from API params to CR structure
	promotionPaths := []choreov1.PromotionPath{}
	for _, path := range params.PromotionPaths {
		targetEnvRefs := []choreov1.TargetEnvironmentRef{}
		for _, target := range path.TargetEnvironments {
			targetEnvRefs = append(targetEnvRefs, choreov1.TargetEnvironmentRef{
				Name:                     target.Name,
				RequiresApproval:         target.RequiresApproval,
				IsManualApprovalRequired: target.IsManualApprovalRequired,
			})
		}

		promotionPaths = append(promotionPaths, choreov1.PromotionPath{
			SourceEnvironmentRef:  path.SourceEnvironment,
			TargetEnvironmentRefs: targetEnvRefs,
		})
	}

	// Create the DeploymentPipeline resource
	deploymentPipeline := &choreov1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sName,
			Namespace: params.Organization,
			Annotations: map[string]string{
				constants.AnnotationDisplayName: resources.DefaultIfEmpty(params.DisplayName, params.Name),
				constants.AnnotationDescription: params.Description,
			},
			Labels: map[string]string{
				constants.LabelName:         params.Name,
				constants.LabelOrganization: params.Organization,
			},
		},
		Spec: choreov1.DeploymentPipelineSpec{
			PromotionPaths: promotionPaths,
		},
	}

	// Create the deployment pipeline using the base create method
	if err := d.Create(deploymentPipeline); err != nil {
		return fmt.Errorf("failed to create deployment pipeline: %w", err)
	}

	fmt.Printf("Deployment pipeline '%s' created successfully in organization '%s'\n",
		params.Name, params.Organization)
	return nil
}
