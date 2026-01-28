// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

// ClusterGatewayConfig defines cluster gateway connection settings for communicating
// with build planes and data planes through the cluster gateway proxy.
type ClusterGatewayConfig struct {
	// Enabled controls whether cluster gateway integration is enabled.
	Enabled bool `koanf:"enabled"`
	// URL is the cluster gateway service URL.
	URL string `koanf:"url"`
	// TLS defines TLS settings for the connection.
	TLS ClusterGatewayTLSConfig `koanf:"tls"`
}

// ClusterGatewayTLSConfig defines TLS settings for cluster gateway connections.
type ClusterGatewayTLSConfig struct {
	// CACertPath is the path to the CA certificate file for verifying the cluster gateway server.
	CACertPath string `koanf:"ca_cert_path"`
	// ClientCertPath is the path to the client certificate file for mTLS authentication.
	ClientCertPath string `koanf:"client_cert_path"`
	// ClientKeyPath is the path to the client private key file for mTLS authentication.
	ClientKeyPath string `koanf:"client_key_path"`
}

// ClusterGatewayDefaults returns the default cluster gateway configuration.
func ClusterGatewayDefaults() ClusterGatewayConfig {
	return ClusterGatewayConfig{
		Enabled: true,
		URL:     "https://localhost:8443",
		TLS: ClusterGatewayTLSConfig{
			CACertPath:     "", // Optional - for server verification
			ClientCertPath: "", // Optional - for mTLS
			ClientKeyPath:  "", // Optional - for mTLS
		},
	}
}

// Validate validates the cluster gateway configuration.
func (c *ClusterGatewayConfig) Validate(path *coreconfig.Path) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors
	if c.Enabled && c.URL == "" {
		errs = append(errs, coreconfig.Required(path.Child("url")))
	}
	return errs
}
