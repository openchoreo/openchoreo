// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the workflow run service interface.
type Service interface {
	CreateWorkflowRun(ctx context.Context, namespaceName string, wfRun *openchoreov1alpha1.WorkflowRun) (*openchoreov1alpha1.WorkflowRun, error)
	UpdateWorkflowRun(ctx context.Context, namespaceName string, wfRun *openchoreov1alpha1.WorkflowRun) (*openchoreov1alpha1.WorkflowRun, error)
	ListWorkflowRuns(ctx context.Context, namespaceName, projectName, componentName, workflowName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.WorkflowRun], error)
	GetWorkflowRun(ctx context.Context, namespaceName, runName string) (*openchoreov1alpha1.WorkflowRun, error)
	GetWorkflowRunStatus(ctx context.Context, namespaceName, runName, gatewayURL string) (*models.WorkflowRunStatusResponse, error)
	GetWorkflowRunLogs(ctx context.Context, namespaceName, runName, taskName, gatewayURL string, sinceSeconds *int64) ([]models.WorkflowRunLogEntry, error)
	GetWorkflowRunEvents(ctx context.Context, namespaceName, runName, taskName, gatewayURL string) ([]models.WorkflowRunEventEntry, error)
	DeleteWorkflowRun(ctx context.Context, namespaceName, runName string) error
	// TriggerWorkflow creates a WorkflowRun from a component's workflow configuration.
	// The authorized version is used by API handlers; the unauthz version is used by webhook processing.
	TriggerWorkflow(ctx context.Context, namespaceName, projectName, componentName, commit string) (*models.WorkflowRunTriggerResponse, error)
}
