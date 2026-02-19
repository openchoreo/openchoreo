// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

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
	// Deprecated: Use CreateClusterRole or CreateNamespacedRole instead.
	AddRole(ctx context.Context, role *Role) error

	// Deprecated: Use the K8s client directly to delete roles.
	RemoveRole(ctx context.Context, roleRef *RoleRef) error

	// Deprecated: Use GetClusterRole or GetNamespacedRole instead.
	GetRole(ctx context.Context, roleRef *RoleRef) (*Role, error)

	// Deprecated: Use UpdateClusterRole or UpdateNamespacedRole instead.
	UpdateRole(ctx context.Context, role *Role) error

	// Deprecated: Use ListClusterRoles or ListNamespacedRoles instead.
	ListRoles(ctx context.Context, filter *RoleFilter) ([]*Role, error)

	// Deprecated: Use GetClusterRoleBinding or GetNamespacedRoleBinding instead.
	GetRoleEntitlementMapping(ctx context.Context, mappingRef *MappingRef) (*RoleEntitlementMapping, error)

	// Deprecated: Use CreateClusterRoleBinding or CreateNamespacedRoleBinding instead.
	AddRoleEntitlementMapping(ctx context.Context, mapping *RoleEntitlementMapping) error

	// Deprecated: Use UpdateClusterRoleBinding or UpdateNamespacedRoleBinding instead.
	UpdateRoleEntitlementMapping(ctx context.Context, mapping *RoleEntitlementMapping) error

	// Deprecated: Use the K8s client directly to delete role bindings.
	RemoveRoleEntitlementMapping(ctx context.Context, mappingRef *MappingRef) error

	// Deprecated: Use ListClusterRoleBindings or ListNamespacedRoleBindings instead.
	ListRoleEntitlementMappings(ctx context.Context, filter *RoleEntitlementMappingFilter) ([]*RoleEntitlementMapping, error)

	// ListActions lists all defined actions in the system
	ListActions(ctx context.Context) ([]string, error)

	// Roles - Cluster scoped

	// CreateClusterRole creates a new cluster-scoped role and returns the full CRD object
	CreateClusterRole(ctx context.Context, role *openchoreov1alpha1.AuthzClusterRole) (*openchoreov1alpha1.AuthzClusterRole, error)
	// GetClusterRole retrieves a cluster-scoped role by name
	GetClusterRole(ctx context.Context, name string) (*openchoreov1alpha1.AuthzClusterRole, error)
	// ListClusterRoles lists all cluster-scoped roles
	ListClusterRoles(ctx context.Context) (*openchoreov1alpha1.AuthzClusterRoleList, error)
	// UpdateClusterRole updates a cluster-scoped role and returns the updated CRD object
	UpdateClusterRole(ctx context.Context, role *openchoreov1alpha1.AuthzClusterRole) (*openchoreov1alpha1.AuthzClusterRole, error)

	// Roles - Namespace scoped

	// CreateNamespacedRole creates a new namespace-scoped role and returns the full CRD object
	CreateNamespacedRole(ctx context.Context, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error)
	// GetNamespacedRole retrieves a namespace-scoped role by name and namespace
	GetNamespacedRole(ctx context.Context, name string, namespace string) (*openchoreov1alpha1.AuthzRole, error)
	// ListNamespacedRoles lists namespace-scoped roles in the given namespace
	ListNamespacedRoles(ctx context.Context, namespace string) (*openchoreov1alpha1.AuthzRoleList, error)
	// UpdateNamespacedRole updates a namespace-scoped role and returns the updated CRD object
	UpdateNamespacedRole(ctx context.Context, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error)

	// Bindings - Cluster scoped

	// CreateClusterRoleBinding creates a new cluster-scoped role binding and returns the full CRD object
	CreateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzClusterRoleBinding) (*openchoreov1alpha1.AuthzClusterRoleBinding, error)
	// GetClusterRoleBinding retrieves a cluster-scoped role binding by name
	GetClusterRoleBinding(ctx context.Context, name string) (*openchoreov1alpha1.AuthzClusterRoleBinding, error)
	// ListClusterRoleBindings lists all cluster-scoped role bindings
	ListClusterRoleBindings(ctx context.Context) (*openchoreov1alpha1.AuthzClusterRoleBindingList, error)
	// UpdateClusterRoleBinding updates a cluster-scoped role binding and returns the updated CRD object
	UpdateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzClusterRoleBinding) (*openchoreov1alpha1.AuthzClusterRoleBinding, error)

	// Bindings - Namespace scoped

	// CreateNamespacedRoleBinding creates a new namespace-scoped role binding and returns the full CRD object
	CreateNamespacedRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error)
	// GetNamespacedRoleBinding retrieves a namespace-scoped role binding by name and namespace
	GetNamespacedRoleBinding(ctx context.Context, name string, namespace string) (*openchoreov1alpha1.AuthzRoleBinding, error)
	// ListNamespacedRoleBindings lists namespace-scoped role bindings in the given namespace
	ListNamespacedRoleBindings(ctx context.Context, namespace string) (*openchoreov1alpha1.AuthzRoleBindingList, error)
	// UpdateNamespacedRoleBinding updates a namespace-scoped role binding and returns the updated CRD object
	UpdateNamespacedRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error)
}
