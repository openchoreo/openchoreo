// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretManagementDefaults(t *testing.T) {
	cfg := SecretManagementDefaults()
	if cfg.Enabled {
		t.Error("secret management should default to disabled")
	}
	// The default prefix must stay "secret" so existing installs keep the key
	// layout they already have in the external store.
	if cfg.RemoteKeyPrefix != DefaultRemoteKeyPrefix {
		t.Errorf("RemoteKeyPrefix = %q, want %q", cfg.RemoteKeyPrefix, DefaultRemoteKeyPrefix)
	}
}

func TestNormalizedRemoteKeyPrefix(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"secret", "secret"},
		{"/secret", "secret"},
		{"secret/", "secret"},
		{"/secret/", "secret"},
		{"oc/secrets", "oc/secrets"},
		// Interior slash runs must collapse too, not just the surrounding ones,
		// otherwise the joined key carries an empty segment.
		{"team//prod", "team/prod"},
		{"//team///prod//", "team/prod"},
		{"", ""},
		{"/", ""},
		{"///", ""},
	}
	for _, tt := range tests {
		got := SecretManagementConfig{RemoteKeyPrefix: tt.in}.NormalizedRemoteKeyPrefix()
		if got != tt.want {
			t.Errorf("NormalizedRemoteKeyPrefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRemoteKeyPrefixLoadsFromYAML feeds the loader the same config shape the
// Helm chart renders, checking that remote_key_prefix reaches the struct and
// that an explicitly empty value is preserved rather than falling back to the
// default.
func TestRemoteKeyPrefixLoadsFromYAML(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"explicit default", "secret_management:\n  enabled: true\n  remote_key_prefix: \"secret\"\n", "secret"},
		{"explicit empty", "secret_management:\n  enabled: true\n  remote_key_prefix: \"\"\n", ""},
		{"custom", "secret_management:\n  enabled: true\n  remote_key_prefix: \"openchoreo\"\n", "openchoreo"},
		{"omitted falls back to default", "secret_management:\n  enabled: true\n", DefaultRemoteKeyPrefix},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			loader, err := NewLoader(path, nil)
			if err != nil {
				t.Fatalf("NewLoader: %v", err)
			}
			var cfg Config
			if err := loader.Unmarshal("", &cfg); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if cfg.SecretManagement.RemoteKeyPrefix != tt.want {
				t.Errorf("RemoteKeyPrefix = %q, want %q", cfg.SecretManagement.RemoteKeyPrefix, tt.want)
			}
		})
	}
}
