// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateWorkflowRun = "workflowrun:create"
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
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &workflowRunServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *workflowRunServiceWithAuthz) CreateWorkflowRun(ctx context.Context, namespaceName string, wfRun *openchoreov1alpha1.WorkflowRun) (*openchoreov1alpha1.WorkflowRun, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateWorkflowRun,
		ResourceType: resourceTypeWorkflowRun,
		ResourceID:   wfRun.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateWorkflowRun(ctx, namespaceName, wfRun)
}

func (s *workflowRunServiceWithAuthz) ListWorkflowRuns(ctx context.Context, namespaceName, projectName, componentName, workflowName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.WorkflowRun], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.WorkflowRun], error) {
			return s.internal.ListWorkflowRuns(ctx, namespaceName, projectName, componentName, workflowName, pageOpts)
		},
		func(wr openchoreov1alpha1.WorkflowRun) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewWorkflowRun,
				ResourceType: resourceTypeWorkflowRun,
				ResourceID:   wr.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *workflowRunServiceWithAuthz) GetWorkflowRun(ctx context.Context, namespaceName, runName string) (*openchoreov1alpha1.WorkflowRun, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewWorkflowRun,
		ResourceType: resourceTypeWorkflowRun,
		ResourceID:   runName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetWorkflowRun(ctx, namespaceName, runName)
}
