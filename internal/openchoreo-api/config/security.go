// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/openchoreo/openchoreo/internal/authz"
	"github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/subject"
)

// SecurityConfig defines all security-related settings.
type SecurityConfig struct {
	// Authentication defines authentication settings.
	Authentication AuthenticationConfig `koanf:"authentication"`
	// Subjects defines subject types for identity classification.
	// Keys are subject type identifiers (e.g., "user", "service_account").
	Subjects map[string]SubjectConfig `koanf:"subjects"`
	// Authorization defines authorization (Casbin) settings.
	Authorization AuthorizationConfig `koanf:"authorization"`
}

// SecurityDefaults returns the default security configuration.
func SecurityDefaults() SecurityConfig {
	return SecurityConfig{
		Authentication: AuthenticationDefaults(),
		Subjects:       nil,
		Authorization:  AuthorizationDefaults(),
	}
}

// Validate validates the security configuration.
func (c *SecurityConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	errs = append(errs, c.Authentication.Validate(path.Child("authentication"))...)
	errs = append(errs, c.validateSubjects(path.Child("subjects"))...)
	errs = append(errs, c.Authorization.Validate(path.Child("authorization"))...)

	return errs
}

// validateSubjects validates the subjects map configuration.
func (c *SecurityConfig) validateSubjects(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	// Check for duplicate priorities
	prioritiesSeen := make(map[int]string)
	for name, subj := range c.Subjects {
		errs = append(errs, subj.Validate(path.Child(name))...)

		if existingName, exists := prioritiesSeen[subj.Priority]; exists {
			errs = append(errs, config.Invalid(path.Child(name).Child("priority"),
				fmt.Sprintf("duplicate priority %d (also used by %q)", subj.Priority, existingName)))
		} else {
			prioritiesSeen[subj.Priority] = name
		}
	}

	return errs
}

// AuthenticationConfig defines authentication settings.
type AuthenticationConfig struct {
	// JWT defines JWT authentication settings.
	JWT JWTConfig `koanf:"jwt"`
}

// AuthenticationDefaults returns the default authentication configuration.
func AuthenticationDefaults() AuthenticationConfig {
	return AuthenticationConfig{
		JWT: JWTDefaults(),
	}
}

// Validate validates the authentication configuration.
func (c *AuthenticationConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors
	errs = append(errs, c.JWT.Validate(path.Child("jwt"))...)
	return errs
}

// JWTConfig defines JWT authentication settings.
type JWTConfig struct {
	// Enabled enables JWT authentication.
	Enabled bool `koanf:"enabled"`
	// Audiences is the list of acceptable token audiences (aud claim).
	// Token must contain at least one of these audiences. Optional.
	Audiences []string `koanf:"audiences"`
	// ClockSkew is the allowed clock skew for token validation.
	ClockSkew time.Duration `koanf:"clock_skew"`
	// JWKS defines JSON Web Key Set operational settings.
	JWKS JWKSConfig `koanf:"jwks"`
}

// JWTDefaults returns the default JWT configuration.
func JWTDefaults() JWTConfig {
	return JWTConfig{
		Enabled:   false,
		ClockSkew: 0,
		JWKS:      JWKSDefaults(),
	}
}

// Validate validates the JWT configuration.
func (c *JWTConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if !c.Enabled {
		return errs // skip validation if disabled
	}

	if err := config.MustBeNonNegative(path.Child("clock_skew"), c.ClockSkew); err != nil {
		errs = append(errs, err)
	}

	errs = append(errs, c.JWKS.Validate(path.Child("jwks"))...)

	return errs
}

// ToJWTMiddlewareConfig converts to the JWT middleware library config.
// The oidc parameter provides issuer and JWKS URL from identity configuration.
func (c *JWTConfig) ToJWTMiddlewareConfig(oidc *OIDCConfig, logger *slog.Logger, resolver *jwt.Resolver) jwt.Config {
	return jwt.Config{
		Disabled:                     !c.Enabled,
		JWKSURL:                      oidc.JWKSURL,
		JWKSRefreshInterval:          c.JWKS.RefreshInterval,
		JWKSURLTLSInsecureSkipVerify: c.JWKS.SkipTLSVerify,
		ValidateIssuer:               oidc.Issuer,
		ValidateAudiences:            c.Audiences,
		ClockSkew:                    c.ClockSkew,
		Detector:                     resolver,
		Logger:                       logger,
	}
}

// JWKSConfig defines JWKS (JSON Web Key Set) operational settings.
// Note: The JWKS URL comes from identity.oidc.jwks_url.
type JWKSConfig struct {
	// RefreshInterval is how often to refresh keys from the JWKS URL.
	RefreshInterval time.Duration `koanf:"refresh_interval"`
	// SkipTLSVerify skips TLS certificate verification for JWKS URL.
	// WARNING: Only use for development/testing.
	SkipTLSVerify bool `koanf:"skip_tls_verify"`
}

// JWKSDefaults returns the default JWKS configuration.
func JWKSDefaults() JWKSConfig {
	return JWKSConfig{
		RefreshInterval: 1 * time.Hour,
		SkipTLSVerify:   false,
	}
}

// Validate validates the JWKS configuration.
func (c *JWKSConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if err := config.MustBeNonNegative(path.Child("refresh_interval"), c.RefreshInterval); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// SubjectConfig defines a subject type for identity classification.
type SubjectConfig struct {
	// DisplayName is the human-readable name for this subject type.
	DisplayName string `koanf:"display_name"`
	// Priority determines check order (lower = higher priority).
	Priority int `koanf:"priority"`
	// Mechanisms defines authentication mechanisms and their entitlement extraction.
	// Keys are mechanism types (e.g., "jwt").
	Mechanisms map[string]MechanismConfig `koanf:"mechanisms"`
}

// Validate validates the subject configuration.
func (c *SubjectConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if c.DisplayName == "" {
		errs = append(errs, config.Required(path.Child("display_name")))
	}

	if len(c.Mechanisms) == 0 {
		errs = append(errs, config.Required(path.Child("mechanisms")))
	}

	for mechType, mech := range c.Mechanisms {
		errs = append(errs, mech.Validate(path.Child("mechanisms").Child(mechType))...)
	}

	return errs
}

// MechanismConfig defines an authentication mechanism for a subject type.
type MechanismConfig struct {
	// Entitlement defines how to extract entitlement claims.
	Entitlement EntitlementConfig `koanf:"entitlement"`
}

// Validate validates the mechanism configuration.
func (c *MechanismConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors
	errs = append(errs, c.Entitlement.Validate(path.Child("entitlement"))...)
	return errs
}

// EntitlementConfig defines how to extract entitlement claims from tokens.
type EntitlementConfig struct {
	// Claim is the claim name for entitlement extraction.
	Claim string `koanf:"claim"`
	// DisplayName is the human-readable name for the claim.
	DisplayName string `koanf:"display_name"`
}

// Validate validates the entitlement configuration.
func (c *EntitlementConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if c.Claim == "" {
		errs = append(errs, config.Required(path.Child("claim")))
	}

	if c.DisplayName == "" {
		errs = append(errs, config.Required(path.Child("display_name")))
	}

	return errs
}

// ToSubjectUserTypeConfigs converts the map-based subjects config to the subject library's slice format.
func (c *SecurityConfig) ToSubjectUserTypeConfigs() []subject.UserTypeConfig {
	if len(c.Subjects) == 0 {
		return nil
	}

	result := make([]subject.UserTypeConfig, 0, len(c.Subjects))
	for typeName, subj := range c.Subjects {
		mechanisms := make([]subject.AuthMechanismConfig, 0, len(subj.Mechanisms))
		for mechType, mech := range subj.Mechanisms {
			mechanisms = append(mechanisms, subject.AuthMechanismConfig{
				Type: mechType,
				Entitlement: subject.EntitlementConfig{
					Claim:       mech.Entitlement.Claim,
					DisplayName: mech.Entitlement.DisplayName,
				},
			})
		}

		result = append(result, subject.UserTypeConfig{
			Type:           typeName,
			DisplayName:    subj.DisplayName,
			Priority:       subj.Priority,
			AuthMechanisms: mechanisms,
		})
	}

	// Sort by priority to ensure deterministic order
	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority < result[j].Priority
	})

	return result
}

// AuthorizationConfig defines authorization (Casbin) settings.
type AuthorizationConfig struct {
	// Enabled enables authorization enforcement.
	Enabled bool `koanf:"enabled"`
	// DatabasePath is the path to the Casbin SQLite database.
	DatabasePath string `koanf:"database_path"`
	// RolesFile is the path to the roles YAML file (contains roles and mappings).
	RolesFile string `koanf:"roles_file"`
	// Cache defines caching settings for authorization decisions.
	Cache AuthzCacheConfig `koanf:"cache"`
}

// AuthzCacheConfig defines caching settings for authorization.
type AuthzCacheConfig struct {
	// Enabled enables the Casbin enforcer cache.
	Enabled bool `koanf:"enabled"`
	// TTL is the cache time-to-live duration.
	TTL time.Duration `koanf:"ttl"`
}

// AuthzCacheDefaults returns the default cache configuration.
func AuthzCacheDefaults() AuthzCacheConfig {
	return AuthzCacheConfig{
		Enabled: false,
		TTL:     5 * time.Minute,
	}
}

// AuthorizationDefaults returns the default authorization configuration.
func AuthorizationDefaults() AuthorizationConfig {
	return AuthorizationConfig{
		Enabled: false,
		Cache:   AuthzCacheDefaults(),
	}
}

// Validate validates the authorization configuration.
func (c *AuthorizationConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if !c.Enabled {
		return errs // skip validation if disabled
	}

	if c.DatabasePath == "" {
		errs = append(errs, config.Required(path.Child("database_path")))
	}

	errs = append(errs, c.Cache.Validate(path.Child("cache"))...)

	return errs
}

// Validate validates the cache configuration.
func (c *AuthzCacheConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if c.Enabled {
		if err := config.MustBeGreaterThan(path.Child("ttl"), c.TTL, 0); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

// ToAuthzConfig converts to the authz library config.
func (c *AuthorizationConfig) ToAuthzConfig() authz.Config {
	return authz.Config{
		Enabled:      c.Enabled,
		DatabasePath: c.DatabasePath,
		RolesFile:    c.RolesFile,
		CacheEnabled: c.Cache.Enabled,
		CacheTTL:     c.Cache.TTL,
	}
}
