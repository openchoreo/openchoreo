// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// Detector is responsible for detecting user types from JWT tokens
type Detector interface {
	// DetectUserType analyzes a JWT token and returns the SubjectContext
	DetectUserType(jwtToken string) (*SubjectContext, error)
}

// ConfigurableDetector implements Detector using configuration
type ConfigurableDetector struct {
	userTypes []UserTypeConfig
}

// NewDetector creates a new user type detector from pre-loaded configuration
func NewDetector(userTypes []UserTypeConfig) (Detector, error) {
	if len(userTypes) == 0 {
		return nil, fmt.Errorf("user types configuration cannot be empty")
	}

	// Make a copy and sort by priority
	sortedTypes := make([]UserTypeConfig, len(userTypes))
	copy(sortedTypes, userTypes)
	SortByPriority(sortedTypes)

	return &ConfigurableDetector{
		userTypes: sortedTypes,
	}, nil
}

// DetectUserType implements the Detector interface
func (d *ConfigurableDetector) DetectUserType(jwtToken string) (*SubjectContext, error) {
	// Parse JWT without verification (just to extract claims)
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(jwtToken, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("failed to parse JWT claims")
	}

	// Try each user type in priority order
	for _, userTypeConfig := range d.userTypes {
		if matches, entitlements := d.detectUserTypeFromClaims(claims, userTypeConfig); matches {
			return &SubjectContext{
				Type:              userTypeConfig.Type,
				EntitlementClaim:  userTypeConfig.Entitlement.Claim,
				EntitlementValues: entitlements,
			}, nil
		}
	}

	return nil, fmt.Errorf("no valid user type detected from token")
}

// detectUserTypeFromClaims checks if claims match a specific user type configuration
func (d *ConfigurableDetector) detectUserTypeFromClaims(claims jwt.MapClaims, config UserTypeConfig) (bool, []string) {
	// Check if the entitlement claim exists in the token
	claimValue, exists := claims[config.Entitlement.Claim]
	if !exists {
		return false, nil
	}

	// Extract entitlement values from the claim
	entitlements := extractEntitlements(claimValue)

	// Only consider it a match if we have entitlement values
	// Empty claim indicates presence but no actual values
	if len(entitlements) == 0 {
		// Special case: if claim exists but is empty string, still consider it a match
		// but return empty entitlements (maintains backward compatibility)
		if str, ok := claimValue.(string); ok && str == "" {
			return true, []string{}
		}
		return false, nil
	}

	return true, entitlements
}

// extractEntitlements extracts string array from a claim (handles string or []interface{})
func extractEntitlements(claimValue interface{}) []string {
	var entitlements []string

	switch v := claimValue.(type) {
	case string:
		if v != "" {
			entitlements = append(entitlements, v)
		}
	case []interface{}:
		for _, item := range v {
			if str, ok := item.(string); ok && str != "" {
				entitlements = append(entitlements, str)
			}
		}
	}

	return entitlements
}
