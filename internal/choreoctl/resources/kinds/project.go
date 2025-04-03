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

package kinds

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// ProjectResource provides operations for Project CRs.
type ProjectResource struct {
	*resources.BaseResource[*choreov1.Project, *choreov1.ProjectList]
}

// NewProjectResource constructs a ProjectResource with CRDConfig and optionally sets organization.
func NewProjectResource(cfg constants.CRDConfig, org string) (*ProjectResource, error) {
	cli, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf(ErrCreateKubeClient, err)
	}

	options := []resources.ResourceOption[*choreov1.Project, *choreov1.ProjectList]{
		resources.WithClient[*choreov1.Project, *choreov1.ProjectList](cli),
		resources.WithConfig[*choreov1.Project, *choreov1.ProjectList](cfg),
	}

	// Add organization namespace if provided
	if org != "" {
		options = append(options, resources.WithNamespace[*choreov1.Project, *choreov1.ProjectList](org))
	}

	return &ProjectResource{
		BaseResource: resources.NewBaseResource(options...),
	}, nil
}

// WithNamespace sets the namespace for the project resource (usually the organization name)
func (p *ProjectResource) WithNamespace(namespace string) {
	p.BaseResource.WithNamespace(namespace)
}

// GetStatus returns the status of a Project with detailed information.
func (p *ProjectResource) GetStatus(proj *choreov1.Project) string {
	// Project has both Created and Ready conditions to check
	priorityConditions := []string{ConditionTypeCreated, ConditionTypeReady}
	return resources.GetResourceStatus(
		proj.Status.Conditions,
		priorityConditions,
		StatusPending,
		StatusReady,
		StatusNotReady,
	)
}

// GetAge returns the age of a Project.
func (p *ProjectResource) GetAge(proj *choreov1.Project) string {
	return resources.FormatAge(proj.GetCreationTimestamp().Time)
}

// PrintTableItems formats projects into a table
func (p *ProjectResource) PrintTableItems(projects []resources.ResourceWrapper[*choreov1.Project]) error {
	if len(projects) == 0 {
		// Provide a more descriptive message
		namespaceName := p.GetNamespace()

		message := "No projects found"

		if namespaceName != "" {
			message += " in organization " + namespaceName
		}

		fmt.Println(message)
		return nil
	}

	rows := make([][]string, 0, len(projects))

	for _, wrapper := range projects {
		proj := wrapper.Resource
		rows = append(rows, []string{
			wrapper.LogicalName,
			p.GetStatus(proj),
			p.GetAge(proj),
			proj.GetLabels()[constants.LabelOrganization],
		})
	}
	return resources.PrintTable(HeadersProject, rows)
}

// Print overrides the base Print method to ensure our custom PrintTableItems is called
func (p *ProjectResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	// List resources
	projects, err := p.List()
	if err != nil {
		return err
	}

	// Apply name filter if specified
	if filter != nil && filter.Name != "" {
		filtered, err := resources.FilterByName(projects, filter.Name)
		if err != nil {
			return err
		}
		projects = filtered
	}

	// Call the appropriate print method based on format
	switch format {
	case resources.OutputFormatTable:
		return p.PrintTableItems(projects)
	case resources.OutputFormatYAML:
		return p.BaseResource.PrintItems(projects, format)
	default:
		return fmt.Errorf(ErrFormatUnsupported, format)
	}
}

// CreateProject creates a new Project CR.
func (p *ProjectResource) CreateProject(params api.CreateProjectParams) error {
	project := &choreov1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resources.GenerateResourceName(params.Organization, params.Name),
			Namespace: params.Organization,
			Annotations: map[string]string{
				constants.AnnotationDisplayName: params.DisplayName,
				constants.AnnotationDescription: params.Description,
			},
			Labels: map[string]string{
				constants.LabelName:         params.Name,
				constants.LabelOrganization: params.Organization,
			},
		},
		Spec: choreov1.ProjectSpec{
			DeploymentPipelineRef: func() string {
				if params.DeploymentPipeline != "" {
					return params.DeploymentPipeline
				}
				return DefaultDeploymentPipeline
			}(),
		},
	}
	if err := p.Create(project); err != nil {
		return fmt.Errorf(ErrCreateProject, err)
	}

	fmt.Printf(FmtProjectSuccess, params.Name, params.Organization)
	return nil
}
