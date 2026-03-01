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
		runName, err = resolveLatestRun(params.Namespace, params.WorkflowName)
		if err != nil {
			return err
		}
	}

	return workflowrun.New().Logs(workflowrun.LogsParams{
		Namespace:       params.Namespace,
		WorkflowRunName: runName,
		Follow:          params.Follow,
		Since:           params.Since,
	})
}

// resolveLatestRun finds the most recent workflow run for the given workflow,
// excluding component-owned runs.
func resolveLatestRun(namespace, workflowName string) (string, error) {
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

	filtered := workflowrun.ExcludeComponentRuns(items)
	if len(filtered) == 0 {
		return "", fmt.Errorf("no workflow runs found for workflow %q", workflowName)
	}

	// Sort by creation timestamp descending (newest first)
	sort.Slice(filtered, func(i, j int) bool {
		ti := filtered[i].Metadata.CreationTimestamp
		tj := filtered[j].Metadata.CreationTimestamp
		if ti == nil {
			return false
		}
		if tj == nil {
			return true
		}
		return ti.After(*tj)
	})

	return filtered[0].Metadata.Name, nil
}
