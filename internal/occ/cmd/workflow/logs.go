// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"fmt"
	"sort"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflowrun"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Logs fetches and displays logs for a workflow.
// If RunName is provided, it delegates directly to workflowrun.Logs.
// Otherwise, it finds the latest workflow run and uses that.
func (w *Workflow) Logs(params LogsParams) error {
	if err := validation.ValidateParams(validation.CmdLogs, validation.ResourceWorkflow, params); err != nil {
		return err
	}

	if params.WorkflowName == "" {
		return fmt.Errorf("workflow name is required")
	}

	runName := params.RunName
	if runName == "" {
		var err error
		runName, err = ResolveLatestRun(params.Namespace, params.WorkflowName, workflowrun.ExcludeComponentRuns)
		if err != nil {
			return err
		}
	}

	return workflowrun.New(nil).Logs(workflowrun.LogsParams{
		Namespace:       params.Namespace,
		WorkflowRunName: runName,
		Follow:          params.Follow,
		Since:           params.Since,
	})
}

// RunFilter transforms a slice of workflow runs (e.g. to exclude/include certain runs).
type RunFilter func([]gen.WorkflowRun) []gen.WorkflowRun

// ResolveLatestRun finds the most recent workflow run for the given workflow.
// An optional filter can narrow the results (e.g. exclude or include component runs).
// Pass nil for no filtering.
func ResolveLatestRun(namespace, workflowName string, filter RunFilter) (string, error) {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create API client: %w", err)
	}

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.WorkflowRun, string, error) {
		p := &gen.ListWorkflowRunsParams{
			Workflow: &workflowName,
		}
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
	if err != nil {
		return "", fmt.Errorf("failed to list workflow runs: %w", err)
	}

	if filter != nil {
		items = filter(items)
	}
	if len(items) == 0 {
		return "", fmt.Errorf("no workflow runs found for workflow %q", workflowName)
	}

	// Sort by creation timestamp descending (newest first)
	sort.Slice(items, func(i, j int) bool {
		ti := items[i].Metadata.CreationTimestamp
		tj := items[j].Metadata.CreationTimestamp
		if ti == nil {
			return false
		}
		if tj == nil {
			return true
		}
		return ti.After(*tj)
	})

	return items[0].Metadata.Name, nil
}
