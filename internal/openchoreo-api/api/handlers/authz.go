// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ListRoles returns all defined authorization roles
func (h *Handler) ListRoles(
	ctx context.Context,
	request gen.ListRolesRequestObject,
) (gen.ListRolesResponseObject, error) {
	return nil, errNotImplemented
}

// AddRole creates a new authorization role
func (h *Handler) AddRole(
	ctx context.Context,
	request gen.AddRoleRequestObject,
) (gen.AddRoleResponseObject, error) {
	return nil, errNotImplemented
}

// GetRole returns details of a specific role
func (h *Handler) GetRole(
	ctx context.Context,
	request gen.GetRoleRequestObject,
) (gen.GetRoleResponseObject, error) {
	return nil, errNotImplemented
}

// UpdateRole updates an existing role
func (h *Handler) UpdateRole(
	ctx context.Context,
	request gen.UpdateRoleRequestObject,
) (gen.UpdateRoleResponseObject, error) {
	return nil, errNotImplemented
}

// RemoveRole deletes a role
func (h *Handler) RemoveRole(
	ctx context.Context,
	request gen.RemoveRoleRequestObject,
) (gen.RemoveRoleResponseObject, error) {
	return nil, errNotImplemented
}

// ListRoleMappings returns role-entitlement mappings
func (h *Handler) ListRoleMappings(
	ctx context.Context,
	request gen.ListRoleMappingsRequestObject,
) (gen.ListRoleMappingsResponseObject, error) {
	return nil, errNotImplemented
}

// AddRoleMapping creates a new role-entitlement mapping
func (h *Handler) AddRoleMapping(
	ctx context.Context,
	request gen.AddRoleMappingRequestObject,
) (gen.AddRoleMappingResponseObject, error) {
	return nil, errNotImplemented
}

// UpdateRoleMapping updates an existing role-entitlement mapping
func (h *Handler) UpdateRoleMapping(
	ctx context.Context,
	request gen.UpdateRoleMappingRequestObject,
) (gen.UpdateRoleMappingResponseObject, error) {
	return nil, errNotImplemented
}

// RemoveRoleMapping deletes a role-entitlement mapping
func (h *Handler) RemoveRoleMapping(
	ctx context.Context,
	request gen.RemoveRoleMappingRequestObject,
) (gen.RemoveRoleMappingResponseObject, error) {
	return nil, errNotImplemented
}

// ListActions returns all defined authorization actions
func (h *Handler) ListActions(
	ctx context.Context,
	request gen.ListActionsRequestObject,
) (gen.ListActionsResponseObject, error) {
	return nil, errNotImplemented
}

// Evaluate evaluates a single authorization request
func (h *Handler) Evaluate(
	ctx context.Context,
	request gen.EvaluateRequestObject,
) (gen.EvaluateResponseObject, error) {
	return nil, errNotImplemented
}

// BatchEvaluate evaluates multiple authorization requests
func (h *Handler) BatchEvaluate(
	ctx context.Context,
	request gen.BatchEvaluateRequestObject,
) (gen.BatchEvaluateResponseObject, error) {
	return nil, errNotImplemented
}

// GetSubjectProfile returns the authorization profile for the authenticated subject
func (h *Handler) GetSubjectProfile(
	ctx context.Context,
	request gen.GetSubjectProfileRequestObject,
) (gen.GetSubjectProfileResponseObject, error) {
	return nil, errNotImplemented
}

// ListUserTypes returns the configured user types
func (h *Handler) ListUserTypes(
	ctx context.Context,
	request gen.ListUserTypesRequestObject,
) (gen.ListUserTypesResponseObject, error) {
	return nil, errNotImplemented
}
