// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// PaginatedList holds a page of items along with pagination metadata.
// This type is used by the PAP layer to return paginated results
type PaginatedList[T any] struct {
	Items      []T
	NextCursor string
}

// PDP (Policy Decision Point) interface defines the contract for authorization evaluation
type PDP interface {
	// Evaluate evaluates a single authorization request and returns a decision
	Evaluate(ctx context.Context, request *EvaluateRequest) (*Decision, error)

	// BatchEvaluate evaluates multiple authorization requests and returns corresponding decisions
	BatchEvaluate(ctx context.Context, request *BatchEvaluateRequest) (*BatchEvaluateResponse, error)

	// GetSubjectProfile retrieves the authorization profile for a given subject
	GetSubjectProfile(ctx context.Context, request *ProfileRequest) (*UserCapabilitiesResponse, error)
}

// PAP (Policy Administration Point) interface defines the contract for policy management
type PAP interface {
	// ListActions lists all public actions in the system
	ListActions(ctx context.Context) ([]Action, error)

	// Roles - Cluster scoped

	// CreateClusterRole creates a new cluster-scoped role and returns the full CRD object
	CreateClusterRole(ctx context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error)
	// GetClusterRole retrieves a cluster-scoped role by name
	GetClusterRole(ctx context.Context, name string) (*openchoreov1alpha1.ClusterAuthzRole, error)
	// ListClusterRoles lists all cluster-scoped roles
	ListClusterRoles(ctx context.Context, limit int, cursor string) (*PaginatedList[openchoreov1alpha1.ClusterAuthzRole], error)
	// UpdateClusterRole updates a cluster-scoped role and returns the updated CRD object
	UpdateClusterRole(ctx context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error)
	// DeleteClusterRole deletes a cluster-scoped role by name
	DeleteClusterRole(ctx context.Context, name string) error

	// Roles - Namespace scoped

	// CreateNamespacedRole creates a new namespace-scoped role and returns the full CRD object
	CreateNamespacedRole(ctx context.Context, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error)
	// GetNamespacedRole retrieves a namespace-scoped role by name and namespace
	GetNamespacedRole(ctx context.Context, name string, namespace string) (*openchoreov1alpha1.AuthzRole, error)
	// ListNamespacedRoles lists namespace-scoped roles in the given namespace
	ListNamespacedRoles(ctx context.Context, namespace string, limit int, cursor string) (*PaginatedList[openchoreov1alpha1.AuthzRole], error)
	// UpdateNamespacedRole updates a namespace-scoped role and returns the updated CRD object
	UpdateNamespacedRole(ctx context.Context, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error)
	// DeleteNamespacedRole deletes a namespace-scoped role by name and namespace
	DeleteNamespacedRole(ctx context.Context, name string, namespace string) error

	// Bindings - Cluster scoped

	// CreateClusterRoleBinding creates a new cluster-scoped role binding and returns the full CRD object
	CreateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error)
	// GetClusterRoleBinding retrieves a cluster-scoped role binding by name
	GetClusterRoleBinding(ctx context.Context, name string) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error)
	// ListClusterRoleBindings lists all cluster-scoped role bindings
	ListClusterRoleBindings(ctx context.Context, limit int, cursor string) (*PaginatedList[openchoreov1alpha1.ClusterAuthzRoleBinding], error)
	// UpdateClusterRoleBinding updates a cluster-scoped role binding and returns the updated CRD object
	UpdateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error)
	// DeleteClusterRoleBinding deletes a cluster-scoped role binding by name
	DeleteClusterRoleBinding(ctx context.Context, name string) error

	// Bindings - Namespace scoped

	// CreateNamespacedRoleBinding creates a new namespace-scoped role binding and returns the full CRD object
	CreateNamespacedRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error)
	// GetNamespacedRoleBinding retrieves a namespace-scoped role binding by name and namespace
	GetNamespacedRoleBinding(ctx context.Context, name string, namespace string) (*openchoreov1alpha1.AuthzRoleBinding, error)
	// ListNamespacedRoleBindings lists namespace-scoped role bindings in the given namespace
	ListNamespacedRoleBindings(ctx context.Context, namespace string, limit int, cursor string) (*PaginatedList[openchoreov1alpha1.AuthzRoleBinding], error)
	// UpdateNamespacedRoleBinding updates a namespace-scoped role binding and returns the updated CRD object
	UpdateNamespacedRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error)
	// DeleteNamespacedRoleBinding deletes a namespace-scoped role binding by name and namespace
	DeleteNamespacedRoleBinding(ctx context.Context, name string, namespace string) error
}
