// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// workflowRunService handles workflow run business logic without authorization checks.
type workflowRunService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*workflowRunService)(nil)

// NewService creates a new workflow run service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &workflowRunService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *workflowRunService) CreateWorkflowRun(ctx context.Context, namespaceName string, wfRun *openchoreov1alpha1.WorkflowRun) (*openchoreov1alpha1.WorkflowRun, error) {
	if wfRun == nil {
		return nil, fmt.Errorf("workflow run cannot be nil")
	}

	s.logger.Debug("Creating workflow run", "namespace", namespaceName, "name", wfRun.Name)

	// Verify the referenced workflow exists
	workflow := &openchoreov1alpha1.Workflow{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      wfRun.Spec.Workflow.Name,
		Namespace: namespaceName,
	}, workflow); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Referenced workflow not found", "namespace", namespaceName, "workflow", wfRun.Spec.Workflow.Name)
			return nil, ErrWorkflowNotFound
		}
		s.logger.Error("Failed to get referenced workflow", "error", err)
		return nil, fmt.Errorf("failed to get referenced workflow: %w", err)
	}

	// Ensure namespace is set
	wfRun.Namespace = namespaceName

	if err := s.k8sClient.Create(ctx, wfRun); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Workflow run already exists", "namespace", namespaceName, "name", wfRun.Name)
			return nil, ErrWorkflowRunAlreadyExists
		}
		s.logger.Error("Failed to create workflow run", "error", err)
		return nil, fmt.Errorf("failed to create workflow run: %w", err)
	}

	s.logger.Debug("Workflow run created successfully", "namespace", namespaceName, "name", wfRun.Name)
	return wfRun, nil
}

func (s *workflowRunService) ListWorkflowRuns(ctx context.Context, namespaceName, projectName, componentName, workflowName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.WorkflowRun], error) {
	s.logger.Debug("Listing workflow runs", "namespace", namespaceName, "project", projectName, "component", componentName, "workflow", workflowName, "limit", opts.Limit, "cursor", opts.Cursor)

	listResource := s.listWorkflowRunsResource(namespaceName)

	// Apply label filters if project or component specified
	var filters []services.ItemFilter[openchoreov1alpha1.WorkflowRun]
	if projectName != "" {
		filters = append(filters, func(wr openchoreov1alpha1.WorkflowRun) bool {
			return wr.Labels[labels.LabelKeyProjectName] == projectName
		})
	}
	if componentName != "" {
		filters = append(filters, func(wr openchoreov1alpha1.WorkflowRun) bool {
			return wr.Labels[labels.LabelKeyComponentName] == componentName
		})
	}
	if workflowName != "" {
		filters = append(filters, func(wr openchoreov1alpha1.WorkflowRun) bool {
			return wr.Spec.Workflow.Name == workflowName
		})
	}

	return services.PreFilteredList(listResource, filters...)(ctx, opts)
}

// listWorkflowRunsResource returns a ListResource that fetches workflow runs from K8s for the given namespace.
func (s *workflowRunService) listWorkflowRunsResource(namespaceName string) services.ListResource[openchoreov1alpha1.WorkflowRun] {
	return func(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.WorkflowRun], error) {
		listOpts := []client.ListOption{
			client.InNamespace(namespaceName),
		}
		if opts.Limit > 0 {
			listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
		}
		if opts.Cursor != "" {
			listOpts = append(listOpts, client.Continue(opts.Cursor))
		}

		var wfRunList openchoreov1alpha1.WorkflowRunList
		if err := s.k8sClient.List(ctx, &wfRunList, listOpts...); err != nil {
			s.logger.Error("Failed to list workflow runs", "error", err)
			return nil, fmt.Errorf("failed to list workflow runs: %w", err)
		}

		result := &services.ListResult[openchoreov1alpha1.WorkflowRun]{
			Items:      wfRunList.Items,
			NextCursor: wfRunList.Continue,
		}
		if wfRunList.RemainingItemCount != nil {
			remaining := *wfRunList.RemainingItemCount
			result.RemainingCount = &remaining
		}

		return result, nil
	}
}

func (s *workflowRunService) GetWorkflowRun(ctx context.Context, namespaceName, runName string) (*openchoreov1alpha1.WorkflowRun, error) {
	s.logger.Debug("Getting workflow run", "namespace", namespaceName, "run", runName)

	wfRun := &openchoreov1alpha1.WorkflowRun{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      runName,
		Namespace: namespaceName,
	}, wfRun); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workflow run not found", "namespace", namespaceName, "run", runName)
			return nil, ErrWorkflowRunNotFound
		}
		s.logger.Error("Failed to get workflow run", "error", err)
		return nil, fmt.Errorf("failed to get workflow run: %w", err)
	}

	return wfRun, nil
}
