// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// ListActions returns all public actions in the system
func (ce *CasbinEnforcer) ListActions(ctx context.Context) ([]authzcore.Action, error) {
	return authzcore.PublicActions(), nil
}

// CreateClusterRole creates a new cluster-scoped role and returns the full CRD object
func (ce *CasbinEnforcer) CreateClusterRole(ctx context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	if role == nil {
		return nil, fmt.Errorf("%w: cluster role cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("create cluster role called", "name", role.Name)

	if err := ce.k8sClient.Create(ctx, role); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil, authzcore.ErrRoleAlreadyExists
		}
		return nil, fmt.Errorf("failed to create ClusterAuthzRole: %w", err)
	}
	return role, nil
}

// GetClusterRole retrieves a cluster-scoped role by name
func (ce *CasbinEnforcer) GetClusterRole(ctx context.Context, name string) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	role := &openchoreov1alpha1.ClusterAuthzRole{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: name}, role); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get ClusterAuthzRole: %w", err)
	}
	return role, nil
}

// ListClusterRoles lists all cluster-scoped roles
func (ce *CasbinEnforcer) ListClusterRoles(ctx context.Context, limit int, cursor string) (*authzcore.PaginatedList[openchoreov1alpha1.ClusterAuthzRole], error) {
	list := &openchoreov1alpha1.ClusterAuthzRoleList{}
	opts := []client.ListOption{}
	if limit > 0 {
		opts = append(opts, client.Limit(int64(limit)))
	}
	if cursor != "" && limit > 0 {
		opts = append(opts, client.Continue(cursor))
	}
	if err := ce.k8sClient.List(ctx, list, opts...); err != nil {
		return nil, fmt.Errorf("failed to list ClusterAuthzRoles: %w", err)
	}
	return &authzcore.PaginatedList[openchoreov1alpha1.ClusterAuthzRole]{
		Items:      list.Items,
		NextCursor: list.Continue,
	}, nil
}

// UpdateClusterRole updates a cluster-scoped role and returns the updated CRD object
func (ce *CasbinEnforcer) UpdateClusterRole(ctx context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	if role == nil {
		return nil, fmt.Errorf("%w: cluster role cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("update cluster role called", "name", role.Name)

	existing := &openchoreov1alpha1.ClusterAuthzRole{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: role.Name}, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get ClusterAuthzRole: %w", err)
	}

	// Apply incoming spec directly, preserving server-managed ObjectMeta fields
	existing.Labels = role.Labels
	existing.Annotations = role.Annotations
	existing.Spec = role.Spec

	if err := ce.k8sClient.Update(ctx, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to update ClusterAuthzRole: %w", err)
	}
	return existing, nil
}

// DeleteClusterRole deletes a cluster-scoped role by name
func (ce *CasbinEnforcer) DeleteClusterRole(ctx context.Context, name string) error {
	role := &openchoreov1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	if err := ce.k8sClient.Delete(ctx, role); err != nil {
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleNotFound
		}
		return fmt.Errorf("failed to delete ClusterAuthzRole: %w", err)
	}
	ce.logger.Debug("deleted ClusterAuthzRole", "name", name)
	return nil
}

// CreateNamespacedRole creates a new namespace-scoped role and returns the full CRD object
func (ce *CasbinEnforcer) CreateNamespacedRole(ctx context.Context, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	if role == nil {
		return nil, fmt.Errorf("%w: namespaced role cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("create namespaced role called", "name", role.Name, "namespace", role.Namespace)

	if err := ce.k8sClient.Create(ctx, role); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil, authzcore.ErrRoleAlreadyExists
		}
		return nil, fmt.Errorf("failed to create AuthzRole: %w", err)
	}
	return role, nil
}

// GetNamespacedRole retrieves a namespace-scoped role by name and namespace
func (ce *CasbinEnforcer) GetNamespacedRole(ctx context.Context, name string, namespace string) (*openchoreov1alpha1.AuthzRole, error) {
	role := &openchoreov1alpha1.AuthzRole{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, role); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzRole: %w", err)
	}
	return role, nil
}

// ListNamespacedRoles lists namespace-scoped roles in the given namespace
func (ce *CasbinEnforcer) ListNamespacedRoles(ctx context.Context, namespace string, limit int, cursor string) (*authzcore.PaginatedList[openchoreov1alpha1.AuthzRole], error) {
	list := &openchoreov1alpha1.AuthzRoleList{}
	opts := []client.ListOption{client.InNamespace(namespace)}
	if limit > 0 {
		opts = append(opts, client.Limit(int64(limit)))
	}
	if cursor != "" {
		opts = append(opts, client.Continue(cursor))
	}
	if err := ce.k8sClient.List(ctx, list, opts...); err != nil {
		return nil, fmt.Errorf("failed to list AuthzRoles: %w", err)
	}
	return &authzcore.PaginatedList[openchoreov1alpha1.AuthzRole]{
		Items:      list.Items,
		NextCursor: list.Continue,
	}, nil
}

// UpdateNamespacedRole updates a namespace-scoped role and returns the updated CRD object
func (ce *CasbinEnforcer) UpdateNamespacedRole(ctx context.Context, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	if role == nil {
		return nil, fmt.Errorf("%w: namespaced role cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("update namespaced role called", "name", role.Name, "namespace", role.Namespace)

	existing := &openchoreov1alpha1.AuthzRole{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: role.Name, Namespace: role.Namespace}, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzRole: %w", err)
	}

	// Apply incoming spec directly, preserving server-managed ObjectMeta fields
	existing.Labels = role.Labels
	existing.Annotations = role.Annotations
	existing.Spec = role.Spec

	if err := ce.k8sClient.Update(ctx, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to update AuthzRole: %w", err)
	}
	return existing, nil
}

// DeleteNamespacedRole deletes a namespace-scoped role by name and namespace
func (ce *CasbinEnforcer) DeleteNamespacedRole(ctx context.Context, name string, namespace string) error {
	role := &openchoreov1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	if err := ce.k8sClient.Delete(ctx, role); err != nil {
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleNotFound
		}
		return fmt.Errorf("failed to delete AuthzRole: %w", err)
	}
	ce.logger.Debug("deleted AuthzRole", "name", name, "namespace", namespace)
	return nil
}

// CreateClusterRoleBinding creates a new cluster-scoped role binding and returns the full CRD object
func (ce *CasbinEnforcer) CreateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("%w: cluster role binding cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("create cluster role binding called", "name", binding.Name)

	if err := ce.k8sClient.Create(ctx, binding); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil, authzcore.ErrRoleMappingAlreadyExists
		}
		return nil, fmt.Errorf("failed to create ClusterAuthzRoleBinding: %w", err)
	}
	return binding, nil
}

// GetClusterRoleBinding retrieves a cluster-scoped role binding by name
func (ce *CasbinEnforcer) GetClusterRoleBinding(ctx context.Context, name string) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: name}, binding); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to get ClusterAuthzRoleBinding: %w", err)
	}
	return binding, nil
}

// ListClusterRoleBindings lists all cluster-scoped role bindings
func (ce *CasbinEnforcer) ListClusterRoleBindings(ctx context.Context, limit int, cursor string) (*authzcore.PaginatedList[openchoreov1alpha1.ClusterAuthzRoleBinding], error) {
	list := &openchoreov1alpha1.ClusterAuthzRoleBindingList{}
	opts := []client.ListOption{}
	if limit > 0 {
		opts = append(opts, client.Limit(int64(limit)))
	}
	if cursor != "" {
		opts = append(opts, client.Continue(cursor))
	}
	if err := ce.k8sClient.List(ctx, list, opts...); err != nil {
		return nil, fmt.Errorf("failed to list ClusterAuthzRoleBindings: %w", err)
	}
	return &authzcore.PaginatedList[openchoreov1alpha1.ClusterAuthzRoleBinding]{
		Items:      list.Items,
		NextCursor: list.Continue,
	}, nil
}

// UpdateClusterRoleBinding updates a cluster-scoped role binding and returns the updated CRD object
func (ce *CasbinEnforcer) UpdateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("%w: cluster role binding cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("update cluster role binding called", "name", binding.Name)

	existing := &openchoreov1alpha1.ClusterAuthzRoleBinding{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: binding.Name}, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to get ClusterAuthzRoleBinding: %w", err)
	}

	// Apply incoming spec directly, preserving server-managed ObjectMeta fields
	existing.Labels = binding.Labels
	existing.Annotations = binding.Annotations
	existing.Spec = binding.Spec

	if err := ce.k8sClient.Update(ctx, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to update ClusterAuthzRoleBinding: %w", err)
	}
	return existing, nil
}

// DeleteClusterRoleBinding deletes a cluster-scoped role binding by name
func (ce *CasbinEnforcer) DeleteClusterRoleBinding(ctx context.Context, name string) error {
	binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	if err := ce.k8sClient.Delete(ctx, binding); err != nil {
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleMappingNotFound
		}
		return fmt.Errorf("failed to delete ClusterAuthzRoleBinding: %w", err)
	}
	ce.logger.Debug("deleted ClusterAuthzRoleBinding", "name", name)
	return nil
}

// CreateNamespacedRoleBinding creates a new namespace-scoped role binding and returns the full CRD object
func (ce *CasbinEnforcer) CreateNamespacedRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("%w: namespaced role binding cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("create namespaced role binding called", "name", binding.Name, "namespace", binding.Namespace)

	if err := ce.k8sClient.Create(ctx, binding); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil, authzcore.ErrRoleMappingAlreadyExists
		}
		return nil, fmt.Errorf("failed to create AuthzRoleBinding: %w", err)
	}
	return binding, nil
}

// GetNamespacedRoleBinding retrieves a namespace-scoped role binding by name and namespace
func (ce *CasbinEnforcer) GetNamespacedRoleBinding(ctx context.Context, name string, namespace string) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	binding := &openchoreov1alpha1.AuthzRoleBinding{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, binding); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzRoleBinding: %w", err)
	}
	return binding, nil
}

// ListNamespacedRoleBindings lists namespace-scoped role bindings in the given namespace
func (ce *CasbinEnforcer) ListNamespacedRoleBindings(ctx context.Context, namespace string, limit int, cursor string) (*authzcore.PaginatedList[openchoreov1alpha1.AuthzRoleBinding], error) {
	list := &openchoreov1alpha1.AuthzRoleBindingList{}
	opts := []client.ListOption{client.InNamespace(namespace)}
	if limit > 0 {
		opts = append(opts, client.Limit(int64(limit)))
	}
	if cursor != "" {
		opts = append(opts, client.Continue(cursor))
	}
	if err := ce.k8sClient.List(ctx, list, opts...); err != nil {
		return nil, fmt.Errorf("failed to list AuthzRoleBindings: %w", err)
	}
	return &authzcore.PaginatedList[openchoreov1alpha1.AuthzRoleBinding]{
		Items:      list.Items,
		NextCursor: list.Continue,
	}, nil
}

// UpdateNamespacedRoleBinding updates a namespace-scoped role binding and returns the updated CRD object
func (ce *CasbinEnforcer) UpdateNamespacedRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("%w: namespaced role binding cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("update namespaced role binding called", "name", binding.Name, "namespace", binding.Namespace)

	existing := &openchoreov1alpha1.AuthzRoleBinding{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: binding.Name, Namespace: binding.Namespace}, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzRoleBinding: %w", err)
	}

	// Apply incoming spec directly, preserving server-managed ObjectMeta fields
	existing.Labels = binding.Labels
	existing.Annotations = binding.Annotations
	existing.Spec = binding.Spec

	if err := ce.k8sClient.Update(ctx, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to update AuthzRoleBinding: %w", err)
	}
	return existing, nil
}

// DeleteNamespacedRoleBinding deletes a namespace-scoped role binding by name and namespace
func (ce *CasbinEnforcer) DeleteNamespacedRoleBinding(ctx context.Context, name string, namespace string) error {
	binding := &openchoreov1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	if err := ce.k8sClient.Delete(ctx, binding); err != nil {
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleMappingNotFound
		}
		return fmt.Errorf("failed to delete AuthzRoleBinding: %w", err)
	}
	ce.logger.Debug("deleted AuthzRoleBinding", "name", name, "namespace", namespace)
	return nil
}
