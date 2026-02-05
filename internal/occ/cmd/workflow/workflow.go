// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type WorkflowImpl struct{}

func NewWorkflowImpl() *WorkflowImpl {
	return &WorkflowImpl{}
}

func (s *WorkflowImpl) StartWorkflowRun(params api.StartWorkflowRunParams) error {
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
		return fmt.Errorf("failed to create workflow run: %w", err)
	}

	fmt.Printf("Successfully started workflow run: %s\n", workflowRun.Name)
	fmt.Printf("  Workflow: %s\n", workflowRun.WorkflowName)
	fmt.Printf("  Namespace: %s\n", workflowRun.OrgName)
	fmt.Printf("  Status: %s\n", workflowRun.Status)

	return nil
}
