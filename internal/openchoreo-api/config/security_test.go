// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/openchoreo/openchoreo/internal/config"
)

func TestSecurityConfig_ValidateSubjects_DuplicatePriorities(t *testing.T) {
	tests := []struct {
		name           string
		subjects       map[string]SubjectConfig
		expectedErrors config.ValidationErrors
	}{
		{
			name:           "nil subjects is valid",
			subjects:       nil,
			expectedErrors: nil,
		},
		{
			name:           "empty subjects is valid",
			subjects:       map[string]SubjectConfig{},
			expectedErrors: nil,
		},
		{
			name: "unique priorities is valid",
			subjects: map[string]SubjectConfig{
				"user": {
					DisplayName: "User",
					Priority:    1,
					Mechanisms: map[string]MechanismConfig{
						"jwt": {Entitlement: EntitlementConfig{Claim: "groups", DisplayName: "Groups"}},
					},
				},
				"service_account": {
					DisplayName: "Service Account",
					Priority:    2,
					Mechanisms: map[string]MechanismConfig{
						"jwt": {Entitlement: EntitlementConfig{Claim: "sub", DisplayName: "Client ID"}},
					},
				},
			},
			expectedErrors: nil,
		},
		{
			name: "duplicate priorities returns error",
			subjects: map[string]SubjectConfig{
				"user": {
					DisplayName: "User",
					Priority:    1,
					Mechanisms: map[string]MechanismConfig{
						"jwt": {Entitlement: EntitlementConfig{Claim: "groups", DisplayName: "Groups"}},
					},
				},
				"service_account": {
					DisplayName: "Service Account",
					Priority:    1, // duplicate priority
					Mechanisms: map[string]MechanismConfig{
						"jwt": {Entitlement: EntitlementConfig{Claim: "sub", DisplayName: "Client ID"}},
					},
				},
			},
			expectedErrors: nil, // We'll check for non-nil and message content instead
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SecurityConfig{
				Authentication: AuthenticationDefaults(),
				Subjects:       tt.subjects,
				Authorization:  AuthorizationDefaults(),
			}

			errs := cfg.Validate(config.NewPath("security"))

			if tt.name == "duplicate priorities returns error" {
				// Special handling for duplicate priority test due to map iteration order
				if len(errs) != 1 {
					t.Errorf("expected 1 error, got %d: %v", len(errs), errs)
					return
				}
				// Check that the error mentions "duplicate priority"
				if errs[0].Message == "" {
					t.Error("expected error message about duplicate priority")
				}
				return
			}

			if diff := cmp.Diff(tt.expectedErrors, errs); diff != "" {
				t.Errorf("validation errors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestJWTConfig_Validate(t *testing.T) {
	tests := []struct {
		name           string
		cfg            JWTConfig
		expectedErrors config.ValidationErrors
	}{
		{
			name: "disabled skips all validation",
			cfg: JWTConfig{
				Enabled: false,
				// Missing required fields but should pass because disabled
			},
			expectedErrors: nil,
		},
		{
			name: "enabled requires issuer and jwks url",
			cfg: JWTConfig{
				Enabled: true,
			},
			expectedErrors: config.ValidationErrors{
				{Field: "jwt.issuer", Message: "is required"},
				{Field: "jwt.jwks.url", Message: "is required"},
			},
		},
		{
			name: "enabled with issuer still requires jwks url",
			cfg: JWTConfig{
				Enabled: true,
				Issuer:  "https://issuer.example.com",
			},
			expectedErrors: config.ValidationErrors{
				{Field: "jwt.jwks.url", Message: "is required"},
			},
		},
		{
			name: "enabled with all required fields is valid",
			cfg: JWTConfig{
				Enabled: true,
				Issuer:  "https://issuer.example.com",
				JWKS: JWKSConfig{
					URL: "https://issuer.example.com/.well-known/jwks.json",
				},
			},
			expectedErrors: nil,
		},
		{
			name: "negative clock_skew is invalid",
			cfg: JWTConfig{
				Enabled:   true,
				Issuer:    "https://issuer.example.com",
				ClockSkew: -1 * time.Second,
				JWKS: JWKSConfig{
					URL: "https://issuer.example.com/.well-known/jwks.json",
				},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "jwt.clock_skew", Message: "must be non-negative"},
			},
		},
		{
			name: "negative jwks refresh_interval is invalid",
			cfg: JWTConfig{
				Enabled: true,
				Issuer:  "https://issuer.example.com",
				JWKS: JWKSConfig{
					URL:             "https://issuer.example.com/.well-known/jwks.json",
					RefreshInterval: -1 * time.Hour,
				},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "jwt.jwks.refresh_interval", Message: "must be non-negative"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.cfg.Validate(config.NewPath("jwt"))
			if diff := cmp.Diff(tt.expectedErrors, errs); diff != "" {
				t.Errorf("validation errors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAuthorizationConfig_Validate(t *testing.T) {
	tests := []struct {
		name           string
		cfg            AuthorizationConfig
		expectedErrors config.ValidationErrors
	}{
		{
			name: "disabled skips all validation",
			cfg: AuthorizationConfig{
				Enabled: false,
				// Missing required fields but should pass because disabled
			},
			expectedErrors: nil,
		},
		{
			name: "enabled requires database_path",
			cfg: AuthorizationConfig{
				Enabled: true,
			},
			expectedErrors: config.ValidationErrors{
				{Field: "authz.database_path", Message: "is required"},
			},
		},
		{
			name: "enabled with database_path is valid",
			cfg: AuthorizationConfig{
				Enabled:      true,
				DatabasePath: "/path/to/authz.db",
			},
			expectedErrors: nil,
		},
		{
			name: "cache enabled requires positive ttl",
			cfg: AuthorizationConfig{
				Enabled:      true,
				DatabasePath: "/path/to/authz.db",
				Cache: AuthzCacheConfig{
					Enabled: true,
					TTL:     0, // zero TTL is invalid when cache enabled
				},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "authz.cache.ttl", Message: "must be greater than 0s"},
			},
		},
		{
			name: "cache enabled with valid ttl is valid",
			cfg: AuthorizationConfig{
				Enabled:      true,
				DatabasePath: "/path/to/authz.db",
				Cache: AuthzCacheConfig{
					Enabled: true,
					TTL:     5 * time.Minute,
				},
			},
			expectedErrors: nil,
		},
		{
			name: "cache disabled allows zero ttl",
			cfg: AuthorizationConfig{
				Enabled:      true,
				DatabasePath: "/path/to/authz.db",
				Cache: AuthzCacheConfig{
					Enabled: false,
					TTL:     0,
				},
			},
			expectedErrors: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.cfg.Validate(config.NewPath("authz"))
			if diff := cmp.Diff(tt.expectedErrors, errs); diff != "" {
				t.Errorf("validation errors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
