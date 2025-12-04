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

//go:embed actions.yaml
var actionsYAML []byte

//go:embed default_roles.yaml
var defaultRolesYAML []byte

// ActionsData represents the structure of the actions data file
type ActionsData struct {
	Actions []string `yaml:"actions"`
}

// RolesData represents the structure of the roles data file
type RolesData struct {
	Roles []authzcore.Role `yaml:"roles"`
}

// LoadActions loads the embedded actions data
func LoadActions() ([]string, error) {
	var data ActionsData
	if err := yaml.Unmarshal(actionsYAML, &data); err != nil {
		return nil, fmt.Errorf("failed to parse actions data: %w", err)
	}

	if len(data.Actions) == 0 {
		return nil, fmt.Errorf("actions list cannot be empty")
	}

	return data.Actions, nil
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
