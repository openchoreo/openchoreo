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

//go:embed default_roles.yaml
var defaultRolesYAML []byte

// RolesData represents the structure of the roles data file
type RolesData struct {
	Roles []authzcore.Role `yaml:"roles"`
}

// ActionData represents an action name and whether it is visibility
type ActionData struct {
	Name      string
	IsPrivate bool
}

// systemActions defines all available actions in the system
// IsPrivate indicates if the action is private (not publicly visible)
var systemActions = []ActionData{
	// Organization
	{Name: "organization:view", IsPrivate: false},

	// Project
	{Name: "project:view", IsPrivate: false},
	{Name: "project:create", IsPrivate: false},

	// Component
	{Name: "component:view", IsPrivate: false},
	{Name: "component:create", IsPrivate: false},
	{Name: "component:update", IsPrivate: false},
	{Name: "component:deploy", IsPrivate: false},

	// ComponentRelease
	{Name: "componentrelease:view", IsPrivate: false},
	{Name: "componentrelease:create", IsPrivate: false},

	// ReleaseBinding
	{Name: "releasebinding:view", IsPrivate: false},
	{Name: "releasebinding:update", IsPrivate: false},

	// ComponentBinding
	{Name: "componentbinding:view", IsPrivate: false},
	{Name: "componentbinding:update", IsPrivate: false},

	// ComponentType
	{Name: "componenttype:view", IsPrivate: false},

	// ComponentWorkflow
	{Name: "componentworkflow:view", IsPrivate: false},
	{Name: "componentworkflow:trigger", IsPrivate: false},

	// Workflow
	{Name: "workflow:view", IsPrivate: false},

	// Trait
	{Name: "trait:view", IsPrivate: false},

	// Environment
	{Name: "environment:view", IsPrivate: false},
	{Name: "environment:create", IsPrivate: false},

	// DataPlane
	{Name: "dataplane:view", IsPrivate: false},
	{Name: "dataplane:create", IsPrivate: false},

	// BuildPlane
	{Name: "buildplane:view", IsPrivate: false},

	// DeploymentPipeline
	{Name: "deploymentpipeline:view", IsPrivate: false},

	// Schema
	{Name: "schema:view", IsPrivate: false},

	// SecretReference
	{Name: "secretreference:view", IsPrivate: false},

	// Workload
	{Name: "workload:view", IsPrivate: false},
	{Name: "workload:create", IsPrivate: false},
}

// LoadActions returns the system-defined actions
func LoadActions() ([]ActionData, error) {
	return systemActions, nil
}

// LoadRolesFromFile loads roles with the following priority:
// 1. If filePath is provided, load from file
// 2. else, fall back to embedded default roles
func LoadRolesFromFile(filePath string) ([]authzcore.Role, error) {
	if filePath == "" {
		return LoadEmbeddedRoles()
	}
	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("failed to access roles file %s: %w", filePath, err)
	}
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read roles file %s: %w", filePath, err)
	}

	var data RolesData
	if err := yaml.Unmarshal(fileData, &data); err != nil {
		return nil, fmt.Errorf("failed to parse roles data from %s: %w", filePath, err)
	}

	if err := validateRoles(data.Roles); err != nil {
		return nil, fmt.Errorf("invalid roles data in %s: %w", filePath, err)
	}

	return data.Roles, nil
}

// LoadEmbeddedRoles loads the embedded default roles
func LoadEmbeddedRoles() ([]authzcore.Role, error) {
	var data RolesData
	if err := yaml.Unmarshal(defaultRolesYAML, &data); err != nil {
		return nil, fmt.Errorf("failed to parse embedded roles data: %w", err)
	}

	if err := validateRoles(data.Roles); err != nil {
		return nil, fmt.Errorf("invalid embedded roles data: %w", err)
	}

	return data.Roles, nil
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
