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

// TestJWTDetectorResolvesIssuerAndSession covers the iss and sid claims the
// audit record publishes. sid is optional in OIDC, so a token without one must
// resolve to an empty SessionID rather than failing.
func TestJWTDetectorResolvesIssuerAndSession(t *testing.T) {
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
	}

	detector, err := NewResolver(userTypes)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}

	const issuer, sessionID = "https://idp.example.com/oauth2/token", "b3f1c2d4"

	withSession, err := detector.ResolveUserType(createTestJWT(jwt.MapClaims{
		"group": "admin", "sub": "user-1", "iss": issuer, "sid": sessionID,
	}))
	if err != nil {
		t.Fatalf("ResolveUserType() error = %v", err)
	}
	if withSession.Issuer != issuer {
		t.Errorf("Issuer = %q, want %q", withSession.Issuer, issuer)
	}
	if withSession.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", withSession.SessionID, sessionID)
	}

	withoutSession, err := detector.ResolveUserType(createTestJWT(jwt.MapClaims{
		"group": "admin", "sub": "user-1", "iss": issuer,
	}))
	if err != nil {
		t.Fatalf("ResolveUserType() on a token with no sid claim: error = %v", err)
	}
	if withoutSession.SessionID != "" {
		t.Errorf("SessionID = %q, want empty for a token without a sid claim", withoutSession.SessionID)
	}
}

// TestJWTDetectorResolvesReadableID covers two subject types being identified
// by claims their own tokens actually carry, since the claim is read from
// whichever mechanism matched.
func TestJWTDetectorResolvesReadableID(t *testing.T) {
	mechanism := func(readableIDClaim, entitlementClaim string) []subject.AuthMechanismConfig {
		return []subject.AuthMechanismConfig{
			{
				Type:            "jwt",
				ReadableIDClaim: readableIDClaim,
				Entitlement: subject.EntitlementConfig{
					Claim:       entitlementClaim,
					DisplayName: "Entitlement",
				},
			},
		}
	}

	userTypes := []subject.UserTypeConfig{
		{Type: user, DisplayName: "Human User", Priority: 1, AuthMechanisms: mechanism("username", "groups")},
		{Type: "service_account", DisplayName: "Service Account", Priority: 2, AuthMechanisms: mechanism("client_id", "client_id")},
	}

	detector, err := NewResolver(userTypes)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}

	tests := []struct {
		name           string
		claims         jwt.MapClaims
		wantType       string
		wantReadableID string
	}{
		{
			name:           "user is identified by its own username claim",
			claims:         jwt.MapClaims{"groups": "admin", "sub": "user-1", "username": "alice@example.com"},
			wantType:       user,
			wantReadableID: "alice@example.com",
		},
		{
			// A client_credentials token carries no username, so the user
			// type's claim would leave it unidentified.
			name:           "service account is identified by its client_id claim",
			claims:         jwt.MapClaims{"client_id": "system-app", "sub": "svc-1"},
			wantType:       "service_account",
			wantReadableID: "system-app",
		},
		{
			name:           "absent claim leaves ReadableID empty for the audit fallback",
			claims:         jwt.MapClaims{"groups": "admin", "sub": "user-1"},
			wantType:       user,
			wantReadableID: "",
		},
		{
			name:           "non-string claim leaves ReadableID empty",
			claims:         jwt.MapClaims{"groups": "admin", "sub": "user-1", "username": []any{"alice"}},
			wantType:       user,
			wantReadableID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detector.ResolveUserType(createTestJWT(tt.claims))
			if err != nil {
				t.Fatalf("ResolveUserType() error = %v", err)
			}
			if got.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", got.Type, tt.wantType)
			}
			if got.ReadableID != tt.wantReadableID {
				t.Errorf("ReadableID = %q, want %q", got.ReadableID, tt.wantReadableID)
			}
		})
	}
}

// TestJWTDetectorReadableIDUnsetLeavesEmpty guards that a mechanism naming no
// readable_id_claim never borrows one from elsewhere — the audit fallback is
// the only thing that fills the gap.
func TestJWTDetectorReadableIDUnsetLeavesEmpty(t *testing.T) {
	userTypes := []subject.UserTypeConfig{
		{
			Type:        user,
			DisplayName: "Human User",
			Priority:    1,
			AuthMechanisms: []subject.AuthMechanismConfig{
				{Type: "jwt", Entitlement: subject.EntitlementConfig{Claim: "groups", DisplayName: "User Group"}},
			},
		},
	}

	detector, err := NewResolver(userTypes)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}

	got, err := detector.ResolveUserType(createTestJWT(jwt.MapClaims{
		"groups": "admin", "sub": "user-1", "username": "alice@example.com",
	}))
	if err != nil {
		t.Fatalf("ResolveUserType() error = %v", err)
	}
	if got.ReadableID != "" {
		t.Errorf("ReadableID = %q, want empty when the mechanism names no claim", got.ReadableID)
	}
}

func TestJWTDetectorMissingSubClaim(t *testing.T) {
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
	}

	detector, err := NewResolver(userTypes)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}

	token := createTestJWT(jwt.MapClaims{"group": "admin"})
	result, err := detector.ResolveUserType(token)
	if err != nil {
		t.Fatalf("ResolveUserType() error = %v", err)
	}

	// A token without a sub claim must yield an empty ID, never the string "<nil>"
	// that fmt.Sprintf("%v", nil) would produce.
	if result.ID != "" {
		t.Errorf("ID = %q, want empty string for a token without a sub claim", result.ID)
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
