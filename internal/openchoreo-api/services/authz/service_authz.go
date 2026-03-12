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
	actionCreateClusterAuthzRole = "clusterauthzrole:create"
	actionViewClusterAuthzRole   = "clusterauthzrole:view"
	actionUpdateClusterAuthzRole = "clusterauthzrole:update"
	actionDeleteClusterAuthzRole = "clusterauthzrole:delete"

	actionCreateAuthzRole = "authzrole:create"
	actionViewAuthzRole   = "authzrole:view"
	actionUpdateAuthzRole = "authzrole:update"
	actionDeleteAuthzRole = "authzrole:delete"

	actionCreateClusterAuthzRoleBinding = "clusterauthzrolebinding:create"
	actionViewClusterAuthzRoleBinding   = "clusterauthzrolebinding:view"
	actionUpdateClusterAuthzRoleBinding = "clusterauthzrolebinding:update"
	actionDeleteClusterAuthzRoleBinding = "clusterauthzrolebinding:delete"

	actionCreateAuthzRoleBinding = "authzrolebinding:create"
	actionViewAuthzRoleBinding   = "authzrolebinding:view"
	actionUpdateAuthzRoleBinding = "authzrolebinding:update"
	actionDeleteAuthzRoleBinding = "authzrolebinding:delete"

	resourceTypeClusterAuthzRole        = "clusterAuthzRole"
	resourceTypeAuthzRole               = "authzRole"
	resourceTypeClusterAuthzRoleBinding = "clusterAuthzRoleBinding"
	resourceTypeAuthzRoleBinding        = "authzRoleBinding"
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
		internal: NewService(pap, pdp, k8sClient, logger),
		authz:    services.NewAuthzChecker(pdp, logger),
	}
}

// --- Cluster Roles ---

func (s *authzServiceWithAuthz) CreateClusterRole(ctx context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateClusterAuthzRole,
		ResourceType: resourceTypeClusterAuthzRole,
		ResourceID:   role.Name,
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateClusterRole(ctx, role)
}

func (s *authzServiceWithAuthz) GetClusterRole(ctx context.Context, name string) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewClusterAuthzRole,
		ResourceType: resourceTypeClusterAuthzRole,
		ResourceID:   name,
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterRole(ctx, name)
}

func (s *authzServiceWithAuthz) ListClusterRoles(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterAuthzRole], error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewClusterAuthzRole,
		ResourceType: resourceTypeClusterAuthzRole,
	}); err != nil {
		return nil, err
	}
	return s.internal.ListClusterRoles(ctx, opts)
}

func (s *authzServiceWithAuthz) UpdateClusterRole(ctx context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateClusterAuthzRole,
		ResourceType: resourceTypeClusterAuthzRole,
		ResourceID:   role.Name,
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateClusterRole(ctx, role)
}

func (s *authzServiceWithAuthz) DeleteClusterRole(ctx context.Context, name string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteClusterAuthzRole,
		ResourceType: resourceTypeClusterAuthzRole,
		ResourceID:   name,
	}); err != nil {
		return err
	}
	return s.internal.DeleteClusterRole(ctx, name)
}

// --- Namespace Roles ---

func (s *authzServiceWithAuthz) CreateNamespaceRole(ctx context.Context, namespace string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateAuthzRole,
		ResourceType: resourceTypeAuthzRole,
		ResourceID:   role.Name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateNamespaceRole(ctx, namespace, role)
}

func (s *authzServiceWithAuthz) GetNamespaceRole(ctx context.Context, namespace, name string) (*openchoreov1alpha1.AuthzRole, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewAuthzRole,
		ResourceType: resourceTypeAuthzRole,
		ResourceID:   name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetNamespaceRole(ctx, namespace, name)
}

func (s *authzServiceWithAuthz) ListNamespaceRoles(ctx context.Context, namespace string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.AuthzRole], error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewAuthzRole,
		ResourceType: resourceTypeAuthzRole,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.ListNamespaceRoles(ctx, namespace, opts)
}

func (s *authzServiceWithAuthz) UpdateNamespaceRole(ctx context.Context, namespace string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateAuthzRole,
		ResourceType: resourceTypeAuthzRole,
		ResourceID:   role.Name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateNamespaceRole(ctx, namespace, role)
}

func (s *authzServiceWithAuthz) DeleteNamespaceRole(ctx context.Context, namespace, name string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteAuthzRole,
		ResourceType: resourceTypeAuthzRole,
		ResourceID:   name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return err
	}
	return s.internal.DeleteNamespaceRole(ctx, namespace, name)
}

// --- Cluster Role Bindings ---

func (s *authzServiceWithAuthz) CreateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateClusterAuthzRoleBinding,
		ResourceType: resourceTypeClusterAuthzRoleBinding,
		ResourceID:   binding.Name,
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateClusterRoleBinding(ctx, binding)
}

func (s *authzServiceWithAuthz) GetClusterRoleBinding(ctx context.Context, name string) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewClusterAuthzRoleBinding,
		ResourceType: resourceTypeClusterAuthzRoleBinding,
		ResourceID:   name,
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterRoleBinding(ctx, name)
}

func (s *authzServiceWithAuthz) ListClusterRoleBindings(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterAuthzRoleBinding], error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewClusterAuthzRoleBinding,
		ResourceType: resourceTypeClusterAuthzRoleBinding,
	}); err != nil {
		return nil, err
	}
	return s.internal.ListClusterRoleBindings(ctx, opts)
}

func (s *authzServiceWithAuthz) UpdateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateClusterAuthzRoleBinding,
		ResourceType: resourceTypeClusterAuthzRoleBinding,
		ResourceID:   binding.Name,
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateClusterRoleBinding(ctx, binding)
}

func (s *authzServiceWithAuthz) DeleteClusterRoleBinding(ctx context.Context, name string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteClusterAuthzRoleBinding,
		ResourceType: resourceTypeClusterAuthzRoleBinding,
		ResourceID:   name,
	}); err != nil {
		return err
	}
	return s.internal.DeleteClusterRoleBinding(ctx, name)
}

// --- Namespace Role Bindings ---

func (s *authzServiceWithAuthz) CreateNamespaceRoleBinding(ctx context.Context, namespace string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateAuthzRoleBinding,
		ResourceType: resourceTypeAuthzRoleBinding,
		ResourceID:   binding.Name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateNamespaceRoleBinding(ctx, namespace, binding)
}

func (s *authzServiceWithAuthz) GetNamespaceRoleBinding(ctx context.Context, namespace, name string) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewAuthzRoleBinding,
		ResourceType: resourceTypeAuthzRoleBinding,
		ResourceID:   name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetNamespaceRoleBinding(ctx, namespace, name)
}

func (s *authzServiceWithAuthz) ListNamespaceRoleBindings(ctx context.Context, namespace string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.AuthzRoleBinding], error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewAuthzRoleBinding,
		ResourceType: resourceTypeAuthzRoleBinding,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.ListNamespaceRoleBindings(ctx, namespace, opts)
}

func (s *authzServiceWithAuthz) UpdateNamespaceRoleBinding(ctx context.Context, namespace string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateAuthzRoleBinding,
		ResourceType: resourceTypeAuthzRoleBinding,
		ResourceID:   binding.Name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateNamespaceRoleBinding(ctx, namespace, binding)
}

func (s *authzServiceWithAuthz) DeleteNamespaceRoleBinding(ctx context.Context, namespace, name string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteAuthzRoleBinding,
		ResourceType: resourceTypeAuthzRoleBinding,
		ResourceID:   name,
		Hierarchy:    authzcore.ResourceHierarchy{Namespace: namespace},
	}); err != nil {
		return err
	}
	return s.internal.DeleteNamespaceRoleBinding(ctx, namespace, name)
}

// --- Evaluation & Profile ---

// Evaluate delegates to the internal service without authz checks (requests contain their own subject context).
func (s *authzServiceWithAuthz) Evaluate(ctx context.Context, requests []authzcore.EvaluateRequest) ([]authzcore.Decision, error) {
	return s.internal.Evaluate(ctx, requests)
}

// ListActions delegates to the internal service without authz checks (actions are public metadata).
func (s *authzServiceWithAuthz) ListActions(ctx context.Context) ([]authzcore.Action, error) {
	return s.internal.ListActions(ctx)
}

// GetSubjectProfile delegates to the internal service without authz checks (returns profile for the caller).
func (s *authzServiceWithAuthz) GetSubjectProfile(ctx context.Context, request *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return s.internal.GetSubjectProfile(ctx, request)
}
