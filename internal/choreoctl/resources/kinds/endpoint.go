/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package kinds

import (
	"fmt"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
)

// EndpointResource provides operations for Endpoint CRs.
type EndpointResource struct {
	*resources.BaseResource[*choreov1.Endpoint, *choreov1.EndpointList]
}

// NewEndpointResource constructs an EndpointResource with CRDConfig and optionally sets organization, project, component, and environment.
func NewEndpointResource(cfg constants.CRDConfig, org string, project string, component string, environment string) (*EndpointResource, error) {
	cli, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf(ErrCreateKubeClient, err)
	}

	options := []resources.ResourceOption[*choreov1.Endpoint, *choreov1.EndpointList]{
		resources.WithClient[*choreov1.Endpoint, *choreov1.EndpointList](cli),
		resources.WithConfig[*choreov1.Endpoint, *choreov1.EndpointList](cfg),
	}

	// Add organization namespace if provided
	if org != "" {
		options = append(options, resources.WithNamespace[*choreov1.Endpoint, *choreov1.EndpointList](org))
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
		options = append(options, resources.WithLabels[*choreov1.Endpoint, *choreov1.EndpointList](labels))
	}

	return &EndpointResource{
		BaseResource: resources.NewBaseResource[*choreov1.Endpoint, *choreov1.EndpointList](options...),
	}, nil
}

// WithNamespace sets the namespace for the endpoint resource (usually the organization name)
func (e *EndpointResource) WithNamespace(namespace string) {
	e.BaseResource.WithNamespace(namespace)
}

// GetStatus returns the status of an Endpoint with detailed information.
func (e *EndpointResource) GetStatus(endpoint *choreov1.Endpoint) string {
	return resources.GetReadyStatus(
		endpoint.Status.Conditions,
		StatusPending,
		StatusReady,
		StatusNotReady,
	)
}

// GetAge returns the age of an Endpoint.
func (e *EndpointResource) GetAge(endpoint *choreov1.Endpoint) string {
	return resources.FormatAge(endpoint.GetCreationTimestamp().Time)
}

// GetAddress returns the address of an Endpoint.
func (e *EndpointResource) GetAddress(endpoint *choreov1.Endpoint) string {
	if endpoint.Status.Address == "" {
		return resources.GetPlaceholder()
	}
	return endpoint.Status.Address
}

// PrintTableItems formats endpoints into a table
func (e *EndpointResource) PrintTableItems(endpoints []resources.ResourceWrapper[*choreov1.Endpoint]) error {
	if len(endpoints) == 0 {
		// Provide a more descriptive message
		namespaceName := e.GetNamespace()
		labels := e.GetLabels()

		message := "No endpoints found"

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

	rows := make([][]string, 0, len(endpoints))

	for _, wrapper := range endpoints {
		endpoint := wrapper.Resource
		rows = append(rows, []string{
			wrapper.LogicalName,
			string(endpoint.Spec.Type),
			e.GetAddress(endpoint),
			e.GetStatus(endpoint),
			resources.FormatAge(endpoint.GetCreationTimestamp().Time),
			endpoint.GetLabels()[constants.LabelComponent],
			endpoint.GetLabels()[constants.LabelProject],
			endpoint.GetLabels()[constants.LabelOrganization],
			endpoint.GetLabels()[constants.LabelEnvironment],
		})
	}
	return resources.PrintTable(HeadersEndpoint, rows)
}

// Print overrides the base Print method to ensure our custom PrintTableItems is called
func (e *EndpointResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	// List resources
	endpoints, err := e.List()
	if err != nil {
		return err
	}

	// Apply name filter if specified
	if filter != nil && filter.Name != "" {
		filtered, err := resources.FilterByName(endpoints, filter.Name)
		if err != nil {
			return err
		}
		endpoints = filtered
	}

	// Call the appropriate print method based on format
	switch format {
	case resources.OutputFormatTable:
		return e.PrintTableItems(endpoints)
	case resources.OutputFormatYAML:
		return e.BaseResource.PrintItems(endpoints, format)
	default:
		return fmt.Errorf(ErrFormatUnsupported, format)
	}
}

// GetEndpointsForComponent returns endpoints filtered by component
func (e *EndpointResource) GetEndpointsForComponent(componentName string) ([]resources.ResourceWrapper[*choreov1.Endpoint], error) {
	// List all endpoints in the namespace
	allEndpoints, err := e.List()
	if err != nil {
		return nil, err
	}

	// Filter by component
	var endpoints []resources.ResourceWrapper[*choreov1.Endpoint]
	for _, wrapper := range allEndpoints {
		if wrapper.Resource.GetLabels()[constants.LabelComponent] == componentName {
			endpoints = append(endpoints, wrapper)
		}
	}

	return endpoints, nil
}

// GetEndpointsForEnvironment returns endpoints filtered by environment
func (e *EndpointResource) GetEndpointsForEnvironment(environmentName string) ([]resources.ResourceWrapper[*choreov1.Endpoint], error) {
	// List all endpoints in the namespace
	allEndpoints, err := e.List()
	if err != nil {
		return nil, err
	}

	// Filter by environment
	var endpoints []resources.ResourceWrapper[*choreov1.Endpoint]
	for _, wrapper := range allEndpoints {
		if wrapper.Resource.GetLabels()[constants.LabelEnvironment] == environmentName {
			endpoints = append(endpoints, wrapper)
		}
	}

	return endpoints, nil
}
