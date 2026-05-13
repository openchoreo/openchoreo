// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

// SecretManagementConfig defines settings for the Secret management API endpoints.
type SecretManagementConfig struct {
	// Enabled toggles the Secret management API (POST/PUT/GET/LIST/DELETE under
	// /api/v1alpha1/namespaces/{ns}/secrets). When false, all five
	// endpoints return 501 Not Implemented.
	Enabled bool `koanf:"enabled"`
}

// SecretManagementDefaults returns the default Secret management configuration.
func SecretManagementDefaults() SecretManagementConfig {
	return SecretManagementConfig{
		Enabled: false,
	}
}
