// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Workflow implements workflow operations
type Workflow struct{}

// New creates a new workflow implementation
func New() *Workflow {
	return &Workflow{}
}

// List lists all workflows in a namespace
func (w *Workflow) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceWorkflow, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Workflow, string, error) {
		p := &gen.ListWorkflowsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.ListWorkflows(ctx, params.Namespace, p)
		if err != nil {
			return nil, "", err
		}
		next := ""
		if result.Pagination.NextCursor != nil {
			next = *result.Pagination.NextCursor
		}
		return result.Items, next, nil
	})
	if err != nil {
		return err
	}
	return printList(items)
}

// StartRun starts a workflow run
func (w *Workflow) StartRun(params StartRunParams) error {
	if params.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if params.WorkflowName == "" {
		return fmt.Errorf("workflow name is required")
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	workflowRun, err := c.CreateWorkflowRun(ctx, params.Namespace, params.WorkflowName, nil)
	if err != nil {
		return err
	}

	workflowName := ""
	if workflowRun.Spec != nil {
		workflowName = workflowRun.Spec.Workflow.Name
	}
	ns := ""
	if workflowRun.Metadata.Namespace != nil {
		ns = *workflowRun.Metadata.Namespace
	}
	fmt.Printf("Successfully started workflow run: %s\n", workflowRun.Metadata.Name)
	fmt.Printf("  Workflow: %s\n", workflowName)
	fmt.Printf("  Namespace: %s\n", ns)

	return nil
}

func printList(items []gen.Workflow) error {
	if len(items) == 0 {
		fmt.Println("No workflows found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, wf := range items {
		age := "<unknown>"
		if wf.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*wf.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			wf.Metadata.Name,
			age,
		)
	}

	return w.Flush()
}
