// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// GetActionDefinitions returns all audit action definitions for openchoreo-api
// Only state-modifying operations (POST, PUT, PATCH, DELETE) are audited
func GetActionDefinitions() []audit.ActionDefinition {
	return []audit.ActionDefinition{
		// Project operations
		{
			Method:   "POST",
			Pattern:  "/api/v1/orgs/{orgName}/projects",
			Action:   "create_project",
			Category: audit.CategoryResource,
		},

		// Component operations
		{
			Method:   "POST",
			Pattern:  "/api/v1/orgs/{orgName}/projects/{projectName}/components",
			Action:   "create_component",
			Category: audit.CategoryResource,
		},
		{
			Method:   "PATCH",
			Pattern:  "/api/v1/orgs/{orgName}/projects/{projectName}/components/{componentName}",
			Action:   "update_component",
			Category: audit.CategoryResource,
		},

		// DataPlane operations
		{
			Method:   "POST",
			Pattern:  "/api/v1/orgs/{orgName}/dataplanes",
			Action:   "create_dataplane",
			Category: audit.CategoryResource,
		},

		// Environment operations
		{
			Method:   "POST",
			Pattern:  "/api/v1/orgs/{orgName}/environments",
			Action:   "create_environment",
			Category: audit.CategoryResource,
		},

		// Apply/Delete operations (kubectl-like)
		{
			Method:   "POST",
			Pattern:  "/api/v1/apply",
			Action:   "apply_resource",
			Category: audit.CategoryResource,
		},
		{
			Method:   "DELETE",
			Pattern:  "/api/v1/delete",
			Action:   "delete_resource",
			Category: audit.CategoryResource,
		},

		// Component Release operations
		{
			Method:   "POST",
			Pattern:  "/api/v1/orgs/{orgName}/projects/{projectName}/components/{componentName}/component-releases",
			Action:   "create_component_release",
			Category: audit.CategoryResource,
		},

		// Deployment operations
		{
			Method:   "POST",
			Pattern:  "/api/v1/orgs/{orgName}/projects/{projectName}/components/{componentName}/deploy",
			Action:   "deploy_component",
			Category: audit.CategoryResource,
		},
		{
			Method:   "POST",
			Pattern:  "/api/v1/orgs/{orgName}/projects/{projectName}/components/{componentName}/promote",
			Action:   "promote_component",
			Category: audit.CategoryResource,
		},

		// Trait operations
		{
			Method:   "PUT",
			Pattern:  "/api/v1/orgs/{orgName}/projects/{projectName}/components/{componentName}/traits",
			Action:   "update_component_traits",
			Category: audit.CategoryResource,
		},

		// Component Binding operations
		{
			Method:   "PATCH",
			Pattern:  "/api/v1/orgs/{orgName}/projects/{projectName}/components/{componentName}/bindings/{bindingName}",
			Action:   "update_component_binding",
			Category: audit.CategoryResource,
		},

		// Workflow operations
		{
			Method:   "PATCH",
			Pattern:  "/api/v1/orgs/{orgName}/projects/{projectName}/components/{componentName}/workflow-parameters",
			Action:   "update_workflow_parameters",
			Category: audit.CategoryResource,
		},
		{
			Method:   "POST",
			Pattern:  "/api/v1/orgs/{orgName}/projects/{projectName}/components/{componentName}/workflow-runs",
			Action:   "create_workflow_run",
			Category: audit.CategoryResource,
		},

		// Workload operations
		{
			Method:   "POST",
			Pattern:  "/api/v1/orgs/{orgName}/projects/{projectName}/components/{componentName}/workloads",
			Action:   "create_workload",
			Category: audit.CategoryResource,
		},

		// Authorization role operations
		{
			Method:   "POST",
			Pattern:  "/api/v1/authz/roles",
			Action:   "create_authz_role",
			Category: audit.CategoryAuth,
		},
		{
			Method:   "PUT",
			Pattern:  "/api/v1/authz/roles/{roleName}",
			Action:   "update_authz_role",
			Category: audit.CategoryAuth,
		},
		{
			Method:   "DELETE",
			Pattern:  "/api/v1/authz/roles/{roleName}",
			Action:   "delete_authz_role",
			Category: audit.CategoryAuth,
		},

		// Authorization role mapping operations
		{
			Method:   "POST",
			Pattern:  "/api/v1/authz/role-mappings",
			Action:   "create_authz_role_mapping",
			Category: audit.CategoryAuth,
		},
		{
			Method:   "PUT",
			Pattern:  "/api/v1/authz/role-mappings/{mappingId}",
			Action:   "update_authz_role_mapping",
			Category: audit.CategoryAuth,
		},
		{
			Method:   "DELETE",
			Pattern:  "/api/v1/authz/role-mappings/{mappingId}",
			Action:   "delete_authz_role_mapping",
			Category: audit.CategoryAuth,
		},
	}
}
