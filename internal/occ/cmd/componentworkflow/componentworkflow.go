// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentworkflow

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/list/output"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type ComponentWorkflowImpl struct{}

func NewComponentWorkflowImpl() *ComponentWorkflowImpl {
	return &ComponentWorkflowImpl{}
}

func (s *ComponentWorkflowImpl) StartComponentWorkflowRun(params api.StartComponentWorkflowRunParams) error {
	if params.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if params.Project == "" {
		return fmt.Errorf("project is required")
	}
	if params.Component == "" {
		return fmt.Errorf("component is required")
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// If parameters are provided, update the component workflow parameters first
	if len(params.Parameters) > 0 {
		// Get existing component to merge with existing workflow parameters
		component, err := c.GetComponent(ctx, params.Namespace, params.Component)
		if err != nil {
			return fmt.Errorf("failed to get component: %w", err)
		}

		// Merge --set values with existing workflow parameters and systemParameters
		body, err := mergeParametersWithComponent(component, params.Parameters)
		if err != nil {
			return fmt.Errorf("failed to merge parameters: %w", err)
		}

		err = c.UpdateComponentWorkflowParameters(
			ctx,
			params.Namespace,
			params.Project,
			params.Component,
			*body,
		)
		if err != nil {
			return fmt.Errorf("failed to update workflow parameters: %w", err)
		}
	}

	workflowRun, err := c.CreateComponentWorkflowRun(
		ctx,
		params.Namespace,
		params.Project,
		params.Component,
		params.Commit,
	)
	if err != nil {
		return fmt.Errorf("failed to create component workflow run: %w", err)
	}

	fmt.Printf("Successfully started component workflow run: %s\n", workflowRun.Name)
	fmt.Printf("  Component: %s\n", workflowRun.ComponentName)
	fmt.Printf("  Project: %s\n", workflowRun.ProjectName)
	fmt.Printf("  Namespace: %s\n", workflowRun.NamespaceName)
	if workflowRun.Status != nil {
		fmt.Printf("  Status: %s\n", *workflowRun.Status)
	}

	return nil
}

// ListComponentWorkflows lists all component workflows in a namespace
func (l *ComponentWorkflowImpl) ListComponentWorkflows(params api.ListComponentWorkflowsParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceComponentWorkflow, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListComponentWorkflows(ctx, params.Namespace, &gen.ListComponentWorkflowsParams{})
	if err != nil {
		return fmt.Errorf("failed to list component workflows: %w", err)
	}

	return output.PrintComponentWorkflows(result)
}
