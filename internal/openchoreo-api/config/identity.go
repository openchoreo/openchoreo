// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/openchoreo/openchoreo/internal/config"
)

// IdentityConfig defines identity provider settings.
type IdentityConfig struct {
	// OIDC defines OpenID Connect provider settings.
	OIDC OIDCConfig `koanf:"oidc"`
	// Clients defines OAuth client configurations for external integrations.
	// Keys are client identifiers (e.g., "cli", "ci").
	Clients map[string]ClientConfig `koanf:"clients"`
}

// IdentityDefaults returns the default identity configuration.
func IdentityDefaults() IdentityConfig {
	return IdentityConfig{
		OIDC:    OIDCDefaults(),
		Clients: nil,
	}
}

// Validate validates the identity configuration.
func (c *IdentityConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	errs = append(errs, c.OIDC.Validate(path.Child("oidc"))...)

	for name, client := range c.Clients {
		errs = append(errs, client.Validate(path.Child("clients").Child(name))...)
	}

	return errs
}

// OIDCConfig defines OpenID Connect provider settings.
type OIDCConfig struct {
	// Issuer is the OIDC provider issuer URL.
	// Used for token validation and as the base for OAuth metadata.
	Issuer string `koanf:"issuer"`
	// AuthorizationEndpoint is the OAuth authorization endpoint URL.
	AuthorizationEndpoint string `koanf:"authorization_endpoint"`
	// TokenEndpoint is the OAuth token endpoint URL.
	TokenEndpoint string `koanf:"token_endpoint"`
}

// OIDCDefaults returns the default OIDC configuration.
func OIDCDefaults() OIDCConfig {
	return OIDCConfig{
		Issuer:                "http://sts.openchoreo.localhost",
		AuthorizationEndpoint: "http://sts.openchoreo.localhost/oauth2/authorize",
		TokenEndpoint:         "http://sts.openchoreo.localhost/oauth2/token",
	}
}

// Validate validates the OIDC configuration.
// No validation required - defaults are provided, and incorrect values
// will fail at runtime when the features are used.
func (c *OIDCConfig) Validate(_ *config.Path) config.ValidationErrors {
	return nil
}

// ClientConfig defines an OAuth client configuration for external integrations.
type ClientConfig struct {
	// ClientID is the OAuth client identifier.
	ClientID string `koanf:"client_id"`
	// Scopes is the list of OAuth scopes to request.
	Scopes []string `koanf:"scopes"`
}

// Validate validates the client configuration.
func (c *ClientConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if c.ClientID == "" {
		errs = append(errs, config.Required(path.Child("client_id")))
	}

	return errs
}
