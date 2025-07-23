// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kinds

import (
	"fmt"
	"os"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	synth "github.com/openchoreo/openchoreo/internal/choreoctl/resources/workload"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// WorkloadResource provides operations for Workload CRs.
type WorkloadResource struct {
	*resources.ResourceBase
}

// NewWorkloadResource constructs a WorkloadResource with CRDConfig and optionally sets organization.
func NewWorkloadResource(cfg constants.CRDConfig, org string) (*WorkloadResource, error) {
	options := []resources.ResourceBaseOption{
		resources.WithResourceConfig(cfg),
	}

	// Add organization namespace if provided
	if org != "" {
		options = append(options, resources.WithResourceNamespace(org))
	}

	return &WorkloadResource{
		ResourceBase: resources.NewResourceBase(options...),
	}, nil
}

// WithNamespace sets the namespace for the workload resource (usually the organization name)
func (w *WorkloadResource) SetNamespace(namespace string) {
	w.ResourceBase.SetNamespace(namespace)
}

// CreateWorkload creates a Workload CR from a descriptor file or basic parameters.
func (w *WorkloadResource) CreateWorkload(params api.CreateWorkloadParams) error {
	// Validate required parameters
	if params.OrganizationName == "" {
		return fmt.Errorf("organization name is required (--organization)")
	}
	if params.ProjectName == "" {
		return fmt.Errorf("project name is required (--project)")
	}
	if params.ComponentName == "" {
		return fmt.Errorf("component name is required (--component)")
	}
	if params.ImageUrl == "" {
		return fmt.Errorf("image URL is required (--image)")
	}

	var workloadCR *openchoreov1alpha1.Workload
	var err error

	// Check if a descriptor file is provided
	if params.FilePath != "" {
		// Create workload from descriptor file
		workloadCR, err = synth.ConvertWorkloadDescriptorToWorkloadCR(params.FilePath, params)
		if err != nil {
			return fmt.Errorf("failed to convert workload descriptor: %w", err)
		}
	} else {
		// Create basic workload from command line parameters
		workloadCR, err = synth.CreateBasicWorkload(params)
		if err != nil {
			return fmt.Errorf("failed to create basic workload CR: %w", err)
		}
	}

	// Convert to YAML
	yamlBytes, err := synth.ConvertWorkloadCRToYAML(workloadCR)
	if err != nil {
		return fmt.Errorf("failed to convert Workload CR to YAML: %w", err)
	}

	// Output to file or stdout
	if params.OutputPath != "" {
		// Write to file
		if err := os.WriteFile(params.OutputPath, yamlBytes, 0644); err != nil {
			return fmt.Errorf("failed to write output file %s: %w", params.OutputPath, err)
		}
		fmt.Printf("Workload CR written to %s\n", params.OutputPath)
	} else {
		// Write to stdout
		fmt.Print(string(yamlBytes))
	}

	return nil
}

// GetStatus returns the status of a Workload with detailed information.
func (w *WorkloadResource) GetStatus(workload *openchoreov1alpha1.Workload) string {
	// For workload, we'll use a simple status based on creation time
	// TODO: Implement proper status checking when WorkloadStatus has condition fields
	if workload.GetCreationTimestamp().Time.IsZero() {
		return StatusPending
	}
	return StatusReady
}

// GetAge returns the age of a Workload.
func (w *WorkloadResource) GetAge(workload *openchoreov1alpha1.Workload) string {
	return resources.FormatAge(workload.GetCreationTimestamp().Time)
}

// PrintTableItems formats workloads into a table
func (w *WorkloadResource) PrintTableItems(workloads []resources.ResourceWrapper[*openchoreov1alpha1.Workload]) error {
	if len(workloads) == 0 {
		// Provide a more descriptive message
		namespaceName := w.GetNamespace()

		message := "No workloads found"

		if namespaceName != "" {
			message += " in organization " + namespaceName
		}

		fmt.Println(message)
		return nil
	}

	rows := make([][]string, 0, len(workloads))

	for _, wrapper := range workloads {
		workload := wrapper.Resource
		rows = append(rows, []string{
			wrapper.LogicalName,
			w.GetStatus(workload),
			w.GetAge(workload),
			workload.GetLabels()[constants.LabelOrganization],
		})
	}
	return resources.PrintTable(HeadersWorkload, rows)
}

// Print prints workloads using the API client
func (w *WorkloadResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	// TODO: Implement workload-specific listing using API client when listing endpoints are available
	// For now, return a helpful message
	return fmt.Errorf("workload listing not yet implemented - use 'choreoctl get workload' command instead")
}
