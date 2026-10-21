// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"

	"github.com/openchoreo/openchoreo/internal/config"
)

// DefaultRemoteKeyPrefix is the first segment prepended to every remote key
// written to the external secret store. It matches the value used before the
// prefix became configurable, so existing installs keep their key layout.
const DefaultRemoteKeyPrefix = "secret"

// DefaultRemoteKeySeparator joins the segments of every remote key. It matches
// the path-style layout used before the separator became configurable, so
// existing installs keep their key layout.
const DefaultRemoteKeySeparator = "/"

// remoteKeySeparators lists the accepted separators: the characters, other
// than letters and digits, that AWS Secrets Manager allows in secret names,
// all of which OpenBao and Vault accept too. "/" gives path-style keys. "-" is
// the only one Azure Key Vault allows, and GCP Secret Manager allows "-" and
// "_". "-" and "." can also appear inside namespace and secret names, so with
// those two, different secrets can map to the same key.
var remoteKeySeparators = []string{"/", "-", "_", ".", "+", "=", "@"}

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

	// RemoteKeySeparator joins the segments of every key written to the
	// external secret store, including the segments of a nested prefix. "/"
	// gives "<prefix>/<namespace>/<segment>/<name>". "-" gives
	// "<prefix>-<namespace>-<segment>-<name>", for stores that reject "/" in
	// secret names, such as Azure Key Vault. See remoteKeySeparators for the
	// accepted values.
	//
	// Keys are joined as-is, never rewritten or shortened, so the separator
	// and the resulting keys must fit the store's naming rules, including its
	// length limit (127 characters for Azure Key Vault, 255 for GCP Secret
	// Manager, 512 for AWS Secrets Manager).
	//
	// Like RemoteKeyPrefix, this decides where secrets are stored, so it is
	// install-time configuration.
	RemoteKeySeparator string `koanf:"remote_key_separator"`
}

// SecretManagementDefaults returns the default Secret management configuration.
func SecretManagementDefaults() SecretManagementConfig {
	return SecretManagementConfig{
		Enabled:            false,
		RemoteKeyPrefix:    DefaultRemoteKeyPrefix,
		RemoteKeySeparator: DefaultRemoteKeySeparator,
	}
}

// Validate validates the Secret management configuration.
func (c *SecretManagementConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if err := config.MustBeOneOf(path.Child("remote_key_separator"), c.RemoteKeySeparator, remoteKeySeparators); err != nil {
		errs = append(errs, err)
	}

	return errs
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
