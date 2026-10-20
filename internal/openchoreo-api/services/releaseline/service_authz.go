// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeReleaseLine = "releaseline"
)

// releaseLineServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type releaseLineServiceWithAuthz struct {
	internal  Service
	k8sClient client.Client
	authz     *services.AuthzChecker
}

var _ Service = (*releaseLineServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a release line service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &releaseLineServiceWithAuthz{
		internal:  NewService(k8sClient, logger),
		k8sClient: k8sClient,
		authz:     services.NewAuthzChecker(authzPDP, logger),
	}
}

// checkAction authorizes a release line action against the line's owning component.
func (s *releaseLineServiceWithAuthz) checkAction(ctx context.Context, action, namespaceName, componentName, releaseLineName string) error {
	comp, err := getComponent(ctx, s.k8sClient, namespaceName, componentName)
	if err != nil {
		return err
	}
	return s.authz.Check(ctx, services.CheckRequest{
		Action:       action,
		ResourceType: resourceTypeReleaseLine,
		ResourceID:   releaseLineName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   comp.Spec.Owner.ProjectName,
			Component: componentName,
		},
		// The line is its own release line, so line-scoped conditions and audit filters match it.
		Context: authz.Context{Resource: authz.ResourceAttribute{ReleaseLine: releaseLineName}},
	})
}

func (s *releaseLineServiceWithAuthz) ListReleaseLines(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseLine], error) {
	if err := s.checkAction(ctx, authz.ActionViewReleaseLine, namespaceName, componentName, ""); err != nil {
		return nil, err
	}
	return s.internal.ListReleaseLines(ctx, namespaceName, componentName, opts)
}

func (s *releaseLineServiceWithAuthz) GetReleaseLine(ctx context.Context, namespaceName, componentName, releaseLineName string) (*openchoreov1alpha1.ReleaseLine, error) {
	if err := s.checkAction(ctx, authz.ActionViewReleaseLine, namespaceName, componentName, releaseLineName); err != nil {
		return nil, err
	}
	return s.internal.GetReleaseLine(ctx, namespaceName, componentName, releaseLineName)
}

func (s *releaseLineServiceWithAuthz) CreateReleaseLine(ctx context.Context, namespaceName, componentName string, releaseLine *openchoreov1alpha1.ReleaseLine) (*openchoreov1alpha1.ReleaseLine, error) {
	if err := s.checkAction(ctx, authz.ActionCreateReleaseLine, namespaceName, componentName, releaseLine.Name); err != nil {
		return nil, err
	}
	return s.internal.CreateReleaseLine(ctx, namespaceName, componentName, releaseLine)
}

func (s *releaseLineServiceWithAuthz) UpdateReleaseLine(ctx context.Context, namespaceName, componentName string, releaseLine *openchoreov1alpha1.ReleaseLine) (*openchoreov1alpha1.ReleaseLine, error) {
	if err := s.checkAction(ctx, authz.ActionUpdateReleaseLine, namespaceName, componentName, releaseLine.Name); err != nil {
		return nil, err
	}
	return s.internal.UpdateReleaseLine(ctx, namespaceName, componentName, releaseLine)
}

func (s *releaseLineServiceWithAuthz) DeleteReleaseLine(ctx context.Context, namespaceName, componentName, releaseLineName string) error {
	if err := s.checkAction(ctx, authz.ActionDeleteReleaseLine, namespaceName, componentName, releaseLineName); err != nil {
		return err
	}
	return s.internal.DeleteReleaseLine(ctx, namespaceName, componentName, releaseLineName)
}
