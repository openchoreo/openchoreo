// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateWorkload = "workload:create"
	actionUpdateWorkload = "workload:update"
	actionViewWorkload   = "workload:view"
	actionDeleteWorkload = "workload:delete"

	resourceTypeWorkload = "workload"
)

// workloadServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type workloadServiceWithAuthz struct {
	internal  Service
	k8sClient client.Client
	authz     *services.AuthzChecker
}

var _ Service = (*workloadServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a workload service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &workloadServiceWithAuthz{
		internal:  NewService(k8sClient, logger),
		k8sClient: k8sClient,
		authz:     services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *workloadServiceWithAuthz) CreateWorkload(ctx context.Context, namespaceName string, w *openchoreov1alpha1.Workload) (*openchoreov1alpha1.Workload, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateWorkload,
		ResourceType: resourceTypeWorkload,
		ResourceID:   w.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   w.Spec.Owner.ProjectName,
			Component: w.Spec.Owner.ComponentName,
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateWorkload(ctx, namespaceName, w)
}

func (s *workloadServiceWithAuthz) UpdateWorkload(ctx context.Context, namespaceName string, w *openchoreov1alpha1.Workload) (*openchoreov1alpha1.Workload, error) {
	// Fetch the existing workload to get owner info for authz
	existing, err := s.internal.GetWorkload(ctx, namespaceName, w.Name)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateWorkload,
		ResourceType: resourceTypeWorkload,
		ResourceID:   w.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   existing.Spec.Owner.ProjectName,
			Component: existing.Spec.Owner.ComponentName,
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateWorkload(ctx, namespaceName, w)
}

func (s *workloadServiceWithAuthz) ListWorkloads(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Workload], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Workload], error) {
			return s.internal.ListWorkloads(ctx, namespaceName, componentName, pageOpts)
		},
		func(w openchoreov1alpha1.Workload) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewWorkload,
				ResourceType: resourceTypeWorkload,
				ResourceID:   w.Name,
				Hierarchy: authz.ResourceHierarchy{
					Namespace: namespaceName,
					Project:   w.Spec.Owner.ProjectName,
					Component: w.Spec.Owner.ComponentName,
				},
			}
		},
	)
}

func (s *workloadServiceWithAuthz) GetWorkload(ctx context.Context, namespaceName, workloadName string) (*openchoreov1alpha1.Workload, error) {
	// Fetch the workload first to get owner info for authz
	w, err := s.internal.GetWorkload(ctx, namespaceName, workloadName)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewWorkload,
		ResourceType: resourceTypeWorkload,
		ResourceID:   workloadName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   w.Spec.Owner.ProjectName,
			Component: w.Spec.Owner.ComponentName,
		},
	}); err != nil {
		return nil, err
	}
	return w, nil
}

func (s *workloadServiceWithAuthz) DeleteWorkload(ctx context.Context, namespaceName, workloadName string) error {
	// Fetch the workload first to get owner info for authz
	w, err := s.internal.GetWorkload(ctx, namespaceName, workloadName)
	if err != nil {
		return err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteWorkload,
		ResourceType: resourceTypeWorkload,
		ResourceID:   workloadName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   w.Spec.Owner.ProjectName,
			Component: w.Spec.Owner.ComponentName,
		},
	}); err != nil {
		return err
	}
	return s.internal.DeleteWorkload(ctx, namespaceName, workloadName)
}
