// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package data

import (
	_ "embed"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

//go:embed default_authz_data.yaml
var defaultAuthzDataYAML []byte

// DefaultRolesAndMappingsData represents the structure of the default roles and mappings data file
type DefaultRolesAndMappingsData struct {
	Roles    []authzcore.Role                   `yaml:"roles"`
	Mappings []authzcore.RoleEntitlementMapping `yaml:"mappings"`
}

// ActionData represents an action name and whether it is visibility
type ActionData struct {
	Name       string
	IsInternal bool
}

// systemActions defines all available actions in the system
// IsInternal indicates if the action is internal (not publicly visible)
var systemActions = []ActionData{
	// Organization
	{Name: "organization:view", IsInternal: false},

	// Project
	{Name: "project:view", IsInternal: false},
	{Name: "project:create", IsInternal: false},

	// Component
	{Name: "component:view", IsInternal: false},
	{Name: "component:create", IsInternal: false},
	{Name: "component:update", IsInternal: false},
	{Name: "component:deploy", IsInternal: false},

	// ComponentRelease
	{Name: "componentrelease:view", IsInternal: false},
	{Name: "componentrelease:create", IsInternal: false},

	// ReleaseBinding
	{Name: "releasebinding:view", IsInternal: false},
	{Name: "releasebinding:update", IsInternal: false},

	// ComponentType
	{Name: "componenttype:view", IsInternal: false},

	// ComponentWorkflow
	{Name: "componentworkflow:view", IsInternal: false},
	{Name: "componentworkflow:create", IsInternal: false},

	// ComponentWorkflowRun
	{Name: "componentworkflowrun:view", IsInternal: false},

	// Workflow
	{Name: "workflow:view", IsInternal: false},

	// Trait
	{Name: "trait:view", IsInternal: false},

	// Environment
	{Name: "environment:view", IsInternal: false},
	{Name: "environment:create", IsInternal: false},

	// DataPlane
	{Name: "dataplane:view", IsInternal: false},
	{Name: "dataplane:create", IsInternal: false},

	// BuildPlane
	{Name: "buildplane:view", IsInternal: false},

	// DeploymentPipeline
	{Name: "deploymentpipeline:view", IsInternal: false},

	// SecretReference
	{Name: "secretreference:view", IsInternal: false},

	// Workload
	{Name: "workload:view", IsInternal: false},
	{Name: "workload:create", IsInternal: false},

	// roles
	{Name: "role:view", IsInternal: false},
	{Name: "role:create", IsInternal: false},
	{Name: "role:delete", IsInternal: false},
	{Name: "role:update", IsInternal: false},

	// role mapping
	{Name: "rolemapping:view", IsInternal: false},
	{Name: "rolemapping:create", IsInternal: false},
	{Name: "rolemapping:delete", IsInternal: false},
	{Name: "rolemapping:update", IsInternal: false},

	// logs
	{Name: "logs:view", IsInternal: false},

	// metrics
	{Name: "metrics:view", IsInternal: false},

	// traces
	{Name: "traces:view", IsInternal: false},

	// alerts
	{Name: "alerts:view", IsInternal: false},
}

// LoadActions returns the system-defined actions
func LoadActions() ([]ActionData, error) {
	return systemActions, nil
}

// LoadDefaultAuthzDataFromFile loads both roles and mappings from a file with the following priority:
// 1. If filePath is provided, load from file
// 2. else, fall back to embedded default data
func LoadDefaultAuthzDataFromFile(filePath string) (*DefaultRolesAndMappingsData, error) {
	if filePath == "" {
		return LoadEmbeddedAuthzData()
	}
	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("failed to access authz data file %s: %w", filePath, err)
	}
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read authz data file %s: %w", filePath, err)
	}

	var data DefaultRolesAndMappingsData
	if err := yaml.Unmarshal(fileData, &data); err != nil {
		return nil, fmt.Errorf("failed to parse authz data from %s: %w", filePath, err)
	}

	if err := validateRoles(data.Roles); err != nil {
		return nil, fmt.Errorf("invalid roles data in %s: %w", filePath, err)
	}

	if err := validateMappings(data.Mappings); err != nil {
		return nil, fmt.Errorf("invalid mappings data in %s: %w", filePath, err)
	}

	return &data, nil
}

// LoadEmbeddedAuthzData loads the embedded default roles and mappings
func LoadEmbeddedAuthzData() (*DefaultRolesAndMappingsData, error) {
	var data DefaultRolesAndMappingsData
	if err := yaml.Unmarshal(defaultAuthzDataYAML, &data); err != nil {
		return nil, fmt.Errorf("failed to parse embedded authz data: %w", err)
	}

	if err := validateRoles(data.Roles); err != nil {
		return nil, fmt.Errorf("invalid embedded roles data: %w", err)
	}

	if err := validateMappings(data.Mappings); err != nil {
		return nil, fmt.Errorf("invalid embedded mappings data: %w", err)
	}

	return &data, nil
}

// validateRoles ensures the roles data is valid
func validateRoles(roles []authzcore.Role) error {
	if len(roles) == 0 {
		return fmt.Errorf("roles list cannot be empty")
	}

	// Validate each role has a name and actions
	for i, role := range roles {
		if role.Name == "" {
			return fmt.Errorf("role at index %d has empty name", i)
		}
		if len(role.Actions) == 0 {
			return fmt.Errorf("role %q has no actions", role.Name)
		}
	}

	return nil
}

// validateMappings ensures the mappings data is valid
func validateMappings(mappings []authzcore.RoleEntitlementMapping) error {
	for i, mapping := range mappings {
		if mapping.RoleName == "" {
			return fmt.Errorf("mapping at index %d has empty role_name", i)
		}
		if mapping.Entitlement.Claim == "" {
			return fmt.Errorf("mapping at index %d has empty entitlement claim", i)
		}
		if mapping.Entitlement.Value == "" {
			return fmt.Errorf("mapping at index %d has empty entitlement value", i)
		}
		if mapping.Effect != authzcore.PolicyEffectAllow && mapping.Effect != authzcore.PolicyEffectDeny {
			return fmt.Errorf("mapping at index %d has invalid effect %q (must be 'allow' or 'deny')", i, mapping.Effect)
		}
	}

	return nil
}
