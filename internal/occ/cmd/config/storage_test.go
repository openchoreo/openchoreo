// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestHome sets home-directory env vars to a temp dir so os.UserHomeDir()
// resolves to the test directory on all platforms (HOME on Unix, USERPROFILE on Windows).
func setupTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	return home
}

func TestLoadStoredConfig(t *testing.T) {
	t.Run("returns empty config when file does not exist", func(t *testing.T) {
		setupTestHome(t)

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		assert.Empty(t, cfg.CurrentContext)
		assert.Empty(t, cfg.Contexts)
		assert.Empty(t, cfg.ControlPlanes)
		assert.Empty(t, cfg.Credentials)
	})

	t.Run("loads valid YAML config", func(t *testing.T) {
		home := setupTestHome(t)
		configDir := filepath.Join(home, ".openchoreo")
		require.NoError(t, os.MkdirAll(configDir, 0755))

		yamlContent := `currentContext: my-ctx
controlplanes:
  - name: my-cp
    url: http://localhost:8080
credentials:
  - name: my-cred
    authMethod: pkce
contexts:
  - name: my-ctx
    controlplane: my-cp
    credentials: my-cred
    namespace: default
`
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "config"), []byte(yamlContent), 0600))

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		assert.Equal(t, "my-ctx", cfg.CurrentContext)
		require.Len(t, cfg.ControlPlanes, 1)
		assert.Equal(t, "my-cp", cfg.ControlPlanes[0].Name)
		assert.Equal(t, "http://localhost:8080", cfg.ControlPlanes[0].URL)
		require.Len(t, cfg.Credentials, 1)
		assert.Equal(t, "my-cred", cfg.Credentials[0].Name)
		assert.Equal(t, "pkce", cfg.Credentials[0].AuthMethod)
		require.Len(t, cfg.Contexts, 1)
		assert.Equal(t, "my-ctx", cfg.Contexts[0].Name)
		assert.Equal(t, "default", cfg.Contexts[0].Namespace)
	})

	t.Run("returns error for invalid YAML", func(t *testing.T) {
		home := setupTestHome(t)
		configDir := filepath.Join(home, ".openchoreo")
		require.NoError(t, os.MkdirAll(configDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "config"), []byte("{{invalid yaml"), 0600))

		_, err := LoadStoredConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse config")
	})
}

func TestSaveStoredConfig(t *testing.T) {
	t.Run("creates directory and file", func(t *testing.T) {
		home := setupTestHome(t)

		cfg := &StoredConfig{
			CurrentContext: "test-ctx",
			ControlPlanes:  []ControlPlane{{Name: "cp1", URL: "http://localhost:8080"}},
			Credentials:    []Credential{{Name: "cred1", AuthMethod: "pkce"}},
			Contexts:       []Context{{Name: "test-ctx", ControlPlane: "cp1", Credentials: "cred1"}},
		}

		err := SaveStoredConfig(cfg)
		require.NoError(t, err)

		// Verify file was created
		configPath := filepath.Join(home, ".openchoreo", "config")
		_, err = os.Stat(configPath)
		require.NoError(t, err)

		// Load it back and verify round-trip
		loaded, err := LoadStoredConfig()
		require.NoError(t, err)
		assert.Equal(t, "test-ctx", loaded.CurrentContext)
		require.Len(t, loaded.ControlPlanes, 1)
		assert.Equal(t, "cp1", loaded.ControlPlanes[0].Name)
		require.Len(t, loaded.Credentials, 1)
		assert.Equal(t, "cred1", loaded.Credentials[0].Name)
		require.Len(t, loaded.Contexts, 1)
		assert.Equal(t, "test-ctx", loaded.Contexts[0].Name)
	})

	t.Run("overwrites existing config", func(t *testing.T) {
		setupTestHome(t)

		original := &StoredConfig{
			CurrentContext: "old",
			Contexts:       []Context{{Name: "old"}},
		}
		require.NoError(t, SaveStoredConfig(original))

		updated := &StoredConfig{
			CurrentContext: "new",
			Contexts:       []Context{{Name: "new"}},
		}
		require.NoError(t, SaveStoredConfig(updated))

		loaded, err := LoadStoredConfig()
		require.NoError(t, err)
		assert.Equal(t, "new", loaded.CurrentContext)
		require.Len(t, loaded.Contexts, 1)
		assert.Equal(t, "new", loaded.Contexts[0].Name)
	})
}

func TestIsConfigFileExists(t *testing.T) {
	t.Run("returns false when file does not exist", func(t *testing.T) {
		setupTestHome(t)
		assert.False(t, IsConfigFileExists())
	})

	t.Run("returns true when file exists", func(t *testing.T) {
		setupTestHome(t)

		require.NoError(t, SaveStoredConfig(&StoredConfig{
			Contexts: []Context{{Name: "test"}},
		}))

		assert.True(t, IsConfigFileExists())
	})
}

// The config holds the only copy of the user's credentials, so a partial write would
// cost a re-login.
func TestSaveStoredConfigWritesAtomically(t *testing.T) {
	home := setupTestHome(t)
	cfgDir := filepath.Join(home, ".openchoreo")

	cfg := &StoredConfig{
		CurrentContext: "ctx",
		ControlPlanes:  []ControlPlane{{Name: "cp", URL: "http://cp.example"}},
		Credentials:    []Credential{{Name: "cred", Token: "t"}},
		Contexts:       []Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
	}
	require.NoError(t, SaveStoredConfig(cfg))
	require.NoError(t, SaveStoredConfig(cfg))

	info, err := os.Stat(filepath.Join(cfgDir, "config"))
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "credentials must not be world-readable")
	}

	entries, err := os.ReadDir(cfgDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.Equal(t, "config", e.Name(), "the staging file should not survive a successful save")
	}

	loaded, err := LoadStoredConfig()
	require.NoError(t, err)
	assert.Equal(t, "cred", loaded.Credentials[0].Name)
}
