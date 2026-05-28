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
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
)

const (
	actionCreateWorkflowRun = "workflowrun:create"
	actionUpdateWorkflowRun = "workflowrun:update"
	actionViewWorkflowRun   = "workflowrun:view"
	resourceTypeWorkflowRun = "workflowrun"
)

// workflowRunServiceWithAuthz wraps a Service and adds authorization checks.
type workflowRunServiceWithAuthz struct {
	internal  Service
	authz     *services.AuthzChecker
	k8sClient client.Client
}

var _ Service = (*workflowRunServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a workflow run service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, planeClientProvider kubernetesClient.WorkflowPlaneClientProvider, gwClient *gatewayClient.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &workflowRunServiceWithAuthz{
		internal:  NewService(k8sClient, planeClientProvider, gwClient, logger),
		authz:     services.NewAuthzChecker(authzPDP, logger),
		k8sClient: k8sClient,
	}
}

// formatWorkflowAttr returns the authz-engine identifier for the Workflow
// (or ClusterWorkflow) referenced by a WorkflowRun, suitable for the
// resource.workflow ABAC attribute. An empty kind defaults to ClusterWorkflow
// to match the WorkflowRunConfig / ComponentWorkflowConfig CRD defaults.
func formatWorkflowAttr(namespace string, kind openchoreov1alpha1.WorkflowRefKind, name string) string {
	isClusterScoped := kind == "" || kind == openchoreov1alpha1.WorkflowRefKindClusterWorkflow
	return services.FormatDualScopedResourceName(namespace, name, isClusterScoped)
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
		Context: authz.Context{
			Resource: authz.ResourceAttribute{
				Workflow: formatWorkflowAttr(namespaceName, wfRun.Spec.Workflow.Kind, wfRun.Spec.Workflow.Name),
			},
		},
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
		Context: authz.Context{
			Resource: authz.ResourceAttribute{
				Workflow: formatWorkflowAttr(namespaceName, wfRun.Spec.Workflow.Kind, wfRun.Spec.Workflow.Name),
			},
		},
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
		Context: authz.Context{
			Resource: authz.ResourceAttribute{
				Workflow: formatWorkflowAttr(namespaceName, wr.Spec.Workflow.Kind, wr.Spec.Workflow.Name),
			},
		},
	}); err != nil {
		return err
	}
	return s.internal.DeleteWorkflowRun(ctx, namespaceName, runName)
}

func (s *workflowRunServiceWithAuthz) GetWorkflowRunLogs(ctx context.Context, namespaceName, runName, taskName string, sinceSeconds *int64) ([]models.WorkflowRunLogEntry, error) {
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
	return s.internal.GetWorkflowRunLogs(ctx, namespaceName, runName, taskName, sinceSeconds)
}

func (s *workflowRunServiceWithAuthz) GetWorkflowRunEvents(ctx context.Context, namespaceName, runName, taskName string) ([]models.WorkflowRunEventEntry, error) {
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
	return s.internal.GetWorkflowRunEvents(ctx, namespaceName, runName, taskName)
}

func (s *workflowRunServiceWithAuthz) GetWorkflowRunStatus(ctx context.Context, namespaceName, runName string) (*models.WorkflowRunStatusResponse, error) {
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
	return s.internal.GetWorkflowRunStatus(ctx, namespaceName, runName)
}

func (s *workflowRunServiceWithAuthz) TriggerWorkflow(ctx context.Context, namespaceName, projectName, componentName, commit string) (*models.WorkflowRunTriggerResponse, error) {
	// Resolve the component's workflow reference for the authz check
	var workflowAttr string
	var comp openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: componentName, Namespace: namespaceName}, &comp); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, component.ErrComponentNotFound
		}
		return nil, fmt.Errorf("failed to resolve component %s/%s for authz check: %w", namespaceName, componentName, err)
	}
	if comp.Spec.Workflow != nil {
		workflowAttr = formatWorkflowAttr(namespaceName, comp.Spec.Workflow.Kind, comp.Spec.Workflow.Name)
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateWorkflowRun,
		ResourceType: resourceTypeWorkflowRun,
		ResourceID:   componentName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   projectName,
			Component: componentName,
		},
		Context: authz.Context{
			Resource: authz.ResourceAttribute{
				Workflow: workflowAttr,
			},
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.TriggerWorkflow(ctx, namespaceName, projectName, componentName, commit)
}
