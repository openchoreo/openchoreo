// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package subject

import (
	"fmt"
	"sort"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// UserTypeConfig represents configuration for a single user type
type UserTypeConfig struct {
	Type           auth.SubjectType      `yaml:"type"`            // "user" or "service_account"
	DisplayName    string                `yaml:"display_name"`    // Human-readable name for user type
	Priority       int                   `yaml:"priority"`        // Check order (lower = higher priority)
	AuthMechanisms []AuthMechanismConfig `yaml:"auth_mechanisms"` // Supported authentication mechanisms
}

// AuthMechanismConfig represents configuration for a specific authentication mechanism
type AuthMechanismConfig struct {
	Type        string            `yaml:"type"`        // Authentication mechanism type (e.g., "jwt", "oauth2", "api_key")
	Entitlement EntitlementConfig `yaml:"entitlement"` // Entitlement configuration for this mechanism
}

// EntitlementConfig defines how to extract entitlement claims from authentication tokens
type EntitlementConfig struct {
	Claim       string `yaml:"claim"`        // Claim name for detection and entitlement (e.g., "groups", "scopes")
	DisplayName string `yaml:"display_name"` // Human-readable name for the claim
}

// ValidateConfig validates an array of user type configurations
func ValidateConfig(userTypes []UserTypeConfig) error {
	if len(userTypes) == 0 {
		return fmt.Errorf("at least one user type must be configured")
	}

	typesSeen := make(map[auth.SubjectType]bool)
	prioritiesSeen := make(map[int]bool)

	for i, ut := range userTypes {
		// Validate type
		if ut.Type == "" {
			return fmt.Errorf("user type at index %d has empty type", i)
		}

		// Check for duplicate types
		if typesSeen[ut.Type] {
			return fmt.Errorf("duplicate user type: %s", ut.Type)
		}
		typesSeen[ut.Type] = true

		// Validate display name
		if ut.DisplayName == "" {
			return fmt.Errorf("user type %s has empty display_name", ut.Type)
		}

		// Validate auth mechanisms
		if len(ut.AuthMechanisms) == 0 {
			return fmt.Errorf("user type %s has no auth_mechanisms configured", ut.Type)
		}

		// Check for duplicate priorities
		if prioritiesSeen[ut.Priority] {
			return fmt.Errorf("duplicate priority %d for user type %s", ut.Priority, ut.Type)
		}
		prioritiesSeen[ut.Priority] = true

		// Validate each auth mechanism
		authMechanismTypes := make(map[string]bool)
		for j, am := range ut.AuthMechanisms {
			// Validate mechanism type
			if am.Type == "" {
				return fmt.Errorf("user type %s has empty auth mechanism type at index %d", ut.Type, j)
			}

			// Check for duplicate auth mechanism types
			if authMechanismTypes[am.Type] {
				return fmt.Errorf("user type %s has duplicate auth mechanism type: %s", ut.Type, am.Type)
			}
			authMechanismTypes[am.Type] = true

			// Validate entitlement claim
			if am.Entitlement.Claim == "" {
				return fmt.Errorf("user type %s auth mechanism %s has empty entitlement claim", ut.Type, am.Type)
			}

			// Validate entitlement display name
			if am.Entitlement.DisplayName == "" {
				return fmt.Errorf("user type %s auth mechanism %s has empty entitlement display_name", ut.Type, am.Type)
			}
		}
	}

	return nil
}

// SortByPriority sorts user type configurations by priority (lower = higher priority)
func SortByPriority(userTypes []UserTypeConfig) {
	sort.Slice(userTypes, func(i, j int) bool {
		return userTypes[i].Priority < userTypes[j].Priority
	})
}
