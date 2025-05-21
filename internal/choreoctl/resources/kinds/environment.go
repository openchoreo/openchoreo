/*
 * Copyright OpenChoreo Authors
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
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// EnvironmentResource provides operations for Environment CRs.
type EnvironmentResource struct {
	*resources.BaseResource[*choreov1.Environment, *choreov1.EnvironmentList]
}

// NewEnvironmentResource constructs an EnvironmentResource with CRDConfig and optionally sets organization.
func NewEnvironmentResource(cfg constants.CRDConfig, org string) (*EnvironmentResource, error) {
	cli, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf(ErrCreateKubeClient, err)
	}

	options := []resources.ResourceOption[*choreov1.Environment, *choreov1.EnvironmentList]{
		resources.WithClient[*choreov1.Environment, *choreov1.EnvironmentList](cli),
		resources.WithConfig[*choreov1.Environment, *choreov1.EnvironmentList](cfg),
	}

	// Add organization namespace if provided
	if org != "" {
		options = append(options, resources.WithNamespace[*choreov1.Environment, *choreov1.EnvironmentList](org))
	}

	return &EnvironmentResource{
		BaseResource: resources.NewBaseResource[*choreov1.Environment, *choreov1.EnvironmentList](options...),
	}, nil
}

// WithNamespace sets the namespace for the environment resource (usually the organization name)
func (e *EnvironmentResource) WithNamespace(namespace string) {
	e.BaseResource.WithNamespace(namespace)
}

// GetStatus returns the status of an Environment with detailed information.
func (e *EnvironmentResource) GetStatus(env *choreov1.Environment) string {
	// Environment can have Ready or Configured conditions
	priorityConditions := []string{ConditionTypeReady, ConditionTypeConfigured}

	return resources.GetResourceStatus(
		env.Status.Conditions,
		priorityConditions,
		StatusPending,
		StatusReady,
		StatusNotReady,
	)
}

// GetAge returns the age of an Environment.
func (e *EnvironmentResource) GetAge(env *choreov1.Environment) string {
	return resources.FormatAge(env.GetCreationTimestamp().Time)
}

// PrintTableItems formats environments into a table
func (e *EnvironmentResource) PrintTableItems(environments []resources.ResourceWrapper[*choreov1.Environment]) error {
	if len(environments) == 0 {
		// Provide a more descriptive message
		namespaceName := e.GetNamespace()

		message := "No environments found"

		if namespaceName != "" {
			message += " in organization " + namespaceName
		}

		fmt.Println(message)
		return nil
	}

	rows := make([][]string, 0, len(environments))

	for _, wrapper := range environments {
		env := wrapper.Resource
		rows = append(rows, []string{
			wrapper.LogicalName,
			resources.FormatValueOrPlaceholder(env.Spec.DataPlaneRef),
			resources.FormatBoolAsYesNo(env.Spec.IsProduction),
			resources.FormatValueOrPlaceholder(env.Spec.Gateway.DNSPrefix),
			resources.FormatAge(env.GetCreationTimestamp().Time),
			env.GetLabels()[constants.LabelOrganization],
		})
	}
	return resources.PrintTable(HeadersEnvironment, rows)
}

// Print overrides the base Print method to ensure our custom PrintTableItems is called
func (e *EnvironmentResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	// List resources
	environments, err := e.List()
	if err != nil {
		return err
	}

	// Apply name filter if specified
	if filter != nil && filter.Name != "" {
		filtered, err := resources.FilterByName(environments, filter.Name)
		if err != nil {
			return err
		}
		environments = filtered
	}

	// Call the appropriate print method based on format
	switch format {
	case resources.OutputFormatTable:
		return e.PrintTableItems(environments)
	case resources.OutputFormatYAML:
		return e.BaseResource.PrintItems(environments, format)
	default:
		return fmt.Errorf(ErrFormatUnsupported, format)
	}
}

// CreateEnvironment creates a new Environment CR.
func (e *EnvironmentResource) CreateEnvironment(params api.CreateEnvironmentParams) error {
	// Generate a K8s-compliant name for the environment
	k8sName := resources.GenerateResourceName(params.Organization, params.Name)

	// Create the Environment resource
	environment := &choreov1.Environment{
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
		Spec: choreov1.EnvironmentSpec{
			DataPlaneRef: params.DataPlaneRef,
			IsProduction: params.IsProduction,
			Gateway: choreov1.GatewayConfig{
				DNSPrefix: params.DNSPrefix,
			},
		},
	}

	// Create the environment using the base create method
	if err := e.Create(environment); err != nil {
		return fmt.Errorf(ErrCreateEnvironment, err)
	}

	fmt.Printf(FmtEnvironmentSuccess, params.Name, params.Organization)
	return nil
}

// GetEnvironmentsForOrganization returns environments filtered by organization
func (e *EnvironmentResource) GetEnvironmentsForOrganization(orgName string) ([]resources.ResourceWrapper[*choreov1.Environment], error) {
	// List all environments in the namespace
	allEnvironments, err := e.List()
	if err != nil {
		return nil, err
	}

	// Filter by organization
	var environments []resources.ResourceWrapper[*choreov1.Environment]
	for _, wrapper := range allEnvironments {
		if wrapper.Resource.GetLabels()[constants.LabelOrganization] == orgName {
			environments = append(environments, wrapper)
		}
	}

	return environments, nil
}
