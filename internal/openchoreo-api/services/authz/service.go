// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// authzService handles authz CRUD operations without authorization checks.
type authzService struct {
	pap       authzcore.PAP
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*authzService)(nil)

// NewService creates a new authz service without authorization checks.
func NewService(pap authzcore.PAP, k8sClient client.Client, logger *slog.Logger) Service {
	return &authzService{
		pap:       pap,
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// --- Cluster Roles ---

func (s *authzService) CreateClusterRole(ctx context.Context, role *openchoreov1alpha1.AuthzClusterRole) (*openchoreov1alpha1.AuthzClusterRole, error) {
	if role == nil {
		return nil, fmt.Errorf("cluster role cannot be nil")
	}
	s.logger.Debug("Creating cluster role", "name", role.Name)
	return s.pap.CreateClusterRole(ctx, role)
}

func (s *authzService) GetClusterRole(ctx context.Context, name string) (*openchoreov1alpha1.AuthzClusterRole, error) {
	s.logger.Debug("Getting cluster role", "name", name)
	return s.pap.GetClusterRole(ctx, name)
}

func (s *authzService) ListClusterRoles(ctx context.Context) (*openchoreov1alpha1.AuthzClusterRoleList, error) {
	s.logger.Debug("Listing cluster roles")
	return s.pap.ListClusterRoles(ctx)
}

func (s *authzService) UpdateClusterRole(ctx context.Context, role *openchoreov1alpha1.AuthzClusterRole) (*openchoreov1alpha1.AuthzClusterRole, error) {
	if role == nil {
		return nil, fmt.Errorf("cluster role cannot be nil")
	}
	s.logger.Debug("Updating cluster role", "name", role.Name)
	return s.pap.UpdateClusterRole(ctx, role)
}

func (s *authzService) DeleteClusterRole(ctx context.Context, name string) error {
	s.logger.Debug("Deleting cluster role", "name", name)
	role := &openchoreov1alpha1.AuthzClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	if err := s.k8sClient.Delete(ctx, role); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrRoleNotFound
		}
		return fmt.Errorf("failed to delete cluster role: %w", err)
	}
	return nil
}

// --- Namespace Roles ---

func (s *authzService) CreateNamespaceRole(ctx context.Context, namespace string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	if role == nil {
		return nil, fmt.Errorf("namespace role cannot be nil")
	}
	role.Namespace = namespace
	s.logger.Debug("Creating namespace role", "namespace", namespace, "name", role.Name)
	return s.pap.CreateNamespacedRole(ctx, role)
}

func (s *authzService) GetNamespaceRole(ctx context.Context, namespace, name string) (*openchoreov1alpha1.AuthzRole, error) {
	s.logger.Debug("Getting namespace role", "namespace", namespace, "name", name)
	return s.pap.GetNamespacedRole(ctx, name, namespace)
}

func (s *authzService) ListNamespaceRoles(ctx context.Context, namespace string) (*openchoreov1alpha1.AuthzRoleList, error) {
	s.logger.Debug("Listing namespace roles", "namespace", namespace)
	return s.pap.ListNamespacedRoles(ctx, namespace)
}

func (s *authzService) UpdateNamespaceRole(ctx context.Context, namespace string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	if role == nil {
		return nil, fmt.Errorf("namespace role cannot be nil")
	}
	role.Namespace = namespace
	s.logger.Debug("Updating namespace role", "namespace", namespace, "name", role.Name)
	return s.pap.UpdateNamespacedRole(ctx, role)
}

func (s *authzService) DeleteNamespaceRole(ctx context.Context, namespace, name string) error {
	s.logger.Debug("Deleting namespace role", "namespace", namespace, "name", name)
	role := &openchoreov1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	if err := s.k8sClient.Delete(ctx, role); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrRoleNotFound
		}
		return fmt.Errorf("failed to delete namespace role: %w", err)
	}
	return nil
}

// --- Cluster Role Bindings ---

func (s *authzService) CreateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzClusterRoleBinding) (*openchoreov1alpha1.AuthzClusterRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("cluster role binding cannot be nil")
	}
	s.logger.Debug("Creating cluster role binding", "name", binding.Name)
	return s.pap.CreateClusterRoleBinding(ctx, binding)
}

func (s *authzService) GetClusterRoleBinding(ctx context.Context, name string) (*openchoreov1alpha1.AuthzClusterRoleBinding, error) {
	s.logger.Debug("Getting cluster role binding", "name", name)
	return s.pap.GetClusterRoleBinding(ctx, name)
}

func (s *authzService) ListClusterRoleBindings(ctx context.Context) (*openchoreov1alpha1.AuthzClusterRoleBindingList, error) {
	s.logger.Debug("Listing cluster role bindings")
	return s.pap.ListClusterRoleBindings(ctx)
}

func (s *authzService) UpdateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzClusterRoleBinding) (*openchoreov1alpha1.AuthzClusterRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("cluster role binding cannot be nil")
	}
	s.logger.Debug("Updating cluster role binding", "name", binding.Name)
	return s.pap.UpdateClusterRoleBinding(ctx, binding)
}

func (s *authzService) DeleteClusterRoleBinding(ctx context.Context, name string) error {
	s.logger.Debug("Deleting cluster role binding", "name", name)
	binding := &openchoreov1alpha1.AuthzClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	if err := s.k8sClient.Delete(ctx, binding); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrRoleBindingNotFound
		}
		return fmt.Errorf("failed to delete cluster role binding: %w", err)
	}
	return nil
}

// --- Namespace Role Bindings ---

func (s *authzService) CreateNamespaceRoleBinding(ctx context.Context, namespace string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("namespace role binding cannot be nil")
	}
	binding.Namespace = namespace
	s.logger.Debug("Creating namespace role binding", "namespace", namespace, "name", binding.Name)
	return s.pap.CreateNamespacedRoleBinding(ctx, binding)
}

func (s *authzService) GetNamespaceRoleBinding(ctx context.Context, namespace, name string) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	s.logger.Debug("Getting namespace role binding", "namespace", namespace, "name", name)
	return s.pap.GetNamespacedRoleBinding(ctx, name, namespace)
}

func (s *authzService) ListNamespaceRoleBindings(ctx context.Context, namespace string) (*openchoreov1alpha1.AuthzRoleBindingList, error) {
	s.logger.Debug("Listing namespace role bindings", "namespace", namespace)
	return s.pap.ListNamespacedRoleBindings(ctx, namespace)
}

func (s *authzService) UpdateNamespaceRoleBinding(ctx context.Context, namespace string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("namespace role binding cannot be nil")
	}
	binding.Namespace = namespace
	s.logger.Debug("Updating namespace role binding", "namespace", namespace, "name", binding.Name)
	return s.pap.UpdateNamespacedRoleBinding(ctx, binding)
}

func (s *authzService) DeleteNamespaceRoleBinding(ctx context.Context, namespace, name string) error {
	s.logger.Debug("Deleting namespace role binding", "namespace", namespace, "name", name)
	binding := &openchoreov1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	if err := s.k8sClient.Delete(ctx, binding); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrRoleBindingNotFound
		}
		return fmt.Errorf("failed to delete namespace role binding: %w", err)
	}
	return nil
}
