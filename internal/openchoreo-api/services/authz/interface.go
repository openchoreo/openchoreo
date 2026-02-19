// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Service defines the authz service interface covering all 4 resource types.
// Both the core service (no authz) and the authz-wrapped service implement this.
type Service interface {
	// Cluster Roles
	CreateClusterRole(ctx context.Context, role *openchoreov1alpha1.AuthzClusterRole) (*openchoreov1alpha1.AuthzClusterRole, error)
	GetClusterRole(ctx context.Context, name string) (*openchoreov1alpha1.AuthzClusterRole, error)
	ListClusterRoles(ctx context.Context) (*openchoreov1alpha1.AuthzClusterRoleList, error)
	UpdateClusterRole(ctx context.Context, role *openchoreov1alpha1.AuthzClusterRole) (*openchoreov1alpha1.AuthzClusterRole, error)
	DeleteClusterRole(ctx context.Context, name string) error

	// Namespace Roles
	CreateNamespaceRole(ctx context.Context, namespace string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error)
	GetNamespaceRole(ctx context.Context, namespace, name string) (*openchoreov1alpha1.AuthzRole, error)
	ListNamespaceRoles(ctx context.Context, namespace string) (*openchoreov1alpha1.AuthzRoleList, error)
	UpdateNamespaceRole(ctx context.Context, namespace string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error)
	DeleteNamespaceRole(ctx context.Context, namespace, name string) error

	// Cluster Role Bindings
	CreateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzClusterRoleBinding) (*openchoreov1alpha1.AuthzClusterRoleBinding, error)
	GetClusterRoleBinding(ctx context.Context, name string) (*openchoreov1alpha1.AuthzClusterRoleBinding, error)
	ListClusterRoleBindings(ctx context.Context) (*openchoreov1alpha1.AuthzClusterRoleBindingList, error)
	UpdateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzClusterRoleBinding) (*openchoreov1alpha1.AuthzClusterRoleBinding, error)
	DeleteClusterRoleBinding(ctx context.Context, name string) error

	// Namespace Role Bindings
	CreateNamespaceRoleBinding(ctx context.Context, namespace string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error)
	GetNamespaceRoleBinding(ctx context.Context, namespace, name string) (*openchoreov1alpha1.AuthzRoleBinding, error)
	ListNamespaceRoleBindings(ctx context.Context, namespace string) (*openchoreov1alpha1.AuthzRoleBindingList, error)
	UpdateNamespaceRoleBinding(ctx context.Context, namespace string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error)
	DeleteNamespaceRoleBinding(ctx context.Context, namespace, name string) error
}
