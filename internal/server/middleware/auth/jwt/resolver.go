// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/subject"
)

// Ensure Detector implements subject.Resolver interface at compile time
var _ subject.Resolver = (*Resolver)(nil)

// Resolver implements subject resolution specifically for JWT tokens
type Resolver struct {
	userTypes []subject.UserTypeConfig
}

// NewResolver creates a new JWT-specific user type resolver.
// It filters the provided user types to only include those with JWT auth mechanism configured.
func NewResolver(userTypes []subject.UserTypeConfig) (*Resolver, error) {
	if len(userTypes) == 0 {
		return nil, fmt.Errorf("user types configuration cannot be empty")
	}

	// Filter to only user types that have JWT auth mechanism configured
	var jwtUserTypes []subject.UserTypeConfig
	for _, ut := range userTypes {
		hasJWT := false
		for _, am := range ut.AuthMechanisms {
			if am.Type == "jwt" {
				hasJWT = true
				break
			}
		}
		if hasJWT {
			jwtUserTypes = append(jwtUserTypes, ut)
		}
	}

	// Validate that at least one user type has JWT configured
	if len(jwtUserTypes) == 0 {
		return nil, fmt.Errorf("no user types have JWT auth mechanism configured")
	}

	// Sort by priority
	subject.SortByPriority(jwtUserTypes)

	return &Resolver{
		userTypes: jwtUserTypes,
	}, nil
}

// ResolveUserType analyzes a JWT token and returns the SubjectContext
func (d *Resolver) ResolveUserType(jwtToken string) (*auth.SubjectContext, error) {
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
		// Find JWT auth mechanism for this user type
		var jwtMechanism *subject.AuthMechanismConfig
		for i := range userTypeConfig.AuthMechanisms {
			if userTypeConfig.AuthMechanisms[i].Type == "jwt" {
				jwtMechanism = &userTypeConfig.AuthMechanisms[i]
				break
			}
		}

		// Skip if no JWT mechanism (should not happen since we filtered in constructor)
		if jwtMechanism == nil {
			continue
		}

		// Check if claims match this user type with the JWT mechanism's entitlement config
		if matches, entitlements := detectUserTypeFromClaims(claims, jwtMechanism.Entitlement); matches {
			return &auth.SubjectContext{
				ID:                fmt.Sprintf("%v", claims["sub"]),
				Type:              userTypeConfig.Type,
				EntitlementClaim:  jwtMechanism.Entitlement.Claim,
				EntitlementValues: entitlements,
			}, nil
		}
	}

	return nil, fmt.Errorf("no valid user type detected from JWT token")
}

// detectUserTypeFromClaims checks if JWT claims match a specific entitlement configuration
func detectUserTypeFromClaims(claims jwt.MapClaims, entitlementConfig subject.EntitlementConfig) (bool, []string) {
	// Check if the entitlement claim exists in the token
	claimValue, exists := claims[entitlementConfig.Claim]
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
