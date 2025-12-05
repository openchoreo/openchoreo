// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package usertype

import (
	"fmt"
	"sort"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// UserTypeConfig represents configuration for a single user type
type UserTypeConfig struct {
	Type        authzcore.SubjectType `yaml:"type"`         // "user" or "service_account"
	DisplayName string                 `yaml:"display_name"` // Human-readable name for user type
	Priority    int                    `yaml:"priority"`     // Check order (lower = higher priority)
	Entitlement EntitlementConfig      `yaml:"entitlement"`  // Entitlement configuration
}

// EntitlementConfig defines how to extract entitlement claims from JWT tokens
type EntitlementConfig struct {
	Claim       string `yaml:"claim"`        // JWT claim for detection and entitlement
	DisplayName string `yaml:"display_name"` // Human-readable name for the claim
}

// ValidateConfig validates an array of user type configurations
func ValidateConfig(userTypes []UserTypeConfig) error {
	if len(userTypes) == 0 {
		return fmt.Errorf("at least one user type must be configured")
	}

	typesSeen := make(map[authzcore.SubjectType]bool)
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

		// Check for duplicate priorities
		if prioritiesSeen[ut.Priority] {
			return fmt.Errorf("duplicate priority %d for user type %s", ut.Priority, ut.Type)
		}
		prioritiesSeen[ut.Priority] = true

		// Validate entitlement claim
		if ut.Entitlement.Claim == "" {
			return fmt.Errorf("user type %s has empty entitlement claim", ut.Type)
		}

		// Validate entitlement display name
		if ut.Entitlement.DisplayName == "" {
			return fmt.Errorf("user type %s has empty entitlement display_name", ut.Type)
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
