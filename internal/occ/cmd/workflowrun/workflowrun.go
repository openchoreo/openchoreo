// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// WorkflowRun implements workflow run operations
type WorkflowRun struct{}

// New creates a new workflow run implementation
func New() *WorkflowRun {
	return &WorkflowRun{}
}

// List lists all workflow runs in a namespace
func (w *WorkflowRun) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceWorkflowRun, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListWorkflowRuns(ctx, params.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list workflow runs: %w", err)
	}

	return printList(result)
}

func printList(list *gen.WorkflowRunList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No workflow runs found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tWORKFLOW\tSTATUS\tAGE")

	for _, run := range list.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			run.Name,
			run.WorkflowName,
			run.Status,
			utils.FormatAge(run.CreatedAt))
	}

	return w.Flush()
}
