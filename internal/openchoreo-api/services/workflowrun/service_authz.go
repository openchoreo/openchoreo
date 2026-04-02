// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateWorkflowRun = "workflowrun:create"
	actionUpdateWorkflowRun = "workflowrun:update"
	actionViewWorkflowRun   = "workflowrun:view"
	resourceTypeWorkflowRun = "workflowrun"
)

// workflowRunServiceWithAuthz wraps a Service and adds authorization checks.
type workflowRunServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*workflowRunServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a workflow run service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, wpClientMgr *kubernetesClient.KubeMultiClientManager, gwClient *gatewayClient.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &workflowRunServiceWithAuthz{
		internal: NewService(k8sClient, wpClientMgr, gwClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

// constructHierarchyForAuthzCheck builds a ResourceHierarchy from workflow run labels.
// If both project and component labels are present, it returns a component-level hierarchy; otherwise it falls back to namespace-level.
func constructHierarchyForAuthzCheck(namespaceName string, labels map[string]string) authz.ResourceHierarchy {
	project := labels[ocLabels.LabelKeyProjectName]
	component := labels[ocLabels.LabelKeyComponentName]
	if project != "" && component != "" {
		return authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   project,
			Component: component,
		}
	}
	return authz.ResourceHierarchy{Namespace: namespaceName}
}

func (s *workflowRunServiceWithAuthz) CreateWorkflowRun(ctx context.Context, namespaceName string, wfRun *openchoreov1alpha1.WorkflowRun) (*openchoreov1alpha1.WorkflowRun, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateWorkflowRun,
		ResourceType: resourceTypeWorkflowRun,
		ResourceID:   wfRun.Name,
		Hierarchy:    constructHierarchyForAuthzCheck(namespaceName, wfRun.Labels),
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateWorkflowRun(ctx, namespaceName, wfRun)
}

func (s *workflowRunServiceWithAuthz) UpdateWorkflowRun(ctx context.Context, namespaceName string, wfRun *openchoreov1alpha1.WorkflowRun) (*openchoreov1alpha1.WorkflowRun, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateWorkflowRun,
		ResourceType: resourceTypeWorkflowRun,
		ResourceID:   wfRun.Name,
		Hierarchy:    constructHierarchyForAuthzCheck(namespaceName, wfRun.Labels),
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateWorkflowRun(ctx, namespaceName, wfRun)
}

func (s *workflowRunServiceWithAuthz) ListWorkflowRuns(ctx context.Context, namespaceName, projectName, componentName, workflowName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.WorkflowRun], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.WorkflowRun], error) {
			return s.internal.ListWorkflowRuns(ctx, namespaceName, projectName, componentName, workflowName, pageOpts)
		},
		func(wr openchoreov1alpha1.WorkflowRun) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewWorkflowRun,
				ResourceType: resourceTypeWorkflowRun,
				ResourceID:   wr.Name,
				Hierarchy:    constructHierarchyForAuthzCheck(namespaceName, wr.Labels),
			}
		},
	)
}

func (s *workflowRunServiceWithAuthz) GetWorkflowRun(ctx context.Context, namespaceName, runName string) (*openchoreov1alpha1.WorkflowRun, error) {
	wr, err := s.internal.GetWorkflowRun(ctx, namespaceName, runName)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewWorkflowRun,
		ResourceType: resourceTypeWorkflowRun,
		ResourceID:   runName,
		Hierarchy:    constructHierarchyForAuthzCheck(namespaceName, wr.Labels),
	}); err != nil {
		return nil, err
	}
	return wr, nil
}

func (s *workflowRunServiceWithAuthz) DeleteWorkflowRun(ctx context.Context, namespaceName, runName string) error {
	wr, err := s.internal.GetWorkflowRun(ctx, namespaceName, runName)
	if err != nil {
		return err
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteWorkflowRun,
		ResourceType: resourceTypeWorkflowRun,
		ResourceID:   runName,
		Hierarchy:    constructHierarchyForAuthzCheck(namespaceName, wr.Labels),
	}); err != nil {
		return err
	}
	return s.internal.DeleteWorkflowRun(ctx, namespaceName, runName)
}

func (s *workflowRunServiceWithAuthz) GetWorkflowRunLogs(ctx context.Context, namespaceName, runName, taskName, gatewayURL string, sinceSeconds *int64) ([]models.WorkflowRunLogEntry, error) {
	wr, err := s.internal.GetWorkflowRun(ctx, namespaceName, runName)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewWorkflowRun,
		ResourceType: resourceTypeWorkflowRun,
		ResourceID:   runName,
		Hierarchy:    constructHierarchyForAuthzCheck(namespaceName, wr.Labels),
	}); err != nil {
		return nil, err
	}
	return s.internal.GetWorkflowRunLogs(ctx, namespaceName, runName, taskName, gatewayURL, sinceSeconds)
}

func (s *workflowRunServiceWithAuthz) GetWorkflowRunEvents(ctx context.Context, namespaceName, runName, taskName, gatewayURL string) ([]models.WorkflowRunEventEntry, error) {
	wr, err := s.internal.GetWorkflowRun(ctx, namespaceName, runName)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewWorkflowRun,
		ResourceType: resourceTypeWorkflowRun,
		ResourceID:   runName,
		Hierarchy:    constructHierarchyForAuthzCheck(namespaceName, wr.Labels),
	}); err != nil {
		return nil, err
	}
	return s.internal.GetWorkflowRunEvents(ctx, namespaceName, runName, taskName, gatewayURL)
}

func (s *workflowRunServiceWithAuthz) GetWorkflowRunStatus(ctx context.Context, namespaceName, runName, gatewayURL string) (*models.WorkflowRunStatusResponse, error) {
	wr, err := s.internal.GetWorkflowRun(ctx, namespaceName, runName)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewWorkflowRun,
		ResourceType: resourceTypeWorkflowRun,
		ResourceID:   runName,
		Hierarchy:    constructHierarchyForAuthzCheck(namespaceName, wr.Labels),
	}); err != nil {
		return nil, err
	}
	return s.internal.GetWorkflowRunStatus(ctx, namespaceName, runName, gatewayURL)
}

func (s *workflowRunServiceWithAuthz) TriggerWorkflow(ctx context.Context, namespaceName, projectName, componentName, commit string) (*models.WorkflowRunTriggerResponse, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateWorkflowRun,
		ResourceType: resourceTypeWorkflowRun,
		ResourceID:   componentName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   projectName,
			Component: componentName,
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.TriggerWorkflow(ctx, namespaceName, projectName, componentName, commit)
}
