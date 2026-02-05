// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type StartImpl struct{}

func NewStartImpl() *StartImpl {
	return &StartImpl{}
}

func (s *StartImpl) StartWorkflowRun(params api.StartWorkflowRunParams) error {
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

	// Parse --set parameters into nested map
	parameters, err := ParseSetParameters(params.Parameters)
	if err != nil {
		return fmt.Errorf("failed to parse parameters: %w", err)
	}

	// Create workflow run via API
	workflowRun, err := c.CreateWorkflowRun(ctx, params.Namespace, params.WorkflowName, parameters)
	if err != nil {
		return fmt.Errorf("failed to create workflow run: %w", err)
	}

	fmt.Printf("Successfully started workflow run: %s\n", workflowRun.Name)
	fmt.Printf("  Workflow: %s\n", workflowRun.WorkflowName)
	fmt.Printf("  Namespace: %s\n", workflowRun.OrgName)
	fmt.Printf("  Status: %s\n", workflowRun.Status)
	if workflowRun.Uuid != nil {
		fmt.Printf("  UUID: %s\n", *workflowRun.Uuid)
	}

	return nil
}
