// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/subject"
)

const (
	user           = "user"
	serviceAccount = "service_account"
)

func createTestJWT(claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("test-secret"))
	return tokenString
}

func TestJWTDetectorUserTypeDetection(t *testing.T) {
	userTypes := []subject.UserTypeConfig{
		{
			Type:        user,
			DisplayName: "Human User",
			Priority:    1,
			AuthMechanisms: []subject.AuthMechanismConfig{
				{
					Type: "jwt",
					Entitlement: subject.EntitlementConfig{
						Claim:       "group",
						DisplayName: "User Group",
					},
				},
			},
		},
		{
			Type:        serviceAccount,
			DisplayName: "Service Account",
			Priority:    2,
			AuthMechanisms: []subject.AuthMechanismConfig{
				{
					Type: "jwt",
					Entitlement: subject.EntitlementConfig{
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
		expectedType   string
		expectedClaim  string
		expectedValues []string
		wantErr        bool
	}{
		{
			name: "user with single group",
			claims: jwt.MapClaims{
				"group": "admin",
			},
			expectedType:   "user",
			expectedClaim:  "group",
			expectedValues: []string{"admin"},
			wantErr:        false,
		},
		{
			name: "user with multiple groups",
			claims: jwt.MapClaims{
				"group": []interface{}{"admin", "developer"},
			},
			expectedType:   "user",
			expectedClaim:  "group",
			expectedValues: []string{"admin", "developer"},
			wantErr:        false,
		},
		{
			name: "service account",
			claims: jwt.MapClaims{
				"service_account": "api-service",
			},
			expectedType:   "service_account",
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
			expectedType:   "user",
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
			expectedType:   "user",
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
	// User type without JWT mechanism (using API key instead)
	userTypes := []subject.UserTypeConfig{
		{
			Type:        "user",
			DisplayName: "API Key User",
			Priority:    1,
			AuthMechanisms: []subject.AuthMechanismConfig{
				{
					Type: "api_key",
					Entitlement: subject.EntitlementConfig{
						Claim:       "key_id",
						DisplayName: "API Key ID",
					},
				},
			},
		},
	}

	_, err := NewResolver(userTypes)
	if err == nil {
		t.Error("NewResolver() should fail when no user types have JWT mechanism")
	}
	if err != nil && err.Error() != "no user types have JWT auth mechanism configured" {
		t.Errorf("NewResolver() error = %v, want error about no JWT mechanism", err)
	}
}

func TestJWTDetectorFiltersNonJWTUserTypes(t *testing.T) {
	// Mix of JWT and non-JWT user types
	userTypes := []subject.UserTypeConfig{
		{
			Type:        "user",
			DisplayName: "JWT User",
			Priority:    1,
			AuthMechanisms: []subject.AuthMechanismConfig{
				{
					Type: "jwt",
					Entitlement: subject.EntitlementConfig{
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
			AuthMechanisms: []subject.AuthMechanismConfig{
				{
					Type: "api_key",
					Entitlement: subject.EntitlementConfig{
						Claim:       "key_id",
						DisplayName: "API Key ID",
					},
				},
			},
		},
	}

	detector, err := NewResolver(userTypes)
	if err != nil {
		t.Fatalf("NewResolver() should succeed with at least one JWT user type: %v", err)
	}

	// Should only detect JWT user type
	token := createTestJWT(jwt.MapClaims{"groups": "admin"})
	result, err := detector.ResolveUserType(token)
	if err != nil {
		t.Fatalf("DetectUserType() error = %v", err)
	}

	if result.Type != "user" {
		t.Errorf("Type = %v, want %v", result.Type, "user")
	}
}
