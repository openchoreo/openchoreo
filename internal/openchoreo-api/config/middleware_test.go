// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/openchoreo/openchoreo/internal/config"
)

func TestEntitlementConfig_Validate(t *testing.T) {
	tests := []struct {
		name           string
		cfg            EntitlementConfig
		expectedErrors config.ValidationErrors
	}{
		{
			name: "valid config",
			cfg: EntitlementConfig{
				Claim:       "groups",
				DisplayName: "Groups",
			},
			expectedErrors: nil,
		},
		{
			name: "missing claim",
			cfg: EntitlementConfig{
				DisplayName: "Groups",
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.claim", Message: "is required"},
			},
		},
		{
			name: "missing display_name",
			cfg: EntitlementConfig{
				Claim: "groups",
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.display_name", Message: "is required"},
			},
		},
		{
			name: "missing all required fields",
			cfg:  EntitlementConfig{},
			expectedErrors: config.ValidationErrors{
				{Field: "test.claim", Message: "is required"},
				{Field: "test.display_name", Message: "is required"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.cfg.Validate(config.NewPath("test"))
			if diff := cmp.Diff(tt.expectedErrors, errs); diff != "" {
				t.Errorf("validation errors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAuthMechanismConfig_Validate(t *testing.T) {
	validEntitlement := EntitlementConfig{
		Claim:       "groups",
		DisplayName: "Groups",
	}

	tests := []struct {
		name           string
		cfg            AuthMechanismConfig
		expectedErrors config.ValidationErrors
	}{
		{
			name: "valid config",
			cfg: AuthMechanismConfig{
				Type:        "oauth2",
				Entitlement: validEntitlement,
			},
			expectedErrors: nil,
		},
		{
			name: "missing type",
			cfg: AuthMechanismConfig{
				Entitlement: validEntitlement,
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.type", Message: "is required"},
			},
		},
		{
			name: "invalid type",
			cfg: AuthMechanismConfig{
				Type:        "invalid",
				Entitlement: validEntitlement,
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.type", Message: "must be one of: oauth2"},
			},
		},
		{
			name: "missing entitlement fields",
			cfg: AuthMechanismConfig{
				Type:        "oauth2",
				Entitlement: EntitlementConfig{},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.entitlement.claim", Message: "is required"},
				{Field: "test.entitlement.display_name", Message: "is required"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.cfg.Validate(config.NewPath("test"))
			if diff := cmp.Diff(tt.expectedErrors, errs); diff != "" {
				t.Errorf("validation errors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUserTypeConfig_Validate(t *testing.T) {
	validAuthMechanism := AuthMechanismConfig{
		Type: "oauth2",
		Entitlement: EntitlementConfig{
			Claim:       "groups",
			DisplayName: "Groups",
		},
	}

	tests := []struct {
		name           string
		cfg            UserTypeConfig
		expectedErrors config.ValidationErrors
	}{
		{
			name: "valid config",
			cfg: UserTypeConfig{
				Type:           "user",
				DisplayName:    "User",
				Priority:       1,
				AuthMechanisms: []AuthMechanismConfig{validAuthMechanism},
			},
			expectedErrors: nil,
		},
		{
			name: "missing type",
			cfg: UserTypeConfig{
				DisplayName:    "User",
				Priority:       1,
				AuthMechanisms: []AuthMechanismConfig{validAuthMechanism},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.type", Message: "is required"},
			},
		},
		{
			name: "missing display_name",
			cfg: UserTypeConfig{
				Type:           "user",
				Priority:       1,
				AuthMechanisms: []AuthMechanismConfig{validAuthMechanism},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.display_name", Message: "is required"},
			},
		},
		{
			name: "missing auth_mechanisms",
			cfg: UserTypeConfig{
				Type:        "user",
				DisplayName: "User",
				Priority:    1,
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.auth_mechanisms", Message: "is required"},
			},
		},
		{
			name: "duplicate auth mechanism types",
			cfg: UserTypeConfig{
				Type:        "user",
				DisplayName: "User",
				Priority:    1,
				AuthMechanisms: []AuthMechanismConfig{
					validAuthMechanism,
					validAuthMechanism, // duplicate
				},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.auth_mechanisms[1].type", Message: "duplicate mechanism type \"oauth2\" (first defined at index 0)"},
			},
		},
		{
			name: "missing all required fields",
			cfg:  UserTypeConfig{},
			expectedErrors: config.ValidationErrors{
				{Field: "test.type", Message: "is required"},
				{Field: "test.display_name", Message: "is required"},
				{Field: "test.auth_mechanisms", Message: "is required"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.cfg.Validate(config.NewPath("test"))
			if diff := cmp.Diff(tt.expectedErrors, errs); diff != "" {
				t.Errorf("validation errors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestJWTConfig_Validate(t *testing.T) {
	validUserType := UserTypeConfig{
		Type:        "user",
		DisplayName: "User",
		Priority:    1,
		AuthMechanisms: []AuthMechanismConfig{
			{
				Type: "oauth2",
				Entitlement: EntitlementConfig{
					Claim:       "groups",
					DisplayName: "Groups",
				},
			},
		},
	}

	tests := []struct {
		name           string
		cfg            JWTConfig
		expectedErrors config.ValidationErrors
	}{
		{
			name: "valid config",
			cfg: JWTConfig{
				Enabled:   true,
				Issuer:    "https://issuer.example.com",
				JWKS:      JWKSConfig{URL: "https://issuer.example.com/.well-known/jwks.json"},
				UserTypes: []UserTypeConfig{validUserType},
			},
			expectedErrors: nil,
		},
		{
			name: "disabled skips validation",
			cfg: JWTConfig{
				Enabled: false,
				// Missing required fields but should pass because disabled
			},
			expectedErrors: nil,
		},
		{
			name: "missing issuer",
			cfg: JWTConfig{
				Enabled:   true,
				JWKS:      JWKSConfig{URL: "https://issuer.example.com/.well-known/jwks.json"},
				UserTypes: []UserTypeConfig{validUserType},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.issuer", Message: "is required"},
			},
		},
		{
			name: "missing jwks url",
			cfg: JWTConfig{
				Enabled:   true,
				Issuer:    "https://issuer.example.com",
				UserTypes: []UserTypeConfig{validUserType},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.jwks.url", Message: "is required"},
			},
		},
		{
			name: "negative clock_skew",
			cfg: JWTConfig{
				Enabled:   true,
				Issuer:    "https://issuer.example.com",
				ClockSkew: -1,
				JWKS:      JWKSConfig{URL: "https://issuer.example.com/.well-known/jwks.json"},
				UserTypes: []UserTypeConfig{validUserType},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.clock_skew", Message: "must be non-negative"},
			},
		},
		{
			name: "duplicate user type types",
			cfg: JWTConfig{
				Enabled: true,
				Issuer:  "https://issuer.example.com",
				JWKS:    JWKSConfig{URL: "https://issuer.example.com/.well-known/jwks.json"},
				UserTypes: []UserTypeConfig{
					validUserType,
					{
						Type:           "user", // duplicate type
						DisplayName:    "Another User",
						Priority:       2,
						AuthMechanisms: validUserType.AuthMechanisms,
					},
				},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.user_types[1].type", Message: "duplicate type \"user\" (first defined at index 0)"},
			},
		},
		{
			name: "duplicate user type priorities",
			cfg: JWTConfig{
				Enabled: true,
				Issuer:  "https://issuer.example.com",
				JWKS:    JWKSConfig{URL: "https://issuer.example.com/.well-known/jwks.json"},
				UserTypes: []UserTypeConfig{
					validUserType,
					{
						Type:           "service_account",
						DisplayName:    "Service Account",
						Priority:       1, // duplicate priority
						AuthMechanisms: validUserType.AuthMechanisms,
					},
				},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "test.user_types[1].priority", Message: "duplicate priority 1 (first defined at index 0)"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.cfg.Validate(config.NewPath("test"))
			if diff := cmp.Diff(tt.expectedErrors, errs); diff != "" {
				t.Errorf("validation errors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
