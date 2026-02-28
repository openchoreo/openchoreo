// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// componentLabel is the label key that identifies component-owned workflow runs.
const componentLabel = "openchoreo.dev/component"

// WorkflowRun implements workflow run operations
type WorkflowRun struct{}

// New creates a new workflow run implementation
func New() *WorkflowRun {
	return &WorkflowRun{}
}

// List lists workflow runs in a namespace, excluding component workflow runs.
func (w *WorkflowRun) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceWorkflowRun, params); err != nil {
		return err
	}

	items, err := FetchAll(params.Namespace)
	if err != nil {
		return err
	}

	// Exclude workflow runs that belong to a component
	filtered := ExcludeComponentRuns(items)
	return PrintList(filtered)
}

// FetchAll fetches all workflow runs from a namespace.
func FetchAll(namespace string) ([]gen.WorkflowRun, error) {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	return pagination.FetchAll(func(limit int, cursor string) ([]gen.WorkflowRun, string, error) {
		p := &gen.ListWorkflowRunsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.ListWorkflowRuns(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		next := ""
		if result.Pagination.NextCursor != nil {
			next = *result.Pagination.NextCursor
		}
		return result.Items, next, nil
	})
}

// ExcludeComponentRuns returns only workflow runs that do NOT have the component label.
func ExcludeComponentRuns(items []gen.WorkflowRun) []gen.WorkflowRun {
	var filtered []gen.WorkflowRun
	for _, run := range items {
		if getComponentLabel(run) == "" {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

// FilterByComponent returns only workflow runs whose component label matches the given name.
func FilterByComponent(items []gen.WorkflowRun, componentName string) []gen.WorkflowRun {
	var filtered []gen.WorkflowRun
	for _, run := range items {
		if getComponentLabel(run) == componentName {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

// getComponentLabel returns the value of the component label, or empty string if not present.
func getComponentLabel(run gen.WorkflowRun) string {
	if run.Metadata.Labels == nil {
		return ""
	}
	return (*run.Metadata.Labels)[componentLabel]
}

// Get retrieves a single workflow run and outputs it as YAML
func (w *WorkflowRun) Get(params GetParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceWorkflowRun, params); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.GetWorkflowRun(ctx, params.Namespace, params.WorkflowRunName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow run to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

func PrintList(items []gen.WorkflowRun) error {
	if len(items) == 0 {
		fmt.Println("No workflow runs found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tWORKFLOW\tSTATUS\tAGE")

	for _, run := range items {
		workflowName := ""
		if run.Spec != nil {
			workflowName = run.Spec.Workflow.Name
		}
		age := "<unknown>"
		if run.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*run.Metadata.CreationTimestamp)
		}
		status := "Pending"
		if run.Status != nil && run.Status.Conditions != nil {
			for _, c := range *run.Status.Conditions {
				if c.Type == "Ready" {
					if c.Status == "True" {
						status = "Ready"
					} else {
						status = c.Reason
					}
					break
				}
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			run.Metadata.Name,
			workflowName,
			status,
			age)
	}

	return w.Flush()
}
