// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateRole = "role:create"
	actionViewRole   = "role:view"
	actionUpdateRole = "role:update"
	actionDeleteRole = "role:delete"

	actionCreateRoleMapping = "rolemapping:create"
	actionViewRoleMapping   = "rolemapping:view"
	actionUpdateRoleMapping = "rolemapping:update"
	actionDeleteRoleMapping = "rolemapping:delete"

	resourceTypeRole        = "role"
	resourceTypeRoleMapping = "roleMapping"
)

// authzServiceWithAuthz wraps a Service and adds authorization checks.
type authzServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*authzServiceWithAuthz)(nil)

// NewServiceWithAuthz creates an authz service with authorization checks.
func NewServiceWithAuthz(pap authzcore.PAP, pdp authzcore.PDP, k8sClient client.Client, logger *slog.Logger) Service {
	return &authzServiceWithAuthz{
		internal: NewService(pap, k8sClient, logger),
		authz:    services.NewAuthzChecker(pdp, logger),
	}
}

// --- Cluster Roles ---

func (s *authzServiceWithAuthz) CreateClusterRole(ctx context.Context, role *openchoreov1alpha1.AuthzClusterRole) (*openchoreov1alpha1.AuthzClusterRole, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateRole,
		ResourceType: resourceTypeRole,
		ResourceID:   role.Name,
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateClusterRole(ctx, role)
}

func (s *authzServiceWithAuthz) GetClusterRole(ctx context.Context, name string) (*openchoreov1alpha1.AuthzClusterRole, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewRole,
		ResourceType: resourceTypeRole,
		ResourceID:   name,
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterRole(ctx, name)
}

func (s *authzServiceWithAuthz) ListClusterRoles(ctx context.Context) (*openchoreov1alpha1.AuthzClusterRoleList, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewRole,
		ResourceType: resourceTypeRole,
	}); err != nil {
		return nil, err
	}
	return s.internal.ListClusterRoles(ctx)
}

func (s *authzServiceWithAuthz) UpdateClusterRole(ctx context.Context, role *openchoreov1alpha1.AuthzClusterRole) (*openchoreov1alpha1.AuthzClusterRole, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateRole,
		ResourceType: resourceTypeRole,
		ResourceID:   role.Name,
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateClusterRole(ctx, role)
}

func (s *authzServiceWithAuthz) DeleteClusterRole(ctx context.Context, name string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteRole,
		ResourceType: resourceTypeRole,
		ResourceID:   name,
	}); err != nil {
		return err
	}
	return s.internal.DeleteClusterRole(ctx, name)
}

// --- Namespace Roles ---

func (s *authzServiceWithAuthz) CreateNamespaceRole(ctx context.Context, namespace string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateRole,
		ResourceType: resourceTypeRole,
		ResourceID:   role.Name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateNamespaceRole(ctx, namespace, role)
}

func (s *authzServiceWithAuthz) GetNamespaceRole(ctx context.Context, namespace, name string) (*openchoreov1alpha1.AuthzRole, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewRole,
		ResourceType: resourceTypeRole,
		ResourceID:   name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetNamespaceRole(ctx, namespace, name)
}

func (s *authzServiceWithAuthz) ListNamespaceRoles(ctx context.Context, namespace string) (*openchoreov1alpha1.AuthzRoleList, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewRole,
		ResourceType: resourceTypeRole,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.ListNamespaceRoles(ctx, namespace)
}

func (s *authzServiceWithAuthz) UpdateNamespaceRole(ctx context.Context, namespace string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateRole,
		ResourceType: resourceTypeRole,
		ResourceID:   role.Name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateNamespaceRole(ctx, namespace, role)
}

func (s *authzServiceWithAuthz) DeleteNamespaceRole(ctx context.Context, namespace, name string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteRole,
		ResourceType: resourceTypeRole,
		ResourceID:   name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return err
	}
	return s.internal.DeleteNamespaceRole(ctx, namespace, name)
}

// --- Cluster Role Bindings ---

func (s *authzServiceWithAuthz) CreateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzClusterRoleBinding) (*openchoreov1alpha1.AuthzClusterRoleBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateRoleMapping,
		ResourceType: resourceTypeRoleMapping,
		ResourceID:   binding.Name,
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateClusterRoleBinding(ctx, binding)
}

func (s *authzServiceWithAuthz) GetClusterRoleBinding(ctx context.Context, name string) (*openchoreov1alpha1.AuthzClusterRoleBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewRoleMapping,
		ResourceType: resourceTypeRoleMapping,
		ResourceID:   name,
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterRoleBinding(ctx, name)
}

func (s *authzServiceWithAuthz) ListClusterRoleBindings(ctx context.Context) (*openchoreov1alpha1.AuthzClusterRoleBindingList, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewRoleMapping,
		ResourceType: resourceTypeRoleMapping,
	}); err != nil {
		return nil, err
	}
	return s.internal.ListClusterRoleBindings(ctx)
}

func (s *authzServiceWithAuthz) UpdateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzClusterRoleBinding) (*openchoreov1alpha1.AuthzClusterRoleBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateRoleMapping,
		ResourceType: resourceTypeRoleMapping,
		ResourceID:   binding.Name,
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateClusterRoleBinding(ctx, binding)
}

func (s *authzServiceWithAuthz) DeleteClusterRoleBinding(ctx context.Context, name string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteRoleMapping,
		ResourceType: resourceTypeRoleMapping,
		ResourceID:   name,
	}); err != nil {
		return err
	}
	return s.internal.DeleteClusterRoleBinding(ctx, name)
}

// --- Namespace Role Bindings ---

func (s *authzServiceWithAuthz) CreateNamespaceRoleBinding(ctx context.Context, namespace string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateRoleMapping,
		ResourceType: resourceTypeRoleMapping,
		ResourceID:   binding.Name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateNamespaceRoleBinding(ctx, namespace, binding)
}

func (s *authzServiceWithAuthz) GetNamespaceRoleBinding(ctx context.Context, namespace, name string) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewRoleMapping,
		ResourceType: resourceTypeRoleMapping,
		ResourceID:   name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetNamespaceRoleBinding(ctx, namespace, name)
}

func (s *authzServiceWithAuthz) ListNamespaceRoleBindings(ctx context.Context, namespace string) (*openchoreov1alpha1.AuthzRoleBindingList, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewRoleMapping,
		ResourceType: resourceTypeRoleMapping,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.ListNamespaceRoleBindings(ctx, namespace)
}

func (s *authzServiceWithAuthz) UpdateNamespaceRoleBinding(ctx context.Context, namespace string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateRoleMapping,
		ResourceType: resourceTypeRoleMapping,
		ResourceID:   binding.Name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateNamespaceRoleBinding(ctx, namespace, binding)
}

func (s *authzServiceWithAuthz) DeleteNamespaceRoleBinding(ctx context.Context, namespace, name string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteRoleMapping,
		ResourceType: resourceTypeRoleMapping,
		ResourceID:   name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return err
	}
	return s.internal.DeleteNamespaceRoleBinding(ctx, namespace, name)
}
