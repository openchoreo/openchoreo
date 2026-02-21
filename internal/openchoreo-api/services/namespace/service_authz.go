// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

import (
	"context"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateNamespace = "namespace:create"
	actionUpdateNamespace = "namespace:update"
	actionViewNamespace   = "namespace:view"
	actionDeleteNamespace = "namespace:delete"

	resourceTypeNamespace = "namespace"
)

// namespaceServiceWithAuthz wraps a Service and adds authorization checks.
type namespaceServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*namespaceServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a namespace service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &namespaceServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *namespaceServiceWithAuthz) CreateNamespace(ctx context.Context, ns *corev1.Namespace) (*corev1.Namespace, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateNamespace,
		ResourceType: resourceTypeNamespace,
		ResourceID:   ns.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: ns.Name},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateNamespace(ctx, ns)
}

func (s *namespaceServiceWithAuthz) UpdateNamespace(ctx context.Context, ns *corev1.Namespace) (*corev1.Namespace, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateNamespace,
		ResourceType: resourceTypeNamespace,
		ResourceID:   ns.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: ns.Name},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateNamespace(ctx, ns)
}

func (s *namespaceServiceWithAuthz) ListNamespaces(ctx context.Context, opts services.ListOptions) (*services.ListResult[corev1.Namespace], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[corev1.Namespace], error) {
			return s.internal.ListNamespaces(ctx, pageOpts)
		},
		func(ns corev1.Namespace) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewNamespace,
				ResourceType: resourceTypeNamespace,
				ResourceID:   ns.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: ns.Name},
			}
		},
	)
}

func (s *namespaceServiceWithAuthz) GetNamespace(ctx context.Context, namespaceName string) (*corev1.Namespace, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewNamespace,
		ResourceType: resourceTypeNamespace,
		ResourceID:   namespaceName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetNamespace(ctx, namespaceName)
}

func (s *namespaceServiceWithAuthz) DeleteNamespace(ctx context.Context, namespaceName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteNamespace,
		ResourceType: resourceTypeNamespace,
		ResourceID:   namespaceName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteNamespace(ctx, namespaceName)
}
