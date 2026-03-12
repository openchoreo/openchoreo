// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflow"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflowrun"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// WorkflowRunLogs fetches and displays logs for a component's workflow run.
// If RunName is provided, it delegates directly to workflowrun.Logs.
// Otherwise, it resolves the component's workflow and finds the latest run.
func (cp *Component) WorkflowRunLogs(params WorkflowRunLogsParams) error {
	if params.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if params.ComponentName == "" {
		return fmt.Errorf("component name is required")
	}

	runName := params.RunName
	if runName == "" {
		workflowName, err := resolveComponentWorkflowName(params.Namespace, params.ComponentName)
		if err != nil {
			return err
		}

		componentName := params.ComponentName
		runName, err = workflow.ResolveLatestRun(params.Namespace, workflowName, func(items []gen.WorkflowRun) []gen.WorkflowRun {
			return workflowrun.FilterByComponent(items, componentName)
		})
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

// resolveComponentWorkflowName gets the workflow name configured on a component.
func resolveComponentWorkflowName(namespace, componentName string) (string, error) {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create API client: %w", err)
	}

	comp, err := c.GetComponent(ctx, namespace, componentName)
	if err != nil {
		return "", err
	}

	if comp.Spec == nil || comp.Spec.Workflow == nil || comp.Spec.Workflow.Name == "" {
		return "", fmt.Errorf("component %q has no workflow configured", componentName)
	}

	return comp.Spec.Workflow.Name, nil
}
