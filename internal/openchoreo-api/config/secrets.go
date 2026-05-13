// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

// SecretsConfig defines settings for the Secret API endpoints.
type SecretsConfig struct {
	// Enabled toggles the Secret API (POST/PUT/GET/LIST/DELETE under
	// /api/v1alpha1/namespaces/{ns}/secrets). When false, all five
	// endpoints return 501 Not Implemented.
	Enabled bool `koanf:"enabled"`
}

// SecretsDefaults returns the default Secrets configuration.
func SecretsDefaults() SecretsConfig {
	return SecretsConfig{
		Enabled: true,
	}
}
