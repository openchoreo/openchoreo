// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/casbin/casbin/v2"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"

	authzv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// authzInformerHandler implements cache.ResourceEventHandler with incremental updates
type authzInformerHandler struct {
	enforcer casbin.IEnforcer
	logger   *slog.Logger
	crdType  string // "AuthzRole", "AuthzClusterRole", "AuthzRoleBinding", "AuthzClusterRoleBinding"
}

var _ cache.ResourceEventHandler = (*authzInformerHandler)(nil)

// SetupAuthzWatchers sets up informer-based watchers with incremental updates
func SetupAuthzWatchers(
	ctx context.Context,
	mgr ctrl.Manager,
	enforcer casbin.IEnforcer,
	logger *slog.Logger,
) error {
	if err := setupAuthzRoleWatcher(ctx, mgr, enforcer, logger); err != nil {
		return err
	}

	if err := setupAuthzClusterRoleWatcher(ctx, mgr, enforcer, logger); err != nil {
		return err
	}

	if err := setupAuthzRoleBindingWatcher(ctx, mgr, enforcer, logger); err != nil {
		return err
	}

	if err := setupAuthzClusterRoleBindingWatcher(ctx, mgr, enforcer, logger); err != nil {
		return err
	}

	return nil
}

// setupAuthzRoleWatcher sets up the informer for AuthzRole CRDs
func setupAuthzRoleWatcher(
	ctx context.Context,
	mgr ctrl.Manager,
	enforcer casbin.IEnforcer,
	logger *slog.Logger,
) error {
	handler := &authzInformerHandler{
		enforcer: enforcer,
		logger:   logger.With("crdType", "AuthzRole"),
		crdType:  "AuthzRole",
	}

	informer, err := mgr.GetCache().GetInformer(ctx, &authzv1alpha1.AuthzRole{})
	if err != nil {
		return fmt.Errorf("failed to get AuthzRole informer: %w", err)
	}

	if _, err = informer.AddEventHandler(handler); err != nil {
		return fmt.Errorf("failed to add AuthzRole event handler: %w", err)
	}

	logger.Info("Set up informer for AuthzRole CRDs")
	return nil
}

// setupAuthzClusterRoleWatcher sets up the informer for AuthzClusterRole CRDs
func setupAuthzClusterRoleWatcher(
	ctx context.Context,
	mgr ctrl.Manager,
	enforcer casbin.IEnforcer,
	logger *slog.Logger,
) error {
	handler := &authzInformerHandler{
		enforcer: enforcer,
		logger:   logger.With("crdType", "AuthzClusterRole"),
		crdType:  "AuthzClusterRole",
	}

	informer, err := mgr.GetCache().GetInformer(ctx, &authzv1alpha1.AuthzClusterRole{})
	if err != nil {
		return fmt.Errorf("failed to get AuthzClusterRole informer: %w", err)
	}

	if _, err = informer.AddEventHandler(handler); err != nil {
		return fmt.Errorf("failed to add AuthzClusterRole event handler: %w", err)
	}

	logger.Info("Set up informer for AuthzClusterRole CRDs")
	return nil
}

// setupAuthzRoleBindingWatcher sets up the informer for AuthzRoleBinding CRDs
func setupAuthzRoleBindingWatcher(
	ctx context.Context,
	mgr ctrl.Manager,
	enforcer casbin.IEnforcer,
	logger *slog.Logger,
) error {
	handler := &authzInformerHandler{
		enforcer: enforcer,
		logger:   logger.With("crdType", "AuthzRoleBinding"),
		crdType:  "AuthzRoleBinding",
	}

	informer, err := mgr.GetCache().GetInformer(ctx, &authzv1alpha1.AuthzRoleBinding{})
	if err != nil {
		return fmt.Errorf("failed to get AuthzRoleBinding informer: %w", err)
	}

	if _, err = informer.AddEventHandler(handler); err != nil {
		return fmt.Errorf("failed to add AuthzRoleBinding event handler: %w", err)
	}

	logger.Info("Set up informer for AuthzRoleBinding CRDs")
	return nil
}

// setupAuthzClusterRoleBindingWatcher sets up the informer for AuthzClusterRoleBinding CRDs
func setupAuthzClusterRoleBindingWatcher(
	ctx context.Context,
	mgr ctrl.Manager,
	enforcer casbin.IEnforcer,
	logger *slog.Logger,
) error {
	handler := &authzInformerHandler{
		enforcer: enforcer,
		logger:   logger.With("crdType", "AuthzClusterRoleBinding"),
		crdType:  "AuthzClusterRoleBinding",
	}

	informer, err := mgr.GetCache().GetInformer(ctx, &authzv1alpha1.AuthzClusterRoleBinding{})
	if err != nil {
		return fmt.Errorf("failed to get AuthzClusterRoleBinding informer: %w", err)
	}

	if _, err = informer.AddEventHandler(handler); err != nil {
		return fmt.Errorf("failed to add AuthzClusterRoleBinding event handler: %w", err)
	}

	logger.Info("Set up informer for AuthzClusterRoleBinding CRDs")
	return nil
}

// OnAdd handles CREATE events with incremental policy addition
func (h *authzInformerHandler) OnAdd(obj interface{}, isInInitialList bool) {
	if err := h.handleAdd(obj); err != nil {
		h.logger.Error("Incremental add failed, will recover on next periodic sync", "error", err)
	}
}

// OnUpdate handles UPDATE events by removing old and adding new
func (h *authzInformerHandler) OnUpdate(oldObj, newObj interface{}) {
	if err := h.handleUpdate(oldObj, newObj); err != nil {
		h.logger.Error("Incremental update failed, will recover on next periodic sync", "error", err)
	}
}

// OnDelete handles DELETE events with incremental policy removal
func (h *authzInformerHandler) OnDelete(obj interface{}) {
	if err := h.handleDelete(obj); err != nil {
		h.logger.Warn("Incremental delete failed, will recover on next periodic sync", "error", err)
	}
}

func (h *authzInformerHandler) handleAdd(obj interface{}) error {
	switch h.crdType {
	case CRDTypeAuthzRole:
		return h.handleAddRole(obj)
	case CRDTypeAuthzClusterRole:
		return h.handleAddClusterRole(obj)
	case CRDTypeAuthzRoleBinding:
		return h.handleAddBinding(obj)
	case CRDTypeAuthzClusterRoleBinding:
		return h.handleAddClusterBinding(obj)
	default:
		h.logger.Warn("Unknown CRD type in handleAdd", "crdType", h.crdType)
	}
	return nil
}

func (h *authzInformerHandler) handleAddRole(obj interface{}) error {
	role, ok := obj.(*authzv1alpha1.AuthzRole)
	if !ok {
		h.logger.Warn("Received non-AuthzRole object in OnAdd")
		return nil
	}

	// Add each action as a grouping policy: g, roleName, action, namespace
	// Check existence first to avoid unnecessary writes
	for _, action := range role.Spec.Actions {
		exists, err := h.enforcer.HasGroupingPolicy(role.Name, action, role.Namespace)
		if err != nil {
			h.logger.Warn("failed to check grouping policy for role", "role", role.Name, "error", err)
		}
		if exists {
			continue
		}
		if _, err := h.enforcer.AddGroupingPolicy(role.Name, action, role.Namespace); err != nil {
			return fmt.Errorf("failed to add grouping policy for role %s: %w", role.Name, err)
		}
	}

	h.logger.Debug("role policies added successfully",
		"role", role.Name,
		"namespace", role.Namespace,
		"actions", role.Spec.Actions)

	return nil
}

func (h *authzInformerHandler) handleAddClusterRole(obj interface{}) error {
	clusterRole, ok := obj.(*authzv1alpha1.AuthzClusterRole)
	if !ok {
		h.logger.Warn("Received non-AuthzClusterRole object in OnAdd")
		return nil
	}

	// Add each action as a grouping policy: g, roleName, action, "*" (cluster-scoped)
	for _, action := range clusterRole.Spec.Actions {
		exists, err := h.enforcer.HasGroupingPolicy(clusterRole.Name, action, "*")
		if err != nil {
			h.logger.Warn("failed to check grouping policy for cluster role", "role", clusterRole.Name, "error", err)
		}
		if exists {
			continue
		}
		if _, err := h.enforcer.AddGroupingPolicy(clusterRole.Name, action, "*"); err != nil {
			return fmt.Errorf("failed to add grouping policy for cluster role %s: %w", clusterRole.Name, err)
		}
	}

	h.logger.Debug("cluster role policies added successfully",
		"role", clusterRole.Name,
		"actions", clusterRole.Spec.Actions)

	return nil
}

func (h *authzInformerHandler) handleAddBinding(obj interface{}) error {
	binding, ok := obj.(*authzv1alpha1.AuthzRoleBinding)
	if !ok {
		h.logger.Warn("Received non-AuthzRoleBinding object in OnAdd")
		return nil
	}

	// Format subject as "claim:value"
	subject, err := formatSubject(binding.Spec.Entitlement.Claim, binding.Spec.Entitlement.Value)
	if err != nil {
		return fmt.Errorf("failed to format subject in handleAddBinding: %w", err)
	}

	// Convert targetPath to resource path
	resourcePath := resourceHierarchyToPath(authzcore.ResourceHierarchy{
		Namespace: binding.Namespace,
		Project:   binding.Spec.TargetPath.Project,
		Component: binding.Spec.TargetPath.Component,
	})

	// Determine role namespace based on roleRef.Kind
	roleNamespace := binding.Namespace
	if binding.Spec.RoleRef.Kind == CRDTypeAuthzClusterRole {
		roleNamespace = "*"
	}

	// Get effect (default to "allow" if not specified)
	effect := string(binding.Spec.Effect)
	if effect == "" {
		return fmt.Errorf("effect not specified in binding %s", binding.Name)
	}

	// Check existence
	exists, err := h.enforcer.HasPolicy(
		subject,
		resourcePath,
		binding.Spec.RoleRef.Name,
		roleNamespace,
		effect,
		emptyContextJSON,
	)
	if err != nil {
		h.logger.Warn("failed to check policy for binding", "binding", binding.Name, "error", err)
	}
	if exists {
		return nil
	}

	// Format: p, subject, resourcePath, roleName, roleNamespace, effect, context
	if _, err := h.enforcer.AddPolicy(
		subject,
		resourcePath,
		binding.Spec.RoleRef.Name,
		roleNamespace,
		effect,
		emptyContextJSON,
	); err != nil {
		return fmt.Errorf("failed to add policy for binding %s: %w", binding.Name, err)
	}

	h.logger.Debug("binding policy added successfully",
		"binding", binding.Name,
		"namespace", binding.Namespace,
		"subject", subject,
		"resource", resourcePath,
		"role", binding.Spec.RoleRef.Name)

	return nil
}

func (h *authzInformerHandler) handleAddClusterBinding(obj interface{}) error {
	binding, ok := obj.(*authzv1alpha1.AuthzClusterRoleBinding)
	if !ok {
		h.logger.Warn("Received non-AuthzClusterRoleBinding object in OnAdd")
		return nil
	}

	// Format subject as "claim:value"
	subject, err := formatSubject(binding.Spec.Entitlement.Claim, binding.Spec.Entitlement.Value)
	if err != nil {
		return fmt.Errorf("failed to format subject in handleAddClusterBinding: %w", err)
	}

	// Cluster bindings apply to all resources
	resourcePath := "*"
	roleNamespace := "*"

	// Get effect (default to "allow" if not specified)
	effect := string(binding.Spec.Effect)
	if effect == "" {
		return fmt.Errorf("effect not specified in cluster binding %s", binding.Name)
	}

	// Check existence first (read lock) to avoid unnecessary write locks
	exists, err := h.enforcer.HasPolicy(
		subject,
		resourcePath,
		binding.Spec.RoleRef.Name,
		roleNamespace,
		effect,
		emptyContextJSON,
	)
	if err != nil {
		h.logger.Warn("failed to check policy for cluster binding", "binding", binding.Name, "error", err)
	}
	if exists {
		return nil
	}

	// Format: p, subject, resourcePath, roleName, "*", effect, context
	if _, err := h.enforcer.AddPolicy(
		subject,
		resourcePath,
		binding.Spec.RoleRef.Name,
		roleNamespace,
		effect,
		emptyContextJSON,
	); err != nil {
		return fmt.Errorf("failed to add policy for cluster binding %s: %w", binding.Name, err)
	}

	h.logger.Debug("cluster binding policy added successfully",
		"binding", binding.Name,
		"subject", subject,
		"role", binding.Spec.RoleRef.Name)

	return nil
}

func (h *authzInformerHandler) handleUpdate(oldObj, newObj interface{}) error {
	switch h.crdType {
	case CRDTypeAuthzRole:
		return h.handleUpdateRole(oldObj, newObj)
	case CRDTypeAuthzClusterRole:
		return h.handleUpdateClusterRole(oldObj, newObj)
	case CRDTypeAuthzRoleBinding:
		return h.handleUpdateBinding(oldObj, newObj)
	case CRDTypeAuthzClusterRoleBinding:
		return h.handleUpdateClusterBinding(oldObj, newObj)
	default:
		h.logger.Warn("Unknown CRD type in handleUpdate", "crdType", h.crdType)
	}
	return nil
}

func (h *authzInformerHandler) handleUpdateRole(oldObj, newObj interface{}) error {
	oldRole, ok1 := oldObj.(*authzv1alpha1.AuthzRole)
	newRole, ok2 := newObj.(*authzv1alpha1.AuthzRole)
	if !ok1 || !ok2 {
		h.logger.Warn("Received non-AuthzRole object in OnUpdate")
		return nil
	}

	// Check if generation changed (spec changed, not just metadata)
	if oldRole.Generation == newRole.Generation {
		h.logger.Debug("Skipping update - only metadata changed",
			"role", newRole.Name)
		return nil
	}

	// Compute actions diff
	addedActions, removedActions := computeActionsDiff(oldRole.Spec.Actions, newRole.Spec.Actions)

	// Remove old actions
	for _, action := range removedActions {
		removed, err := h.enforcer.RemoveGroupingPolicy(oldRole.Name, action, oldRole.Namespace)
		if err != nil {
			h.logger.Warn("failed to remove old grouping policy", "error", err)
		}
		if !removed {
			h.logger.Debug("Old grouping policy did not exist",
				"role", oldRole.Name,
				"action", action)
		}
	}

	// Add new actions
	for _, action := range addedActions {
		added, err := h.enforcer.AddGroupingPolicy(newRole.Name, action, newRole.Namespace)
		if err != nil {
			h.logger.Warn("failed to add new grouping policy", "error", err)
		}
		if !added {
			h.logger.Debug("New grouping policy already exists",
				"role", newRole.Name,
				"action", action)
		}
	}

	h.logger.Debug("role policies updated successfully",
		"role", newRole.Name,
		"namespace", newRole.Namespace,
		"oldActions", oldRole.Spec.Actions,
		"newActions", newRole.Spec.Actions)

	return nil
}

func (h *authzInformerHandler) handleUpdateClusterRole(oldObj, newObj interface{}) error {
	oldRole, ok1 := oldObj.(*authzv1alpha1.AuthzClusterRole)
	newRole, ok2 := newObj.(*authzv1alpha1.AuthzClusterRole)
	if !ok1 || !ok2 {
		h.logger.Error("Received non-AuthzClusterRole object in OnUpdate")
		return nil
	}

	// Check if generation changed
	if oldRole.Generation == newRole.Generation {
		h.logger.Debug("Skipping update - only metadata changed",
			"role", newRole.Name)
		return nil
	}

	// Compute actions diff
	addedActions, removedActions := computeActionsDiff(oldRole.Spec.Actions, newRole.Spec.Actions)

	// Remove old actions
	for _, action := range removedActions {
		removed, err := h.enforcer.RemoveGroupingPolicy(oldRole.Name, action, "*")
		if err != nil {
			return fmt.Errorf("failed to remove old cluster grouping policy: %w", err)
		}
		if !removed {
			h.logger.Debug("Old cluster grouping policy did not exist",
				"role", oldRole.Name,
				"action", action)
		}
	}

	// Add new actions
	for _, action := range addedActions {
		added, err := h.enforcer.AddGroupingPolicy(newRole.Name, action, "*")
		if err != nil {
			return fmt.Errorf("failed to add new cluster grouping policy: %w", err)
		}
		if !added {
			h.logger.Debug("New cluster grouping policy already exists",
				"role", newRole.Name,
				"action", action)
		}
	}

	h.logger.Debug("cluster role policies updated successfully",
		"role", newRole.Name,
		"oldActions", oldRole.Spec.Actions,
		"newActions", newRole.Spec.Actions)

	return nil
}

func (h *authzInformerHandler) handleUpdateBinding(oldObj, newObj interface{}) error {
	oldBinding, ok1 := oldObj.(*authzv1alpha1.AuthzRoleBinding)
	newBinding, ok2 := newObj.(*authzv1alpha1.AuthzRoleBinding)
	if !ok1 || !ok2 {
		h.logger.Error("Received non-AuthzRoleBinding object in OnUpdate")
		return nil
	}

	// Check if generation changed
	if oldBinding.Generation == newBinding.Generation {
		h.logger.Debug("Skipping update - only metadata changed",
			"binding", newBinding.Name)
		return nil
	}

	//  existing policy
	oldSubject, err := formatSubject(oldBinding.Spec.Entitlement.Claim, oldBinding.Spec.Entitlement.Value)
	if err != nil {
		return fmt.Errorf("failed to format old subject: %w", err)
	}
	oldResourcePath := resourceHierarchyToPath(authzcore.ResourceHierarchy{
		Namespace: oldBinding.Namespace,
		Project:   oldBinding.Spec.TargetPath.Project,
		Component: oldBinding.Spec.TargetPath.Component,
	})
	oldRoleNamespace := oldBinding.Namespace
	if oldBinding.Spec.RoleRef.Kind == CRDTypeAuthzClusterRole {
		oldRoleNamespace = "*"
	}
	oldEffect := string(oldBinding.Spec.Effect)
	if oldEffect == "" {
		return fmt.Errorf("old binding effect not specified")
	}
	existingPolicy := []string{
		oldSubject,
		oldResourcePath,
		oldBinding.Spec.RoleRef.Name,
		oldRoleNamespace,
		oldEffect,
		string(emptyContextJSON),
	}

	//  new policy
	newSubject, err := formatSubject(newBinding.Spec.Entitlement.Claim, newBinding.Spec.Entitlement.Value)
	if err != nil {
		return fmt.Errorf("failed to format new subject: %w", err)
	}

	newResourcePath := resourceHierarchyToPath(authzcore.ResourceHierarchy{
		Namespace: newBinding.Namespace,
		Project:   newBinding.Spec.TargetPath.Project,
		Component: newBinding.Spec.TargetPath.Component,
	})
	newRoleNamespace := newBinding.Namespace
	if newBinding.Spec.RoleRef.Kind == CRDTypeAuthzClusterRole {
		newRoleNamespace = "*"
	}
	newEffect := string(newBinding.Spec.Effect)
	if newEffect == "" {
		return fmt.Errorf("new binding effect not specified")
	}
	newPolicy := []string{
		newSubject,
		newResourcePath,
		newBinding.Spec.RoleRef.Name,
		newRoleNamespace,
		newEffect,
		string(emptyContextJSON),
	}

	ok, err := h.enforcer.UpdatePolicy(existingPolicy, newPolicy)
	if err != nil {
		return fmt.Errorf("failed to update binding policy: %w", err)
	}
	if !ok {
		h.logger.Debug("Binding policy did not exist to update", "binding", newBinding.Name)
	}

	h.logger.Debug("binding policy updated successfully",
		"binding", newBinding.Name,
		"namespace", newBinding.Namespace)

	return nil
}

func (h *authzInformerHandler) handleUpdateClusterBinding(oldObj, newObj interface{}) error {
	oldBinding, ok1 := oldObj.(*authzv1alpha1.AuthzClusterRoleBinding)
	newBinding, ok2 := newObj.(*authzv1alpha1.AuthzClusterRoleBinding)
	if !ok1 || !ok2 {
		h.logger.Error("Received non-AuthzClusterRoleBinding object in OnUpdate")
		return nil
	}

	// Check if generation changed
	if oldBinding.Generation == newBinding.Generation {
		h.logger.Debug("Skipping update - only metadata changed",
			"binding", newBinding.Name)
		return nil
	}

	// Build existing policy
	oldSubject, err := formatSubject(oldBinding.Spec.Entitlement.Claim, oldBinding.Spec.Entitlement.Value)
	if err != nil {
		return fmt.Errorf("failed to format old subject: %w", err)
	}
	oldEffect := string(oldBinding.Spec.Effect)
	if oldEffect == "" {
		return fmt.Errorf("effect not specified in cluster binding %s", oldBinding.Name)
	}
	existingPolicy := []string{
		oldSubject,
		"*",
		oldBinding.Spec.RoleRef.Name,
		"*",
		oldEffect,
		string(emptyContextJSON),
	}

	// Build new policy
	newSubject, err := formatSubject(newBinding.Spec.Entitlement.Claim, newBinding.Spec.Entitlement.Value)
	if err != nil {
		return fmt.Errorf("failed to format new subject: %w", err)
	}
	newEffect := string(newBinding.Spec.Effect)
	if newEffect == "" {
		return fmt.Errorf("effect not specified in cluster binding %s", newBinding.Name)
	}
	newPolicy := []string{
		newSubject,
		"*",
		newBinding.Spec.RoleRef.Name,
		"*",
		newEffect,
		string(emptyContextJSON),
	}

	ok, err := h.enforcer.UpdatePolicy(existingPolicy, newPolicy)
	if err != nil {
		return fmt.Errorf("failed to update cluster binding policy: %w", err)
	}
	if !ok {
		h.logger.Debug("Cluster binding policy did not exist to update", "binding", newBinding.Name)
	}

	h.logger.Debug("cluster binding policy updated successfully",
		"binding", newBinding.Name)

	return nil
}

func (h *authzInformerHandler) handleDelete(obj interface{}) error {
	// Handle DeletedFinalStateUnknown
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	switch h.crdType {
	case CRDTypeAuthzRole:
		return h.handleDeleteRole(obj)
	case CRDTypeAuthzClusterRole:
		return h.handleDeleteClusterRole(obj)
	case CRDTypeAuthzRoleBinding:
		return h.handleDeleteBinding(obj)
	case CRDTypeAuthzClusterRoleBinding:
		return h.handleDeleteClusterBinding(obj)
	default:
		h.logger.Warn("Unknown CRD type in handleDelete", "crdType", h.crdType)
	}
	return nil
}

func (h *authzInformerHandler) handleDeleteRole(obj interface{}) error {
	role, ok := obj.(*authzv1alpha1.AuthzRole)
	if !ok {
		h.logger.Warn("Received non-AuthzRole object in OnDelete")
		return nil
	}

	// Remove each action's grouping policy
	for _, action := range role.Spec.Actions {
		removed, err := h.enforcer.RemoveGroupingPolicy(role.Name, action, role.Namespace)
		if err != nil {
			return fmt.Errorf("failed to remove grouping policy: %w", err)
		}
		if !removed {
			h.logger.Debug("Grouping policy did not exist",
				"role", role.Name,
				"action", action)
		}
	}

	h.logger.Debug("role policies removed successfully",
		"role", role.Name,
		"namespace", role.Namespace,
		"actions", role.Spec.Actions)

	return nil
}

func (h *authzInformerHandler) handleDeleteClusterRole(obj interface{}) error {
	clusterRole, ok := obj.(*authzv1alpha1.AuthzClusterRole)
	if !ok {
		h.logger.Warn("Received non-AuthzClusterRole object in OnDelete")
		return nil
	}

	// Remove each action's grouping policy
	for _, action := range clusterRole.Spec.Actions {
		removed, err := h.enforcer.RemoveGroupingPolicy(clusterRole.Name, action, "*")
		if err != nil {
			return fmt.Errorf("failed to remove cluster grouping policy: %w", err)
		}
		if !removed {
			h.logger.Debug("Cluster grouping policy did not exist",
				"role", clusterRole.Name,
				"action", action)
		}
	}

	h.logger.Debug("cluster role policies removed successfully",
		"role", clusterRole.Name,
		"actions", clusterRole.Spec.Actions)

	return nil
}

func (h *authzInformerHandler) handleDeleteBinding(obj interface{}) error {
	binding, ok := obj.(*authzv1alpha1.AuthzRoleBinding)
	if !ok {
		h.logger.Warn("Received non-AuthzRoleBinding object in OnDelete")
		return nil
	}

	subject, err := formatSubject(binding.Spec.Entitlement.Claim, binding.Spec.Entitlement.Value)
	if err != nil {
		return fmt.Errorf("failed to format subject in binding %s: %w", binding.Name, err)
	}
	resourcePath := resourceHierarchyToPath(authzcore.ResourceHierarchy{
		Namespace: binding.Namespace,
		Project:   binding.Spec.TargetPath.Project,
		Component: binding.Spec.TargetPath.Component,
	})
	roleNamespace := binding.Namespace
	if binding.Spec.RoleRef.Kind == "AuthzClusterRole" {
		roleNamespace = "*"
	}
	effect := string(binding.Spec.Effect)
	if effect == "" {
		return fmt.Errorf("effect not specified in binding %s", binding.Name)
	}

	removed, err := h.enforcer.RemovePolicy(
		subject,
		resourcePath,
		binding.Spec.RoleRef.Name,
		roleNamespace,
		effect,
		emptyContextJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to remove binding policy: %w", err)
	}
	if !removed {
		h.logger.Debug("Policy did not exist", "binding", binding.Name)
	}

	h.logger.Debug("binding policy removed successfully",
		"binding", binding.Name,
		"namespace", binding.Namespace)

	return nil
}

func (h *authzInformerHandler) handleDeleteClusterBinding(obj interface{}) error {
	binding, ok := obj.(*authzv1alpha1.AuthzClusterRoleBinding)
	if !ok {
		h.logger.Warn("Received non-AuthzClusterRoleBinding object in OnDelete")
		return nil
	}

	subject, err := formatSubject(binding.Spec.Entitlement.Claim, binding.Spec.Entitlement.Value)
	if err != nil {
		return fmt.Errorf("failed to format subject in cluster binding %s: %w", binding.Name, err)
	}
	effect := string(binding.Spec.Effect)
	if effect == "" {
		return fmt.Errorf("effect not specified in cluster binding %s", binding.Name)
	}

	removed, err := h.enforcer.RemovePolicy(
		subject,
		"*",
		binding.Spec.RoleRef.Name,
		"*",
		effect,
		emptyContextJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to remove cluster binding policy: %w", err)
	}
	if !removed {
		h.logger.Debug("Cluster policy did not exist", "binding", binding.Name)
	}

	h.logger.Debug("cluster binding policy removed successfully",
		"binding", binding.Name)

	return nil
}
