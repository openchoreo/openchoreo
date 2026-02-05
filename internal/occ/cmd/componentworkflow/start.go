// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentworkflow

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

func (s *StartImpl) StartComponentWorkflowRun(params api.StartComponentWorkflowRunParams) error {
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

	// Create component workflow run via API
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
	if workflowRun.Commit != nil {
		fmt.Printf("  Commit: %s\n", *workflowRun.Commit)
	}
	if workflowRun.Status != nil {
		fmt.Printf("  Status: %s\n", *workflowRun.Status)
	}
	if workflowRun.Uuid != nil {
		fmt.Printf("  UUID: %s\n", *workflowRun.Uuid)
	}

	return nil
}
