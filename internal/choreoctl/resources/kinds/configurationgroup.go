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
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
)

// ConfigurationGroupResource provides operations for ConfigurationGroup CRs.
type ConfigurationGroupResource struct {
	*resources.BaseResource[*choreov1.ConfigurationGroup, *choreov1.ConfigurationGroupList]
}

// NewConfigurationGroupResource constructs a ConfigurationGroupResource with CRDConfig and optionally sets organization.
func NewConfigurationGroupResource(cfg constants.CRDConfig, org string) (*ConfigurationGroupResource, error) {
	cli, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	options := []resources.ResourceOption[*choreov1.ConfigurationGroup, *choreov1.ConfigurationGroupList]{
		resources.WithClient[*choreov1.ConfigurationGroup, *choreov1.ConfigurationGroupList](cli),
		resources.WithConfig[*choreov1.ConfigurationGroup, *choreov1.ConfigurationGroupList](cfg),
	}

	// Add organization namespace if provided
	if org != "" {
		options = append(options, resources.WithNamespace[*choreov1.ConfigurationGroup, *choreov1.ConfigurationGroupList](org))
	}

	// Create labels for filtering
	labels := map[string]string{}
	if org != "" {
		labels[constants.LabelOrganization] = org
	}

	// Add labels if any were set
	if len(labels) > 0 {
		options = append(options, resources.WithLabels[*choreov1.ConfigurationGroup, *choreov1.ConfigurationGroupList](labels))
	}

	return &ConfigurationGroupResource{
		BaseResource: resources.NewBaseResource[*choreov1.ConfigurationGroup, *choreov1.ConfigurationGroupList](options...),
	}, nil
}

// WithNamespace sets the namespace for the configuration group resource (usually the organization name)
func (d *ConfigurationGroupResource) WithNamespace(namespace string) {
	d.BaseResource.WithNamespace(namespace)
}

// GetStatus returns the status of a ConfigurationGroup with detailed information.
func (d *ConfigurationGroupResource) GetStatus(cg *choreov1.ConfigurationGroup) string {
	// ConfigurationGroup uses the Available condition type
	priorityConditions := []string{
		controller.TypeAvailable,
	}

	return resources.GetResourceStatus(
		cg.Status.Conditions,
		priorityConditions,
		StatusPending,
		StatusReady,
		StatusFailed,
	)
}

// GetAge returns the age of a ConfigurationGroup.
func (d *ConfigurationGroupResource) GetAge(cg *choreov1.ConfigurationGroup) string {
	return resources.FormatAge(cg.GetCreationTimestamp().Time)
}

// PrintTableItems formats configuration groups into a table
func (d *ConfigurationGroupResource) PrintTableItems(cgs []resources.ResourceWrapper[*choreov1.ConfigurationGroup]) error {
	if len(cgs) == 0 {
		namespaceName := d.GetNamespace()
		message := "No configuration groups found"
		if namespaceName != "" {
			message += " in organization " + namespaceName
		}
		fmt.Println(message)
		return nil
	}

	rows := make([][]string, 0, len(cgs))

	for _, wrapper := range cgs {
		cg := wrapper.Resource
		rows = append(rows, []string{
			cg.Name,
			d.GetStatus(cg),
			d.GetAge(cg),
		})
	}

	headers := []string{"Name", "Status", "Age"}
	return resources.PrintTable(headers, rows)
}

// Print overrides the base Print method to ensure our custom PrintTableItems is called
func (d *ConfigurationGroupResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	return d.BaseResource.Print(format, filter)
}
