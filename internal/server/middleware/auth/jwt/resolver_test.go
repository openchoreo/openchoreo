// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/subject_resolver"
)

func createTestJWT(claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("test-secret"))
	return tokenString
}

func TestJWTDetectorUserTypeDetection(t *testing.T) {
	userTypes := []subject_resolver.UserTypeConfig{
		{
			Type:        auth.SubjectTypeUser,
			DisplayName: "Human User",
			Priority:    1,
			AuthMechanisms: []subject_resolver.AuthMechanismConfig{
				{
					Type: "jwt",
					Entitlement: subject_resolver.EntitlementConfig{
						Claim:       "group",
						DisplayName: "User Group",
					},
				},
			},
		},
		{
			Type:        auth.SubjectTypeServiceAccount,
			DisplayName: "Service Account",
			Priority:    2,
			AuthMechanisms: []subject_resolver.AuthMechanismConfig{
				{
					Type: "jwt",
					Entitlement: subject_resolver.EntitlementConfig{
						Claim:       "service_account",
						DisplayName: "Service Account ID",
					},
				},
			},
		},
	}

	detector, err := NewResolver(userTypes)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}

	tests := []struct {
		name           string
		claims         jwt.MapClaims
		expectedType   auth.SubjectType
		expectedClaim  string
		expectedValues []string
		wantErr        bool
	}{
		{
			name: "user with single group",
			claims: jwt.MapClaims{
				"group": "admin",
			},
			expectedType:   auth.SubjectTypeUser,
			expectedClaim:  "group",
			expectedValues: []string{"admin"},
			wantErr:        false,
		},
		{
			name: "user with multiple groups",
			claims: jwt.MapClaims{
				"group": []interface{}{"admin", "developer"},
			},
			expectedType:   auth.SubjectTypeUser,
			expectedClaim:  "group",
			expectedValues: []string{"admin", "developer"},
			wantErr:        false,
		},
		{
			name: "service account",
			claims: jwt.MapClaims{
				"service_account": "api-service",
			},
			expectedType:   auth.SubjectTypeServiceAccount,
			expectedClaim:  "service_account",
			expectedValues: []string{"api-service"},
			wantErr:        false,
		},
		{
			name: "priority - user takes precedence over service account",
			claims: jwt.MapClaims{
				"group":           "admin",
				"service_account": "api-service",
			},
			expectedType:   auth.SubjectTypeUser,
			expectedClaim:  "group",
			expectedValues: []string{"admin"},
			wantErr:        false,
		},
		{
			name: "no matching claims",
			claims: jwt.MapClaims{
				"email": "user@example.com",
			},
			wantErr: true,
		},
		{
			name: "empty group value",
			claims: jwt.MapClaims{
				"group": "",
			},
			expectedType:   auth.SubjectTypeUser,
			expectedClaim:  "group",
			expectedValues: []string{},
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := createTestJWT(tt.claims)
			result, err := detector.ResolveUserType(token)

			if (err != nil) != tt.wantErr {
				t.Errorf("DetectUserType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if result.Type != tt.expectedType {
					t.Errorf("Type = %v, want %v", result.Type, tt.expectedType)
				}
				if result.EntitlementClaim != tt.expectedClaim {
					t.Errorf("EntitlementClaim = %v, want %v", result.EntitlementClaim, tt.expectedClaim)
				}
				if len(result.EntitlementValues) != len(tt.expectedValues) {
					t.Errorf("EntitlementValues length = %v, want %v", len(result.EntitlementValues), len(tt.expectedValues))
				} else {
					for i, v := range result.EntitlementValues {
						if v != tt.expectedValues[i] {
							t.Errorf("EntitlementValues[%d] = %v, want %v", i, v, tt.expectedValues[i])
						}
					}
				}
			}
		})
	}
}

func TestJWTDetectorWithoutJWTMechanism(t *testing.T) {
	// User type without JWT mechanism
	userTypes := []subject_resolver.UserTypeConfig{
		{
			Type:        auth.SubjectTypeUser,
			DisplayName: "OAuth User",
			Priority:    1,
			AuthMechanisms: []subject_resolver.AuthMechanismConfig{
				{
					Type: "oauth2",
					Entitlement: subject_resolver.EntitlementConfig{
						Claim:       "scopes",
						DisplayName: "OAuth Scopes",
					},
				},
			},
		},
	}

	_, err := NewResolver(userTypes)
	if err == nil {
		t.Error("NewDetector() should fail when no user types have JWT mechanism")
	}
	if err != nil && err.Error() != "no user types have JWT auth mechanism configured" {
		t.Errorf("NewDetector() error = %v, want error about no JWT mechanism", err)
	}
}

func TestJWTDetectorFiltersNonJWTUserTypes(t *testing.T) {
	// Mix of JWT and non-JWT user types
	userTypes := []subject_resolver.UserTypeConfig{
		{
			Type:        auth.SubjectTypeUser,
			DisplayName: "JWT User",
			Priority:    1,
			AuthMechanisms: []subject_resolver.AuthMechanismConfig{
				{
					Type: "jwt",
					Entitlement: subject_resolver.EntitlementConfig{
						Claim:       "groups",
						DisplayName: "User Groups",
					},
				},
			},
		},
		{
			Type:        "api_client",
			DisplayName: "API Client",
			Priority:    2,
			AuthMechanisms: []subject_resolver.AuthMechanismConfig{
				{
					Type: "api_key",
					Entitlement: subject_resolver.EntitlementConfig{
						Claim:       "key_id",
						DisplayName: "API Key ID",
					},
				},
			},
		},
	}

	detector, err := NewResolver(userTypes)
	if err != nil {
		t.Fatalf("NewDetector() should succeed with at least one JWT user type: %v", err)
	}

	// Should only detect JWT user type
	token := createTestJWT(jwt.MapClaims{"groups": "admin"})
	result, err := detector.ResolveUserType(token)
	if err != nil {
		t.Fatalf("DetectUserType() error = %v", err)
	}

	if result.Type != auth.SubjectTypeUser {
		t.Errorf("Type = %v, want %v", result.Type, auth.SubjectTypeUser)
	}
}
