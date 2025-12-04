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

// systemActions defines all available actions in the system
var systemActions = []string{
	// Organization
	"organization:view",

	// Project
	"project:view",
	"project:create",

	// Component
	"component:view",
	"component:create",
	"component:update",
	"component:deploy",
	"component:promote",

	// ComponentRelease
	"componentrelease:view",
	"componentrelease:create",

	// ReleaseBinding
	"releasebinding:view",
	"releasebinding:update",

	// ComponentBinding
	"componentbinding:view",
	"componentbinding:update",

	// ComponentType
	"componenttype:view",

	// ComponentWorkflow
	"componentworkflow:view",
	"componentworkflow:trigger",

	// Workflow
	"workflow:view",

	// Trait
	"trait:view",

	// Environment
	"environment:view",
	"environment:create",

	// DataPlane
	"dataplane:view",
	"dataplane:create",

	// BuildPlane
	"buildplane:view",

	// DeploymentPipeline
	"deploymentpipeline:view",

	// Schema
	"schema:view",

	// SecretReference
	"secretreference:view",

	// Workload
	"workload:view",
	"workload:create",
}

// LoadActions returns the system-defined actions
func LoadActions() ([]string, error) {
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
