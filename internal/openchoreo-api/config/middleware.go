// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/subject"
)

// MiddlewareConfig defines configurations for server middlewares.
type MiddlewareConfig struct {
	// JWT defines JWT authentication middleware settings.
	JWT JWTConfig `koanf:"jwt"`
}

// MiddlewareDefaults returns the default middleware configuration.
func MiddlewareDefaults() MiddlewareConfig {
	return MiddlewareConfig{
		JWT: JWTDefaults(),
	}
}

// Validate validates the middleware configuration.
func (c *MiddlewareConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors
	errs = append(errs, c.JWT.Validate(path.Child("jwt"))...)
	return errs
}

// JWTConfig defines JWT authentication settings.
type JWTConfig struct {
	// Enabled enables JWT authentication.
	Enabled bool `koanf:"enabled"`
	// Issuer is the expected token issuer (iss claim).
	Issuer string `koanf:"issuer"`
	// Audience is the expected token audience (aud claim). Optional.
	Audience string `koanf:"audience"`
	// ClockSkew is the allowed clock skew for token validation.
	ClockSkew time.Duration `koanf:"clock_skew"`
	// JWKS defines JSON Web Key Set settings.
	JWKS JWKSConfig `koanf:"jwks"`
	// UserTypes defines user type detection configurations.
	UserTypes []UserTypeConfig `koanf:"user_types"`
}

// JWTDefaults returns the default JWT configuration.
func JWTDefaults() JWTConfig {
	return JWTConfig{
		Enabled:   true,
		ClockSkew: 0,
		JWKS:      JWKSDefaults(),
		UserTypes: nil,
	}
}

// Validate validates the JWT configuration.
func (c *JWTConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if !c.Enabled {
		return errs // skip validation if disabled
	}

	if c.Issuer == "" {
		errs = append(errs, config.Required(path.Child("issuer")))
	}

	if err := config.MustBeNonNegative(path.Child("clock_skew"), c.ClockSkew); err != nil {
		errs = append(errs, err)
	}

	errs = append(errs, c.JWKS.Validate(path.Child("jwks"))...)

	// Validate individual user types and check for duplicates
	typesSeen := make(map[string]int)
	prioritiesSeen := make(map[int]int)
	for i, ut := range c.UserTypes {
		errs = append(errs, ut.Validate(path.Child("user_types").Index(i))...)

		// Check for duplicate types
		if prevIdx, exists := typesSeen[ut.Type]; exists {
			errs = append(errs, config.Invalid(path.Child("user_types").Index(i).Child("type"),
				fmt.Sprintf("duplicate type %q (first defined at index %d)", ut.Type, prevIdx)))
		} else if ut.Type != "" {
			typesSeen[ut.Type] = i
		}

		// Check for duplicate priorities
		if prevIdx, exists := prioritiesSeen[ut.Priority]; exists {
			errs = append(errs, config.Invalid(path.Child("user_types").Index(i).Child("priority"),
				fmt.Sprintf("duplicate priority %d (first defined at index %d)", ut.Priority, prevIdx)))
		} else {
			prioritiesSeen[ut.Priority] = i
		}
	}

	return errs
}

// ToJWTConfig converts to the JWT middleware library config.
func (c *JWTConfig) ToJWTConfig(logger *slog.Logger, resolver *jwt.Resolver) jwt.Config {
	return jwt.Config{
		Disabled:                     !c.Enabled,
		JWKSURL:                      c.JWKS.URL,
		JWKSRefreshInterval:          c.JWKS.RefreshInterval,
		JWKSURLTLSInsecureSkipVerify: c.JWKS.SkipTLSVerify,
		ValidateIssuer:               c.Issuer,
		ValidateAudience:             c.Audience,
		ClockSkew:                    c.ClockSkew,
		Detector:                     resolver,
		Logger:                       logger,
	}
}

// JWKSConfig defines JWKS (JSON Web Key Set) settings.
type JWKSConfig struct {
	// URL is the JWKS endpoint URL.
	URL string `koanf:"url"`
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

	if c.URL == "" {
		errs = append(errs, config.Required(path.Child("url")))
	}

	if err := config.MustBeNonNegative(path.Child("refresh_interval"), c.RefreshInterval); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// UserTypeConfig defines a user type for JWT claim-based detection.
type UserTypeConfig struct {
	// Type is the user type identifier (e.g., "user", "service_account").
	Type string `koanf:"type"`
	// DisplayName is the human-readable name for this user type.
	DisplayName string `koanf:"display_name"`
	// Priority determines check order (lower = higher priority).
	Priority int `koanf:"priority"`
	// AuthMechanisms defines supported authentication mechanisms.
	AuthMechanisms []AuthMechanismConfig `koanf:"auth_mechanisms"`
}

// Validate validates the user type configuration.
func (c *UserTypeConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if c.Type == "" {
		errs = append(errs, config.Required(path.Child("type")))
	}

	if c.DisplayName == "" {
		errs = append(errs, config.Required(path.Child("display_name")))
	}

	if len(c.AuthMechanisms) == 0 {
		errs = append(errs, config.Required(path.Child("auth_mechanisms")))
	}

	// Validate individual auth mechanisms and check for duplicates
	mechanismsSeen := make(map[string]int)
	for i, am := range c.AuthMechanisms {
		errs = append(errs, am.Validate(path.Child("auth_mechanisms").Index(i))...)

		// Check for duplicate mechanism types
		if prevIdx, exists := mechanismsSeen[am.Type]; exists {
			errs = append(errs, config.Invalid(path.Child("auth_mechanisms").Index(i).Child("type"),
				fmt.Sprintf("duplicate mechanism type %q (first defined at index %d)", am.Type, prevIdx)))
		} else if am.Type != "" {
			mechanismsSeen[am.Type] = i
		}
	}

	return errs
}

// supportedAuthMechanisms lists the currently supported authentication mechanism types.
var supportedAuthMechanisms = []string{"oauth2"}

// AuthMechanismConfig defines an authentication mechanism for a user type.
type AuthMechanismConfig struct {
	// Type is the authentication mechanism type. Currently only "oauth2" is supported.
	Type string `koanf:"type"`
	// Entitlement defines how to extract entitlement claims.
	Entitlement EntitlementConfig `koanf:"entitlement"`
}

// Validate validates the auth mechanism configuration.
func (c *AuthMechanismConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if c.Type == "" {
		errs = append(errs, config.Required(path.Child("type")))
	} else if err := config.MustBeOneOf(path.Child("type"), c.Type, supportedAuthMechanisms); err != nil {
		errs = append(errs, err)
	}

	errs = append(errs, c.Entitlement.Validate(path.Child("entitlement"))...)

	return errs
}

// EntitlementConfig defines how to extract entitlement claims from tokens.
type EntitlementConfig struct {
	// Claim is the claim name for detection and entitlement.
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

// ToSubjectEntitlementConfig converts to the subject package's EntitlementConfig.
func (c *EntitlementConfig) ToSubjectEntitlementConfig() subject.EntitlementConfig {
	return subject.EntitlementConfig{
		Claim:       c.Claim,
		DisplayName: c.DisplayName,
	}
}

// ToSubjectAuthMechanismConfig converts to the subject package's AuthMechanismConfig.
func (c *AuthMechanismConfig) ToSubjectAuthMechanismConfig() subject.AuthMechanismConfig {
	return subject.AuthMechanismConfig{
		Type:        c.Type,
		Entitlement: c.Entitlement.ToSubjectEntitlementConfig(),
	}
}

// ToSubjectUserTypeConfig converts to the subject package's UserTypeConfig.
func (c *UserTypeConfig) ToSubjectUserTypeConfig() subject.UserTypeConfig {
	mechanisms := make([]subject.AuthMechanismConfig, len(c.AuthMechanisms))
	for i, am := range c.AuthMechanisms {
		mechanisms[i] = am.ToSubjectAuthMechanismConfig()
	}
	return subject.UserTypeConfig{
		Type:           c.Type,
		DisplayName:    c.DisplayName,
		Priority:       c.Priority,
		AuthMechanisms: mechanisms,
	}
}

// ToSubjectUserTypeConfigs converts a slice of UserTypeConfig to subject.UserTypeConfig.
func ToSubjectUserTypeConfigs(configs []UserTypeConfig) []subject.UserTypeConfig {
	result := make([]subject.UserTypeConfig, len(configs))
	for i, c := range configs {
		result[i] = c.ToSubjectUserTypeConfig()
	}
	return result
}
