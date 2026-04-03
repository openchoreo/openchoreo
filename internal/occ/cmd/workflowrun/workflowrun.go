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
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const (
	// componentLabel is the label key that identifies component-owned workflow runs.
	componentLabel = "openchoreo.dev/component"

	conditionStatusTrue = "True"
)

// Client defines the client methods used by WorkflowRun operations.
type Client interface {
	ListWorkflowRuns(ctx context.Context, namespaceName string, params *gen.ListWorkflowRunsParams) (*gen.WorkflowRunList, error)
	GetWorkflowRun(ctx context.Context, namespaceName, workflowRunName string) (*gen.WorkflowRun, error)
}

// WorkflowRun implements workflow run operations
type WorkflowRun struct {
	client Client
}

// New creates a new workflow run implementation
func New(client Client) *WorkflowRun {
	return &WorkflowRun{client: client}
}

// List lists workflow runs in a namespace (includes component workflow runs).
func (w *WorkflowRun) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceWorkflowRun, params); err != nil {
		return err
	}

	items, err := w.FetchAll(params.Namespace, params.Workflow)
	if err != nil {
		return err
	}

	return PrintList(items)
}

// FetchAll fetches all workflow runs from a namespace.
// If workflow is non-empty, results are filtered by that workflow name.
func (w *WorkflowRun) FetchAll(namespace, workflow string) ([]gen.WorkflowRun, error) {
	ctx := context.Background()

	return pagination.FetchAll(func(limit int, cursor string) ([]gen.WorkflowRun, string, error) {
		p := &gen.ListWorkflowRunsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		if workflow != "" {
			p.Workflow = &workflow
		}
		result, err := w.client.ListWorkflowRuns(ctx, namespace, p)
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

	result, err := w.client.GetWorkflowRun(ctx, params.Namespace, params.WorkflowRunName)
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
			status = deriveStatus(*run.Status.Conditions)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			run.Metadata.Name,
			workflowName,
			status,
			age)
	}

	return w.Flush()
}

// deriveStatus maps WorkflowRun conditions to a human-readable status string.
// The controller sets WorkflowCompleted, WorkflowRunning, WorkflowSucceeded, and
// WorkflowFailed conditions — there is no "Ready" condition.
func deriveStatus(conditions []gen.Condition) string {
	conds := make(map[string]gen.Condition, len(conditions))
	for _, c := range conditions {
		conds[c.Type] = c
	}

	if c, ok := conds["WorkflowSucceeded"]; ok && c.Status == conditionStatusTrue {
		return "Succeeded"
	}
	if c, ok := conds["WorkflowFailed"]; ok && c.Status == conditionStatusTrue {
		return "Failed"
	}
	if c, ok := conds["WorkflowRunning"]; ok && c.Status == conditionStatusTrue {
		return "Running"
	}
	if c, ok := conds["WorkflowCompleted"]; ok {
		return c.Reason
	}
	return "Pending"
}
