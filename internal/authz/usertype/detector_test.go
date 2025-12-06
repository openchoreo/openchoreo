// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package usertype

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

func createTestJWT(claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("test-secret"))
	return tokenString
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		configs []UserTypeConfig
		wantErr bool
	}{
		{
			name: "valid configuration",
			configs: []UserTypeConfig{
				{
					Type:        authzcore.SubjectTypeUser,
					DisplayName: "User",
					Priority:    1,
					Entitlement: EntitlementConfig{
						Claim:       "group",
						DisplayName: "User Group",
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "empty configuration",
			configs: []UserTypeConfig{},
			wantErr: true,
		},
		{
			name: "duplicate type",
			configs: []UserTypeConfig{
				{
					Type:        authzcore.SubjectTypeUser,
					DisplayName: "User1",
					Priority:    1,
					Entitlement: EntitlementConfig{Claim: "group", DisplayName: "Group"},
				},
				{
					Type:        authzcore.SubjectTypeUser,
					DisplayName: "User2",
					Priority:    2,
					Entitlement: EntitlementConfig{Claim: "group2", DisplayName: "Group2"},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate priority",
			configs: []UserTypeConfig{
				{
					Type:        authzcore.SubjectTypeUser,
					DisplayName: "User",
					Priority:    1,
					Entitlement: EntitlementConfig{Claim: "group", DisplayName: "Group"},
				},
				{
					Type:        authzcore.SubjectTypeServiceAccount,
					DisplayName: "Service Account",
					Priority:    1,
					Entitlement: EntitlementConfig{Claim: "sa", DisplayName: "SA"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty entitlement claim",
			configs: []UserTypeConfig{
				{
					Type:        authzcore.SubjectTypeUser,
					DisplayName: "User",
					Priority:    1,
					Entitlement: EntitlementConfig{Claim: "", DisplayName: "Group"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.configs)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDetectorUserTypeDetection(t *testing.T) {
	userTypes := []UserTypeConfig{
		{
			Type:        authzcore.SubjectTypeUser,
			DisplayName: "Human User",
			Priority:    1,
			Entitlement: EntitlementConfig{
				Claim:       "group",
				DisplayName: "User Group",
			},
		},
		{
			Type:        authzcore.SubjectTypeServiceAccount,
			DisplayName: "Service Account",
			Priority:    2,
			Entitlement: EntitlementConfig{
				Claim:       "service_account",
				DisplayName: "Service Account ID",
			},
		},
	}

	detector, err := NewDetector(userTypes)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}

	tests := []struct {
		name             string
		claims           jwt.MapClaims
		expectedType     authzcore.SubjectType
		expectedClaim    string
		expectedValues   []string
		wantErr          bool
	}{
		{
			name: "user with single group",
			claims: jwt.MapClaims{
				"group": "admin",
			},
			expectedType:   authzcore.SubjectTypeUser,
			expectedClaim:  "group",
			expectedValues: []string{"admin"},
			wantErr:        false,
		},
		{
			name: "user with multiple groups",
			claims: jwt.MapClaims{
				"group": []interface{}{"admin", "developer"},
			},
			expectedType:   authzcore.SubjectTypeUser,
			expectedClaim:  "group",
			expectedValues: []string{"admin", "developer"},
			wantErr:        false,
		},
		{
			name: "service account",
			claims: jwt.MapClaims{
				"service_account": "api-service",
			},
			expectedType:   authzcore.SubjectTypeServiceAccount,
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
			expectedType:   authzcore.SubjectTypeUser,
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
			expectedType:   authzcore.SubjectTypeUser,
			expectedClaim:  "group",
			expectedValues: []string{},
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := createTestJWT(tt.claims)
			result, err := detector.DetectUserType(token)

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

func TestSortByPriority(t *testing.T) {
	userTypes := []UserTypeConfig{
		{Type: authzcore.SubjectTypeServiceAccount, Priority: 3},
		{Type: authzcore.SubjectTypeUser, Priority: 1},
		{Type: "custom", Priority: 2},
	}

	SortByPriority(userTypes)

	if userTypes[0].Priority != 1 || userTypes[1].Priority != 2 || userTypes[2].Priority != 3 {
		t.Errorf("SortByPriority() did not sort correctly, got priorities: %d, %d, %d",
			userTypes[0].Priority, userTypes[1].Priority, userTypes[2].Priority)
	}
}
