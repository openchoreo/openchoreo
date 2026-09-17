// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import "strings"

// DefaultRemoteKeyPrefix is the first segment prepended to every remote key
// written to the external secret store. It matches the value used before the
// prefix became configurable, so existing installs keep their key layout.
const DefaultRemoteKeyPrefix = "secret"

// SecretManagementConfig defines settings for the Secret management API endpoints.
type SecretManagementConfig struct {
	// Enabled toggles the Secret management API (POST/PUT/GET/LIST/DELETE under
	// /api/v1alpha1/namespaces/{ns}/secrets). When false, all five
	// endpoints return 501 Not Implemented.
	Enabled bool `koanf:"enabled"`

	// RemoteKeyPrefix is the first segment of every key written to the external
	// secret store, as in "<prefix>/<namespace>/<segment>/<name>". An empty
	// value drops the segment entirely, giving "<namespace>/<segment>/<name>".
	//
	// The prefix must not equal the name of the KV mount the ClusterSecretStore
	// points at. When the two collide, external-secrets strips the duplicated
	// segment when writing the value but not when writing the ownership
	// metadata, so each secret lands as two entries: the value at the stripped
	// path and a data-less metadata entry at the un-stripped path.
	//
	// This setting decides where secrets are stored, so it is install-time
	// configuration. Changing it after secrets exist does not move the existing
	// entries; they must be migrated separately.
	RemoteKeyPrefix string `koanf:"remote_key_prefix"`
}

// SecretManagementDefaults returns the default Secret management configuration.
func SecretManagementDefaults() SecretManagementConfig {
	return SecretManagementConfig{
		Enabled:         false,
		RemoteKeyPrefix: DefaultRemoteKeyPrefix,
	}
}

// NormalizedRemoteKeyPrefix returns the configured prefix with every empty
// slash-separated component dropped, so callers can join it into a key path
// without producing empty or doubled segments. Leading, trailing and interior
// slash runs are all collapsed, so "//team///prod//" normalizes to "team/prod".
// A prefix that holds no components normalizes to the empty string, which omits
// the prefix segment entirely.
func (c SecretManagementConfig) NormalizedRemoteKeyPrefix() string {
	parts := strings.Split(c.RemoteKeyPrefix, "/")
	kept := parts[:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "/")
}
